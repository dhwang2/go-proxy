package pages

import (
	"github.com/rivo/tview"

	"go-proxy/internal/tui"
	"go-proxy/internal/tui/dialog"
)

// SelfUpdatePage handles self-update.
type SelfUpdatePage struct {
	state *tui.AppState
	box   *tview.Box
}

// NewSelfUpdatePage creates the self-update page.
func NewSelfUpdatePage(state *tui.AppState) *SelfUpdatePage {
	return &SelfUpdatePage{
		state: state,
		box:   tview.NewBox(),
	}
}

func (p *SelfUpdatePage) Name() string          { return "self-update" }
func (p *SelfUpdatePage) Root() tview.Primitive { return p.box }

func (p *SelfUpdatePage) OnEnter() {
	d := dialog.NewResult("Self-update not yet implemented", func() {
		p.state.DismissDialog("update-result")
		p.state.Back()
	})
	p.state.ShowDialog("update-result", d)
}
