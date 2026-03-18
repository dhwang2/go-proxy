package views

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-proxy/internal/derived"
	"go-proxy/internal/service"
	"go-proxy/internal/tui"
	"go-proxy/internal/tui/components"
)

type svcStep int

const (
	svcMenuMain svcStep = iota
	svcMenuIndividual
	svcResult
)

type ServiceView struct {
	model   *tui.Model
	menu    tui.MenuModel
	subMenu tui.MenuModel
	step    svcStep
	target  service.Name // selected individual service
}

func NewServiceView(model *tui.Model) *ServiceView {
	v := &ServiceView{model: model}
	v.menu = tui.NewMenu("󰒓 协议管理", []tui.MenuItem{
		{Key: '1', Label: "󰋼 查看服务状态", ID: "status"},
		{Key: '2', Label: "󰑓 重启所有服务", ID: "restart-all"},
		{Key: '3', Label: "󰓛 停止所有服务", ID: "stop-all"},
		{Key: '4', Label: "󰐊 启动所有服务", ID: "start-all"},
		{Key: '5', Label: "󰒓 管理单个服务", ID: "individual"},
		{Key: '0', Label: "󰌍 返回", ID: "back"},
	})
	return v
}

func (v *ServiceView) Name() string { return "service" }

func (v *ServiceView) Init() tea.Cmd {
	v.step = svcMenuMain
	return nil
}

func (v *ServiceView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.MenuSelectMsg:
		return v.handleMenuSelect(msg)

	case svcActionDoneMsg:
		v.step = svcResult
		return v, func() tea.Msg {
			return tui.ShowOverlayMsg{
				Overlay: components.NewResult(msg.result),
			}
		}

	case tui.ResultDismissedMsg:
		v.step = svcMenuMain
		return v, nil

	default:
		return v.handleDefault(msg)
	}
}

func (v *ServiceView) handleMenuSelect(msg tui.MenuSelectMsg) (tui.View, tea.Cmd) {
	switch v.step {
	case svcMenuMain:
		switch msg.ID {
		case "back":
			return v, tui.BackCmd
		case "status":
			return v, func() tea.Msg { return v.doStatusTable() }
		case "restart-all":
			return v, func() tea.Msg { return v.doBulkAction("restart") }
		case "stop-all":
			return v, func() tea.Msg { return v.doBulkAction("stop") }
		case "start-all":
			return v, func() tea.Msg { return v.doBulkAction("start") }
		case "individual":
			v.step = svcMenuIndividual
			v.subMenu = v.buildServiceSelectMenu()
			return v, nil
		}

	case svcMenuIndividual:
		if msg.ID == "back" {
			v.step = svcMenuMain
			return v, nil
		}
		// First selection: pick the service.
		if strings.HasPrefix(msg.ID, "svc:") {
			svcName := service.Name(strings.TrimPrefix(msg.ID, "svc:"))
			v.target = svcName
			v.subMenu = v.buildActionMenu(svcName)
			return v, nil
		}
		// Second selection: pick the action.
		svcName := v.target
		action := msg.ID
		return v, func() tea.Msg { return v.doSingleAction(svcName, action) }
	}
	return v, nil
}

func (v *ServiceView) handleDefault(msg tea.Msg) (tui.View, tea.Cmd) {
	var cmd tea.Cmd
	switch v.step {
	case svcMenuMain:
		v.menu, cmd = v.menu.Update(msg)
	case svcMenuIndividual:
		v.subMenu, cmd = v.subMenu.Update(msg)
	}
	return v, cmd
}

func (v *ServiceView) View() string {
	if v.step == svcMenuIndividual {
		return tui.RenderSubMenuBody(v.subMenu.View(), v.model.ContentWidth())
	}
	return tui.RenderSubMenuBody(v.menu.View(), v.model.ContentWidth())
}

type svcActionDoneMsg struct{ result string }

// doStatusTable renders a shell-proxy style protocol/service table.
func (v *ServiceView) doStatusTable() tea.Msg {
	ctx := context.Background()
	inv := derived.Inventory(v.model.Store())

	// Styles for the table.
	headerStyle := lipgloss.NewStyle().Foreground(tui.ColorLabel).Bold(true)
	greenDot := lipgloss.NewStyle().Foreground(tui.ColorSuccess).Render("●")
	redDot := lipgloss.NewStyle().Foreground(tui.ColorError).Render("●")
	grayDot := lipgloss.NewStyle().Foreground(tui.ColorMuted).Render("●")
	sepStyle := lipgloss.NewStyle().Foreground(tui.ColorMuted)

	var sb strings.Builder
	sb.WriteString("  协议/服务状态\n\n")

	// Table header.
	sb.WriteString(fmt.Sprintf("  %s  %s  %s  %s  %s\n",
		headerStyle.Render(fmt.Sprintf("%-14s", "协议")),
		headerStyle.Render(fmt.Sprintf("%-8s", "状态")),
		headerStyle.Render(fmt.Sprintf("%-7s", "端口")),
		headerStyle.Render(fmt.Sprintf("%-6s", "用户")),
		headerStyle.Render("详情"),
	))
	sb.WriteString(sepStyle.Render("  "+strings.Repeat("─", 56)) + "\n")

	// Protocol rows from inventory.
	for _, p := range inv {
		svcName := protocolServiceName(p.Type)
		st, _ := service.GetStatus(ctx, svcName)

		var dot string
		var statusText string
		if st == nil {
			dot = grayDot
			statusText = "未安装"
		} else if st.Running {
			dot = greenDot
			statusText = "运行中"
		} else {
			dot = redDot
			statusText = "已停止"
		}

		detail := ""
		if p.HasReality {
			detail = "Reality"
		}

		sb.WriteString(fmt.Sprintf("  %-14s  %s %-6s  %-7d  %-6d  %s\n",
			p.Type, dot, statusText, p.Port, p.UserCount, detail))
	}

	// Snell row (if configured).
	if v.model.Store().SnellConf != nil {
		st, _ := service.GetStatus(ctx, service.Snell)
		var dot, statusText string
		if st == nil {
			dot = grayDot
			statusText = "未安装"
		} else if st.Running {
			dot = greenDot
			statusText = "运行中"
		} else {
			dot = redDot
			statusText = "已停止"
		}
		port := snellPort(v.model.Store().SnellConf.Listen)
		sb.WriteString(fmt.Sprintf("  %-14s  %s %-6s  %-7d  %-6d  %s\n",
			"snell", dot, statusText, port, 1, "psk"))
	}

	// All managed services.
	sb.WriteString(sepStyle.Render("  "+strings.Repeat("─", 56)) + "\n")
	for _, extra := range []struct {
		name    string
		svcName service.Name
	}{
		{"sing-box", service.SingBox},
		{"snell-v5", service.Snell},
		{"shadow-tls", service.ShadowTLS},
		{"caddy-sub", service.CaddySub},
		{"watchdog", service.Watchdog},
	} {
		st, _ := service.GetStatus(ctx, extra.svcName)
		var dot, statusText string
		if st == nil {
			dot = grayDot
			statusText = "未安装"
		} else if st.Running {
			dot = greenDot
			statusText = "运行中"
		} else {
			dot = redDot
			statusText = "已停止"
		}
		sb.WriteString(fmt.Sprintf("  %-14s  %s %s\n", extra.name, dot, statusText))
	}

	return svcActionDoneMsg{result: sb.String()}
}

func (v *ServiceView) doBulkAction(action string) tea.Msg {
	ctx := context.Background()
	svcs := service.AllServices()

	var sb strings.Builder
	var actionName string
	switch action {
	case "restart":
		actionName = "重启"
	case "stop":
		actionName = "停止"
	case "start":
		actionName = "启动"
	}

	for _, svc := range svcs {
		// Skip services that are not installed.
		if !service.IsInstalled(ctx, svc) {
			sb.WriteString(fmt.Sprintf("  %s %s: 未安装\n", actionName, svc))
			continue
		}
		var err error
		switch action {
		case "restart":
			err = service.Restart(ctx, svc)
		case "stop":
			err = service.Stop(ctx, svc)
		case "start":
			err = service.Start(ctx, svc)
		}
		if err != nil {
			sb.WriteString(fmt.Sprintf("  %s %s: 失败 (%s)\n", actionName, svc, err))
		} else {
			sb.WriteString(fmt.Sprintf("  %s %s: 成功\n", actionName, svc))
		}
	}
	return svcActionDoneMsg{result: sb.String()}
}

func (v *ServiceView) doSingleAction(svcName service.Name, action string) tea.Msg {
	ctx := context.Background()

	// Check if service is installed before attempting action.
	if !service.IsInstalled(ctx, svcName) {
		return svcActionDoneMsg{result: fmt.Sprintf("%s 未安装，无法操作", svcName)}
	}

	var err error
	var actionLabel string
	switch action {
	case "restart":
		actionLabel = "重启"
		err = service.Restart(ctx, svcName)
	case "stop":
		actionLabel = "停止"
		err = service.Stop(ctx, svcName)
	case "start":
		actionLabel = "启动"
		err = service.Start(ctx, svcName)
	}
	if err != nil {
		return svcActionDoneMsg{result: fmt.Sprintf("%s %s: 失败 (%s)", actionLabel, svcName, err)}
	}
	return svcActionDoneMsg{result: fmt.Sprintf("%s %s: 成功", actionLabel, svcName)}
}

func (v *ServiceView) buildServiceSelectMenu() tui.MenuModel {
	ctx := context.Background()
	svcs := service.AllServices()
	items := make([]tui.MenuItem, 0, len(svcs)+1)
	for i, svc := range svcs {
		key := rune('1' + i)
		label := string(svc)
		if !service.IsInstalled(ctx, svc) {
			label += " (未安装)"
		}
		items = append(items, tui.MenuItem{
			Key:   key,
			Label: label,
			ID:    "svc:" + string(svc),
		})
	}
	items = append(items, tui.MenuItem{Key: '0', Label: "󰌍 返回", ID: "back"})
	return tui.NewMenu("选择服务", items)
}

func (v *ServiceView) buildActionMenu(svcName service.Name) tui.MenuModel {
	title := fmt.Sprintf("管理: %s", svcName)
	items := []tui.MenuItem{
		{Key: '1', Label: "󰑓 重启", ID: "restart"},
		{Key: '2', Label: "󰓛 停止", ID: "stop"},
		{Key: '3', Label: "󰐊 启动", ID: "start"},
		{Key: '0', Label: "󰌍 返回", ID: "back"},
	}
	return tui.NewMenu(title, items)
}

// snellPort extracts the port number from a listen address like "0.0.0.0:8448".
func snellPort(listen string) int {
	if idx := strings.LastIndex(listen, ":"); idx >= 0 {
		if p, err := strconv.Atoi(listen[idx+1:]); err == nil {
			return p
		}
	}
	return 0
}

// protocolServiceName maps a protocol type to its systemd service name.
func protocolServiceName(protoType string) service.Name {
	switch protoType {
	case "vless", "trojan", "shadowsocks", "tuic", "anytls", "ss":
		return service.SingBox
	case "snell":
		return service.Snell
	default:
		return service.SingBox
	}
}
