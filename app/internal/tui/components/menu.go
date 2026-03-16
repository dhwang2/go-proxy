package components

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// MenuItem is a single menu entry.
type MenuItem struct {
	Label string
	Key   string // display key (e.g., "1", "0")
}

// MenuModel is a numbered menu selection component.
type MenuModel struct {
	Title    string
	Items    []MenuItem
	cursor   int
	selected int
	styles   MenuStyles
}

// MenuStyles holds the styling for the menu.
type MenuStyles struct {
	Title      lipgloss.Style
	Item       lipgloss.Style
	ActiveItem lipgloss.Style
	Cursor     string
}

// DefaultMenuStyles returns the default menu styling.
func DefaultMenuStyles() MenuStyles {
	return MenuStyles{
		Title:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00BCD4")),
		Item:       lipgloss.NewStyle().PaddingLeft(2),
		ActiveItem: lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("#00BCD4")).Bold(true),
		Cursor:     "▸ ",
	}
}

// NewMenu creates a new menu model.
func NewMenu(title string, items []MenuItem) MenuModel {
	return MenuModel{
		Title:    title,
		Items:    items,
		cursor:   0,
		selected: -1,
		styles:   DefaultMenuStyles(),
	}
}

// MenuSelectedMsg is sent when an item is selected.
type MenuSelectedMsg struct {
	Index int
	Item  MenuItem
}

// Init implements tea.Model.
func (m MenuModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m MenuModel) Update(msg tea.Msg) (MenuModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.Items)-1 {
				m.cursor++
			}
		case "enter":
			m.selected = m.cursor
			return m, func() tea.Msg {
				return MenuSelectedMsg{Index: m.cursor, Item: m.Items[m.cursor]}
			}
		default:
			// Number key selection.
			for i, item := range m.Items {
				if msg.String() == item.Key {
					m.selected = i
					m.cursor = i
					return m, func() tea.Msg {
						return MenuSelectedMsg{Index: i, Item: m.Items[i]}
					}
				}
			}
		}
	}
	return m, nil
}

// View implements tea.Model.
func (m MenuModel) View() string {
	var b strings.Builder
	b.WriteString(m.styles.Title.Render(m.Title))
	b.WriteString("\n\n")
	for i, item := range m.Items {
		label := fmt.Sprintf("%s  %s", item.Key, item.Label)
		if i == m.cursor {
			b.WriteString(m.styles.ActiveItem.Render(m.styles.Cursor + label))
		} else {
			b.WriteString(m.styles.Item.Render("  " + label))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// Selected returns the currently selected index, or -1 if none.
func (m MenuModel) Selected() int {
	return m.selected
}

// ResetSelection clears the selection state.
func (m *MenuModel) ResetSelection() {
	m.selected = -1
}
