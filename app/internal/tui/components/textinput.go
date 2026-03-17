package components

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-proxy/internal/tui"
)

// TextInputModel wraps bubbles/textinput with OK/Cancel actions.
type TextInputModel struct {
	prompt  string
	input   textinput.Model
	focused int // 0 = input, 1 = OK, 2 = Cancel
	width   int
}

// NewTextInput creates a new text input overlay.
func NewTextInput(prompt, placeholder string) TextInputModel {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Focus()
	ti.CharLimit = 64
	ti.Width = 30
	return TextInputModel{
		prompt: prompt,
		input:  ti,
		width:  50,
	}
}

func (m TextInputModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m TextInputModel) Update(msg tea.Msg) (tui.OverlayModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, tui.Keys.Cancel) && m.focused == 0:
			return m, func() tea.Msg {
				return tui.InputResultMsg{Cancelled: true}
			}
		case key.Matches(msg, tui.Keys.Tab):
			m.focused = (m.focused + 1) % 3
			if m.focused == 0 {
				m.input.Focus()
			} else {
				m.input.Blur()
			}
			return m, nil
		case key.Matches(msg, tui.Keys.Enter):
			if m.focused == 1 || m.focused == 0 {
				val := m.input.Value()
				return m, func() tea.Msg {
					return tui.InputResultMsg{Value: val}
				}
			}
			return m, func() tea.Msg {
				return tui.InputResultMsg{Cancelled: true}
			}
		}
	}

	if m.focused == 0 {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m TextInputModel) View() string {
	okStyle := lipgloss.NewStyle().Padding(0, 2)
	cancelStyle := lipgloss.NewStyle().Padding(0, 2)

	if m.focused == 1 {
		okStyle = okStyle.Foreground(tui.ColorPrimary).Bold(true)
	}
	if m.focused == 2 {
		cancelStyle = cancelStyle.Foreground(tui.ColorError).Bold(true)
	}

	buttons := lipgloss.JoinHorizontal(lipgloss.Center,
		okStyle.Render("[ 确定 ]"),
		cancelStyle.Render("[ 取消 ]"),
	)

	hintStyle := lipgloss.NewStyle().Foreground(tui.ColorMuted)
	hint := hintStyle.Render("esc 取消 | enter 确认")

	content := lipgloss.JoinVertical(lipgloss.Center,
		"",
		m.prompt,
		"",
		m.input.View(),
		"",
		buttons,
		"",
		hint,
		"",
	)

	return tui.DialogStyle.Render(content)
}
