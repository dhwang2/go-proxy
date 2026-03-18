package tui

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

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

	sepWidth := SeparatorWidth
	if sepWidth > width {
		sepWidth = width
	}

	// Title lines.
	title := HeaderTitleStyle.Width(sepWidth).Render("go-proxy 一键部署 [服务端]")
	subtitle := fmt.Sprintf("作者: dhwang2 · 命令: proxy · 版本: %s", version)
	subtitleRendered := HeaderSubStyle.Width(sepWidth).Render(subtitle)

	// Status panel content — compact layout.
	sysInfo := fmt.Sprintf("%s | %s",
		ValSysStyle.Render(runtime.GOOS),
		ValSysStyle.Render(displayArch()),
	)
	sysLine := fmt.Sprintf("  %s  %s", LabelStyle.Render("系统:"), sysInfo)

	protoLine := fmt.Sprintf("  %s  %s",
		LabelStyle.Render("协议:"),
		ValProtoStyle.Render(stats.Protocols),
	)

	userLine := fmt.Sprintf("  %s  %s",
		LabelStyle.Render("用户:"),
		FormatUserCount(stats.UserCount),
	)

	svcLine := fmt.Sprintf("  %s  %s", LabelStyle.Render("服务:"), renderServiceStatus())

	// Wrap status info in an inset bordered panel.
	statusContent := lipgloss.JoinVertical(lipgloss.Left,
		sysLine,
		protoLine,
		userLine,
		svcLine,
	)
	statusPanel := StatusPanelStyle.Width(sepWidth - 4).Render(statusContent)

	// Center the status panel.
	statusCentered := lipgloss.NewStyle().Width(sepWidth).Align(lipgloss.Center).Render(statusPanel)

	sep := SeparatorDouble(sepWidth)

	content := lipgloss.JoinVertical(lipgloss.Center,
		"",
		title,
		subtitleRendered,
		sep,
		statusCentered,
		sep,
	)

	// Center the entire dashboard block within terminal width.
	style := lipgloss.NewStyle().Width(width).Align(lipgloss.Center)

	return style.Render(content)
}

// serviceStatusEntry holds the display info for one service.
type serviceStatusEntry struct {
	name    string
	running bool
	exists  bool
}

func checkService(name string) serviceStatusEntry {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	entry := serviceStatusEntry{name: name}

	// Check if the binary/unit exists.
	out, err := exec.CommandContext(ctx, "systemctl", "cat", name).CombinedOutput()
	if err != nil {
		// If systemctl cat fails, service is not installed.
		if strings.Contains(string(out), "No files found") || strings.Contains(string(out), "not found") {
			return entry
		}
		// On non-Linux or no systemctl, treat as not installed.
		return entry
	}
	entry.exists = true

	// Check if active.
	out, err = exec.CommandContext(ctx, "systemctl", "is-active", name).CombinedOutput()
	entry.running = err == nil && strings.TrimSpace(string(out)) == "active"
	return entry
}

// Cached service status to avoid shelling out on every View() call.
// Separate caches for full and compact formats to prevent layout corruption
// when both are rendered on the same frame in split-panel mode.
var (
	cachedServiceStatus        string
	serviceStatusExpiry        time.Time
	cachedCompactServiceStatus string
	compactServiceStatusExpiry time.Time
)

const serviceStatusTTL = 5 * time.Second

func renderServiceStatus() string {
	now := time.Now()
	if cachedServiceStatus != "" && now.Before(serviceStatusExpiry) {
		return cachedServiceStatus
	}

	services := []string{"sing-box", "snell-v5", "shadow-tls", "caddy-sub"}

	greenDot := lipgloss.NewStyle().Foreground(ColorSuccess).Render("●")
	redDot := lipgloss.NewStyle().Foreground(ColorError).Render("●")
	grayDot := lipgloss.NewStyle().Foreground(ColorMuted).Render("●")

	var parts []string
	for _, svc := range services {
		entry := checkService(svc)
		var dot string
		if !entry.exists {
			dot = grayDot
		} else if entry.running {
			dot = greenDot
		} else {
			dot = redDot
		}
		parts = append(parts, fmt.Sprintf("%s %s", dot, svc))
	}
	result := strings.Join(parts, "  ")
	cachedServiceStatus = result
	serviceStatusExpiry = now.Add(serviceStatusTTL)
	return result
}

// RenderCompactDashboard returns a compact dashboard for the narrow left panel.
func RenderCompactDashboard(s *store.Store, version string, width int) string {
	stats := derived.Dashboard(s)

	title := HeaderTitleStyle.Width(width).Render("go-proxy")
	sub := HeaderSubStyle.Width(width).Render("作者 dhwang2  " + version)

	lineStyle := lipgloss.NewStyle().Width(width)
	sysInfo := lineStyle.Render(fmt.Sprintf(" %s %s | %s",
		LabelStyle.Render("系统:"),
		ValSysStyle.Render(runtime.GOOS),
		ValSysStyle.Render(displayArch()),
	))
	protoInfo := lineStyle.Render(fmt.Sprintf(" %s %s",
		LabelStyle.Render("协议:"),
		ValProtoStyle.Render(stats.Protocols),
	))
	userInfo := lineStyle.Render(fmt.Sprintf(" %s %s",
		LabelStyle.Render("用户:"),
		FormatUserCount(stats.UserCount),
	))
	svcInfo := lineStyle.Render(fmt.Sprintf(" %s %s",
		LabelStyle.Render("服务:"),
		renderCompactServiceStatus(),
	))

	sep := SeparatorDouble(width)

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		sub,
		sep,
		sysInfo,
		protoInfo,
		userInfo,
		svcInfo,
		sep,
	)
}

// renderCompactServiceStatus renders a brief service status for the narrow left panel.
func renderCompactServiceStatus() string {
	now := time.Now()
	if cachedCompactServiceStatus != "" && now.Before(compactServiceStatusExpiry) {
		return cachedCompactServiceStatus
	}

	services := []string{"sing-box", "snell-v5", "shadow-tls", "caddy-sub"}
	greenDot := lipgloss.NewStyle().Foreground(ColorSuccess).Render("●")
	redDot := lipgloss.NewStyle().Foreground(ColorError).Render("●")
	grayDot := lipgloss.NewStyle().Foreground(ColorMuted).Render("●")

	var parts []string
	for _, svc := range services {
		entry := checkService(svc)
		var dot string
		if !entry.exists {
			dot = grayDot
		} else if entry.running {
			dot = greenDot
		} else {
			dot = redDot
		}
		// Abbreviate service names for compact display.
		short := svc
		switch svc {
		case "shadow-tls":
			short = "stls"
		case "caddy-sub":
			short = "caddy"
		case "snell-v5":
			short = "snell"
		}
		parts = append(parts, dot+" "+short)
	}
	result := strings.Join(parts, " ")
	cachedCompactServiceStatus = result
	compactServiceStatusExpiry = now.Add(serviceStatusTTL)
	return result
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
