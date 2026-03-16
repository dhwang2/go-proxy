package components

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// InputModel wraps a text input with a prompt and validation.
type InputModel struct {
	Prompt    string
	input     textinput.Model
	err       string
	submitted bool
	Validate  func(string) error
}

// InputSubmittedMsg is sent when input is submitted.
type InputSubmittedMsg struct {
	Value string
}

// NewInput creates a new text input component.
func NewInput(prompt, placeholder string) InputModel {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Focus()
	return InputModel{
		Prompt: prompt,
		input:  ti,
	}
}

// Init implements tea.Model.
func (m InputModel) Init() tea.Cmd {
	return textinput.Blink
}

// Update implements tea.Model.
func (m InputModel) Update(msg tea.Msg) (InputModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			val := strings.TrimSpace(m.input.Value())
			if m.Validate != nil {
				if err := m.Validate(val); err != nil {
					m.err = err.Error()
					return m, nil
				}
			}
			m.submitted = true
			return m, func() tea.Msg {
				return InputSubmittedMsg{Value: val}
			}
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.err = ""
	return m, cmd
}

// View implements tea.Model.
func (m InputModel) View() string {
	var b strings.Builder
	b.WriteString(m.Prompt)
	b.WriteString("\n")
	b.WriteString(m.input.View())
	if m.err != "" {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#F44336"))
		b.WriteString("\n")
		b.WriteString(errStyle.Render(m.err))
	}
	return b.String()
}

// Value returns the current input value.
func (m InputModel) Value() string {
	return m.input.Value()
}

// Submitted returns whether the input has been submitted.
func (m InputModel) Submitted() bool {
	return m.submitted
}
