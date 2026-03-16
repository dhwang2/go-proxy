package views

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"go-proxy/internal/derived"
	"go-proxy/internal/protocol"
	"go-proxy/internal/tui"
	"go-proxy/internal/tui/components"
)

type ProtocolRemoveView struct {
	model      *tui.Model
	menu       components.MenuModel
	step       protoRemoveStep
	pendingTag string
}

type protoRemoveStep int

const (
	protoRemoveMenu protoRemoveStep = iota
	protoRemoveConfirm
	protoRemoveResult
)

func NewProtocolRemoveView(model *tui.Model) *ProtocolRemoveView {
	return &ProtocolRemoveView{model: model}
}

func (v *ProtocolRemoveView) Name() string { return "protocol-remove" }

func (v *ProtocolRemoveView) Init() tea.Cmd {
	v.step = protoRemoveMenu
	v.pendingTag = ""
	inv := derived.Inventory(v.model.Store())
	items := make([]components.MenuItem, 0, len(inv)+1)
	for i, info := range inv {
		k := rune('1' + i)
		if i >= 9 {
			k = rune('a' + i - 9)
		}
		items = append(items, components.MenuItem{
			Key:   k,
			Label: fmt.Sprintf("%s (port %d, %d users)", info.Tag, info.Port, info.UserCount),
			ID:    info.Tag,
		})
	}
	items = append(items, components.MenuItem{Key: '0', Label: "返回", ID: "back"})
	v.menu = v.menu.SetItems(items)
	return nil
}

func (v *ProtocolRemoveView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case components.MenuSelectMsg:
		if msg.ID == "back" {
			return v, tui.BackCmd
		}
		v.pendingTag = msg.ID
		v.step = protoRemoveConfirm
		prompt := fmt.Sprintf("Remove %s?", v.pendingTag)
		return v, func() tea.Msg {
			return tui.ShowOverlayMsg{
				Overlay: components.NewConfirm(prompt),
			}
		}

	case tui.ConfirmResultMsg:
		if !msg.Confirmed {
			v.step = protoRemoveMenu
			return v, nil
		}
		tag := v.pendingTag
		return v, func() tea.Msg {
			return protoRemoveDoneMsg{tag: tag, err: v.doRemove(tag)}
		}

	case protoRemoveDoneMsg:
		v.step = protoRemoveResult
		var result string
		if msg.err != nil {
			result = "Error: " + msg.err.Error()
		} else {
			result = "Removed " + msg.tag
		}
		return v, func() tea.Msg {
			return tui.ShowOverlayMsg{
				Overlay: components.NewResult(result),
			}
		}

	case tui.ResultDismissedMsg:
		v.Init()
		return v, nil

	default:
		if v.step == protoRemoveMenu {
			var cmd tea.Cmd
			v.menu, cmd = v.menu.Update(msg)
			return v, cmd
		}
	}
	return v, nil
}

func (v *ProtocolRemoveView) View() string {
	return v.menu.View()
}

type protoRemoveDoneMsg struct {
	tag string
	err error
}

func (v *ProtocolRemoveView) doRemove(tag string) error {
	if err := protocol.Remove(v.model.Store(), tag); err != nil {
		return err
	}
	return v.model.Store().Apply()
}
