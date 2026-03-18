package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Colors — One Dark inspired palette for terminal TUI.
var (
	ColorPrimary     = lipgloss.Color("#61AFEF") // Blue (titles)
	ColorTitle       = lipgloss.Color("#FF9500") // Orange (main title accent)
	ColorLabel       = lipgloss.Color("#ABB2BF") // Light gray (menu items, labels)
	ColorLabelDim    = lipgloss.Color("#5C6370") // Dimmed label (unfocused panel)
	ColorValSys      = lipgloss.Color("#E5C07B") // Yellow (system values)
	ColorSuccess     = lipgloss.Color("#98C379") // Green (running)
	ColorError       = lipgloss.Color("#E06C75") // Red (stopped/error)
	ColorMuted       = lipgloss.Color("#5C6370") // Dark gray (hints, separators)
	ColorAccent      = lipgloss.Color("#61AFEF") // Blue (selected item bg)
	ColorAccentFg    = lipgloss.Color("#282C34") // Dark (selected item fg)
	ColorFooterKey   = lipgloss.Color("#C678DD") // Purple (footer shortcut keys)
	ColorPanelBorder = lipgloss.Color("#3E4452") // Subtle dark gray (left panel border)
	ColorPanelFocus  = lipgloss.Color("#4B5263") // Slightly lighter (focused panel border)
	ColorDragBorder  = lipgloss.Color("#E5C07B") // Yellow (drag resize indicator)
	ColorActiveBg    = lipgloss.Color("#3E4452") // Dark highlight (active menu item bg)
)

// SeparatorWidth is the default width for double-line separators.
const SeparatorWidth = 68

// DefaultSubMenuHint is the standard hint shown at the bottom of sub-menus.
const DefaultSubMenuHint = "返回(esc) | 选择(↑↓) | 确认(enter)"

// Reusable styles.
var (
	HeaderTitleStyle = lipgloss.NewStyle().
				Foreground(ColorTitle).
				Bold(true).
				Align(lipgloss.Center)

	HeaderSubStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Align(lipgloss.Center)

	LabelStyle = lipgloss.NewStyle().
			Foreground(ColorLabel).
			Bold(true)

	DialogStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Padding(1, 3)

	// Value styles used in dashboard and config views.
	ValSysStyle   = lipgloss.NewStyle().Foreground(ColorValSys).Bold(true)
	ValProtoStyle = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
	ValRuleStyle  = lipgloss.NewStyle().Foreground(ColorSuccess).Bold(true)

	// Outer frame style for the main menu (single-panel fallback).
	OuterFrameStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Padding(0, 1)

	// Status panel inset border.
	StatusPanelStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorMuted).
				Padding(0, 2)

	// Split-panel styles.
	LeftPanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPanelBorder).
			Padding(0, 1)

	LeftPanelFocusedStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorPanelFocus).
				Padding(0, 1)

	RightPanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPanelBorder).
			Padding(0, 1)

	RightPanelFocusedStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorPanelFocus).
				Padding(0, 1)

	// Sub-menu title bar (solid colored bar).
	SubMenuTitleStyle = lipgloss.NewStyle().
				Background(ColorAccent).
				Foreground(ColorAccentFg).
				Bold(true).
				Padding(0, 1)

	// Breadcrumb styles.
	BreadcrumbDimStyle    = lipgloss.NewStyle().Foreground(ColorMuted)
	BreadcrumbActiveStyle = lipgloss.NewStyle().Foreground(ColorLabel)

	// Active menu item (sub-menu is open for this item).
	MenuActiveStyle = lipgloss.NewStyle().
			Background(ColorActiveBg).
			Foreground(ColorPrimary).
			Bold(true)
)

// Pre-computed separator at default width to avoid repeated allocation.
var defaultSeparator = lipgloss.NewStyle().Foreground(ColorMuted).Render(strings.Repeat("─", SeparatorWidth))

// SeparatorDouble renders a thin horizontal rule (─) styled with ColorMuted.
func SeparatorDouble(width int) string {
	if width <= 0 {
		return ""
	}
	if width == SeparatorWidth {
		return defaultSeparator
	}
	return lipgloss.NewStyle().Foreground(ColorMuted).Render(strings.Repeat("─", width))
}

// RenderFooterHint renders a footer hint line with highlighted shortcut keys.
// Format: "action(key) | action(key) | ..."
// Keys inside parentheses are rendered in ColorFooterKey, rest in ColorMuted.
func RenderFooterHint(hint string, width int) string {
	keyStyle := lipgloss.NewStyle().Foreground(ColorFooterKey)
	textStyle := lipgloss.NewStyle().Foreground(ColorMuted)

	var result strings.Builder
	i := 0
	for i < len(hint) {
		open := strings.IndexByte(hint[i:], '(')
		if open < 0 {
			result.WriteString(textStyle.Render(hint[i:]))
			break
		}
		close := strings.IndexByte(hint[i+open:], ')')
		if close < 0 {
			result.WriteString(textStyle.Render(hint[i:]))
			break
		}
		// Text before '('
		result.WriteString(textStyle.Render(hint[i : i+open]))
		// Key including parens
		keyText := hint[i+open : i+open+close+1]
		result.WriteString(keyStyle.Render(keyText))
		i = i + open + close + 1
	}

	return lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(result.String())
}

// RenderSubMenuFrame wraps sub-menu content with separators and a hint line.
func RenderSubMenuFrame(content, hint string, width int) string {
	sep := SeparatorDouble(width)
	hintRendered := RenderFooterHint(hint, width)

	return lipgloss.JoinVertical(lipgloss.Center, sep, content, sep, hintRendered, sep)
}

// RenderSubMenuBody wraps sub-menu content with separators only (no hint line).
// Used in split-panel mode where the hint is positioned at the panel bottom.
func RenderSubMenuBody(content string, width int) string {
	sep := SeparatorDouble(width)
	return lipgloss.JoinVertical(lipgloss.Center, sep, content, sep)
}

// FormatUserCount renders the user count with red warning color if zero.
func FormatUserCount(count int) string {
	text := fmt.Sprintf("%d 个用户", count)
	if count == 0 {
		return lipgloss.NewStyle().Foreground(ColorError).Bold(true).Render(text)
	}
	return ValRuleStyle.Render(text)
}
