package layout

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	divLineStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#3a3a3a"))
	divLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#5fd7ff")).Bold(true)
)

// Divider returns a plain horizontal rule.
func Divider(width int) string {
	if width < 40 {
		width = 40
	}
	return strings.Repeat("─", width)
}

// DoubleDivider returns a double-line horizontal rule.
func DoubleDivider(width int) string {
	if width < 40 {
		width = 40
	}
	return strings.Repeat("═", width)
}

// LabeledDivider renders " ─── Label ─────────────" with colored label.
func LabeledDivider(label string, width int) string {
	if width < 40 {
		width = 40
	}
	prefix := " ─── "
	suffix := " "
	fill := width - 5 - len(label) - 1 // 5 = len(" ─── "), 1 = len(" ")
	if fill < 4 {
		fill = 4
	}
	return divLineStyle.Render(prefix) +
		divLabelStyle.Render(label) +
		divLineStyle.Render(suffix+strings.Repeat("─", fill))
}
