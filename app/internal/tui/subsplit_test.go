package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestSubSplitEnablesAtRightPanelContentWidth(t *testing.T) {
	m := NewSubSplit(44, 20)
	if !m.Enabled() {
		t.Fatalf("subsplit should be enabled at width 44")
	}
	if m.LeftWidth() < subSplitMinLeft {
		t.Fatalf("left width = %d, want >= %d", m.LeftWidth(), subSplitMinLeft)
	}
	if m.RightWidth() < subSplitMinRight {
		t.Fatalf("right width = %d, want >= %d", m.RightWidth(), subSplitMinRight)
	}
}

func TestSubSplitAutoFitsLeftPaneToLongestLine(t *testing.T) {
	m := NewSubSplit(60, 20)
	left := "  1. ss\n  2. vless\n  3. tuic\n  4. trojan\n  5. anytls\n  6. snell-v5"
	_ = m.View(left, "")

	wantMin := lipgloss.Width("  6. snell-v5")
	if m.LeftWidth() < wantMin {
		t.Fatalf("left width = %d, want >= %d", m.LeftWidth(), wantMin)
	}
	if strings.Contains(left, "\n") && m.RightWidth() < subSplitMinRight {
		t.Fatalf("right width = %d, want >= %d", m.RightWidth(), subSplitMinRight)
	}
}

func TestSubSplitKeepsRightPaneContentAvailable(t *testing.T) {
	m := NewSubSplit(44, 12)
	right := "1234567890123456789012345678901234567890"

	view := m.View("left", right)
	if !strings.Contains(view, "123456789012345678901") || !strings.Contains(view, "2345678901234567890") {
		t.Fatalf("view should keep right content across wrapped lines: %q", view)
	}
}
