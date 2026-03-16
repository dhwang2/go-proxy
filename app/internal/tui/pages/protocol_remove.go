package pages

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"go-proxy/internal/derived"
	"go-proxy/internal/protocol"
	"go-proxy/internal/tui"
	"go-proxy/internal/tui/dialog"
)

// ProtocolRemovePage handles protocol removal.
type ProtocolRemovePage struct {
	state *tui.AppState
	list  *tview.List
}

// NewProtocolRemovePage creates the protocol removal page.
func NewProtocolRemovePage(state *tui.AppState) *ProtocolRemovePage {
	p := &ProtocolRemovePage{state: state}

	p.list = tview.NewList().
		ShowSecondaryText(false).
		SetHighlightFullLine(true).
		SetMainTextColor(tcell.ColorWhite).
		SetSelectedTextColor(tcell.ColorTeal).
		SetSelectedBackgroundColor(tcell.ColorDarkSlateGray)
	p.list.SetTitle(" Remove Protocol ").
		SetTitleColor(tcell.ColorTeal).
		SetBorder(true).
		SetBorderColor(tcell.ColorTeal)

	return p
}

func (p *ProtocolRemovePage) Name() string          { return "protocol-remove" }
func (p *ProtocolRemovePage) Root() tview.Primitive { return p.list }

func (p *ProtocolRemovePage) OnEnter() {
	p.list.Clear()
	inv := derived.Inventory(p.state.Store)
	for i, info := range inv {
		tag := info.Tag
		key := rune('1' + i)
		if i >= 9 {
			key = rune('a' + i - 9)
		}
		label := fmt.Sprintf("%s (port %d, %d users)", info.Tag, info.Port, info.UserCount)
		p.list.AddItem(label, "", key, func() {
			p.confirmRemove(tag)
		})
	}
	p.list.AddItem("返回  Back", "", '0', func() {
		p.state.Back()
	})
	p.state.App.SetFocus(p.list)
}

func (p *ProtocolRemovePage) confirmRemove(tag string) {
	d := dialog.NewConfirm(fmt.Sprintf("Remove %s?", tag), func(yes bool) {
		p.state.DismissDialog("remove-confirm")
		if !yes {
			return
		}
		if err := protocol.Remove(p.state.Store, tag); err != nil {
			p.showResult("Error: " + err.Error())
			return
		}
		if err := p.state.Store.Apply(); err != nil {
			p.showResult("Failed to save: " + err.Error())
			return
		}
		p.showResult("Removed " + tag)
	})
	p.state.ShowDialog("remove-confirm", d)
}

func (p *ProtocolRemovePage) showResult(msg string) {
	d := dialog.NewResult(msg, func() {
		p.state.DismissDialog("remove-result")
		p.OnEnter()
	})
	p.state.ShowDialog("remove-result", d)
}
