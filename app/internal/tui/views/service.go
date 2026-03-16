package views

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"go-proxy/internal/service"
	"go-proxy/internal/tui"
	"go-proxy/internal/tui/components"
)

type ServiceView struct {
	model *tui.Model
	menu  components.MenuModel
}

func NewServiceView(model *tui.Model) *ServiceView {
	v := &ServiceView{model: model}
	v.menu = components.NewMenu("协议管理", []components.MenuItem{
		{Key: '1', Label: "重启所有服务", ID: "restart"},
		{Key: '2', Label: "停止所有服务", ID: "stop"},
		{Key: '3', Label: "启动所有服务", ID: "start"},
		{Key: '4', Label: "查看服务状态", ID: "status"},
		{Key: '0', Label: "返回", ID: "back"},
	})
	return v
}

func (v *ServiceView) Name() string { return "service" }

func (v *ServiceView) Init() tea.Cmd { return nil }

func (v *ServiceView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case components.MenuSelectMsg:
		if msg.ID == "back" {
			return v, tui.BackCmd
		}
		action := msg.ID
		return v, func() tea.Msg { return v.doAction(action) }

	case svcActionDoneMsg:
		return v, func() tea.Msg {
			return tui.ShowOverlayMsg{
				Overlay: components.NewResult(msg.result),
			}
		}

	case tui.ResultDismissedMsg:
		return v, nil

	default:
		var cmd tea.Cmd
		v.menu, cmd = v.menu.Update(msg)
		return v, cmd
	}
}

func (v *ServiceView) View() string { return v.menu.View() }

type svcActionDoneMsg struct{ result string }

func (v *ServiceView) doAction(action string) tea.Msg {
	ctx := context.Background()
	svcs := service.AllServices()

	if action == "status" {
		return svcActionDoneMsg{result: v.getAllStatus(ctx, svcs)}
	}

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

func (v *ServiceView) getAllStatus(ctx context.Context, svcs []service.Name) string {
	var sb strings.Builder
	sb.WriteString("服务状态\n\n")
	for _, svc := range svcs {
		st, err := service.GetStatus(ctx, svc)
		if err != nil {
			sb.WriteString(fmt.Sprintf("  %s: 未配置\n", svc))
			continue
		}
		status := "未运行"
		if st.Running {
			status = "运行中"
		}
		sb.WriteString(fmt.Sprintf("  %s: %s\n", svc, status))
	}
	return sb.String()
}
