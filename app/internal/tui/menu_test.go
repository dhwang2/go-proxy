package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestMenuNavigationDoesNotAutoSelectOnCursorMove(t *testing.T) {
	menu := NewMenu("", []MenuItem{
		{Key: '1', Label: "one", ID: "one"},
		{Key: '2', Label: "two", ID: "two"},
	})

	updated, cmd := menu.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Fatal("down key should only move cursor, not emit a selection command")
	}
	if updated.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", updated.cursor)
	}

	updated, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyUp})
	if cmd != nil {
		t.Fatal("up key should only move cursor, not emit a selection command")
	}
	if updated.cursor != 0 {
		t.Fatalf("cursor = %d, want 0", updated.cursor)
	}
}
