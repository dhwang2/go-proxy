package pages

import (
	"context"

	"github.com/rivo/tview"

	"go-proxy/internal/service"
	"go-proxy/internal/tui"
	"go-proxy/internal/tui/dialog"
)

// UninstallPage handles service uninstallation.
type UninstallPage struct {
	state *tui.AppState
	box   *tview.Box
}

// NewUninstallPage creates the uninstall page.
func NewUninstallPage(state *tui.AppState) *UninstallPage {
	return &UninstallPage{
		state: state,
		box:   tview.NewBox(),
	}
}

func (p *UninstallPage) Name() string          { return "uninstall" }
func (p *UninstallPage) Root() tview.Primitive { return p.box }

func (p *UninstallPage) OnEnter() {
	d := dialog.NewConfirm("Uninstall all services and configuration?", func(yes bool) {
		p.state.DismissDialog("uninstall-confirm")
		if !yes {
			p.state.Back()
			return
		}
		ctx := context.Background()
		if err := service.Uninstall(ctx); err != nil {
			p.showResult("Uninstall error: " + err.Error())
			return
		}
		p.showResult("Uninstalled successfully")
	})
	p.state.ShowDialog("uninstall-confirm", d)
}

func (p *UninstallPage) showResult(msg string) {
	d := dialog.NewResult(msg, func() {
		p.state.DismissDialog("uninstall-result")
		p.state.App.Stop()
	})
	p.state.ShowDialog("uninstall-result", d)
}
