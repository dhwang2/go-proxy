package tui

import "github.com/charmbracelet/lipgloss"

// Colors matching shell-proxy palette.
var (
	ColorPrimary = lipgloss.Color("#00d7d7") // Cyan (title, borders)
	ColorLabel   = lipgloss.Color("#ffffff") // White bold (dashboard labels)
	ColorValSys  = lipgloss.Color("#d7d700") // Yellow (system/arch values)
	ColorValPort = lipgloss.Color("#d787ff") // Magenta (port values)
	ColorSuccess = lipgloss.Color("#00d700") // Green (status, rules)
	ColorError   = lipgloss.Color("#ff5f5f") // Red (errors, stopped)
	ColorWarning = lipgloss.Color("#ffd700") // Yellow (warnings)
	ColorMuted   = lipgloss.Color("#808080") // Gray (hints, inactive)
	ColorAccent  = lipgloss.Color("#7C4DFF") // Purple (accent)
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
