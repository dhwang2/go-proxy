package components

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-proxy/internal/tui"
)

// ResultModel displays a message with an OK button.
type ResultModel struct {
	message string
}

// NewResult creates a new result overlay.
func NewResult(message string) ResultModel {
	return ResultModel{message: message}
}

func (m ResultModel) Init() tea.Cmd { return nil }

func (m ResultModel) Update(msg tea.Msg) (tui.OverlayModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, tui.Keys.Enter),
			key.Matches(msg, tui.Keys.Cancel),
			msg.Type == tea.KeySpace:
			return m, func() tea.Msg {
				return tui.ResultDismissedMsg{}
			}
		}
	}
	return m, nil
}

func (m ResultModel) View() string {
	okStyle := lipgloss.NewStyle().
		Foreground(tui.ColorPrimary).
		Bold(true).
		Padding(0, 2)

	content := lipgloss.JoinVertical(lipgloss.Center,
		m.message,
		"",
		okStyle.Render("[ 确定 ]"),
	)

	return tui.DialogStyle.Render(content)
}
