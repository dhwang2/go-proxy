package views

import (
	"encoding/json"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-proxy/internal/tui"
)

type ConfigView struct {
	model    *tui.Model
	viewport viewport.Model
	ready    bool
}

func NewConfigView(model *tui.Model) *ConfigView {
	return &ConfigView{model: model}
}

func (v *ConfigView) Name() string { return "config" }

func (v *ConfigView) Init() tea.Cmd {
	data, err := json.MarshalIndent(v.model.Store().SingBox, "", "  ")
	if err != nil {
		v.viewport.SetContent("Error rendering config: " + err.Error())
	} else {
		v.viewport.SetContent(string(data))
	}
	v.viewport.GotoTop()
	return nil
}

func (v *ConfigView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		titleStyle := lipgloss.NewStyle().
			Foreground(tui.ColorPrimary).
			Bold(true).
			PaddingLeft(1)
		headerHeight := lipgloss.Height(titleStyle.Render("sing-box Configuration")) + 1
		footerHeight := 1

		if !v.ready {
			v.viewport = viewport.New(msg.Width, msg.Height-headerHeight-footerHeight)
			v.ready = true
			// Re-init content.
			data, err := json.MarshalIndent(v.model.Store().SingBox, "", "  ")
			if err != nil {
				v.viewport.SetContent("Error rendering config: " + err.Error())
			} else {
				v.viewport.SetContent(string(data))
			}
		} else {
			v.viewport.Width = msg.Width
			v.viewport.Height = msg.Height - headerHeight - footerHeight
		}
		return v, nil
	}

	var cmd tea.Cmd
	v.viewport, cmd = v.viewport.Update(msg)
	return v, cmd
}

func (v *ConfigView) View() string {
	titleStyle := lipgloss.NewStyle().
		Foreground(tui.ColorPrimary).
		Bold(true).
		PaddingLeft(1)

	header := titleStyle.Render("sing-box Configuration")

	footerStyle := lipgloss.NewStyle().
		Foreground(tui.ColorMuted).
		PaddingLeft(1)
	footer := footerStyle.Render("↑/↓ scroll  ESC back")

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		v.viewport.View(),
		footer,
	)
}
