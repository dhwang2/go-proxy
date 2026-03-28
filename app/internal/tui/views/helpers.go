package views

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"go-proxy/internal/tui"
)

// padCell pads text with spaces to reach the given display width.
// Uses lipgloss.Width for correct CJK character measurement.
func padCell(text string, width int) string {
	padding := width - lipgloss.Width(text)
	if padding < 0 {
		padding = 0
	}
	return text + strings.Repeat(" ", padding)
}

type tableSections struct {
	Header string
	Body   string
}

func renderTable(headers []string, rows [][]string, width int, separateRows bool) string {
	sections := renderTableSections(headers, rows, width, separateRows)
	if sections.Body == "" {
		return sections.Header
	}
	return sections.Header + "\n" + sections.Body
}

func renderTableSections(headers []string, rows [][]string, width int, separateRows bool) tableSections {
	labelStyle := lipgloss.NewStyle().Foreground(tui.ColorLabel).Bold(true)
	valStyle := lipgloss.NewStyle().Foreground(tui.ColorValSys)
	sepStyle := lipgloss.NewStyle().Foreground(tui.ColorMuted)

	if len(headers) == 0 {
		return tableSections{}
	}

	widths := make([]int, len(headers))
	minWidths := make([]int, len(headers))
	for i, header := range headers {
		headerWidth := lipgloss.Width(header)
		widths[i] = headerWidth
		switch {
		case len(headers) == 1:
			minWidths[i] = headerWidth
		case i == 0 && len(headers) >= 5:
			minWidths[i] = max(2, headerWidth)
		case i == len(headers)-1:
			minWidths[i] = max(12, headerWidth)
		default:
			minWidths[i] = max(8, headerWidth)
		}
	}
	for _, row := range rows {
		for i := range headers {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			if w := lipgloss.Width(cell); w > widths[i] {
				widths[i] = w
			}
		}
	}

	const (
		indent = 2
		gap    = 2
	)

	if width > 0 {
		usable := width - indent - gap*(len(headers)-1)
		if usable > 0 {
			natural := 0
			minimum := 0
			for i := range widths {
				natural += widths[i]
				minimum += minWidths[i]
			}
			switch {
			case natural <= usable:
			case minimum <= usable:
				for natural > usable {
					idx := widestShrinkableColumn(widths, minWidths)
					if idx < 0 {
						break
					}
					widths[idx]--
					natural--
				}
			default:
				return renderStackedTable(headers, rows, width, separateRows, labelStyle, valStyle, sepStyle)
			}
		}
	}

	var totalWidth int
	for _, width := range widths {
		totalWidth += width
	}
	totalWidth += gap * (len(headers) - 1)

	var sb strings.Builder
	sb.WriteString("  ")
	for i, header := range headers {
		if i == len(headers)-1 {
			sb.WriteString(labelStyle.Render(header))
			continue
		}
		sb.WriteString(labelStyle.Render(padCell(header, widths[i])))
		sb.WriteString(strings.Repeat(" ", gap))
	}
	sb.WriteString("\n  ")
	sb.WriteString(sepStyle.Render(strings.Repeat("─", totalWidth)))
	headerBlock := sb.String()

	sb.Reset()

	for rowIndex, row := range rows {
		linesByColumn := make([][]string, len(headers))
		rowHeight := 1
		for i := range headers {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			linesByColumn[i] = wrapDisplayText(cell, widths[i])
			if len(linesByColumn[i]) > rowHeight {
				rowHeight = len(linesByColumn[i])
			}
		}
		for lineIndex := 0; lineIndex < rowHeight; lineIndex++ {
			sb.WriteString("  ")
			for colIndex := range headers {
				cellLine := ""
				if lineIndex < len(linesByColumn[colIndex]) {
					cellLine = linesByColumn[colIndex][lineIndex]
				}
				if colIndex == len(headers)-1 {
					sb.WriteString(valStyle.Render(cellLine))
					continue
				}
				sb.WriteString(valStyle.Render(padCell(cellLine, widths[colIndex])))
				sb.WriteString(strings.Repeat(" ", gap))
			}
			sb.WriteString("\n")
		}
		if separateRows && rowIndex != len(rows)-1 {
			sb.WriteString("  ")
			sb.WriteString(sepStyle.Render(strings.Repeat("─", totalWidth)))
			sb.WriteString("\n")
		}
	}
	return tableSections{Header: headerBlock, Body: strings.TrimSuffix(sb.String(), "\n")}
}

func renderStackedTable(headers []string, rows [][]string, width int, separateRows bool, labelStyle, valStyle, sepStyle lipgloss.Style) tableSections {
	const indent = "  "
	maxWidth := width
	if maxWidth <= 0 {
		maxWidth = 68
	}
	bodyWidth := maxWidth - len(indent) - 2
	if bodyWidth < 8 {
		bodyWidth = 8
	}

	headerBlock := indent + labelStyle.Render(strings.Join(headers, " / "))
	separator := indent + sepStyle.Render(strings.Repeat("─", max(12, bodyWidth)))

	var sb strings.Builder
	for rowIndex, row := range rows {
		for colIndex, header := range headers {
			cell := ""
			if colIndex < len(row) {
				cell = row[colIndex]
			}
			prefix := indent + labelStyle.Render(header+": ")
			prefixWidth := lipgloss.Width(prefix)
			lines := wrapDisplayText(cell, max(4, maxWidth-prefixWidth))
			if len(lines) == 0 {
				lines = []string{""}
			}
			for lineIndex, line := range lines {
				if lineIndex == 0 {
					sb.WriteString(prefix)
					sb.WriteString(valStyle.Render(line))
				} else {
					sb.WriteString(strings.Repeat(" ", prefixWidth))
					sb.WriteString(valStyle.Render(line))
				}
				sb.WriteString("\n")
			}
		}
		if separateRows && rowIndex != len(rows)-1 {
			sb.WriteString(separator)
			sb.WriteString("\n")
		}
	}

	return tableSections{
		Header: headerBlock + "\n" + separator,
		Body:   strings.TrimSuffix(sb.String(), "\n"),
	}
}

func widestShrinkableColumn(widths, minWidths []int) int {
	best := -1
	bestDelta := 0
	for i := range widths {
		delta := widths[i] - minWidths[i]
		if delta > bestDelta {
			best = i
			bestDelta = delta
		}
	}
	return best
}

func wrapDisplayText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	paragraphs := strings.Split(text, "\n")
	var lines []string
	for _, paragraph := range paragraphs {
		if paragraph == "" {
			lines = append(lines, "")
			continue
		}
		var current strings.Builder
		currentWidth := 0
		for len(paragraph) > 0 {
			r, size := utf8.DecodeRuneInString(paragraph)
			paragraph = paragraph[size:]
			rw := runewidth.RuneWidth(r)
			if currentWidth+rw > width && currentWidth > 0 {
				lines = append(lines, current.String())
				current.Reset()
				currentWidth = 0
			}
			current.WriteRune(r)
			currentWidth += rw
		}
		if current.Len() > 0 {
			lines = append(lines, current.String())
		}
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func wrapPanelContent(content string, width int) string {
	if width <= 0 {
		return content
	}
	return lipgloss.NewStyle().Width(width).Render(content)
}
