package views

import (
	tea "github.com/charmbracelet/bubbletea"

	"go-proxy/internal/tui"
	"go-proxy/internal/tui/components"
)

type CoreView struct {
	model *tui.Model
	menu  components.MenuModel
}

func NewCoreView(model *tui.Model) *CoreView {
	v := &CoreView{model: model}
	v.menu = components.NewMenu("Core Management", []components.MenuItem{
		{Key: '1', Label: "查看版本  View Versions", ID: "versions"},
		{Key: '2', Label: "检查更新  Check Updates", ID: "check"},
		{Key: '3', Label: "更新内核  Update Core", ID: "update"},
		{Key: '0', Label: "返回  Back", ID: "back"},
	})
	return v
}

func (v *CoreView) Name() string { return "core" }

func (v *CoreView) Init() tea.Cmd { return nil }

func (v *CoreView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case components.MenuSelectMsg:
		switch msg.ID {
		case "back":
			return v, tui.BackCmd
		default:
			return v, func() tea.Msg {
				return tui.ShowOverlayMsg{
					Overlay: components.NewResult("Not implemented yet"),
				}
			}
		}
	case tui.ResultDismissedMsg:
		return v, nil
	default:
		var cmd tea.Cmd
		v.menu, cmd = v.menu.Update(msg)
		return v, cmd
	}
}

func (v *CoreView) View() string { return v.menu.View() }
