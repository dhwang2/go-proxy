package pages

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"go-proxy/internal/tui"
	"go-proxy/internal/tui/dialog"
)

// NetworkPage handles network management.
type NetworkPage struct {
	state *tui.AppState
	list  *tview.List
}

// NewNetworkPage creates the network management page.
func NewNetworkPage(state *tui.AppState) *NetworkPage {
	p := &NetworkPage{state: state}

	p.list = tview.NewList().
		ShowSecondaryText(false).
		SetHighlightFullLine(true).
		SetMainTextColor(tcell.ColorWhite).
		SetSelectedTextColor(tcell.ColorTeal).
		SetSelectedBackgroundColor(tcell.ColorDarkSlateGray)
	p.list.SetTitle(" Network Management ").
		SetTitleColor(tcell.ColorTeal).
		SetBorder(true).
		SetBorderColor(tcell.ColorTeal)

	return p
}

func (p *NetworkPage) Name() string          { return "network" }
func (p *NetworkPage) Root() tview.Primitive { return p.list }

func (p *NetworkPage) OnEnter() {
	p.list.Clear()
	p.list.AddItem("BBR 状态  BBR Status", "", '1', func() {
		p.showResult("Not implemented yet (requires Linux)")
	})
	p.list.AddItem("启用 BBR  Enable BBR", "", '2', func() {
		p.showResult("Not implemented yet (requires Linux)")
	})
	p.list.AddItem("防火墙  Firewall Rules", "", '3', func() {
		p.showResult("Not implemented yet (requires Linux)")
	})
	p.list.AddItem("返回  Back", "", '0', func() {
		p.state.Back()
	})
	p.state.App.SetFocus(p.list)
}

func (p *NetworkPage) showResult(msg string) {
	d := dialog.NewResult(msg, func() {
		p.state.DismissDialog("network-result")
	})
	p.state.ShowDialog("network-result", d)
}
