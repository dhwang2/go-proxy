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

	// Styles for colored values.
	titleStyle := lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(ColorLabel).Bold(true)
	sysValStyle := lipgloss.NewStyle().Foreground(ColorValSys).Bold(true)
	portValStyle := lipgloss.NewStyle().Foreground(ColorValPort).Bold(true)
	protoValStyle := lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
	ruleValStyle := lipgloss.NewStyle().Foreground(ColorSuccess).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(ColorMuted)

	// Title lines.
	title := titleStyle.Render("go-proxy 一键部署 [服务端]")
	subtitle := fmt.Sprintf("作者: dhwang2    命令: proxy    版本: %s", version)
	subtitleRendered := mutedStyle.Render(subtitle)

	sep := mutedStyle.Render(strings.Repeat("─", width))

	// Dashboard info lines with colored labels and values.
	sysInfo := fmt.Sprintf("%s | %s",
		sysValStyle.Render(runtime.GOOS),
		sysValStyle.Render(displayArch()),
	)
	sysLine := fmt.Sprintf("  %s  %s", labelStyle.Render("系统:"), sysInfo)

	protoLine := fmt.Sprintf("  %s  %s",
		labelStyle.Render("协议:"),
		protoValStyle.Render(stats.Protocols),
	)

	portLine := fmt.Sprintf("  %s  %s",
		labelStyle.Render("端口:"),
		portValStyle.Render(stats.Ports),
	)

	userLine := fmt.Sprintf("  %s  %s",
		labelStyle.Render("用户:"),
		ruleValStyle.Render(fmt.Sprintf("%d 个用户", stats.UserCount)),
	)

	// Service status bar.
	svcLine := fmt.Sprintf("  %s  %s", labelStyle.Render("服务:"), renderServiceStatus())

	content := lipgloss.JoinVertical(lipgloss.Center,
		title,
		subtitleRendered,
		sep,
		"",
		sysLine,
		protoLine,
		portLine,
		userLine,
		svcLine,
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
var (
	cachedServiceStatus string
	serviceStatusExpiry time.Time
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
