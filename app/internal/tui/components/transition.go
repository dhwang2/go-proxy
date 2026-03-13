package components

import (
	"strings"
	"time"
)

type TransitionStyle int

const (
	TransitionSlideLeft TransitionStyle = iota
	TransitionSlideRight
	TransitionFadeIn
)

type Transition struct {
	From     string
	To       string
	Style    TransitionStyle
	Duration time.Duration
}

func (t Transition) Frame(progress float64) string {
	if progress <= 0 {
		return strings.TrimRight(t.From, "\n")
	}
	if progress >= 1 {
		return strings.TrimRight(t.To, "\n")
	}
	switch t.Style {
	case TransitionSlideLeft:
		return slideLeft(t.From, t.To, progress)
	case TransitionSlideRight:
		return slideRight(t.From, t.To, progress)
	default:
		return fadeIn(t.To, progress)
	}
}

func slideLeft(from, to string, progress float64) string {
	width := maxLineWidth(from, to)
	shift := int(float64(width) * progress)
	if shift <= 0 {
		return strings.TrimRight(from, "\n")
	}
	lines := strings.Split(strings.TrimRight(to, "\n"), "\n")
	for i := range lines {
		pad := strings.Repeat(" ", max(width-shift, 0))
		lines[i] = pad + lines[i]
	}
	return strings.Join(lines, "\n")
}

func slideRight(from, to string, progress float64) string {
	width := maxLineWidth(from, to)
	shift := int(float64(width) * progress)
	if shift <= 0 {
		return strings.TrimRight(from, "\n")
	}
	lines := strings.Split(strings.TrimRight(to, "\n"), "\n")
	for i := range lines {
		if shift >= len(lines[i]) {
			continue
		}
		lines[i] = lines[i][len(lines[i])-max(len(lines[i])-shift, 0):]
	}
	return strings.Join(lines, "\n")
}

func fadeIn(to string, progress float64) string {
	if progress >= 1 {
		return strings.TrimRight(to, "\n")
	}
	lines := strings.Split(strings.TrimRight(to, "\n"), "\n")
	cut := int(float64(len(lines)) * progress)
	if cut <= 0 {
		return ""
	}
	return strings.Join(lines[:cut], "\n")
}

func maxLineWidth(items ...string) int {
	maxWidth := 0
	for _, item := range items {
		lines := strings.Split(strings.TrimRight(item, "\n"), "\n")
		for _, line := range lines {
			if len(line) > maxWidth {
				maxWidth = len(line)
			}
		}
	}
	if maxWidth <= 0 {
		return 1
	}
	return maxWidth
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
