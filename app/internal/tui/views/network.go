package views

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-proxy/internal/network"
	"go-proxy/internal/store"
	"go-proxy/internal/tui"
	"go-proxy/internal/tui/components"
)

type networkStep int

const (
	networkMenu networkStep = iota
	networkConfirm
	networkResult
	networkFirewallMenu
	networkFirewallTCPInput
	networkFirewallUDPInput
)

type NetworkView struct {
	tui.SplitViewBase
	step          networkStep
	pendingAction string
	fail2banState string
	subMenu       tui.MenuModel
	viewport      viewport.Model
	viewportReady bool
	rawDetail     string
}

func NewNetworkView(model *tui.Model) *NetworkView {
	v := &NetworkView{}
	v.Model = model
	v.Menu = tui.NewMenu("", []tui.MenuItem{
		{Key: '1', Label: "󰓅 BBR 网络优化", ID: "bbr"},
		{Key: '2', Label: "󰒃 服务器防火墙收敛", ID: "firewall"},
		{Key: '3', Label: "󰒃 fail2ban 防护", ID: "fail2ban"},
	})
	return v
}

func (v *NetworkView) Name() string { return "network" }

func (v *NetworkView) Init() tea.Cmd {
	v.step = networkMenu
	v.viewportReady = false
	v.rawDetail = ""
	v.InitSplit()
	return nil
}

func (v *NetworkView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.ViewResizeMsg:
		v.HandleResize(msg)
		if v.viewportReady {
			w := msg.ContentWidth
			h := msg.ContentHeight - 5
			if v.Split.Enabled() {
				w = v.Split.RightWidth()
				h = v.Split.TotalHeight()
			}
			v.viewport.Width = w
			v.viewport.Height = h
			v.viewport.SetContent(wrapNetworkContent(v.rawDetail, w))
		}
		return v, nil
	case tui.SubSplitMouseMsg:
		if v.viewportReady && (!v.Split.Enabled() || !v.Split.FocusLeft()) {
			if msg.Button == tea.MouseButtonWheelUp {
				v.viewport.LineUp(3)
				return v, nil
			}
			if msg.Button == tea.MouseButtonWheelDown {
				v.viewport.LineDown(3)
				return v, nil
			}
		}
		return v, v.HandleMouse(msg)
	}
	if cmd, ok := v.HandleMenuNav(msg, v.step == networkMenu, false); ok {
		return v, cmd
	}
	inlineCmd, handled := v.UpdateInline(msg)
	if handled {
		return v, inlineCmd
	}
	switch msg := msg.(type) {
	case tui.MenuCursorChangeMsg:
		return v, nil
	case tui.MenuSelectMsg:
		if v.step == networkFirewallMenu {
			return v, v.handleFirewallMenu(msg.ID)
		}
		v.SetFocus(false)
		return v, v.triggerMenuAction(msg.ID)
	case tui.InputResultMsg:
		return v.handleFirewallInput(msg)
	case networkActionDoneMsg:
		v.SetFocus(false)
		if msg.state != "" {
			v.fail2banState = msg.state
		}
		if msg.needConfirm {
			v.step = networkConfirm
			return v, v.SetInline(components.NewConfirm(msg.result))
		}
		if v.pendingAction == "firewall" {
			v.ClearInline()
			v.step = networkResult
			v.showFirewallDetail(msg.result)
			return v, nil
		}
		v.step = networkResult
		return v, v.SetInline(components.NewResult(msg.result))
	case tui.ConfirmResultMsg:
		if msg.Confirmed {
			switch v.pendingAction {
			case "fail2ban":
				return v, v.doFail2BanAction
			default:
				return v, v.doEnableBBR
			}
		}
		v.step = networkMenu
		v.SetFocus(true)
		return v, nil
	case tui.ResultDismissedMsg:
		v.viewportReady = false
		v.rawDetail = ""
		if v.pendingAction == "firewall" {
			v.step = networkFirewallMenu
			return v, nil
		}
		v.step = networkMenu
		v.SetFocus(true)
		return v, nil
	default:
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.Type == tea.KeyEsc {
			switch v.step {
			case networkFirewallMenu:
				v.step = networkMenu
				v.SetFocus(true)
				return v, nil
			case networkFirewallTCPInput, networkFirewallUDPInput:
				v.step = networkFirewallMenu
				return v, nil
			case networkResult:
				if v.pendingAction == "firewall" {
					v.step = networkFirewallMenu
					v.viewportReady = false
					v.rawDetail = ""
					return v, nil
				}
			}
			return v, tui.BackCmd
		}
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			if v.HandleSplitArrows(keyMsg, v.step == networkMenu, v.HasInline() || v.viewportReady || v.step == networkFirewallMenu) {
				return v, nil
			}
		}
		switch v.step {
		case networkMenu:
			var cmd tea.Cmd
			v.Menu, cmd = v.Menu.Update(msg)
			return v, cmd
		case networkFirewallMenu:
			var cmd tea.Cmd
			v.subMenu, cmd = v.subMenu.Update(msg)
			return v, cmd
		case networkResult:
			if v.viewportReady && (!v.Split.Enabled() || !v.Split.FocusLeft()) {
				var cmd tea.Cmd
				v.viewport, cmd = v.viewport.Update(msg)
				return v, cmd
			}
		}
	}
	return v, inlineCmd
}

func (v *NetworkView) View() string {
	if !v.Split.Enabled() {
		if v.HasInline() {
			return v.ViewInline()
		}
		if v.step == networkResult && v.viewportReady {
			return v.viewport.View()
		}
		if v.step == networkFirewallMenu {
			return tui.RenderSubMenuBody(v.subMenu.View(), v.Model.ContentWidth())
		}
		return tui.RenderSubMenuBody(v.Menu.View(), v.Model.ContentWidth())
	}
	if v.step == networkMenu {
		return tui.RenderSubMenuBody(v.Menu.View(), v.Model.ContentWidth())
	}

	menuContent := v.Menu.View()
	var detailContent string
	if v.step == networkResult && v.viewportReady {
		detailContent = v.viewport.View()
	} else if v.HasInline() {
		tui.InSplitPanel = true
		detailContent = v.ViewInline()
		tui.InSplitPanel = false
	} else if v.step == networkFirewallMenu {
		detailContent = v.subMenu.View()
	} else {
		detailContent = lipgloss.NewStyle().Foreground(tui.ColorMuted).Render("加载中...")
	}

	return v.Split.View(menuContent, detailContent)
}

func (v *NetworkView) triggerMenuAction(id string) tea.Cmd {
	v.pendingAction = id
	v.viewportReady = false
	v.rawDetail = ""
	switch id {
	case "bbr":
		return v.doBBRStatus
	case "firewall":
		v.step = networkFirewallMenu
		v.subMenu = tui.NewMenu("服务器防火墙收敛", []tui.MenuItem{
			{Key: '1', Label: "󰆓 应用防火墙收敛", ID: "apply"},
			{Key: '2', Label: "󰏘 设置自定义 TCP 端口", ID: "custom-tcp"},
			{Key: '3', Label: "󰏘 设置自定义 UDP 端口", ID: "custom-udp"},
			{Key: '4', Label: "󰍹 查看目标端口", ID: "view"},
			{Key: '5', Label: "󰡱 查看当前规则", ID: "current"},
		})
		return nil
	case "fail2ban":
		return v.doFail2BanStatus
	}
	return nil
}

func (v *NetworkView) handleFirewallMenu(id string) tea.Cmd {
	v.pendingAction = "firewall"
	v.viewportReady = false
	v.rawDetail = ""
	switch id {
	case "apply":
		return v.doFirewallApply
	case "custom-tcp":
		v.step = networkFirewallTCPInput
		return v.SetInline(components.NewTextInput("自定义 TCP 端口，逗号分隔，留空清空:", formatPorts(v.currentFirewallConfig().TCP)))
	case "custom-udp":
		v.step = networkFirewallUDPInput
		return v.SetInline(components.NewTextInput("自定义 UDP 端口，逗号分隔，留空清空:", formatPorts(v.currentFirewallConfig().UDP)))
	case "view":
		return v.doFirewallSummary
	case "current":
		return v.doFirewallCurrentRules
	}
	return nil
}

func (v *NetworkView) handleFirewallInput(msg tui.InputResultMsg) (tui.View, tea.Cmd) {
	if msg.Cancelled {
		switch v.step {
		case networkFirewallTCPInput, networkFirewallUDPInput:
			v.step = networkFirewallMenu
			v.ClearInline()
			return v, nil
		}
		return v, nil
	}

	switch v.step {
	case networkFirewallTCPInput:
		ports, err := parsePortCSV(msg.Value)
		if err != nil {
			return v, v.SetInline(components.NewResult("设置失败: " + err.Error()))
		}
		cfg := v.currentFirewallConfig()
		cfg.TCP = ports
		v.Model.Store().Firewall = cfg
		v.Model.Store().MarkDirty(store.FileFirewall)
		if err := v.Model.Store().Apply(); err != nil {
			v.step = networkResult
			v.showFirewallDetail("保存失败: " + err.Error())
			return v, nil
		}
		v.step = networkResult
		v.showFirewallDetail("自定义 TCP 端口已更新\n\n" + v.renderFirewallSummary())
		return v, nil
	case networkFirewallUDPInput:
		ports, err := parsePortCSV(msg.Value)
		if err != nil {
			v.step = networkResult
			v.showFirewallDetail("设置失败: " + err.Error())
			return v, nil
		}
		cfg := v.currentFirewallConfig()
		cfg.UDP = ports
		v.Model.Store().Firewall = cfg
		v.Model.Store().MarkDirty(store.FileFirewall)
		if err := v.Model.Store().Apply(); err != nil {
			v.step = networkResult
			v.showFirewallDetail("保存失败: " + err.Error())
			return v, nil
		}
		v.step = networkResult
		v.showFirewallDetail("自定义 UDP 端口已更新\n\n" + v.renderFirewallSummary())
		return v, nil
	}
	return v, nil
}

type networkActionDoneMsg struct {
	result      string
	needConfirm bool
	state       string
}

func (v *NetworkView) doBBRStatus() tea.Msg {
	enabled, current, err := network.BBRStatus()
	if err != nil {
		return networkActionDoneMsg{result: fmt.Sprintf("BBR 状态\n\n读取失败: %s", err)}
	}
	if enabled {
		return networkActionDoneMsg{result: fmt.Sprintf("BBR 状态\n\n当前拥塞控制: %s\nBBR 已启用", current)}
	}
	return networkActionDoneMsg{
		result:      fmt.Sprintf("当前拥塞控制: %s\nBBR 未启用，是否启用？", current),
		needConfirm: true,
	}
}

func (v *NetworkView) doFail2BanStatus() tea.Msg {
	installed, running, err := network.Fail2BanStatus()
	if err != nil {
		return networkActionDoneMsg{result: fmt.Sprintf("fail2ban 状态\n\n读取失败: %s", err)}
	}
	if !installed {
		return networkActionDoneMsg{
			result:      "fail2ban 未安装，是否安装并启用？",
			needConfirm: true,
			state:       "install",
		}
	}
	if !running {
		return networkActionDoneMsg{
			result:      "fail2ban 已安装但未运行，是否启用？",
			needConfirm: true,
			state:       "enable",
		}
	}
	return networkActionDoneMsg{
		result:      "fail2ban 正在运行，是否停止并禁用？",
		needConfirm: true,
		state:       "disable",
	}
}

func (v *NetworkView) doFail2BanAction() tea.Msg {
	switch v.fail2banState {
	case "install":
		if err := network.Fail2BanInstall(); err != nil {
			return networkActionDoneMsg{result: "安装 fail2ban 失败: " + err.Error()}
		}
		if err := network.Fail2BanEnable(); err != nil {
			return networkActionDoneMsg{result: "启用 fail2ban 失败: " + err.Error()}
		}
		return networkActionDoneMsg{result: "fail2ban 已安装并启用"}
	case "enable":
		if err := network.Fail2BanEnable(); err != nil {
			return networkActionDoneMsg{result: "启用 fail2ban 失败: " + err.Error()}
		}
		return networkActionDoneMsg{result: "fail2ban 已启用"}
	case "disable":
		if err := network.Fail2BanDisable(); err != nil {
			return networkActionDoneMsg{result: "禁用 fail2ban 失败: " + err.Error()}
		}
		return networkActionDoneMsg{result: "fail2ban 已停止并禁用"}
	}
	return networkActionDoneMsg{result: "未知操作"}
}

func (v *NetworkView) doEnableBBR() tea.Msg {
	if err := network.EnableBBR(); err != nil {
		return networkActionDoneMsg{result: "启用 BBR 失败: " + err.Error()}
	}
	return networkActionDoneMsg{result: "BBR 已成功启用"}
}

func (v *NetworkView) doFirewallSummary() tea.Msg {
	return networkActionDoneMsg{result: v.renderFirewallSummary()}
}

func (v *NetworkView) doFirewallCurrentRules() tea.Msg {
	rules, err := network.ListOpenPorts()
	if err != nil {
		return networkActionDoneMsg{result: "读取失败: " + err.Error()}
	}
	return networkActionDoneMsg{result: "当前防火墙规则\n\n" + rules}
}

func (v *NetworkView) doFirewallApply() tea.Msg {
	if err := network.ApplyConvergence(v.Model.Store()); err != nil {
		return networkActionDoneMsg{result: "应用防火墙收敛失败: " + err.Error()}
	}
	return networkActionDoneMsg{result: "防火墙收敛已应用\n\n" + v.renderFirewallSummary()}
}

func (v *NetworkView) renderFirewallSummary() string {
	labelStyle := lipgloss.NewStyle().Foreground(tui.ColorLabel).Bold(true)
	valStyle := lipgloss.NewStyle().Foreground(tui.ColorValSys)
	mutedStyle := lipgloss.NewStyle().Foreground(tui.ColorMuted)

	backend := "unsupported"
	switch {
	case network.HasNftables():
		backend = "nftables"
	case network.FirewallBackend() == "iptables":
		backend = "iptables"
	}

	var sb strings.Builder
	sb.WriteString("防火墙收敛\n\n")
	sb.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("后端:"), valStyle.Render(backend)))
	sb.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("自定义 TCP:"), valStyle.Render(formatPorts(v.currentFirewallConfig().TCP))))
	sb.WriteString(fmt.Sprintf("  %s %s\n\n", labelStyle.Render("自定义 UDP:"), valStyle.Render(formatPorts(v.currentFirewallConfig().UDP))))
	sb.WriteString(labelStyle.Render("  目标开放端口"))
	sb.WriteString("\n")
	sb.WriteString(mutedStyle.Render("  " + strings.Repeat("─", 36)))
	sb.WriteString("\n")
	entries, err := network.DescribeDesiredPorts(v.Model.Store())
	if err != nil {
		sb.WriteString("  读取失败: " + err.Error())
		return sb.String()
	}
	for _, entry := range entries {
		sb.WriteString(fmt.Sprintf("  • %s/%d [%s]\n", entry.Proto, entry.Port, strings.Join(entry.Services, ",")))
	}
	return sb.String()
}

func (v *NetworkView) currentFirewallConfig() *store.FirewallConfig {
	if v.Model.Store().Firewall == nil {
		v.Model.Store().Firewall = &store.FirewallConfig{}
	}
	return v.Model.Store().Firewall
}

func parsePortCSV(value string) ([]int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	tokens := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '，'
	})
	ports := make([]int, 0, len(tokens))
	seen := make(map[int]bool, len(tokens))
	for _, token := range tokens {
		port, err := strconv.Atoi(strings.TrimSpace(token))
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("端口号无效: %s", token)
		}
		if seen[port] {
			continue
		}
		seen[port] = true
		ports = append(ports, port)
	}
	return ports, nil
}

func formatPorts(ports []int) string {
	if len(ports) == 0 {
		return "无"
	}
	values := make([]string, 0, len(ports))
	for _, port := range ports {
		values = append(values, strconv.Itoa(port))
	}
	return strings.Join(values, ",")
}

func (v *NetworkView) showFirewallDetail(content string) {
	w := v.Model.ContentWidth()
	h := v.Model.Height() - 5
	if v.Split.Enabled() {
		w = v.Split.RightWidth()
		h = v.Split.TotalHeight()
	}
	v.rawDetail = content
	v.viewport = viewport.New(w, h)
	v.viewport.SetContent(wrapNetworkContent(content, w))
	v.viewportReady = true
	v.SetFocus(false)
}

func wrapNetworkContent(content string, width int) string {
	if width <= 0 {
		return content
	}
	return lipgloss.NewStyle().Width(width).Render(content)
}
