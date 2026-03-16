package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-proxy/internal/tui"
)

// MenuItem represents a single menu entry.
type MenuItem struct {
	Key   rune
	Label string
	ID    string // action identifier
}

// MenuModel is a numbered item list with shortcut keys.
type MenuModel struct {
	title    string
	items    []MenuItem
	cursor   int
	width    int
	selected bool // set when an item is chosen
}

// MenuSelectMsg is sent when a menu item is selected.
type MenuSelectMsg struct {
	ID    string
	Index int
}

// NewMenu creates a new menu model.
func NewMenu(title string, items []MenuItem) MenuModel {
	return MenuModel{
		title: title,
		items: items,
		width: 60,
	}
}

// SetWidth sets the menu rendering width.
func (m MenuModel) SetWidth(w int) MenuModel {
	m.width = w
	return m
}

// SetItems replaces the menu items and resets the cursor.
func (m MenuModel) SetItems(items []MenuItem) MenuModel {
	m.items = items
	m.cursor = 0
	return m
}

// Init satisfies tea.Model.
func (m MenuModel) Init() tea.Cmd { return nil }

// Update handles key presses for menu navigation.
func (m MenuModel) Update(msg tea.Msg) (MenuModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
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
				return m, selectItem(m.items[m.cursor], m.cursor)
			}
		default:
			// Check for shortcut key matches.
			for i, item := range m.items {
				if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == item.Key {
					m.cursor = i
					return m, selectItem(item, i)
				}
			}
		}
	}
	return m, nil
}

func selectItem(item MenuItem, index int) tea.Cmd {
	return func() tea.Msg {
		return MenuSelectMsg{ID: item.ID, Index: index}
	}
}

// View renders the menu.
func (m MenuModel) View() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().
		Foreground(tui.ColorPrimary).
		Bold(true).
		PaddingLeft(2)

	if m.title != "" {
		b.WriteString(titleStyle.Render(m.title))
		b.WriteString("\n\n")
	}

	selectedStyle := lipgloss.NewStyle().
		Foreground(tui.ColorPrimary).
		Bold(true)

	for i, item := range m.items {
		label := fmt.Sprintf("      (%c)  %s", item.Key, item.Label)

		if i == m.cursor {
			b.WriteString(selectedStyle.Render(fmt.Sprintf("   ▸  (%c)  %s", item.Key, item.Label)))
		} else {
			b.WriteString(label)
		}
		b.WriteString("\n\n")
	}

	return b.String()
}
