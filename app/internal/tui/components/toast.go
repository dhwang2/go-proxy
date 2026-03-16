package components

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ToastModel shows a temporary message that auto-dismisses.
type ToastModel struct {
	Message  string
	IsError  bool
	visible  bool
	duration time.Duration
}

// ToastExpiredMsg signals the toast should be hidden.
type ToastExpiredMsg struct{}

// ShowToast creates a visible toast that auto-dismisses.
func ShowToast(message string, isError bool, duration time.Duration) (ToastModel, tea.Cmd) {
	t := ToastModel{
		Message:  message,
		IsError:  isError,
		visible:  true,
		duration: duration,
	}
	return t, tea.Tick(duration, func(_ time.Time) tea.Msg {
		return ToastExpiredMsg{}
	})
}

// Update implements tea.Model.
func (m ToastModel) Update(msg tea.Msg) (ToastModel, tea.Cmd) {
	if _, ok := msg.(ToastExpiredMsg); ok {
		m.visible = false
	}
	return m, nil
}

// View renders the toast.
func (m ToastModel) View() string {
	if !m.visible {
		return ""
	}
	style := lipgloss.NewStyle().
		Padding(0, 1).
		MarginTop(1)
	if m.IsError {
		style = style.Foreground(lipgloss.Color("#F44336"))
	} else {
		style = style.Foreground(lipgloss.Color("#4CAF50"))
	}
	return style.Render(m.Message)
}

// Visible returns whether the toast is currently shown.
func (m ToastModel) Visible() bool {
	return m.visible
}
