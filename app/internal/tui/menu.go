package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Menu styles — package-level to avoid allocation on every View() call.
var (
	menuTitleStyle = lipgloss.NewStyle().
			Foreground(ColorTitle).
			Bold(true).
			PaddingLeft(2)

	menuSelectedStyle = lipgloss.NewStyle().
				Background(ColorAccent).
				Foreground(ColorAccentFg).
				Bold(true)

	menuNormalStyle = lipgloss.NewStyle().
			Foreground(ColorLabel)
)

// MenuItem represents a single menu entry.
// Label may contain an icon prefix (e.g., "  text") for backwards compatibility.
// The Icon() and Text() methods split them for aligned rendering.
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
	columns  int    // number of columns (1 or 2)
	activeID string // ID of the active item (shown with ◂ indicator)
	dim      bool   // render in dimmed style (unfocused panel)
	selected bool   // set when an item is chosen
}

// Icon returns the icon portion of the label (first whitespace-delimited token
// if it starts with a non-ASCII rune), or an empty string.
func (m MenuItem) Icon() string {
	l := strings.TrimLeft(m.Label, " ")
	if l == "" {
		return ""
	}
	r, _ := utf8.DecodeRuneInString(l)
	if r == utf8.RuneError || r < 128 {
		return ""
	}
	idx := strings.IndexByte(l, ' ')
	if idx < 0 {
		return ""
	}
	return l[:idx]
}

// Text returns the label text after the icon, or the full label if no icon.
func (m MenuItem) Text() string {
	l := strings.TrimLeft(m.Label, " ")
	if l == "" {
		return ""
	}
	r, _ := utf8.DecodeRuneInString(l)
	if r == utf8.RuneError || r < 128 {
		return l
	}
	idx := strings.IndexByte(l, ' ')
	if idx < 0 {
		return l
	}
	return strings.TrimLeft(l[idx:], " ")
}

// MenuSelectMsg is sent when a menu item is selected.
type MenuSelectMsg struct {
	ID    string
	Index int
}

// MenuCursorChangeMsg is sent when the cursor moves to a different item.
type MenuCursorChangeMsg struct {
	ID    string
	Index int
}

// NewMenu creates a new menu model.
func NewMenu(title string, items []MenuItem) MenuModel {
	return MenuModel{
		title:   title,
		items:   items,
		width:   60,
		columns: 1,
	}
}

// SetWidth sets the menu rendering width.
func (m MenuModel) SetWidth(w int) MenuModel {
	m.width = w
	return m
}

// SetColumns sets the number of columns for rendering (1 or 2).
func (m MenuModel) SetColumns(n int) MenuModel {
	if n < 1 {
		n = 1
	}
	if n > 2 {
		n = 2
	}
	m.columns = n
	return m
}

// SetActiveID marks an item as "active" (its sub-menu is open), shown with ◂.
func (m MenuModel) SetActiveID(id string) MenuModel {
	m.activeID = id
	return m
}

// SetDim sets whether the menu renders in dimmed style (unfocused panel).
func (m MenuModel) SetDim(dim bool) MenuModel {
	m.dim = dim
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

// rows returns the number of rows needed for the current column layout.
func (m MenuModel) rows() int {
	n := len(m.items)
	if m.columns <= 1 || n == 0 {
		return n
	}
	return (n + m.columns - 1) / m.columns
}

// cursorCol returns the column index of the current cursor.
func (m MenuModel) cursorCol() int {
	if m.columns <= 1 {
		return 0
	}
	rows := m.rows()
	if rows == 0 {
		return 0
	}
	return m.cursor / rows
}

// cursorRow returns the row index of the current cursor.
func (m MenuModel) cursorRow() int {
	if m.columns <= 1 {
		return m.cursor
	}
	rows := m.rows()
	if rows == 0 {
		return 0
	}
	return m.cursor % rows
}

// Update handles key presses for menu navigation.
func (m MenuModel) Update(msg tea.Msg) (MenuModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, Keys.Up):
			if m.columns <= 1 {
				if m.cursor > 0 {
					m.cursor--
				} else {
					m.cursor = len(m.items) - 1
				}
			} else {
				row := m.cursorRow()
				col := m.cursorCol()
				rows := m.rows()
				if row > 0 {
					m.cursor = col*rows + row - 1
				} else {
					// Wrap to last row in this column.
					last := col*rows + rows - 1
					if last >= len(m.items) {
						last = len(m.items) - 1
					}
					m.cursor = last
				}
			}
		case key.Matches(msg, Keys.Down):
			if m.columns <= 1 {
				if m.cursor < len(m.items)-1 {
					m.cursor++
				} else {
					m.cursor = 0
				}
			} else {
				row := m.cursorRow()
				col := m.cursorCol()
				rows := m.rows()
				next := col*rows + row + 1
				if row+1 < rows && next < len(m.items) {
					m.cursor = next
				} else {
					// Wrap to first row in this column.
					m.cursor = col * rows
				}
			}
		case key.Matches(msg, Keys.Left):
			if m.columns > 1 {
				col := m.cursorCol()
				row := m.cursorRow()
				if col > 0 {
					target := (col-1)*m.rows() + row
					if target < len(m.items) {
						m.cursor = target
					}
				}
			}
		case key.Matches(msg, Keys.Right):
			if m.columns > 1 {
				col := m.cursorCol()
				row := m.cursorRow()
				rows := m.rows()
				if col < m.columns-1 {
					target := (col+1)*rows + row
					if target < len(m.items) {
						m.cursor = target
					}
				}
			}
		case key.Matches(msg, Keys.Enter):
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

func cursorChangeItem(item MenuItem, index int) tea.Cmd {
	return func() tea.Msg {
		return MenuCursorChangeMsg{ID: item.ID, Index: index}
	}
}

// View renders the menu.
func (m MenuModel) View() string {
	if m.columns >= 2 && len(m.items) > 1 {
		return m.viewTwoCol()
	}
	return m.viewSingleCol()
}

func (m MenuModel) viewSingleCol() string {
	var b strings.Builder

	if m.title != "" {
		b.WriteString(menuTitleStyle.Render(m.title))
		b.WriteString("\n")
	}

	normalStyle := menuNormalStyle
	if m.dim {
		normalStyle = lipgloss.NewStyle().Foreground(ColorLabelDim)
	}

	for i, item := range m.items {
		icon := item.Icon()
		label := item.Text()

		switch {
		case i == m.cursor && !m.dim:
			b.WriteString(menuSelectedStyle.Render(fmt.Sprintf(" ▶ %c. %s %s", item.Key, icon, label)))
		case item.ID == m.activeID && m.activeID != "":
			suffix := " ◂"
			b.WriteString(MenuActiveStyle.Render(fmt.Sprintf("   %c. %s %s%s", item.Key, icon, label, suffix)))
		default:
			b.WriteString(normalStyle.Render(fmt.Sprintf("   %c. %s %s", item.Key, icon, label)))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func (m MenuModel) viewTwoCol() string {
	var b strings.Builder

	if m.title != "" {
		b.WriteString(menuTitleStyle.Render(m.title))
		b.WriteString("\n")
	}

	rows := m.rows()
	colWidth := m.width / 2
	if colWidth < 28 {
		colWidth = 28
	}

	// Build left and right column strings.
	var leftLines, rightLines []string
	for row := 0; row < rows; row++ {
		// Left column item.
		leftIdx := row
		leftLines = append(leftLines, m.renderItem(leftIdx, colWidth))

		// Right column item.
		rightIdx := rows + row
		if rightIdx < len(m.items) {
			rightLines = append(rightLines, m.renderItem(rightIdx, colWidth))
		} else {
			rightLines = append(rightLines, strings.Repeat(" ", colWidth))
		}
	}

	leftCol := strings.Join(leftLines, "\n")
	rightCol := strings.Join(rightLines, "\n")

	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, leftCol, rightCol))
	b.WriteString("\n")

	return b.String()
}

func (m MenuModel) renderItem(idx, colWidth int) string {
	if idx >= len(m.items) {
		return strings.Repeat(" ", colWidth)
	}

	item := m.items[idx]
	icon := item.Icon()
	label := item.Text()

	var line string
	if idx == m.cursor {
		line = menuSelectedStyle.Render(fmt.Sprintf(" ▶ %c. %s %s", item.Key, icon, label))
	} else {
		line = menuNormalStyle.Render(fmt.Sprintf("   %c. %s %s", item.Key, icon, label))
	}

	// Pad to column width.
	return lipgloss.NewStyle().Width(colWidth).Render(line)
}
