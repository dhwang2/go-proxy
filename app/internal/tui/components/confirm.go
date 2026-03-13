package components

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

var (
	confirmActiveStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#5fd7ff"))
	confirmInactiveStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#6c6c6c"))
	confirmQuestionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#bcbcbc"))
	confirmHintStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#6c6c6c"))
)

// Confirm is a yes/no confirmation dialog.
type Confirm struct {
	question string
	focused  bool // true = yes, false = no
	answered bool
	result   bool
}

// NewConfirm creates a new confirmation dialog with the given question.
func NewConfirm(question string) Confirm {
	return Confirm{
		question: question,
		focused:  true, // default to yes
	}
}

// Update processes a key string and returns the updated model,
// whether the user has answered, and the result (true=yes, false=no).
func (c Confirm) Update(key string) (Confirm, bool, bool) {
	switch key {
	case "left", "h":
		c.focused = true
	case "right", "l":
		c.focused = false
	case "y", "Y":
		c.answered = true
		c.result = true
		return c, true, true
	case "n", "N":
		c.answered = true
		c.result = false
		return c, true, false
	case "enter":
		c.answered = true
		c.result = c.focused
		return c, true, c.focused
	}
	return c, false, false
}

// View renders the confirmation dialog with hint.
func (c Confirm) View() string {
	var yes, no string
	if c.focused {
		yes = confirmActiveStyle.Render("[ Yes ]")
		no = confirmInactiveStyle.Render("  No  ")
	} else {
		yes = confirmInactiveStyle.Render("  Yes  ")
		no = confirmActiveStyle.Render("[ No ]")
	}
	hint := confirmHintStyle.Render("  ←/→ 选择 · 回车确认 · esc 取消")
	return fmt.Sprintf("\n  %s\n\n  %s  %s\n\n%s",
		confirmQuestionStyle.Render(c.question), yes, no, hint)
}

// Answered returns whether the user has made a choice.
func (c Confirm) Answered() bool {
	return c.answered
}

// Result returns the user's choice (true=yes, false=no).
func (c Confirm) Result() bool {
	return c.result
}
