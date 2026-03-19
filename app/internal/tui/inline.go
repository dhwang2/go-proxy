package tui

import tea "github.com/charmbracelet/bubbletea"

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
	switch msg.(type) {
	case tea.KeyMsg, tea.MouseMsg:
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
