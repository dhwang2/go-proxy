package tui

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"go-proxy/internal/derived"
	"go-proxy/internal/store"
)

// RenderDashboard returns a lipgloss-styled dashboard string.
func RenderDashboard(s *store.Store, version string, width int) string {
	stats := derived.Dashboard(s)

	if width < 40 {
		width = 40
	}
	if width > 80 {
		width = 80
	}
	inner := width - 4 // padding for border

	// Title line.
	title := fmt.Sprintf("go-proxy 一键脚本 [%s]", version)

	// Info lines.
	sysLine := fmt.Sprintf("    系统: %s | 架构: %s", runtime.GOOS, displayArch())
	protoLine := fmt.Sprintf("    协议: %s", stats.Protocols)
	portLine := fmt.Sprintf("    端口: %s", stats.Ports)
	userLine := fmt.Sprintf("    用户: %d个用户", stats.UserCount)

	// Center the title.
	titlePad := inner - lipgloss.Width(title)
	if titlePad < 0 {
		titlePad = 0
	}
	leftPad := titlePad / 2
	centeredTitle := strings.Repeat(" ", leftPad) + title

	sep := strings.Repeat(string(BorderH), inner)

	content := lipgloss.JoinVertical(lipgloss.Left,
		centeredTitle,
		"",
		sep,
		sysLine,
		protoLine,
		portLine,
		userLine,
	)

	style := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(ColorPrimary).
		Width(inner)

	return style.Render(content)
}

func displayArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	default:
		return runtime.GOARCH
	}
}
