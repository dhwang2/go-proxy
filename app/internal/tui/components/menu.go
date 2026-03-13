package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type Item struct {
	Key         string
	Title       string
	Description string
	Value       string
}

type MenuList struct {
	Items  []Item
	Cursor int
	Width  int
}

var (
	// Selected item: bright white on dark cyan, full-width highlight bar
	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ffffff")).
			Background(lipgloss.Color("#005f87")).
			Bold(true)
	// Normal item: soft gray
	normalStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#bcbcbc"))
)

func NewMenuList(items []Item) MenuList {
	m := MenuList{Items: append([]Item(nil), items...)}
	m.ensureCursor()
	return m
}

func (m *MenuList) SetItems(items []Item) {
	m.Items = append([]Item(nil), items...)
	m.ensureCursor()
}

func (m *MenuList) MoveUp() {
	if len(m.Items) == 0 {
		return
	}
	m.Cursor = (m.Cursor - 1 + len(m.Items)) % len(m.Items)
}

func (m *MenuList) MoveDown() {
	if len(m.Items) == 0 {
		return
	}
	m.Cursor = (m.Cursor + 1) % len(m.Items)
}

func (m *MenuList) SelectByKey(key string) bool {
	for i, item := range m.Items {
		if strings.TrimSpace(item.Key) == strings.TrimSpace(key) {
			m.Cursor = i
			return true
		}
	}
	return false
}

func (m MenuList) Selected() Item {
	if len(m.Items) == 0 {
		return Item{}
	}
	idx := m.Cursor
	if idx < 0 || idx >= len(m.Items) {
		idx = 0
	}
	return m.Items[idx]
}

func (m MenuList) View() string {
	if len(m.Items) == 0 {
		return ""
	}
	var b strings.Builder
	for i, item := range m.Items {
		isBack := strings.TrimSpace(item.Key) == "0"

		// Blank line before back/exit item
		if isBack && i > 0 {
			b.WriteString("\n")
		}

		isCurrent := i == m.Cursor
		if isCurrent {
			content := " ▸ " + item.Title
			if m.Width > 0 {
				b.WriteString(selectedStyle.Width(m.Width).Render(content) + "\n")
			} else {
				b.WriteString(selectedStyle.Render(content) + "\n")
			}
		} else {
			b.WriteString("   " + normalStyle.Render(item.Title) + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m *MenuList) ensureCursor() {
	if len(m.Items) == 0 {
		m.Cursor = 0
		return
	}
	if m.Cursor < 0 {
		m.Cursor = 0
	}
	if m.Cursor >= len(m.Items) {
		m.Cursor = len(m.Items) - 1
	}
}
