package tui

import "github.com/charmbracelet/lipgloss"

// Colors matching the original palette.
var (
	ColorPrimary = lipgloss.Color("#00BCD4") // Teal
	ColorSuccess = lipgloss.Color("#4CAF50")
	ColorError   = lipgloss.Color("#F44336")
	ColorWarning = lipgloss.Color("#FFC107")
	ColorMuted   = lipgloss.Color("#9E9E9E")
	ColorAccent  = lipgloss.Color("#7C4DFF")
)

// Reusable styles.
var (
	TitleStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true)

	BorderStyle = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(ColorPrimary)

	DialogStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Padding(1, 2)

	MenuItemStyle = lipgloss.NewStyle().
			PaddingLeft(2)

	MenuSelectedStyle = lipgloss.NewStyle().
				PaddingLeft(2).
				Foreground(ColorPrimary).
				Bold(true)

	StatusStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			PaddingLeft(2)
)

// DashboardBorder characters (double-line).
const (
	BorderH  = '═'
	BorderV  = '║'
	BorderTL = '╔'
	BorderTR = '╗'
	BorderBL = '╚'
	BorderBR = '╝'
	BorderML = '╠'
	BorderMR = '╣'
	Bullet   = '●'
)
