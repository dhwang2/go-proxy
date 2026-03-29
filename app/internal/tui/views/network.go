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
	networkFail2BanMenu
)

type NetworkView struct {
	tui.SplitViewBase
	step           networkStep
	pendingAction  string
	subMenu        tui.MenuModel
	viewport       viewport.Model
	viewportReady  bool
	detailBuilder  func(int) string
	renderedDetail string
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
	v.detailBuilder = nil
	v.renderedDetail = ""
	v.InitSplit()
	return nil
}

func (v *NetworkView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.ViewResizeMsg:
		v.HandleResize(msg)
		v.resizeViewport(msg.ContentWidth, msg.ContentHeight)
		return v, nil
	case tui.SubSplitResizeMsg:
		v.resizeViewport(msg.RightWidth, msg.RightHeight+3)
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
		if v.step == networkFail2BanMenu {
			return v, v.handleFail2BanMenu(msg.ID)
		}
		v.SetFocus(false)
		return v, v.triggerMenuAction(msg.ID)
	case tui.InputResultMsg:
		return v.handleFirewallInput(msg)
	case networkActionDoneMsg:
		v.SetFocus(false)
		if msg.needConfirm {
			v.step = networkConfirm
			return v, v.SetInline(components.NewConfirm(msg.result))
		}
		if v.pendingAction == "firewall" {
			v.ClearInline()
			v.step = networkResult
			if msg.render != nil {
				v.showFirewallDetail(msg.render)
			} else {
				v.showFirewallDetail(func(int) string { return msg.result })
			}
			return v, nil
		}
		if v.pendingAction == "fail2ban-status" {
			v.ClearInline()
			v.step = networkResult
			if msg.render != nil {
				v.showFirewallDetail(msg.render)
			} else {
				v.showFirewallDetail(func(int) string { return msg.result })
			}
			return v, nil
		}
		v.step = networkResult
		return v, v.SetInline(components.NewResult(msg.result))
	case tui.ConfirmResultMsg:
		if msg.Confirmed {
			switch v.pendingAction {
			case "fail2ban-enable":
				return v, tea.Batch(
					v.SetInline(components.NewSpinner("开启 fail2ban...")),
					v.doFail2BanEnable,
				)
			case "fail2ban-disable":
				return v, tea.Batch(
					v.SetInline(components.NewSpinner("关闭 fail2ban...")),
					v.doFail2BanDisable,
				)
			default:
				return v, v.doEnableBBR
			}
		}
		// Cancelled: return to fail2ban sub-menu or main menu
		if v.pendingAction == "fail2ban-enable" || v.pendingAction == "fail2ban-disable" {
			v.step = networkFail2BanMenu
			v.ClearInline()
			return v, nil
		}
		v.step = networkMenu
		v.SetFocus(true)
		return v, nil
	case tui.ResultDismissedMsg:
		v.viewportReady = false
		v.detailBuilder = nil
		v.renderedDetail = ""
		if v.pendingAction == "firewall" {
			v.step = networkFirewallMenu
			return v, nil
		}
		if v.pendingAction == "fail2ban-status" || v.pendingAction == "fail2ban-enable" || v.pendingAction == "fail2ban-disable" {
			v.step = networkFail2BanMenu
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
			case networkFail2BanMenu:
				v.step = networkMenu
				v.SetFocus(true)
				return v, nil
			case networkResult:
				if v.pendingAction == "firewall" {
					v.step = networkFirewallMenu
					v.viewportReady = false
					v.detailBuilder = nil
					v.renderedDetail = ""
					return v, nil
				}
				if v.pendingAction == "fail2ban-status" || v.pendingAction == "fail2ban-enable" || v.pendingAction == "fail2ban-disable" {
					v.step = networkFail2BanMenu
					v.viewportReady = false
					v.detailBuilder = nil
					v.renderedDetail = ""
					return v, nil
				}
			}
			return v, tui.BackCmd
		}
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			if v.HandleSplitArrows(keyMsg, v.step == networkMenu, v.HasInline() || v.viewportReady || v.step == networkFirewallMenu || v.step == networkFail2BanMenu) {
				return v, nil
			}
		}
		switch v.step {
		case networkMenu:
			var cmd tea.Cmd
			v.Menu, cmd = v.Menu.Update(msg)
			return v, cmd
		case networkFirewallMenu, networkFail2BanMenu:
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
		if v.step == networkFirewallMenu || v.step == networkFail2BanMenu {
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
	} else if v.step == networkFirewallMenu || v.step == networkFail2BanMenu {
		detailContent = v.subMenu.View()
	} else {
		detailContent = lipgloss.NewStyle().Foreground(tui.ColorMuted).Render("加载中...")
	}

	return v.Split.View(menuContent, detailContent)
}

func (v *NetworkView) triggerMenuAction(id string) tea.Cmd {
	v.pendingAction = id
	v.viewportReady = false
	v.detailBuilder = nil
	v.renderedDetail = ""
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
		v.step = networkFail2BanMenu
		v.subMenu = tui.NewMenu("fail2ban 防护", []tui.MenuItem{
			{Key: '1', Label: "󰋽 查看状态", ID: "status"},
			{Key: '2', Label: "󰒃 开启防护", ID: "enable"},
			{Key: '3', Label: "󰒄 关闭防护", ID: "disable"},
		})
		return nil
	}
	return nil
}

func (v *NetworkView) handleFirewallMenu(id string) tea.Cmd {
	v.pendingAction = "firewall"
	v.viewportReady = false
	v.detailBuilder = nil
	v.renderedDetail = ""
	switch id {
	case "apply":
		return tea.Batch(
			v.SetInline(components.NewSpinner("正在应用防火墙收敛...")),
			v.doFirewallApply,
		)
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
			v.showFirewallDetail(func(int) string { return "保存失败: " + err.Error() })
			return v, nil
		}
		v.step = networkResult
		v.showFirewallDetail(func(width int) string {
			return "自定义 TCP 端口已更新\n\n" + v.renderFirewallSummary(width)
		})
		return v, nil
	case networkFirewallUDPInput:
		ports, err := parsePortCSV(msg.Value)
		if err != nil {
			v.step = networkResult
			v.showFirewallDetail(func(int) string { return "设置失败: " + err.Error() })
			return v, nil
		}
		cfg := v.currentFirewallConfig()
		cfg.UDP = ports
		v.Model.Store().Firewall = cfg
		v.Model.Store().MarkDirty(store.FileFirewall)
		if err := v.Model.Store().Apply(); err != nil {
			v.step = networkResult
			v.showFirewallDetail(func(int) string { return "保存失败: " + err.Error() })
			return v, nil
		}
		v.step = networkResult
		v.showFirewallDetail(func(width int) string {
			return "自定义 UDP 端口已更新\n\n" + v.renderFirewallSummary(width)
		})
		return v, nil
	}
	return v, nil
}

type networkActionDoneMsg struct {
	result      string
	needConfirm bool
	render      func(int) string
}

func (v *NetworkView) doBBRStatus() tea.Msg {
	enabled, current, err := network.BBRStatus()
	if err != nil {
		return networkActionDoneMsg{result: "读取失败: " + err.Error()}
	}
	width := v.Model.ContentWidth()
	if enabled {
		return networkActionDoneMsg{result: renderBBRStatusTable(current, enabled, width)}
	}
	return networkActionDoneMsg{
		result:      renderBBRStatusTable(current, false, width) + "\n\n  是否启用 BBR？",
		needConfirm: true,
	}
}

func (v *NetworkView) handleFail2BanMenu(id string) tea.Cmd {
	switch id {
	case "status":
		v.pendingAction = "fail2ban-status"
		return tea.Batch(
			v.SetInline(components.NewSpinner("查询 fail2ban 状态...")),
			v.doFail2BanStatus,
		)
	case "enable":
		v.pendingAction = "fail2ban-enable"
		return v.SetInline(components.NewConfirm("开启 fail2ban SSH 防护？"))
	case "disable":
		v.pendingAction = "fail2ban-disable"
		return v.SetInline(components.NewConfirm("关闭 fail2ban 防护？"))
	}
	return nil
}

func (v *NetworkView) doFail2BanStatus() tea.Msg {
	info, err := network.Fail2BanStatus()
	if err != nil {
		return networkActionDoneMsg{result: "读取失败: " + err.Error()}
	}
	width := v.currentViewportWidth()
	return networkActionDoneMsg{
		render: func(w int) string {
			return renderFail2BanStatusTable(info, w)
		},
		result: renderFail2BanStatusTable(info, width),
	}
}

func (v *NetworkView) doFail2BanEnable() tea.Msg {
	if err := network.Fail2BanEnable(); err != nil {
		return networkActionDoneMsg{result: "启用 fail2ban 失败: " + err.Error()}
	}
	return networkActionDoneMsg{result: "fail2ban 已启用"}
}

func (v *NetworkView) doFail2BanDisable() tea.Msg {
	if err := network.Fail2BanDisable(); err != nil {
		return networkActionDoneMsg{result: "禁用 fail2ban 失败: " + err.Error()}
	}
	return networkActionDoneMsg{result: "fail2ban 已停止并禁用"}
}

func (v *NetworkView) doEnableBBR() tea.Msg {
	if err := network.EnableBBR(); err != nil {
		return networkActionDoneMsg{result: "启用 BBR 失败: " + err.Error()}
	}
	enabled, current, err := network.BBRStatus()
	if err != nil {
		return networkActionDoneMsg{result: "BBR 已成功启用"}
	}
	return networkActionDoneMsg{result: renderBBRStatusTable(current, enabled, v.Model.ContentWidth())}
}

func (v *NetworkView) doFirewallSummary() tea.Msg {
	return networkActionDoneMsg{
		render: func(width int) string {
			return v.renderFirewallSummary(width)
		},
	}
}

func (v *NetworkView) doFirewallCurrentRules() tea.Msg {
	entries, err := network.CurrentFirewallPorts()
	if err != nil {
		return networkActionDoneMsg{result: "读取失败: " + err.Error()}
	}
	if len(entries) == 0 {
		return networkActionDoneMsg{result: "当前无防火墙规则"}
	}
	return networkActionDoneMsg{
		render: func(width int) string {
			return renderCurrentRulesTable(entries, width)
		},
	}
}

func (v *NetworkView) doFirewallApply() tea.Msg {
	if err := network.ApplyConvergence(v.Model.Store()); err != nil {
		return networkActionDoneMsg{result: "应用防火墙收敛失败: " + err.Error()}
	}
	return networkActionDoneMsg{
		render: func(width int) string {
			return "防火墙收敛已应用\n\n" + v.renderFirewallSummary(width)
		},
	}
}

func (v *NetworkView) renderFirewallSummary(width int) string {
	entries, err := network.DescribeDesiredPorts(v.Model.Store())
	if err != nil {
		return "  读取失败: " + err.Error()
	}
	if len(entries) == 0 {
		return "  无目标端口"
	}
	return renderDesiredPortsTable(entries, width)
}

func renderDesiredPortsTable(entries []network.DesiredPortEntry, width int) string {
	rows := make([][]string, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, []string{e.Proto, strconv.Itoa(e.Port), strings.Join(e.Services, ", ")})
	}
	return renderTable([]string{"协议", "端口", "来源"}, rows, width, false)
}

func renderCurrentRulesTable(entries []network.CurrentPortEntry, width int) string {
	rows := make([][]string, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, []string{e.Proto, strconv.Itoa(e.Port), e.Action})
	}
	return renderTable([]string{"协议", "端口", "动作"}, rows, width, false)
}

func renderFail2BanStatusTable(info network.Fail2BanInfo, width int) string {
	serviceStatus := "未安装"
	if info.Installed {
		if info.Running {
			serviceStatus = "运行中"
		} else {
			serviceStatus = "已停止"
		}
	}
	jailStatus := "未启用"
	if info.SSHJailEnabled {
		jailStatus = "已启用"
	}
	rows := [][]string{
		{"服务状态", serviceStatus},
		{"SSH Jail", jailStatus},
	}
	if info.MaxRetry != "" {
		rows = append(rows, []string{"最大重试", info.MaxRetry})
	}
	if info.BanTime != "" {
		rows = append(rows, []string{"封禁时长", info.BanTime + "s"})
	}
	if info.FindTime != "" {
		rows = append(rows, []string{"检测时窗", info.FindTime + "s"})
	}
	rows = append(rows,
		[]string{"当前封禁", fmt.Sprintf("%d", info.CurrentlyBanned)},
		[]string{"累计封禁", fmt.Sprintf("%d", info.TotalBanned)},
	)
	if len(info.BannedIPs) > 0 {
		rows = append(rows, []string{"封禁 IP", strings.Join(info.BannedIPs, "\n")})
	}
	return renderTable([]string{"项目", "值"}, rows, width, false)
}

func renderBBRStatusTable(current string, enabled bool, width int) string {
	status := "未启用"
	if enabled {
		status = "已启用"
	}
	return renderTable(
		[]string{"项目", "值"},
		[][]string{
			{"当前拥塞控制", current},
			{"BBR 状态", status},
		},
		width,
		false,
	)
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

func (v *NetworkView) currentViewportWidth() int {
	if v.Split.Enabled() {
		return v.Split.RightWidth()
	}
	return v.Model.ContentWidth()
}

func (v *NetworkView) currentViewportHeight() int {
	if v.Split.Enabled() {
		return v.Split.TotalHeight()
	}
	return v.Model.Height() - 5
}

func (v *NetworkView) resizeViewport(contentWidth, contentHeight int) {
	if !v.viewportReady || v.detailBuilder == nil {
		return
	}
	width := contentWidth
	height := contentHeight - 5
	if v.Split.Enabled() {
		width = v.Split.RightWidth()
		height = v.Split.TotalHeight()
	}
	if height < 1 {
		height = 1
	}
	v.viewport.Width = width
	v.viewport.Height = height
	v.renderedDetail = v.detailBuilder(width)
	v.viewport.SetContent(v.renderedDetail)
}

func (v *NetworkView) showFirewallDetail(builder func(int) string) {
	width := v.currentViewportWidth()
	height := v.currentViewportHeight()
	if height < 1 {
		height = 1
	}
	v.detailBuilder = builder
	v.viewport = viewport.New(width, height)
	v.renderedDetail = builder(width)
	v.viewport.SetContent(v.renderedDetail)
	v.viewportReady = true
	v.SetFocus(false)
}
