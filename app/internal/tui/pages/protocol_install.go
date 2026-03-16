package pages

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"go-proxy/internal/protocol"
	"go-proxy/internal/tui"
	"go-proxy/internal/tui/dialog"
)

// ProtocolInstallPage handles protocol installation.
type ProtocolInstallPage struct {
	state *tui.AppState
	list  *tview.List
}

// NewProtocolInstallPage creates the protocol installation page.
func NewProtocolInstallPage(state *tui.AppState) *ProtocolInstallPage {
	p := &ProtocolInstallPage{state: state}

	p.list = tview.NewList().
		ShowSecondaryText(false).
		SetHighlightFullLine(true).
		SetMainTextColor(tcell.ColorWhite).
		SetSelectedTextColor(tcell.ColorTeal).
		SetSelectedBackgroundColor(tcell.ColorDarkSlateGray)
	p.list.SetTitle(" Select Protocol ").
		SetTitleColor(tcell.ColorTeal).
		SetBorder(true).
		SetBorderColor(tcell.ColorTeal)

	return p
}

func (p *ProtocolInstallPage) Name() string          { return "protocol-install" }
func (p *ProtocolInstallPage) Root() tview.Primitive { return p.list }

func (p *ProtocolInstallPage) OnEnter() {
	p.list.Clear()
	types := protocol.AllTypes()
	specs := protocol.Specs()
	for i, t := range types {
		pt := t
		key := rune('1' + i)
		if i >= 9 {
			key = rune('a' + i - 9)
		}
		p.list.AddItem(specs[pt].DisplayName, "", key, func() {
			p.promptPort(pt)
		})
	}
	p.list.AddItem("返回  Back", "", '0', func() {
		p.state.Back()
	})
	p.state.App.SetFocus(p.list)
}

func (p *ProtocolInstallPage) promptPort(pt protocol.Type) {
	d := dialog.NewInput("Port number:", "8443", func(val string) {
		p.state.DismissDialog("install-port")
		if val == "" {
			return
		}
		var port int
		fmt.Sscanf(val, "%d", &port)
		if port <= 0 || port > 65535 {
			p.showResult("Invalid port number")
			return
		}
		result, err := protocol.Install(p.state.Store, protocol.InstallParams{
			ProtoType: pt,
			Port:      port,
			UserName:  "user",
		})
		if err != nil {
			p.showResult("Error: " + err.Error())
			return
		}
		if err := p.state.Store.Apply(); err != nil {
			p.showResult("Failed to save: " + err.Error())
			return
		}
		msg := fmt.Sprintf("Installed %s on port %d\nTag: %s\nCredential: %s",
			pt, result.Port, result.Tag, result.Credential)
		if result.PublicKey != "" {
			msg += "\nPublic Key: " + result.PublicKey
		}
		p.showResult(msg)
	})
	p.state.ShowDialog("install-port", d)
}

func (p *ProtocolInstallPage) showResult(msg string) {
	d := dialog.NewResult(msg, func() {
		p.state.DismissDialog("install-result")
		p.OnEnter()
	})
	p.state.ShowDialog("install-result", d)
}
