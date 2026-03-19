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
	okStyle := lipgloss.NewStyle().
		Foreground(tui.ColorPrimary).
		Bold(true).
		Padding(0, 2)

	// Wrap message text to prevent dialog overflow.
	maxWidth := 70
	msg := wrapText(m.message, maxWidth)

	// Left-align the message content but center the button.
	button := lipgloss.NewStyle().Width(maxWidth).Align(lipgloss.Center).
		Render(okStyle.Render("[ 确定 ]"))

	content := lipgloss.JoinVertical(lipgloss.Left,
		"",
		msg,
		"",
		button,
		"",
	)

	style := tui.DialogStyle
	if tui.InSplitPanel {
		style = tui.PlainDialogStyle
	}
	return style.Render(content)
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
