package views

import (
	tea "github.com/charmbracelet/bubbletea"

	"go-proxy/internal/tui"
	"go-proxy/internal/tui/components"
)

type LogsView struct {
	model *tui.Model
	menu  components.MenuModel
}

func NewLogsView(model *tui.Model) *LogsView {
	sources := []string{"sing-box", "snell-v5", "shadow-tls", "caddy-sub", "proxy-script", "proxy-watchdog"}
	items := make([]components.MenuItem, 0, len(sources)+1)
	for i, src := range sources {
		items = append(items, components.MenuItem{
			Key:   rune('1' + i),
			Label: src,
			ID:    src,
		})
	}
	items = append(items, components.MenuItem{Key: '0', Label: "返回  Back", ID: "back"})

	v := &LogsView{model: model}
	v.menu = components.NewMenu("Runtime Logs", items)
	return v
}

func (v *LogsView) Name() string { return "logs" }

func (v *LogsView) Init() tea.Cmd { return nil }

func (v *LogsView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case components.MenuSelectMsg:
		if msg.ID == "back" {
			return v, func() tea.Msg { return tui.BackMsg{} }
		}
		hint := "Use 'journalctl -u " + msg.ID + " -f' for logs"
		return v, func() tea.Msg {
			return tui.ShowOverlayMsg{
				Overlay: components.NewResult(hint),
			}
		}
	case tui.ResultDismissedMsg:
		return v, func() tea.Msg { return tui.DismissOverlayMsg{} }
	default:
		var cmd tea.Cmd
		v.menu, cmd = v.menu.Update(msg)
		return v, cmd
	}
}

func (v *LogsView) View() string { return v.menu.View() }
