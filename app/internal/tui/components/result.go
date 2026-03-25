package components

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-proxy/internal/tui"
)

// ResultModel displays a message with an OK button.
type ResultModel struct {
	message string
}

// NewResult creates a new result overlay.
func NewResult(message string) ResultModel {
	return ResultModel{message: message}
}

func (m ResultModel) Init() tea.Cmd { return nil }

func (m ResultModel) Update(msg tea.Msg) (tui.OverlayModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, tui.Keys.Enter),
			key.Matches(msg, tui.Keys.Cancel),
			msg.Type == tea.KeySpace:
			return m, func() tea.Msg {
				return tui.ResultDismissedMsg{}
			}
		}
	}
	return m, nil
}

func (m ResultModel) View() string {
	if tui.InSplitPanel {
		// Skip wrapping — SubSplit truncates overflow.
		return m.message
	}

	// Wrap message text to prevent dialog overflow.
	maxWidth := 70
	msg := wrapText(m.message, maxWidth)

	content := lipgloss.JoinVertical(lipgloss.Left,
		"",
		msg,
		"",
	)
	return tui.DialogStyle.Render(content)
}

// wrapText wraps long lines to fit within maxWidth.
func wrapText(text string, maxWidth int) string {
	lines := strings.Split(text, "\n")
	var result []string
	for _, line := range lines {
		if lipgloss.Width(line) <= maxWidth {
			result = append(result, line)
			continue
		}
		// Break long line at maxWidth boundary.
		for lipgloss.Width(line) > maxWidth {
			cut := maxWidth
			if cut > len(line) {
				cut = len(line)
			}
			result = append(result, line[:cut])
			line = line[cut:]
		}
		if line != "" {
			result = append(result, line)
		}
	}
	return strings.Join(result, "\n")
}
