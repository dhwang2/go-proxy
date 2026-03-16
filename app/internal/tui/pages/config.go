package pages

import (
	"encoding/json"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"go-proxy/internal/tui"
)

// ConfigPage displays the sing-box configuration.
type ConfigPage struct {
	state    *tui.AppState
	textView *tview.TextView
}

// NewConfigPage creates the configuration viewer page.
func NewConfigPage(state *tui.AppState) *ConfigPage {
	p := &ConfigPage{state: state}

	p.textView = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetTextColor(tcell.ColorWhite)
	p.textView.SetTitle(" sing-box Configuration ").
		SetTitleColor(tcell.ColorTeal).
		SetBorder(true).
		SetBorderColor(tcell.ColorTeal)

	return p
}

func (p *ConfigPage) Name() string          { return "config" }
func (p *ConfigPage) Root() tview.Primitive { return p.textView }

func (p *ConfigPage) OnEnter() {
	data, err := json.MarshalIndent(p.state.Store.SingBox, "", "  ")
	if err != nil {
		p.textView.SetText("Error rendering config: " + err.Error())
	} else {
		p.textView.SetText(string(data))
	}
	p.textView.ScrollToBeginning()
	p.state.App.SetFocus(p.textView)
}
