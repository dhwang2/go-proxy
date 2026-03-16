package views

import (
	tea "github.com/charmbracelet/bubbletea"

	"go-proxy/internal/tui"
	"go-proxy/internal/tui/components"
)

type NetworkView struct {
	model *tui.Model
	menu  components.MenuModel
}

func NewNetworkView(model *tui.Model) *NetworkView {
	v := &NetworkView{model: model}
	v.menu = components.NewMenu("网络管理", []components.MenuItem{
		{Key: '1', Label: "BBR 状态", ID: "bbr-status"},
		{Key: '2', Label: "启用 BBR", ID: "bbr-enable"},
		{Key: '3', Label: "防火墙规则", ID: "firewall"},
		{Key: '0', Label: "返回", ID: "back"},
	})
	return v
}

func (v *NetworkView) Name() string { return "network" }

func (v *NetworkView) Init() tea.Cmd { return nil }

func (v *NetworkView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case components.MenuSelectMsg:
		switch msg.ID {
		case "back":
			return v, tui.BackCmd
		default:
			return v, func() tea.Msg {
				return tui.ShowOverlayMsg{
					Overlay: components.NewResult("功能尚未实现（需要 Linux）"),
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

func (v *NetworkView) View() string { return v.menu.View() }
