package tui

import "github.com/gdamore/tcell/v2"

// Colors matching the original palette.
var (
	ColorPrimary = tcell.ColorTeal       // #00BCD4
	ColorSuccess = tcell.ColorGreen      // #4CAF50
	ColorError   = tcell.ColorRed        // #F44336
	ColorWarning = tcell.ColorYellow     // #FFC107
	ColorMuted   = tcell.ColorGray       // #9E9E9E
	ColorAccent  = tcell.ColorDarkViolet // #7C4DFF
	ColorBorder  = tcell.ColorTeal
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
