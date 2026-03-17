package tui

import "github.com/charmbracelet/lipgloss"

// Colors — high-contrast palette (orange/green/purple/black/white/yellow/deep blue).
var (
	ColorPrimary = lipgloss.Color("#1e40af") // Deep blue (titles)
	ColorLabel   = lipgloss.Color("#ffffff") // White (labels/text)
	ColorValSys  = lipgloss.Color("#eab308") // Yellow (system values, warnings)
	ColorSuccess = lipgloss.Color("#22c55e") // Green (success/running)
	ColorError   = lipgloss.Color("#f97316") // Orange (errors, stopped)
	ColorWarning = lipgloss.Color("#eab308") // Yellow (warnings)
	ColorMuted   = lipgloss.Color("#6b7280") // Gray (hints, inactive)
	ColorAccent  = lipgloss.Color("#f97316") // Orange (accent, selected)
)

// Reusable styles.
var (
	TitleStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true)

	LabelStyle = lipgloss.NewStyle().
			Foreground(ColorLabel).
			Bold(true)

	DialogStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Padding(1, 3)

	MenuItemStyle = lipgloss.NewStyle().
			PaddingLeft(2)

	MenuSelectedStyle = lipgloss.NewStyle().
				PaddingLeft(2).
				Foreground(ColorAccent).
				Bold(true)

	StatusStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			PaddingLeft(2)
)

const (
	Bullet = '●'
)
