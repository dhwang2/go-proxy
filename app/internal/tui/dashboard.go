package tui

import (
	"fmt"
	"strings"

	"github.com/rivo/tview"

	"go-proxy/internal/derived"
	"go-proxy/internal/store"
)

// NewDashboard creates a bordered dashboard panel showing stats.
func NewDashboard(s *store.Store, version string) *tview.TextView {
	tv := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	tv.SetBorder(false)
	UpdateDashboard(tv, s, version)
	return tv
}

// UpdateDashboard refreshes dashboard content from store.
func UpdateDashboard(tv *tview.TextView, s *store.Store, version string) {
	stats := derived.Dashboard(s)

	width := 40
	line := strings.Repeat(string(BorderH), width-2)

	var b strings.Builder
	// Top border.
	b.WriteRune(BorderTL)
	b.WriteString(line)
	b.WriteRune(BorderTR)
	b.WriteByte('\n')
	// Title row.
	title := fmt.Sprintf("  go-proxy %s", version)
	pad := width - 2 - len(title)
	if pad < 0 {
		pad = 0
	}
	b.WriteRune(BorderV)
	b.WriteString(title)
	b.WriteString(strings.Repeat(" ", pad))
	b.WriteRune(BorderV)
	b.WriteByte('\n')
	// Mid border.
	b.WriteRune(BorderML)
	b.WriteString(line)
	b.WriteRune(BorderMR)
	b.WriteByte('\n')
	// Stats row 1.
	row1 := fmt.Sprintf("  [green]%c[white] Protocols: %-4d Users: %d",
		Bullet, stats.ProtocolCount, stats.UserCount)
	pad1 := width - 2 - tview.TaggedStringWidth(row1)
	if pad1 < 0 {
		pad1 = 0
	}
	b.WriteRune(BorderV)
	b.WriteString(row1)
	b.WriteString(strings.Repeat(" ", pad1))
	b.WriteRune(BorderV)
	b.WriteByte('\n')
	// Stats row 2.
	status := "[green]active[white]"
	row2 := fmt.Sprintf("  [green]%c[white] Routes: %-5d Status: %s",
		Bullet, stats.RouteCount, status)
	pad2 := width - 2 - tview.TaggedStringWidth(row2)
	if pad2 < 0 {
		pad2 = 0
	}
	b.WriteRune(BorderV)
	b.WriteString(row2)
	b.WriteString(strings.Repeat(" ", pad2))
	b.WriteRune(BorderV)
	b.WriteByte('\n')
	// Bottom border.
	b.WriteRune(BorderBL)
	b.WriteString(line)
	b.WriteRune(BorderBR)

	tv.SetText(b.String())
}
