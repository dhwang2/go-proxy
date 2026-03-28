package views

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// padCell pads text with spaces to reach the given display width.
// Uses lipgloss.Width for correct CJK character measurement.
func padCell(text string, width int) string {
	padding := width - lipgloss.Width(text)
	if padding < 0 {
		padding = 0
	}
	return text + strings.Repeat(" ", padding)
}
