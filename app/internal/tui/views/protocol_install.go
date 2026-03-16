package views

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"go-proxy/internal/protocol"
	"go-proxy/internal/tui"
	"go-proxy/internal/tui/components"
)

type ProtocolInstallView struct {
	model       *tui.Model
	menu        components.MenuModel
	step        protoInstallStep
	pendingType protocol.Type
}

type protoInstallStep int

const (
	protoInstallMenu protoInstallStep = iota
	protoInstallPort
	protoInstallResult
)

func NewProtocolInstallView(model *tui.Model) *ProtocolInstallView {
	return &ProtocolInstallView{model: model}
}

func (v *ProtocolInstallView) Name() string { return "protocol-install" }

func (v *ProtocolInstallView) Init() tea.Cmd {
	v.step = protoInstallMenu
	types := protocol.AllTypes()
	specs := protocol.Specs()
	items := make([]components.MenuItem, 0, len(types)+1)
	for i, t := range types {
		k := rune('1' + i)
		if i >= 9 {
			k = rune('a' + i - 9)
		}
		items = append(items, components.MenuItem{
			Key:   k,
			Label: specs[t].DisplayName,
			ID:    string(t),
		})
	}
	items = append(items, components.MenuItem{Key: '0', Label: "返回  Back", ID: "back"})
	v.menu = v.menu.SetItems(items)
	return nil
}

func (v *ProtocolInstallView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case components.MenuSelectMsg:
		if msg.ID == "back" {
			return v, func() tea.Msg { return tui.BackMsg{} }
		}
		v.pendingType = protocol.Type(msg.ID)
		v.step = protoInstallPort
		return v, func() tea.Msg {
			return tui.ShowOverlayMsg{
				Overlay: components.NewTextInput("Port number:", "8443"),
			}
		}

	case tui.InputResultMsg:
		if msg.Cancelled {
			v.step = protoInstallMenu
			return v, func() tea.Msg { return tui.DismissOverlayMsg{} }
		}
		pt := v.pendingType
		portStr := msg.Value
		return v, func() tea.Msg {
			return v.doInstall(pt, portStr)
		}

	case protoInstallDoneMsg:
		v.step = protoInstallResult
		return v, func() tea.Msg {
			return tui.ShowOverlayMsg{
				Overlay: components.NewResult(msg.result),
			}
		}

	case tui.ResultDismissedMsg:
		v.step = protoInstallMenu
		cmd := v.Init()
		return v, func() tea.Msg {
			if cmd != nil {
				cmd()
			}
			return tui.DismissOverlayMsg{}
		}

	default:
		if v.step == protoInstallMenu {
			var cmd tea.Cmd
			v.menu, cmd = v.menu.Update(msg)
			return v, cmd
		}
	}
	return v, nil
}

func (v *ProtocolInstallView) View() string {
	return v.menu.View()
}

type protoInstallDoneMsg struct{ result string }

func (v *ProtocolInstallView) doInstall(pt protocol.Type, portStr string) tea.Msg {
	var port int
	fmt.Sscanf(portStr, "%d", &port)
	if port <= 0 || port > 65535 {
		return protoInstallDoneMsg{result: "Invalid port number"}
	}
	result, err := protocol.Install(v.model.Store(), protocol.InstallParams{
		ProtoType: pt,
		Port:      port,
		UserName:  "user",
	})
	if err != nil {
		return protoInstallDoneMsg{result: "Error: " + err.Error()}
	}
	if err := v.model.Store().Apply(); err != nil {
		return protoInstallDoneMsg{result: "Failed to save: " + err.Error()}
	}
	msg := fmt.Sprintf("Installed %s on port %d\nTag: %s\nCredential: %s",
		pt, result.Port, result.Tag, result.Credential)
	if result.PublicKey != "" {
		msg += "\nPublic Key: " + result.PublicKey
	}
	return protoInstallDoneMsg{result: msg}
}
