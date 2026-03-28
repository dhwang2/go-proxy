package components

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-proxy/internal/tui"
)

// ConfirmModel is a yes/no prompt overlay.
type ConfirmModel struct {
	prompt  string
	focused int // 0 = Yes, 1 = No
}

// NewConfirm creates a new confirmation overlay.
func NewConfirm(prompt string) ConfirmModel {
	return ConfirmModel{prompt: prompt}
}

func (m ConfirmModel) Init() tea.Cmd    { return nil }
func (m ConfirmModel) UsesCursor() bool { return true }

func (m ConfirmModel) Update(msg tea.Msg) (tui.OverlayModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, tui.Keys.Yes):
			return m, func() tea.Msg {
				return tui.ConfirmResultMsg{Confirmed: true}
			}
		case key.Matches(msg, tui.Keys.No), key.Matches(msg, tui.Keys.Cancel):
			return m, func() tea.Msg {
				return tui.ConfirmResultMsg{Confirmed: false}
			}
		case key.Matches(msg, tui.Keys.Enter):
			return m, func() tea.Msg {
				return tui.ConfirmResultMsg{Confirmed: m.focused == 0}
			}
		case key.Matches(msg, tui.Keys.Tab),
			msg.Type == tea.KeyLeft, msg.Type == tea.KeyRight:
			m.focused = 1 - m.focused
			return m, nil
		}
	}
	return m, nil
}

func (m ConfirmModel) View() string {
	yesStyle := lipgloss.NewStyle().Padding(0, 2)
	noStyle := lipgloss.NewStyle().Padding(0, 2)

	if m.focused == 0 {
		yesStyle = yesStyle.Foreground(tui.ColorSuccess).Bold(true)
	}
	if m.focused == 1 {
		noStyle = noStyle.Foreground(tui.ColorError).Bold(true)
	}

	buttons := lipgloss.JoinHorizontal(lipgloss.Center,
		yesStyle.Render("[ 是 ]"),
		noStyle.Render("[ 否 ]"),
	)

	if tui.InSplitPanel {
		return lipgloss.JoinVertical(lipgloss.Left,
			m.prompt,
			"",
			buttons,
		)
	}

	content := lipgloss.JoinVertical(lipgloss.Center,
		"",
		m.prompt,
		"",
		"",
		buttons,
		"",
	)
	return tui.DialogStyle.Render(content)
}
