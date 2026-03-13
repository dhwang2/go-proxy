package components

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	inputPromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#5fd7ff")).Bold(true)
	inputCursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffd75f")).Bold(true)
	inputErrStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5f5f")).Bold(true)
)

// TextInput is a single-line text input with optional validation.
type TextInput struct {
	prompt     string
	value      string
	cursor     int
	validator  func(string) error
	errMessage string
	submitted  bool
	cancelled  bool
}

// NewTextInput creates a new text input with the given prompt.
func NewTextInput(prompt string) TextInput {
	return TextInput{
		prompt: prompt,
	}
}

// WithValidator returns a copy of the TextInput with the given validation function.
func (t TextInput) WithValidator(fn func(string) error) TextInput {
	t.validator = fn
	return t
}

// Update processes a tea.KeyMsg and returns the updated model.
// Handles paste (multi-rune KeyRunes) and preserves case.
func (t TextInput) Update(msg tea.KeyMsg) TextInput {
	switch msg.Type {
	case tea.KeyEnter:
		if t.validator != nil {
			if err := t.validator(t.value); err != nil {
				t.errMessage = err.Error()
				return t
			}
		}
		t.errMessage = ""
		t.submitted = true
	case tea.KeyEsc:
		t.cancelled = true
	case tea.KeyBackspace:
		if t.cursor > 0 {
			t.value = t.value[:t.cursor-1] + t.value[t.cursor:]
			t.cursor--
			t.errMessage = ""
		}
	case tea.KeyDelete:
		if t.cursor < len(t.value) {
			t.value = t.value[:t.cursor] + t.value[t.cursor+1:]
			t.errMessage = ""
		}
	case tea.KeyLeft:
		if t.cursor > 0 {
			t.cursor--
		}
	case tea.KeyRight:
		if t.cursor < len(t.value) {
			t.cursor++
		}
	case tea.KeyHome:
		t.cursor = 0
	case tea.KeyEnd:
		t.cursor = len(t.value)
	case tea.KeyRunes:
		// Handles single typed characters and multi-character paste.
		s := string(msg.Runes)
		t.value = t.value[:t.cursor] + s + t.value[t.cursor:]
		t.cursor += len(s)
		t.errMessage = ""
	default:
		key := msg.String()
		switch key {
		case "ctrl+a":
			t.cursor = 0
		case "ctrl+e":
			t.cursor = len(t.value)
		}
	}
	return t
}

// View renders the text input.
func (t TextInput) View() string {
	var b strings.Builder
	prompt := inputPromptStyle.Render(t.prompt+": ")

	before := t.value[:t.cursor]
	after := ""
	cursorChar := "█"
	if t.cursor < len(t.value) {
		cursorChar = string(t.value[t.cursor])
		after = t.value[t.cursor+1:]
	}
	cursor := inputCursorStyle.Render(cursorChar)

	b.WriteString(fmt.Sprintf("%s%s%s%s", prompt, before, cursor, after))

	if t.errMessage != "" {
		b.WriteString("\n" + inputErrStyle.Render(t.errMessage))
	}
	return b.String()
}

// Value returns the current text value.
func (t TextInput) Value() string {
	return t.value
}

// Submitted returns true if the user pressed Enter and validation passed.
func (t TextInput) Submitted() bool {
	return t.submitted
}

// Cancelled returns true if the user pressed Escape.
func (t TextInput) Cancelled() bool {
	return t.cancelled
}
