package views

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"go-proxy/internal/service"
	"go-proxy/internal/tui"
	"go-proxy/internal/tui/components"
)

type ServiceView struct {
	model      *tui.Model
	menu       components.MenuModel
	step       serviceStep
	pendingSvc service.Name
}

type serviceStep int

const (
	serviceMenu serviceStep = iota
	serviceActions
	serviceResult
)

func NewServiceView(model *tui.Model) *ServiceView {
	return &ServiceView{model: model}
}

func (v *ServiceView) Name() string { return "service" }

func (v *ServiceView) Init() tea.Cmd {
	v.step = serviceMenu
	svcs := service.AllServices()
	items := make([]components.MenuItem, 0, len(svcs)+1)
	for i, svc := range svcs {
		items = append(items, components.MenuItem{
			Key:   rune('1' + i),
			Label: string(svc),
			ID:    string(svc),
		})
	}
	items = append(items, components.MenuItem{Key: '0', Label: "返回  Back", ID: "back"})
	v.menu = v.menu.SetItems(items)
	return nil
}

func (v *ServiceView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case components.MenuSelectMsg:
		switch v.step {
		case serviceMenu:
			if msg.ID == "back" {
				return v, func() tea.Msg { return tui.BackMsg{} }
			}
			v.pendingSvc = service.Name(msg.ID)
			v.step = serviceActions
			actionMenu := components.NewMenu(string(v.pendingSvc), []components.MenuItem{
				{Key: '1', Label: "启动  Start", ID: "start"},
				{Key: '2', Label: "停止  Stop", ID: "stop"},
				{Key: '3', Label: "重启  Restart", ID: "restart"},
				{Key: '4', Label: "状态  Status", ID: "status"},
				{Key: '0', Label: "返回  Back", ID: "action-back"},
			})
			return v, func() tea.Msg {
				return tui.ShowOverlayMsg{Overlay: menuOverlay{menu: actionMenu}}
			}

		case serviceActions:
			// This comes from the overlay menu.
		}
		return v, nil

	case svcActionSelectMsg:
		if msg.id == "action-back" {
			v.step = serviceMenu
			return v, func() tea.Msg { return tui.DismissOverlayMsg{} }
		}
		svc := v.pendingSvc
		action := msg.id
		return v, func() tea.Msg { return v.doAction(svc, action) }

	case svcActionDoneMsg:
		v.step = serviceResult
		return v, func() tea.Msg {
			return tui.ShowOverlayMsg{
				Overlay: components.NewResult(msg.result),
			}
		}

	case tui.ResultDismissedMsg:
		v.step = serviceMenu
		return v, func() tea.Msg { return tui.DismissOverlayMsg{} }

	default:
		if v.step == serviceMenu {
			var cmd tea.Cmd
			v.menu, cmd = v.menu.Update(msg)
			return v, cmd
		}
	}
	return v, nil
}

func (v *ServiceView) View() string { return v.menu.View() }

type svcActionDoneMsg struct{ result string }
type svcActionSelectMsg struct{ id string }

func (v *ServiceView) doAction(svc service.Name, action string) tea.Msg {
	ctx := context.Background()
	switch action {
	case "start":
		if err := service.Start(ctx, svc); err != nil {
			return svcActionDoneMsg{result: fmt.Sprintf("Start %s failed: %s", svc, err)}
		}
		return svcActionDoneMsg{result: fmt.Sprintf("Start %s: OK", svc)}
	case "stop":
		if err := service.Stop(ctx, svc); err != nil {
			return svcActionDoneMsg{result: fmt.Sprintf("Stop %s failed: %s", svc, err)}
		}
		return svcActionDoneMsg{result: fmt.Sprintf("Stop %s: OK", svc)}
	case "restart":
		if err := service.Restart(ctx, svc); err != nil {
			return svcActionDoneMsg{result: fmt.Sprintf("Restart %s failed: %s", svc, err)}
		}
		return svcActionDoneMsg{result: fmt.Sprintf("Restart %s: OK", svc)}
	case "status":
		st, err := service.GetStatus(ctx, svc)
		if err != nil {
			return svcActionDoneMsg{result: "Error: " + err.Error()}
		}
		status := "inactive"
		if st.Running {
			status = "active (running)"
		}
		return svcActionDoneMsg{result: fmt.Sprintf("%s: %s", svc, status)}
	}
	return svcActionDoneMsg{result: "unknown action"}
}

// menuOverlay wraps a MenuModel as an OverlayModel for service actions.
type menuOverlay struct {
	menu components.MenuModel
}

func (o menuOverlay) Init() tea.Cmd { return nil }

func (o menuOverlay) Update(msg tea.Msg) (tui.OverlayModel, tea.Cmd) {
	switch msg := msg.(type) {
	case components.MenuSelectMsg:
		id := msg.ID
		return o, func() tea.Msg { return svcActionSelectMsg{id: id} }
	default:
		var cmd tea.Cmd
		o.menu, cmd = o.menu.Update(msg)
		return o, cmd
	}
}

func (o menuOverlay) View() string {
	return tui.DialogStyle.Render(o.menu.View())
}
