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
	model *tui.Model
	menu  components.MenuModel
	step  networkStep
}

func NewNetworkView(model *tui.Model) *NetworkView {
	v := &NetworkView{model: model}
	v.menu = components.NewMenu("󰀂 网络管理", []components.MenuItem{
		{Key: '1', Label: "󰓅 BBR 网络优化", ID: "bbr"},
		{Key: '2', Label: "󰒃 服务器防火墙收敛", ID: "firewall"},
		{Key: '0', Label: "󰌍 返回", ID: "back"},
	})
	return v
}

func (v *NetworkView) Name() string { return "network" }

func (v *NetworkView) Init() tea.Cmd {
	v.step = networkMenu
	return nil
}

func (v *NetworkView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case components.MenuSelectMsg:
		switch msg.ID {
		case "back":
			return v, tui.BackCmd
		case "bbr":
			return v, v.doBBRStatus
		case "firewall":
			return v, v.doFirewall
		}

	case networkActionDoneMsg:
		if msg.needConfirm {
			v.step = networkConfirm
			return v, func() tea.Msg {
				return tui.ShowOverlayMsg{
					Overlay: components.NewConfirm(msg.result),
				}
			}
		}
		v.step = networkResult
		return v, func() tea.Msg {
			return tui.ShowOverlayMsg{
				Overlay: components.NewResult(msg.result),
			}
		}

	case tui.ConfirmResultMsg:
		if msg.Confirmed {
			return v, v.doEnableBBR
		}
		v.step = networkMenu
		return v, nil

	case tui.ResultDismissedMsg:
		v.step = networkMenu
		return v, nil

	default:
		if v.step == networkMenu {
			var cmd tea.Cmd
			v.menu, cmd = v.menu.Update(msg)
			return v, cmd
		}
	}
	return v, nil
}

func (v *NetworkView) View() string {
	return tui.RenderSubMenuFrame("", v.menu.View(), "返回(esc) | 选择(↑↓) | 确认(enter)", tui.SeparatorWidth)
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
