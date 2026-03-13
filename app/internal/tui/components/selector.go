package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	selectorActiveStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
	selectorCheckStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	selectorNormalStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
)

// SelectorItem represents a single selectable option.
type SelectorItem struct {
	Key      string
	Label    string
	Selected bool
}

// Selector is a multi-option selector with checkboxes.
type Selector struct {
	Items  []SelectorItem
	Cursor int
}

// NewSelector creates a new selector with the given items.
func NewSelector(items []SelectorItem) Selector {
	copied := make([]SelectorItem, len(items))
	copy(copied, items)
	return Selector{Items: copied}
}

// Toggle toggles the selected state of the item at the cursor.
func (s *Selector) Toggle() {
	if len(s.Items) == 0 {
		return
	}
	s.Items[s.Cursor].Selected = !s.Items[s.Cursor].Selected
}

// MoveUp moves the cursor up, wrapping around.
func (s *Selector) MoveUp() {
	if len(s.Items) == 0 {
		return
	}
	s.Cursor = (s.Cursor - 1 + len(s.Items)) % len(s.Items)
}

// MoveDown moves the cursor down, wrapping around.
func (s *Selector) MoveDown() {
	if len(s.Items) == 0 {
		return
	}
	s.Cursor = (s.Cursor + 1) % len(s.Items)
}

// SelectedKeys returns the keys of all selected items.
func (s Selector) SelectedKeys() []string {
	var keys []string
	for _, item := range s.Items {
		if item.Selected {
			keys = append(keys, item.Key)
		}
	}
	return keys
}

// View renders the selector with checkboxes.
func (s Selector) View() string {
	if len(s.Items) == 0 {
		return ""
	}
	var b strings.Builder
	for i, item := range s.Items {
		check := "[ ]"
		if item.Selected {
			check = selectorCheckStyle.Render("[✓]")
		}

		label := item.Label
		prefix := "  "
		if i == s.Cursor {
			prefix = "▸ "
			label = selectorActiveStyle.Render(label)
		} else {
			label = selectorNormalStyle.Render(label)
		}

		b.WriteString(fmt.Sprintf("%s%s %s\n", prefix, check, label))
	}
	return strings.TrimRight(b.String(), "\n")
}
