package pages

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"go-proxy/internal/tui"
	"go-proxy/internal/tui/dialog"
)

// LogsPage handles runtime log viewing.
type LogsPage struct {
	state *tui.AppState
	list  *tview.List
}

// NewLogsPage creates the logs page.
func NewLogsPage(state *tui.AppState) *LogsPage {
	p := &LogsPage{state: state}

	p.list = tview.NewList().
		ShowSecondaryText(false).
		SetHighlightFullLine(true).
		SetMainTextColor(tcell.ColorWhite).
		SetSelectedTextColor(tcell.ColorTeal).
		SetSelectedBackgroundColor(tcell.ColorDarkSlateGray)
	p.list.SetTitle(" Runtime Logs ").
		SetTitleColor(tcell.ColorTeal).
		SetBorder(true).
		SetBorderColor(tcell.ColorTeal)

	return p
}

func (p *LogsPage) Name() string          { return "logs" }
func (p *LogsPage) Root() tview.Primitive { return p.list }

func (p *LogsPage) OnEnter() {
	p.list.Clear()
	sources := []string{"sing-box", "snell-v5", "shadow-tls", "caddy-sub", "proxy-script", "proxy-watchdog"}
	for i, src := range sources {
		name := src
		key := rune('1' + i)
		p.list.AddItem(name, "", key, func() {
			p.showResult("Use 'journalctl -u " + name + " -f' for logs")
		})
	}
	p.list.AddItem("返回  Back", "", '0', func() {
		p.state.Back()
	})
	p.state.App.SetFocus(p.list)
}

func (p *LogsPage) showResult(msg string) {
	d := dialog.NewResult(msg, func() {
		p.state.DismissDialog("logs-result")
	})
	p.state.ShowDialog("logs-result", d)
}
