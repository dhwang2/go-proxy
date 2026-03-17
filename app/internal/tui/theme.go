package tui

import "github.com/charmbracelet/lipgloss"

// Colors — soft, modern palette (Catppuccin Mocha inspired).
var (
	ColorPrimary = lipgloss.Color("#89b4fa") // Blue (title, borders)
	ColorLabel   = lipgloss.Color("#cdd6f4") // Lavender-white (dashboard labels)
	ColorValSys  = lipgloss.Color("#f9e2af") // Peach-yellow (system/arch values)
	ColorValPort = lipgloss.Color("#cba6f7") // Mauve (port values)
	ColorSuccess = lipgloss.Color("#a6e3a1") // Green (status, rules)
	ColorError   = lipgloss.Color("#f38ba8") // Rosewater-red (errors, stopped)
	ColorWarning = lipgloss.Color("#fab387") // Peach (warnings)
	ColorMuted   = lipgloss.Color("#6c7086") // Overlay0 gray (hints, inactive)
	ColorAccent  = lipgloss.Color("#b4befe") // Lavender (accent)
)

// Reusable styles.
var (
	TitleStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true)

	LabelStyle = lipgloss.NewStyle().
			Foreground(ColorLabel).
			Bold(true)

	BorderStyle = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(ColorPrimary)

	DialogStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Padding(1, 3)

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
