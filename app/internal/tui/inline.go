package tui

import tea "github.com/charmbracelet/bubbletea"

// CursorComponent is implemented by inline components that use left/right keys
// for cursor movement (e.g., text input). When an inline component implements
// this interface and returns true, UpdateInline will intercept left/right keys.
// Otherwise, left/right keys pass through for SubSplit focus navigation.
type CursorComponent interface {
	UsesCursor() bool
}

// InlineState manages an inline component rendered within a view's content area.
// Views embed this struct and call SetInline to show components (text input,
// confirm, result, spinner, select list) directly in the right panel instead of
// as full-screen overlays.
type InlineState struct {
	component OverlayModel
}

// SetInline activates an inline component and returns its Init command.
func (s *InlineState) SetInline(c OverlayModel) tea.Cmd {
	s.component = c
	if c != nil {
		return c.Init()
	}
	return nil
}

// ClearInline removes the active inline component.
func (s *InlineState) ClearInline() {
	s.component = nil
}

// HasInline returns true when an inline component is active.
func (s *InlineState) HasInline() bool {
	return s.component != nil
}

// UpdateInline forwards a message to the inline component.
// Returns (cmd, handled). When handled is true, the caller must not process
// the message further.
func (s *InlineState) UpdateInline(msg tea.Msg) (tea.Cmd, bool) {
	if s.component == nil {
		return nil, false
	}
	// Result messages signal component completion; clear inline and let the
	// view's own handler process the result.
	switch msg.(type) {
	case InputResultMsg, ConfirmResultMsg, ResultDismissedMsg, OverlaySelectMsg:
		s.component = nil
		return nil, false
	}
	var cmd tea.Cmd
	s.component, cmd = s.component.Update(msg)
	// Only intercept user-input events; let domain and tick messages pass
	// through so the view can handle async results while a spinner runs.
	switch m := msg.(type) {
	case tea.KeyMsg:
		// Pass through Left/Right keys for SubSplit focus navigation
		// unless the component uses cursor movement (e.g., TextInput).
		if m.Type == tea.KeyLeft || m.Type == tea.KeyRight {
			if cc, ok := s.component.(CursorComponent); !ok || !cc.UsesCursor() {
				return cmd, false
			}
		}
		return cmd, true
	case tea.MouseMsg:
		return cmd, true
	}
	return cmd, false
}

// ViewInline renders the active inline component.
func (s *InlineState) ViewInline() string {
	if s.component == nil {
		return ""
	}
	return s.component.View()
}
