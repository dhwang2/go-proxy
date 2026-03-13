package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	statusActive   = lipgloss.NewStyle().Foreground(lipgloss.Color("#5fff87")).Bold(true).Render("●")
	statusFailed   = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5f5f")).Bold(true).Render("●")
	statusInactive = lipgloss.NewStyle().Foreground(lipgloss.Color("#6c6c6c")).Render("○")
	statusPending  = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffd75f")).Bold(true).Render("◐")
	statusUnknown  = lipgloss.NewStyle().Foreground(lipgloss.Color("#808080")).Render("?")
)

func StatusDot(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "active":
		return statusActive
	case "failed":
		return statusFailed
	case "inactive", "dead":
		return statusInactive
	case "activating", "reloading":
		return statusPending
	default:
		return statusUnknown
	}
}
