package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// InSplitPanel is set to true during View() rendering when inline content
// is displayed inside a SubSplit right panel. Components check this to
// skip their DialogStyle border.
var InSplitPanel bool

const (
	subSplitMinLeft    = 20
	subSplitMinRight   = 20
	subSplitMinTotal   = 50
	subSplitDividerHit = 2 // mouse hit-test range ±N columns around divider
)

// SubSplitModel manages a left/right sub-split inside the right panel.
type SubSplitModel struct {
	leftWidth   int
	totalWidth  int
	totalHeight int
	dragging    bool
	focusLeft   bool
	enabled     bool
	minLeft     int
	minRight    int
}

// NewSubSplit creates a SubSplitModel with default proportions.
func NewSubSplit(totalWidth, totalHeight int) SubSplitModel {
	m := SubSplitModel{
		minLeft:   subSplitMinLeft,
		minRight:  subSplitMinRight,
		focusLeft: true,
	}
	m.setSize(totalWidth, totalHeight)
	return m
}

func (m *SubSplitModel) setSize(w, h int) {
	m.totalWidth = w
	m.totalHeight = h
	if w < subSplitMinTotal {
		m.enabled = false
		return
	}
	m.enabled = true
	// Default: left gets 28% of width (close to menu text + 4 char padding).
	if m.leftWidth == 0 {
		m.leftWidth = w * 28 / 100
	}
	m.clampLeftWidth()
}

func (m *SubSplitModel) clampLeftWidth() {
	// 1 column reserved for the divider.
	max := m.totalWidth - 1 - m.minRight
	if m.leftWidth < m.minLeft {
		m.leftWidth = m.minLeft
	}
	if m.leftWidth > max {
		m.leftWidth = max
	}
}

// SetSize updates the available dimensions.
func (m *SubSplitModel) SetSize(w, h int) {
	m.setSize(w, h)
}

// TotalWidth returns the total available width.
func (m SubSplitModel) TotalWidth() int { return m.totalWidth }

// Enabled returns whether sub-split is active.
func (m SubSplitModel) Enabled() bool { return m.enabled }

// FocusLeft returns true when focus is on the left sub-panel.
func (m SubSplitModel) FocusLeft() bool { return m.focusLeft }

// ToggleFocus switches focus between left and right sub-panels.
func (m *SubSplitModel) ToggleFocus() { m.focusLeft = !m.focusLeft }

// SetFocusLeft explicitly sets focus to the left sub-panel.
func (m *SubSplitModel) SetFocusLeft(v bool) { m.focusLeft = v }

// LeftWidth returns the left sub-panel width.
func (m SubSplitModel) LeftWidth() int {
	if !m.enabled {
		return m.totalWidth
	}
	return m.leftWidth
}

// RightWidth returns the right sub-panel width (excluding divider).
func (m SubSplitModel) RightWidth() int {
	if !m.enabled {
		return 0
	}
	return m.totalWidth - m.leftWidth - 1
}

// DividerX returns the X position of the divider within the sub-split.
func (m SubSplitModel) DividerX() int {
	return m.leftWidth
}

// Dragging returns whether the divider is being dragged.
func (m SubSplitModel) Dragging() bool { return m.dragging }

// Update handles mouse events for divider dragging.
// The caller must translate mouse coordinates to be relative to the sub-split area.
func (m SubSplitModel) Update(msg tea.Msg) (SubSplitModel, tea.Cmd) {
	if !m.enabled {
		return m, nil
	}
	mouseMsg, ok := msg.(tea.MouseMsg)
	if !ok {
		return m, nil
	}

	switch mouseMsg.Action {
	case tea.MouseActionPress:
		if mouseMsg.Button == tea.MouseButtonLeft {
			divX := m.leftWidth
			if mouseMsg.X >= divX-subSplitDividerHit && mouseMsg.X <= divX+subSplitDividerHit {
				m.dragging = true
			}
		}
	case tea.MouseActionMotion:
		if m.dragging {
			m.leftWidth = mouseMsg.X
			m.clampLeftWidth()
		}
	case tea.MouseActionRelease:
		m.dragging = false
	}
	return m, nil
}

// View renders leftContent and rightContent side-by-side with a divider.
// When disabled, only leftContent is returned.
func (m SubSplitModel) View(leftContent, rightContent string) string {
	if !m.enabled {
		return leftContent
	}

	lw := m.leftWidth
	rw := m.totalWidth - lw - 1

	// Use totalHeight so divider extends to bottom border.
	h := m.totalHeight
	if h < 1 {
		h = 1
	}

	left := lipgloss.NewStyle().Width(lw).Height(h).Render(leftContent)
	right := lipgloss.NewStyle().Width(rw).Height(h).Render(rightContent)

	// Render divider spanning full height.
	divColor := ColorPanelBorder
	if m.dragging {
		divColor = ColorDragBorder
	}
	divStyle := lipgloss.NewStyle().Foreground(divColor)
	divider := divStyle.Render(strings.Repeat("┃\n", h-1) + "┃")

	return lipgloss.JoinHorizontal(lipgloss.Top, left, divider, right)
}
