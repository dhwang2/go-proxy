package views

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-proxy/internal/derived"
	"go-proxy/internal/network"
	"go-proxy/internal/tui"
	"go-proxy/internal/tui/components"
)

type networkStep int

const (
	networkMenu networkStep = iota
	networkConfirm
	networkResult
)

type NetworkView struct {
	tui.InlineState
	model *tui.Model
	menu  tui.MenuModel
	split tui.SubSplitModel
	step  networkStep
}

func NewNetworkView(model *tui.Model) *NetworkView {
	v := &NetworkView{model: model}
	v.menu = tui.NewMenu("󰀂 网络管理", []tui.MenuItem{
		{Key: '1', Label: "󰓅 BBR 网络优化", ID: "bbr"},
		{Key: '2', Label: "󰒃 服务器防火墙收敛", ID: "firewall"},
	})
	return v
}

func (v *NetworkView) Name() string { return "network" }

func (v *NetworkView) Init() tea.Cmd {
	v.step = networkMenu
	v.split.SetFocusLeft(true)
	v.split.SetSize(v.model.ContentWidth(), v.model.Height()-6)
	return nil
}

func (v *NetworkView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.ViewResizeMsg:
		v.split.SetSize(msg.ContentWidth, msg.ContentHeight-6)
		return v, nil
	case tui.SubSplitMouseMsg:
		var cmd tea.Cmd
		v.split, cmd = v.split.Update(msg.MouseMsg)
		return v, cmd
	}
	inlineCmd, handled := v.UpdateInline(msg)
	if handled {
		return v, inlineCmd
	}
	switch msg := msg.(type) {
	case tui.MenuSelectMsg:
		switch msg.ID {
		case "bbr":
			return v, v.doBBRStatus
		case "firewall":
			return v, v.doFirewall
		}

	case networkActionDoneMsg:
		v.split.SetFocusLeft(false)
		if msg.needConfirm {
			v.step = networkConfirm
			return v, v.SetInline(components.NewConfirm(msg.result))
		}
		v.step = networkResult
		return v, v.SetInline(components.NewResult(msg.result))

	case tui.ConfirmResultMsg:
		if msg.Confirmed {
			return v, v.doEnableBBR
		}
		v.step = networkMenu
		v.split.SetFocusLeft(true)
		return v, nil

	case tui.ResultDismissedMsg:
		v.step = networkMenu
		v.split.SetFocusLeft(true)
		return v, nil

	default:
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.Type == tea.KeyEsc {
			return v, tui.BackCmd
		}
		if v.step == networkMenu {
			var cmd tea.Cmd
			v.menu, cmd = v.menu.Update(msg)
			return v, cmd
		}
	}
	return v, inlineCmd
}

func (v *NetworkView) View() string {
	if v.step == networkMenu || !v.split.Enabled() {
		if v.HasInline() {
			return v.ViewInline()
		}
		return tui.RenderSubMenuBody(v.menu.View(), v.model.ContentWidth())
	}

	menuContent := v.menu.View()
	var detailContent string
	if v.HasInline() {
		tui.InSplitPanel = true
		detailContent = v.ViewInline()
		tui.InSplitPanel = false
	} else {
		detailContent = lipgloss.NewStyle().
			Foreground(tui.ColorMuted).
			Render("加载中...")
	}

	return v.split.View(menuContent, detailContent)
}

type networkActionDoneMsg struct {
	result      string
	needConfirm bool
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

func (v *NetworkView) doEnableBBR() tea.Msg {
	if err := network.EnableBBR(); err != nil {
		return networkActionDoneMsg{result: "启用 BBR 失败: " + err.Error()}
	}
	return networkActionDoneMsg{result: "BBR 已成功启用"}
}

// portEntry tracks which services use a port.
type portEntry struct {
	port     int
	proto    string
	services []string
}

func (v *NetworkView) doFirewall() tea.Msg {
	s := v.model.Store()

	// Collect ports from all sources, grouped by port/proto.
	portMap := make(map[string]*portEntry) // key: "proto/port"

	addPort := func(port int, proto string, service string) {
		key := fmt.Sprintf("%s/%d", proto, port)
		if pe, ok := portMap[key]; ok {
			pe.services = append(pe.services, service)
		} else {
			portMap[key] = &portEntry{port: port, proto: proto, services: []string{service}}
		}
	}

	// sing-box inbound ports
	inv := derived.Inventory(s)
	for _, info := range inv {
		if info.Port > 0 {
			proto := "tcp"
			if info.Type == "tuic" {
				proto = "udp"
			}
			addPort(info.Port, proto, info.Type)
		}
	}

	// snell port
	if s.SnellConf != nil {
		port := s.SnellConf.Port()
		if port > 0 {
			addPort(port, "tcp", "snell")
		}
	}

	// System ports: SSH (22), HTTP (80), HTTPS (443)
	addPort(22, "tcp", "ssh")
	addPort(80, "tcp", "caddy")
	addPort(443, "tcp", "caddy")

	// Detect firewall backend
	backend := "iptables"
	if network.HasNftables() {
		backend = "nftables"
	}

	// Render
	labelStyle := lipgloss.NewStyle().Foreground(tui.ColorLabel).Bold(true)
	valStyle := lipgloss.NewStyle().Foreground(tui.ColorValSys)
	mutedStyle := lipgloss.NewStyle().Foreground(tui.ColorMuted)

	var sb strings.Builder
	sb.WriteString("防火墙规则\n\n")
	sb.WriteString(fmt.Sprintf("  %s %s\n\n",
		labelStyle.Render("后端:"),
		valStyle.Render(backend)))

	// Sort by port number
	var entries []*portEntry
	for _, pe := range portMap {
		entries = append(entries, pe)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].port != entries[j].port {
			return entries[i].port < entries[j].port
		}
		return entries[i].proto < entries[j].proto
	})

	sb.WriteString(labelStyle.Render("  所需端口:"))
	sb.WriteString("\n")
	sb.WriteString(mutedStyle.Render("  " + strings.Repeat("─", 36)))
	sb.WriteString("\n")

	for _, pe := range entries {
		portLabel := valStyle.Render(fmt.Sprintf("%s/%d", pe.proto, pe.port))
		svcLabel := mutedStyle.Render("[" + strings.Join(pe.services, ",") + "]")
		sb.WriteString(fmt.Sprintf("  • %s  %s\n", portLabel, svcLabel))
	}

	return networkActionDoneMsg{result: sb.String()}
}
