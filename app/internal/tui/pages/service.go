package pages

import (
	"context"
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"go-proxy/internal/service"
	"go-proxy/internal/tui"
	"go-proxy/internal/tui/dialog"
)

// ServicePage handles service management.
type ServicePage struct {
	state *tui.AppState
	list  *tview.List
}

// NewServicePage creates the service management page.
func NewServicePage(state *tui.AppState) *ServicePage {
	p := &ServicePage{state: state}

	p.list = tview.NewList().
		ShowSecondaryText(false).
		SetHighlightFullLine(true).
		SetMainTextColor(tcell.ColorWhite).
		SetSelectedTextColor(tcell.ColorTeal).
		SetSelectedBackgroundColor(tcell.ColorDarkSlateGray)
	p.list.SetTitle(" Service Management ").
		SetTitleColor(tcell.ColorTeal).
		SetBorder(true).
		SetBorderColor(tcell.ColorTeal)

	return p
}

func (p *ServicePage) Name() string          { return "service" }
func (p *ServicePage) Root() tview.Primitive { return p.list }

func (p *ServicePage) OnEnter() {
	p.list.Clear()
	svcs := service.AllServices()
	for i, svc := range svcs {
		name := svc
		key := rune('1' + i)
		p.list.AddItem(string(name), "", key, func() {
			p.showServiceActions(name)
		})
	}
	p.list.AddItem("返回  Back", "", '0', func() {
		p.state.Back()
	})
	p.state.App.SetFocus(p.list)
}

func (p *ServicePage) showServiceActions(svc service.Name) {
	actionList := tview.NewList().
		ShowSecondaryText(false).
		SetHighlightFullLine(true).
		SetMainTextColor(tcell.ColorWhite).
		SetSelectedTextColor(tcell.ColorTeal).
		SetSelectedBackgroundColor(tcell.ColorDarkSlateGray)
	actionList.SetTitle(fmt.Sprintf(" %s ", svc)).
		SetTitleColor(tcell.ColorTeal).
		SetBorder(true).
		SetBorderColor(tcell.ColorTeal)

	actionList.AddItem("启动  Start", "", '1', func() {
		p.state.DismissDialog("svc-actions")
		err := service.Start(context.Background(), svc)
		p.showActionResult("Start", string(svc), err)
	})
	actionList.AddItem("停止  Stop", "", '2', func() {
		p.state.DismissDialog("svc-actions")
		err := service.Stop(context.Background(), svc)
		p.showActionResult("Stop", string(svc), err)
	})
	actionList.AddItem("重启  Restart", "", '3', func() {
		p.state.DismissDialog("svc-actions")
		err := service.Restart(context.Background(), svc)
		p.showActionResult("Restart", string(svc), err)
	})
	actionList.AddItem("状态  Status", "", '4', func() {
		p.state.DismissDialog("svc-actions")
		st, err := service.GetStatus(context.Background(), svc)
		if err != nil {
			p.showResult("Error: " + err.Error())
			return
		}
		status := "inactive"
		if st.Running {
			status = "active (running)"
		}
		p.showResult(fmt.Sprintf("%s: %s", svc, status))
	})
	actionList.AddItem("返回  Back", "", '0', func() {
		p.state.DismissDialog("svc-actions")
	})

	// Center the action list.
	flex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(actionList, 9, 0, true).
			AddItem(nil, 0, 1, false), 40, 0, true).
		AddItem(nil, 0, 1, false)

	p.state.ShowDialog("svc-actions", flex)
}

func (p *ServicePage) showActionResult(action, target string, err error) {
	var msg string
	if err != nil {
		msg = fmt.Sprintf("%s %s failed: %s", action, target, err)
	} else {
		msg = fmt.Sprintf("%s %s: OK", action, target)
	}
	p.showResult(msg)
}

func (p *ServicePage) showResult(msg string) {
	d := dialog.NewResult(msg, func() {
		p.state.DismissDialog("svc-result")
	})
	p.state.ShowDialog("svc-result", d)
}
