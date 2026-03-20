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
				if val == "" {
					val = m.input.Placeholder
				}
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
	content := lipgloss.JoinVertical(lipgloss.Center,
		"",
		m.prompt,
		"",
		m.input.View(),
		"",
	)

	style := tui.DialogStyle
	if tui.InSplitPanel {
		style = tui.PlainDialogStyle
	}
	return style.Render(content)
}
