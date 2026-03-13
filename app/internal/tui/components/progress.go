package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const defaultProgressWidth = 40

var (
	progressFilledStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	progressEmptyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

// Progress is a simple progress bar.
type Progress struct {
	total   int
	current int
	message string
	style   lipgloss.Style
	width   int
}

// NewProgress creates a new progress bar with the given total and message.
func NewProgress(total int, message string) Progress {
	if total < 0 {
		total = 0
	}
	return Progress{
		total:   total,
		message: message,
		style:   lipgloss.NewStyle(),
		width:   defaultProgressWidth,
	}
}

// SetCurrent sets the current progress value.
func (p *Progress) SetCurrent(n int) {
	if n < 0 {
		n = 0
	}
	if n > p.total {
		n = p.total
	}
	p.current = n
}

// SetMessage sets the progress bar message.
func (p *Progress) SetMessage(msg string) {
	p.message = msg
}

// SetWidth sets the width of the progress bar (number of chars).
func (p *Progress) SetWidth(w int) {
	if w < 4 {
		w = 4
	}
	p.width = w
}

// Percentage returns the current progress as a float between 0 and 1.
func (p Progress) Percentage() float64 {
	if p.total <= 0 {
		return 0
	}
	return float64(p.current) / float64(p.total)
}

// View renders the progress bar.
func (p Progress) View() string {
	pct := p.Percentage()
	filled := int(pct * float64(p.width))
	if filled > p.width {
		filled = p.width
	}

	bar := progressFilledStyle.Render(strings.Repeat("█", filled)) +
		progressEmptyStyle.Render(strings.Repeat("░", p.width-filled))

	pctStr := fmt.Sprintf("%3.0f%%", pct*100)

	if p.message != "" {
		return fmt.Sprintf("[%s] %s %s", bar, pctStr, p.message)
	}
	return fmt.Sprintf("[%s] %s", bar, pctStr)
}
