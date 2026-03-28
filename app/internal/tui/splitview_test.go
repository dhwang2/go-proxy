package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSplitViewBaseHandleMouseEmitsResizeOnReleaseOnly(t *testing.T) {
	base := SplitViewBase{
		Split: NewSubSplit(60, 20),
	}

	press := base.HandleMouse(SubSplitMouseMsg{MouseMsg: tea.MouseMsg{
		X:      base.Split.DividerX(),
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	}})
	if press != nil {
		_ = press()
	}

	// Motion during drag should NOT emit SubSplitResizeMsg (performance).
	move := base.HandleMouse(SubSplitMouseMsg{MouseMsg: tea.MouseMsg{
		X:      base.Split.DividerX() + 5,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionMotion,
	}})
	if move != nil {
		msg := move()
		if _, ok := msg.(SubSplitResizeMsg); ok {
			t.Fatal("expected no SubSplitResizeMsg during drag motion")
		}
	}

	// Release should emit SubSplitResizeMsg.
	release := base.HandleMouse(SubSplitMouseMsg{MouseMsg: tea.MouseMsg{
		X:      base.Split.DividerX() + 5,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionRelease,
	}})
	if release == nil {
		t.Fatal("expected resize cmd on divider release")
	}

	msg := release()
	resize, ok := msg.(SubSplitResizeMsg)
	if !ok {
		t.Fatalf("msg = %#v, want SubSplitResizeMsg", msg)
	}
	if resize.RightWidth != base.Split.RightWidth() {
		t.Fatalf("right width = %d, want %d", resize.RightWidth, base.Split.RightWidth())
	}
	if resize.RightHeight != base.Split.TotalHeight() {
		t.Fatalf("right height = %d, want %d", resize.RightHeight, base.Split.TotalHeight())
	}
}

func TestSubSplitTruncatesRightPaneOverflow(t *testing.T) {
	m := NewSubSplit(40, 10)
	got := m.View("left", "0123456789012345678901234567890123456789TAIL")
	if strings.Contains(got, "TAIL") {
		t.Fatalf("right pane overflow leaked into output: %q", got)
	}
}
