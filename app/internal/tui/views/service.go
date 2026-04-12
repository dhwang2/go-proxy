package views

import (
	"context"
	"fmt"
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
	tui.SplitViewBase
	subMenu tui.MenuModel
	step    svcStep
	target  service.Name // selected individual service
}

func NewServiceView(model *tui.Model) *ServiceView {
	v := &ServiceView{}
	v.Model = model
	v.Menu = tui.NewMenu("", []tui.MenuItem{
		{Key: '1', Label: "󰋼 查看服务状态", ID: "status"},
		{Key: '2', Label: "󰑓 重启所有服务", ID: "restart-all"},
		{Key: '3', Label: "󰓛 停止所有服务", ID: "stop-all"},
		{Key: '4', Label: "󰐊 启动所有服务", ID: "start-all"},
	})
	return v
}

func (v *ServiceView) Name() string { return "service" }

func (v *ServiceView) setFocus(left bool) {
	v.SetFocus(left, func(l bool) { v.subMenu = v.subMenu.SetDim(!l) })
}

func (v *ServiceView) Init() tea.Cmd {
	v.step = svcMenuMain
	v.InitSplit()
	return nil
}

func (v *ServiceView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.ViewResizeMsg:
		v.HandleResize(msg)
		return v, nil
	case tui.SubSplitMouseMsg:
		return v, v.HandleMouse(msg)
	}
	// In split mode, intercept up/down for menu navigation even when content is showing.
	if cmd, ok := v.HandleMenuNav(msg, v.step == svcMenuMain, false); ok {
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
		if v.step == svcMenuMain {
			v.setFocus(false)
			return v, v.triggerMenuAction(msg.ID)
		}
		return v.handleMenuSelect(msg)

	case svcActionDoneMsg:
		v.step = svcResult
		v.setFocus(false)
		return v, v.SetInline(components.NewResult(msg.result))

	case tui.ResultDismissedMsg:
		v.step = svcMenuMain
		v.setFocus(true)
		return v, nil

	default:
		return v.handleDefault(msg)
	}
}

// triggerMenuAction executes the action for the given main menu item ID.
func (v *ServiceView) triggerMenuAction(id string) tea.Cmd {
	switch id {
	case "status":
		return func() tea.Msg { return v.doStatusTable() }
	case "restart-all":
		return func() tea.Msg { return v.doBulkAction("restart") }
	case "stop-all":
		return func() tea.Msg { return v.doBulkAction("stop") }
	case "start-all":
		return func() tea.Msg { return v.doBulkAction("start") }
	case "individual":
		v.step = svcMenuIndividual
		v.subMenu = v.buildServiceSelectMenu()
		return nil
	}
	return nil
}

func (v *ServiceView) handleMenuSelect(msg tui.MenuSelectMsg) (tui.View, tea.Cmd) {
	switch v.step {

	case svcMenuIndividual:
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
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.Type == tea.KeyEsc {
		switch v.step {
		case svcMenuIndividual:
			v.step = svcMenuMain
			v.setFocus(true)
			return v, nil
		default:
			return v, tui.BackCmd
		}
	}
	// Left/Right arrow toggles sub-split focus (use local setFocus to also dim subMenu).
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if v.Split.Enabled() && v.step != svcMenuMain {
			if keyMsg.Type == tea.KeyLeft {
				v.setFocus(true)
				return v, nil
			}
			if keyMsg.Type == tea.KeyRight && v.HasInline() {
				v.setFocus(false)
				return v, nil
			}
		}
	}
	var cmd tea.Cmd
	switch v.step {
	case svcMenuMain:
		v.Menu, cmd = v.Menu.Update(msg)
	case svcMenuIndividual:
		v.subMenu, cmd = v.subMenu.Update(msg)
	}
	return v, cmd
}

func (v *ServiceView) View() string {
	if v.step == svcMenuMain || !v.Split.Enabled() {
		if v.HasInline() {
			return v.ViewInline()
		}
		if v.step == svcMenuIndividual {
			return v.subMenu.View()
		}
		return v.Menu.View()
	}

	var menuContent string
	if v.step == svcMenuIndividual {
		menuContent = v.subMenu.View()
	} else {
		menuContent = v.Menu.View()
	}

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

	return v.Split.View(menuContent, detailContent)
}

type svcActionDoneMsg struct{ result string }

// doStatusTable renders a shell-proxy style protocol overview and service status.
func (v *ServiceView) doStatusTable() tea.Msg {
	ctx := context.Background()
	s := v.Model.Store()

	userStyle := lipgloss.NewStyle().Foreground(tui.ColorAccent).Bold(true)
	protoStyle := lipgloss.NewStyle().Foreground(tui.ColorSuccess)
	portStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#C678DD"))
	backPortStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#56B6C2"))
	greenDot := lipgloss.NewStyle().Foreground(tui.ColorSuccess).Render("●")
	redDot := lipgloss.NewStyle().Foreground(tui.ColorError).Render("●")
	grayCircle := lipgloss.NewStyle().Foreground(tui.ColorMuted).Render("○")
	sepStyle := lipgloss.NewStyle().Foreground(tui.ColorMuted)

	// Build shadow-tls binding lookup: "ss|<backendPort>" or "snell|<backendPort>" -> frontendListenPort
	stlsLookup := make(map[string]int)
	if bindings, err := service.ListShadowTLSBindings(s); err == nil {
		for _, b := range bindings {
			if b.BackendProto == "ss" || b.BackendProto == "snell" {
				key := fmt.Sprintf("%s|%d", b.BackendProto, b.BackendPort)
				stlsLookup[key] = b.ListenPort
			}
		}
	}

	var sb strings.Builder

	// --- Section 1: Protocol overview (per-user) ---
	names := derived.UserNames(s)
	membership := derived.Membership(s)

	if len(names) == 0 {
		sb.WriteString("  ")
		sb.WriteString(grayCircle)
		sb.WriteString(" 未安装\n")
	} else {
		for _, name := range names {
			sb.WriteString("  ")
			sb.WriteString(userStyle.Render(name))
			sb.WriteString("\n")

			entries := membership[name]
			if len(entries) == 0 {
				sb.WriteString("    ")
				sb.WriteString(grayCircle)
				sb.WriteString(" 无协议\n")
			} else {
				for _, e := range entries {
					port := e.Port
					bulletDot := lipgloss.NewStyle().Foreground(tui.ColorSuccess).Render("●")
					// Check shadow-tls binding for ss and snell.
					var stlsKey, stlsProto, stlsBack string
					switch e.Proto {
					case "shadowsocks", "ss":
						stlsKey = fmt.Sprintf("ss|%d", port)
						stlsProto = "ss+shadow-tls-v3"
						stlsBack = "ss"
					case "snell", "snell-v5":
						stlsKey = fmt.Sprintf("snell|%d", port)
						stlsProto = "snell-v5+shadow-tls-v3"
						stlsBack = "snell"
					}
					if stlsKey != "" {
						if frontPort, ok := stlsLookup[stlsKey]; ok {
							sb.WriteString(fmt.Sprintf("    %s %s - shadow-tls:%s -> %s:%s\n",
								bulletDot,
								protoStyle.Render(stlsProto),
								portStyle.Render(fmt.Sprintf("%d", frontPort)),
								stlsBack,
								backPortStyle.Render(fmt.Sprintf("%d", port)),
							))
							continue
						}
					}
					sb.WriteString(fmt.Sprintf("    %s %s - %s\n",
						bulletDot,
						protoStyle.Render(e.Proto),
						portStyle.Render(fmt.Sprintf("%d", port)),
					))
				}
			}
		}
	}

	// Separator between sections.
	sb.WriteString(sepStyle.Render("  " + strings.Repeat("─", 56)))
	sb.WriteString("\n")

	// --- Section 2: Service status ---
	yellowDot := lipgloss.NewStyle().Foreground(lipgloss.Color("#E5C07B")).Render("●")
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
			if service.BinaryInstalled(extra.svcName) {
				dot = yellowDot
				statusText = "已安装(未运行)"
			} else {
				dot = grayCircle
				statusText = "未安装"
			}
		} else if st.Running {
			dot = greenDot
			statusText = "运行中"
		} else {
			dot = redDot
			statusText = "已停止"
		}
		sb.WriteString(fmt.Sprintf("  %-16s %s %s\n", extra.name+":", dot, statusText))
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
	return tui.NewMenu("选择服务", items)
}

func (v *ServiceView) buildActionMenu(svcName service.Name) tui.MenuModel {
	title := fmt.Sprintf("管理: %s", svcName)
	items := []tui.MenuItem{
		{Key: '1', Label: "󰑓 重启", ID: "restart"},
		{Key: '2', Label: "󰓛 停止", ID: "stop"},
		{Key: '3', Label: "󰐊 启动", ID: "start"},
	}
	return tui.NewMenu(title, items)
}
