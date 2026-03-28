package tui

import tea "github.com/charmbracelet/bubbletea"

// NavigateMsg requests navigation to a named view.
type NavigateMsg struct {
	Name string
}

// BackMsg requests returning to the previous view.
type BackMsg struct{}

// BackCmd is a tea.Cmd that emits a BackMsg.
func BackCmd() tea.Msg { return BackMsg{} }

// ShowOverlayMsg requests displaying an overlay.
type ShowOverlayMsg struct {
	Overlay OverlayModel
}

// DismissOverlayMsg requests removing the current overlay.
type DismissOverlayMsg struct{}

// InputResultMsg carries the result from a text input overlay.
type InputResultMsg struct {
	Value     string
	Cancelled bool
}

// ConfirmResultMsg carries the result from a confirm overlay.
type ConfirmResultMsg struct {
	Confirmed bool
}

// ResultDismissedMsg signals a result overlay was dismissed.
type ResultDismissedMsg struct{}

// OverlaySelectMsg carries a selection from an overlay menu.
type OverlaySelectMsg struct {
	ID string
}

// ViewResizeMsg carries the actual content dimensions to views.
// This is needed because views hold a pointer to the original Model which
// does not receive updates from bubbletea's value-based model copy.
type ViewResizeMsg struct {
	ContentWidth  int
	ContentHeight int
}

// SubSplitMouseMsg wraps a mouse event with coordinates relative to the
// right panel's inner area, so views can forward it to their SubSplitModel.
type SubSplitMouseMsg struct {
	tea.MouseMsg
}

// SubSplitResizeMsg signals that a sub-split divider drag changed the
// available right-panel dimensions for the current view.
// Only emitted on mouse release to avoid expensive rebuilds during drag.
type SubSplitResizeMsg struct {
	RightWidth  int
	RightHeight int
}

// OverlayModel is the interface for overlay components.
type OverlayModel interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (OverlayModel, tea.Cmd)
	View() string
}
