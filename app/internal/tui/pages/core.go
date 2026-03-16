package pages

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"go-proxy/internal/tui"
	"go-proxy/internal/tui/dialog"
)

// CorePage handles core binary management.
type CorePage struct {
	state *tui.AppState
	list  *tview.List
}

// NewCorePage creates the core management page.
func NewCorePage(state *tui.AppState) *CorePage {
	p := &CorePage{state: state}

	p.list = tview.NewList().
		ShowSecondaryText(false).
		SetHighlightFullLine(true).
		SetMainTextColor(tcell.ColorWhite).
		SetSelectedTextColor(tcell.ColorTeal).
		SetSelectedBackgroundColor(tcell.ColorDarkSlateGray)
	p.list.SetTitle(" Core Management ").
		SetTitleColor(tcell.ColorTeal).
		SetBorder(true).
		SetBorderColor(tcell.ColorTeal)

	return p
}

func (p *CorePage) Name() string          { return "core" }
func (p *CorePage) Root() tview.Primitive { return p.list }

func (p *CorePage) OnEnter() {
	p.list.Clear()
	p.list.AddItem("查看版本  View Versions", "", '1', func() {
		p.showResult("Not implemented yet")
	})
	p.list.AddItem("检查更新  Check Updates", "", '2', func() {
		p.showResult("Not implemented yet")
	})
	p.list.AddItem("更新内核  Update Core", "", '3', func() {
		p.showResult("Not implemented yet")
	})
	p.list.AddItem("返回  Back", "", '0', func() {
		p.state.Back()
	})
	p.state.App.SetFocus(p.list)
}

func (p *CorePage) showResult(msg string) {
	d := dialog.NewResult(msg, func() {
		p.state.DismissDialog("core-result")
	})
	p.state.ShowDialog("core-result", d)
}
