package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNavigateMsgResetsBreadcrumbStack(t *testing.T) {
	model := NewModel(nil, "dev")

	next, _ := model.Update(NavigateMsg{Name: "routing"})
	model = next.(Model)
	if got := model.nav.Breadcrumb(); len(got) != 1 || got[0] != "routing" {
		t.Fatalf("breadcrumb after first navigate = %#v", got)
	}

	next, _ = model.Update(NavigateMsg{Name: "service"})
	model = next.(Model)
	if got := model.nav.Breadcrumb(); len(got) != 1 || got[0] != "service" {
		t.Fatalf("breadcrumb after second navigate = %#v", got)
	}
}

type resizeCaptureView struct {
	resize ViewResizeMsg
	seen   bool
	count  int
}

func (v *resizeCaptureView) Init() tea.Cmd { return nil }

func (v *resizeCaptureView) Update(msg tea.Msg) (View, tea.Cmd) {
	if resize, ok := msg.(ViewResizeMsg); ok {
		v.resize = resize
		v.seen = true
		v.count++
	}
	return v, nil
}

func (v *resizeCaptureView) View() string { return "" }

func (v *resizeCaptureView) Name() string { return "config" }

func (v *resizeCaptureView) HasInline() bool { return false }

func TestMainDividerDragSendsResizeToCurrentViewOnRelease(t *testing.T) {
	model := NewModel(nil, "dev")
	model.width = 120
	model.height = 30
	model.recalcLayout()
	model.current = "config"
	view := &resizeCaptureView{}
	model.views["config"] = view

	pressX := model.leftWidth
	next, _ := model.Update(tea.MouseMsg{
		X:      pressX,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	model = next.(Model)

	next, _ = model.Update(tea.MouseMsg{
		X:      pressX + 6,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionMotion,
	})
	model = next.(Model)

	// During drag, views should NOT receive resize messages (performance optimization).
	// Only layout dimensions are updated; content rebuild happens on release.
	if view.seen {
		t.Fatal("expected no resize message during divider drag motion")
	}

	next, _ = model.Update(tea.MouseMsg{
		X:      pressX + 6,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionRelease,
	})
	model = next.(Model)

	if !view.seen {
		t.Fatal("expected current view to receive resize message on divider release")
	}
	if view.resize.ContentWidth != model.contentWidth {
		t.Fatalf("resize width = %d, want %d", view.resize.ContentWidth, model.contentWidth)
	}
	if view.resize.ContentHeight != model.height-2 {
		t.Fatalf("resize height = %d, want %d", view.resize.ContentHeight, model.height-2)
	}
	if view.count != 1 {
		t.Fatalf("resize count = %d, want 1", view.count)
	}
}
