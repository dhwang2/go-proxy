package pages

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"go-proxy/internal/routing"
	"go-proxy/internal/tui"
	"go-proxy/internal/tui/dialog"
)

// RoutingPage handles routing management.
type RoutingPage struct {
	state *tui.AppState
	list  *tview.List
}

// NewRoutingPage creates the routing management page.
func NewRoutingPage(state *tui.AppState) *RoutingPage {
	p := &RoutingPage{state: state}

	p.list = tview.NewList().
		ShowSecondaryText(false).
		SetHighlightFullLine(true).
		SetMainTextColor(tcell.ColorWhite).
		SetSelectedTextColor(tcell.ColorTeal).
		SetSelectedBackgroundColor(tcell.ColorDarkSlateGray)
	p.list.SetTitle(" Routing Management ").
		SetTitleColor(tcell.ColorTeal).
		SetBorder(true).
		SetBorderColor(tcell.ColorTeal)

	return p
}

func (p *RoutingPage) Name() string          { return "routing" }
func (p *RoutingPage) Root() tview.Primitive { return p.list }

func (p *RoutingPage) OnEnter() {
	p.list.Clear()
	p.list.AddItem("设置规则  Set Rules", "", '1', func() {
		p.showResult("Not implemented yet")
	})
	p.list.AddItem("清除规则  Clear Rules", "", '2', func() {
		p.showResult("Not implemented yet")
	})
	p.list.AddItem("链式代理  Chain Proxy", "", '3', func() {
		p.showResult("Not implemented yet")
	})
	p.list.AddItem("测试规则  Test Rules", "", '4', func() {
		p.showResult("Not implemented yet")
	})
	p.list.AddItem("同步 DNS  Sync DNS", "", '5', func() {
		routing.SyncDNS(p.state.Store, nil, "ipv4_only")
		routing.SyncRouteRules(p.state.Store)
		if err := p.state.Store.Apply(); err != nil {
			p.showResult("Failed to save: " + err.Error())
			return
		}
		p.showResult("DNS and route rules synced")
	})
	p.list.AddItem("返回  Back", "", '0', func() {
		p.state.Back()
	})
	p.state.App.SetFocus(p.list)
}

func (p *RoutingPage) showResult(msg string) {
	d := dialog.NewResult(msg, func() {
		p.state.DismissDialog("routing-result")
	})
	p.state.ShowDialog("routing-result", d)
}
