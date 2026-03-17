package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-proxy/internal/tui"
)

// SelectListModel is an overlay that shows a list of items for selection.
type SelectListModel struct {
	title  string
	items  []string
	cursor int
}

// NewSelectList creates a new selection list overlay.
func NewSelectList(title string, items []string) SelectListModel {
	return SelectListModel{
		title: title,
		items: items,
	}
}

func (m SelectListModel) Init() tea.Cmd { return nil }

func (m SelectListModel) Update(msg tea.Msg) (tui.OverlayModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, tui.Keys.Cancel):
			return m, func() tea.Msg {
				return tui.InputResultMsg{Cancelled: true}
			}
		case key.Matches(msg, tui.Keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, tui.Keys.Down):
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case key.Matches(msg, tui.Keys.Enter):
			if len(m.items) > 0 {
				val := m.items[m.cursor]
				return m, func() tea.Msg {
					return tui.InputResultMsg{Value: val}
				}
			}
		}
	}
	return m, nil
}

func (m SelectListModel) View() string {
	var b strings.Builder

	for i, item := range m.items {
		if i == m.cursor {
			style := lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true)
			b.WriteString(style.Render(fmt.Sprintf("  ▸ %s", item)))
		} else {
			b.WriteString(fmt.Sprintf("    %s", item))
		}
		b.WriteString("\n")
	}

	hintStyle := lipgloss.NewStyle().Foreground(tui.ColorMuted)
	hint := hintStyle.Render("esc 取消 | enter 确认")

	content := lipgloss.JoinVertical(lipgloss.Center,
		"",
		m.title,
		"",
		b.String(),
		hint,
		"",
	)

	return tui.DialogStyle.Render(content)
}
