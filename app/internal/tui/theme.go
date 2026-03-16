package tui

import "github.com/charmbracelet/lipgloss"

// Colors.
var (
	ColorPrimary = lipgloss.Color("#00BCD4")
	ColorSuccess = lipgloss.Color("#4CAF50")
	ColorError   = lipgloss.Color("#F44336")
	ColorWarning = lipgloss.Color("#FFC107")
	ColorMuted   = lipgloss.Color("#9E9E9E")
	ColorAccent  = lipgloss.Color("#7C4DFF")
)

// Text styles.
var (
	StyleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary)

	StyleSubtitle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	StyleSuccess = lipgloss.NewStyle().
			Foreground(ColorSuccess)

	StyleError = lipgloss.NewStyle().
			Foreground(ColorError)

	StyleWarning = lipgloss.NewStyle().
			Foreground(ColorWarning)

	StyleMuted = lipgloss.NewStyle().
			Foreground(ColorMuted)

	StyleAccent = lipgloss.NewStyle().
			Foreground(ColorAccent).
			Bold(true)

	StyleMenuItem = lipgloss.NewStyle().
			PaddingLeft(2)

	StyleMenuItemActive = lipgloss.NewStyle().
				PaddingLeft(2).
				Foreground(ColorPrimary).
				Bold(true)

	StyleBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Padding(0, 1)

	StyleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			BorderBottom(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(ColorMuted).
			MarginBottom(1)

	StyleStatusBar = lipgloss.NewStyle().
			Foreground(ColorMuted).
			MarginTop(1)
)
