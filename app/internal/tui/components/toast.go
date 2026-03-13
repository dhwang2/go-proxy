package components

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

type ToastStyle int

const (
	ToastInfo ToastStyle = iota
	ToastSuccess
	ToastError
)

type Toast struct {
	Message  string
	Style    ToastStyle
	Deadline time.Time
}

func NewToast(message string, style ToastStyle, duration time.Duration) Toast {
	if duration <= 0 {
		duration = 3 * time.Second
	}
	return Toast{
		Message:  strings.TrimSpace(message),
		Style:    style,
		Deadline: time.Now().Add(duration),
	}
}

func (t Toast) Expired(now time.Time) bool {
	if t.Deadline.IsZero() {
		return false
	}
	return !now.Before(t.Deadline)
}

func (t Toast) View() string {
	msg := strings.TrimSpace(t.Message)
	if msg == "" {
		return ""
	}
	switch t.Style {
	case ToastSuccess:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true).Render(msg)
	case ToastError:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Render(msg)
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true).Render(msg)
	}
}
