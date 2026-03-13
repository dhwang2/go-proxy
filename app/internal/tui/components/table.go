package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	tableHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#5fd7ff"))
	tableCellStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#bcbcbc"))
)

// Table renders an aligned data table with headers.
type Table struct {
	headers []string
	rows    [][]string
	widths  []int
}

// NewTable creates a new table with the given headers.
func NewTable(headers []string) Table {
	copied := make([]string, len(headers))
	copy(copied, headers)
	return Table{headers: copied}
}

// AddRow appends a row to the table.
func (t *Table) AddRow(row []string) {
	copied := make([]string, len(row))
	copy(copied, row)
	t.rows = append(t.rows, copied)
	t.widths = nil // invalidate cached widths
}

// SetRows replaces all rows in the table.
func (t *Table) SetRows(rows [][]string) {
	t.rows = make([][]string, len(rows))
	for i, row := range rows {
		t.rows[i] = make([]string, len(row))
		copy(t.rows[i], row)
	}
	t.widths = nil
}

// computeWidths calculates the column widths from headers and row content.
func (t *Table) computeWidths() {
	cols := len(t.headers)
	for _, row := range t.rows {
		if len(row) > cols {
			cols = len(row)
		}
	}
	if cols == 0 {
		t.widths = nil
		return
	}

	t.widths = make([]int, cols)
	for i, h := range t.headers {
		if len(h) > t.widths[i] {
			t.widths[i] = len(h)
		}
	}
	for _, row := range t.rows {
		for i, cell := range row {
			if len(cell) > t.widths[i] {
				t.widths[i] = len(cell)
			}
		}
	}
}

// View renders the table as a string.
func (t *Table) View() string {
	if len(t.headers) == 0 && len(t.rows) == 0 {
		return ""
	}

	t.computeWidths()

	var b strings.Builder

	// Render header row
	if len(t.headers) > 0 {
		var cells []string
		for i, h := range t.headers {
			w := 0
			if i < len(t.widths) {
				w = t.widths[i]
			}
			cells = append(cells, tableHeaderStyle.Render(pad(h, w)))
		}
		b.WriteString(strings.Join(cells, "  "))
		b.WriteString("\n")

		// Separator
		var sep []string
		for i := range t.headers {
			w := 0
			if i < len(t.widths) {
				w = t.widths[i]
			}
			sep = append(sep, strings.Repeat("─", w))
		}
		b.WriteString(strings.Join(sep, "  "))
		b.WriteString("\n")
	}

	// Render data rows
	for _, row := range t.rows {
		var cells []string
		for i := 0; i < len(t.widths); i++ {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			cells = append(cells, tableCellStyle.Render(pad(cell, t.widths[i])))
		}
		b.WriteString(strings.Join(cells, "  "))
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

// pad right-pads a string to the given width.
func pad(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
