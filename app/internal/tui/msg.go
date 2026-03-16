package tui

import tea "github.com/charmbracelet/bubbletea"

// NavigateMsg requests navigation to a named view.
type NavigateMsg struct {
	Name string
}

// BackMsg requests returning to the previous view.
type BackMsg struct{}

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

// OverlayModel is the interface for overlay components.
type OverlayModel interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (OverlayModel, tea.Cmd)
	View() string
}
