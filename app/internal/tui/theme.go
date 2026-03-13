package tui

import "github.com/charmbracelet/lipgloss"

// ─── Color Palette ────────────────────────────────────────────────
// Vibrant, high-contrast colors for dark terminals.

var (
	// Brand name: bold bright cyan
	brandStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#5fd7ff"))

	// Version tag: dim gray
	versionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6c6c6c"))

	// Section titles: bright cyan, bold
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#5fd7ff"))

	// Subtitle / hint below title
	subtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6c6c6c")).
			Italic(true)

	// Muted / secondary text
	mutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6c6c6c"))

	// Success: bright green
	okStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#5fff87")).
		Bold(true)

	// Error: bright red
	errStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ff5f5f")).
			Bold(true)

	// Warning: bright yellow
	warnStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ffd75f")).
			Bold(true)

	// Accent: bright magenta for ports, special values
	accentStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ff87d7")).
			Bold(true)
)
