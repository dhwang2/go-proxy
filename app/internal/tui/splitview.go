package tui

import tea "github.com/charmbracelet/bubbletea"

// SplitViewBase provides common split-panel, inline-state, menu, and focus management
// for views that use the standard LEFT=menu, RIGHT=detail pattern.
// Views embed this struct and call its helper methods from their own Update/View.
type SplitViewBase struct {
	InlineState
	Model *Model
	Menu  MenuModel
	Split SubSplitModel
}

// SetFocus sets split focus and dims/undims the main menu accordingly.
// onFocus is an optional callback for views that need to dim additional components.
func (b *SplitViewBase) SetFocus(left bool, onFocus ...func(left bool)) {
	b.Split.SetFocusLeft(left)
	b.Menu = b.Menu.SetDim(!left)
	for _, fn := range onFocus {
		fn(left)
	}
}

// InitSplit resets split focus to left and sizes the split panel.
func (b *SplitViewBase) InitSplit() {
	b.ClearInline()
	b.Split.SetFocusLeft(true)
	b.Split.SetSize(b.Model.ContentWidth(), b.Model.Height()-5)
}

// HandleResize updates split dimensions on ViewResizeMsg.
func (b *SplitViewBase) HandleResize(msg ViewResizeMsg) {
	b.Split.SetSize(msg.ContentWidth, msg.ContentHeight-3)
}

// HandleMouse delegates mouse events to the split model. Returns the tea.Cmd.
func (b *SplitViewBase) HandleMouse(msg SubSplitMouseMsg) tea.Cmd {
	var cmd tea.Cmd
	b.Split, cmd = b.Split.Update(msg.MouseMsg)
	return cmd
}

// HandleMenuNav intercepts up/down (and optionally Enter) keys for the main menu
// when split is enabled, left is focused, and view is not on the menu step.
// Returns (cmd, true) if handled, (nil, false) otherwise.
func (b *SplitViewBase) HandleMenuNav(msg tea.Msg, isMenuStep bool, includeEnter bool) (tea.Cmd, bool) {
	if !b.Split.Enabled() || isMenuStep || !b.Split.FocusLeft() {
		return nil, false
	}
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, false
	}
	switch keyMsg.Type {
	case tea.KeyUp, tea.KeyDown:
		var cmd tea.Cmd
		b.Menu, cmd = b.Menu.Update(msg)
		return cmd, true
	case tea.KeyEnter:
		if includeEnter {
			var cmd tea.Cmd
			b.Menu, cmd = b.Menu.Update(msg)
			return cmd, true
		}
	}
	return nil, false
}

// HandleSplitArrows handles Left/Right arrow keys for focus toggling in split mode.
// hasRightContent should be true when the right panel has displayable content.
// Returns true if handled.
func (b *SplitViewBase) HandleSplitArrows(msg tea.KeyMsg, isMenuStep bool, hasRightContent bool) bool {
	if !b.Split.Enabled() || isMenuStep {
		return false
	}
	switch msg.Type {
	case tea.KeyLeft:
		b.SetFocus(true)
		return true
	case tea.KeyRight:
		if hasRightContent {
			b.SetFocus(false)
			return true
		}
	}
	return false
}

// IsSubSplitRightFocused returns true when split is enabled and right panel is focused.
func (b *SplitViewBase) IsSubSplitRightFocused() bool {
	return b.Split.Enabled() && !b.Split.FocusLeft()
}

// RenderSplitView renders the standard split layout with menu LEFT and detail RIGHT.
// Falls back to non-split rendering when split is disabled or on menu step.
func (b *SplitViewBase) RenderSplitView(isMenuStep bool, menuContent string, detailContent string) string {
	if isMenuStep || !b.Split.Enabled() {
		if b.HasInline() {
			return b.ViewInline()
		}
		return RenderSubMenuBody(menuContent, b.Model.ContentWidth())
	}
	return b.Split.View(menuContent, detailContent)
}
