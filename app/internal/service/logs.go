package service

import "strings"

// ANSI color codes.
const (
	ansiReset  = "\033[0m"
	ansiRed    = "\033[31m"
	ansiYellow = "\033[33m"
	ansiGreen  = "\033[32m"
	ansiCyan   = "\033[36m"
	ansiGray   = "\033[90m"
)

// ColorizeLogLine applies ANSI coloring to a log line based on severity keywords.
func ColorizeLogLine(line string) string {
	lower := strings.ToLower(line)
	switch {
	case strings.Contains(lower, "error") || strings.Contains(lower, "fatal") || strings.Contains(lower, "panic"):
		return ansiRed + line + ansiReset
	case strings.Contains(lower, "warn"):
		return ansiYellow + line + ansiReset
	case strings.Contains(lower, "info"):
		return ansiGreen + line + ansiReset
	case strings.Contains(lower, "debug") || strings.Contains(lower, "trace"):
		return ansiGray + line + ansiReset
	default:
		return line
	}
}
