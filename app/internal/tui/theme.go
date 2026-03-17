package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

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

// SeparatorWidth is the default width for double-line separators.
const SeparatorWidth = 68

// Reusable styles.
var (
	TitleStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true)

	HeaderTitleStyle = lipgloss.NewStyle().
				Foreground(ColorPrimary).
				Bold(true).
				Align(lipgloss.Center)

	HeaderSubStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Align(lipgloss.Center)

	FooterHintStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Align(lipgloss.Center)

	InfoLabelStyle = lipgloss.NewStyle().
			Foreground(ColorLabel).
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

// SeparatorDouble renders a double-line separator (═) styled with ColorMuted.
func SeparatorDouble(width int) string {
	return lipgloss.NewStyle().Foreground(ColorMuted).Render(strings.Repeat("═", width))
}

// RenderSubMenuFrame wraps sub-menu content with separators, a title, and a hint line.
// If title is empty, the title and its surrounding separator are omitted.
func RenderSubMenuFrame(title, content, hint string, width int) string {
	sep := SeparatorDouble(width)
	hintRendered := FooterHintStyle.Width(width).Render(hint)

	parts := []string{sep}
	if title != "" {
		parts = append(parts, HeaderTitleStyle.Width(width).Render(title), sep)
	}
	parts = append(parts, content, sep, hintRendered, sep)

	return lipgloss.JoinVertical(lipgloss.Center, parts...)
}
