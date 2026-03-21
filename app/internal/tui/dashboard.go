package tui

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
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
	title := HeaderTitleStyle.Width(sepWidth).Render("go-proxy 快捷指令：gproxy")
	subtitle := fmt.Sprintf("作者: dhwang2 · 命令: gproxy · 版本: %s", version)
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

// Shared async service status cache. Both full and compact renderers
// read from the same cached entries without blocking.
var (
	svcCacheMu      sync.RWMutex
	svcCacheEntries  []serviceStatusEntry
	svcCacheExpiry   time.Time
	svcRefreshing    bool
)

const serviceStatusTTL = 10 * time.Second

var dashboardServices = []string{"sing-box", "snell-v5", "shadow-tls", "caddy-sub"}

// Pre-rendered dot styles to avoid per-frame allocation.
var (
	dotGreen = lipgloss.NewStyle().Foreground(ColorSuccess).Render("●")
	dotRed   = lipgloss.NewStyle().Foreground(ColorError).Render("●")
	dotGray  = lipgloss.NewStyle().Foreground(ColorMuted).Render("●")
)

func serviceDot(e serviceStatusEntry) string {
	if !e.exists {
		return dotGray
	}
	if e.running {
		return dotGreen
	}
	return dotRed
}

// refreshServiceCacheAsync triggers a background refresh if the cache is stale.
func refreshServiceCacheAsync() {
	svcCacheMu.Lock()
	if svcRefreshing || time.Now().Before(svcCacheExpiry) {
		svcCacheMu.Unlock()
		return
	}
	svcRefreshing = true
	svcCacheMu.Unlock()

	go func() {
		entries := make([]serviceStatusEntry, len(dashboardServices))
		for i, svc := range dashboardServices {
			entries[i] = checkService(svc)
		}
		svcCacheMu.Lock()
		svcCacheEntries = entries
		svcCacheExpiry = time.Now().Add(serviceStatusTTL)
		svcRefreshing = false
		svcCacheMu.Unlock()
	}()
}

// getCachedEntries returns the current cached entries (may be nil on first call).
func getCachedEntries() []serviceStatusEntry {
	refreshServiceCacheAsync()
	svcCacheMu.RLock()
	defer svcCacheMu.RUnlock()
	return svcCacheEntries
}

func renderServiceStatus() string {
	entries := getCachedEntries()
	var parts []string
	if entries == nil {
		for _, svc := range dashboardServices {
			parts = append(parts, dotGray+" "+svc)
		}
	} else {
		for _, e := range entries {
			parts = append(parts, serviceDot(e)+" "+e.name)
		}
	}
	return strings.Join(parts, "  ")
}

// RenderCompactDashboard returns a compact dashboard for the narrow left panel.
func RenderCompactDashboard(s *store.Store, version string, width int) string {
	stats := derived.Dashboard(s)

	title := HeaderTitleStyle.Width(width).Render("go-proxy")
	sub := HeaderSubStyle.Width(width).Render("作者: dhwang2  版本号: " + version)

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
	entries := getCachedEntries()

	abbreviate := func(name string) string {
		switch name {
		case "sing-box":
			return "sbox"
		case "shadow-tls":
			return "stls"
		case "caddy-sub":
			return "cdy"
		case "snell-v5":
			return "snl"
		default:
			return name
		}
	}

	var parts []string
	if entries == nil {
		for _, svc := range dashboardServices {
			parts = append(parts, dotGray+" "+abbreviate(svc))
		}
	} else {
		for _, e := range entries {
			parts = append(parts, serviceDot(e)+" "+abbreviate(e.name))
		}
	}
	return strings.Join(parts, " ")
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
