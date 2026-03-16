package components

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ConfirmModel is a yes/no confirmation dialog.
type ConfirmModel struct {
	Prompt    string
	confirmed bool
	answered  bool
}

// ConfirmResultMsg is sent when the user answers.
type ConfirmResultMsg struct {
	Confirmed bool
}

// NewConfirm creates a new confirmation dialog.
func NewConfirm(prompt string) ConfirmModel {
	return ConfirmModel{Prompt: prompt}
}

// Init implements tea.Model.
func (m ConfirmModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m ConfirmModel) Update(msg tea.Msg) (ConfirmModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyEsc {
			m.confirmed = false
			m.answered = true
			return m, func() tea.Msg {
				return ConfirmResultMsg{Confirmed: false}
			}
		}
		switch msg.String() {
		case "y", "Y":
			m.confirmed = true
			m.answered = true
			return m, func() tea.Msg {
				return ConfirmResultMsg{Confirmed: true}
			}
		case "n", "N":
			m.confirmed = false
			m.answered = true
			return m, func() tea.Msg {
				return ConfirmResultMsg{Confirmed: false}
			}
		}
	}
	return m, nil
}

// View implements tea.Model.
func (m ConfirmModel) View() string {
	var b strings.Builder
	prompt := lipgloss.NewStyle().Bold(true).Render(m.Prompt)
	b.WriteString(prompt)
	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("#9E9E9E")).Render(" [y/n]")
	b.WriteString(hint)
	return b.String()
}

// Answered returns whether the user has answered.
func (m ConfirmModel) Answered() bool {
	return m.answered
}

// Confirmed returns the user's answer.
func (m ConfirmModel) Confirmed() bool {
	return m.confirmed
}
