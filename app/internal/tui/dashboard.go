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
	"go-proxy/internal/service"
	"go-proxy/internal/subscription"
)

func renderDashboardHeader(width int, titleText, subtitleText string) (string, string) {
	title := HeaderTitleStyle.Width(width).Render(titleText)
	subtitle := lipgloss.NewStyle().Foreground(ColorBlack).Align(lipgloss.Center).Width(width).Render(subtitleText)
	return title, subtitle
}

var (
	ipCacheMu     sync.RWMutex
	ipCacheV4     string
	ipCacheV6     string
	ipCacheReady  bool
	ipCacheExpiry time.Time
	ipRefreshing  bool
)

const ipCacheTTL = 60 * time.Second

func refreshIPCacheAsync() {
	ipCacheMu.Lock()
	if ipRefreshing || (ipCacheReady && time.Now().Before(ipCacheExpiry)) {
		ipCacheMu.Unlock()
		return
	}
	ipRefreshing = true
	ipCacheMu.Unlock()

	go func() {
		v4 := subscription.DetectIPv4()
		v6 := subscription.DetectIPv6()
		ipCacheMu.Lock()
		ipCacheV4 = v4
		ipCacheV6 = v6
		ipCacheReady = true
		ipCacheExpiry = time.Now().Add(ipCacheTTL)
		ipRefreshing = false
		ipCacheMu.Unlock()
	}()
}

func getCachedNetworkStack() (v4, v6 string, ready bool) {
	refreshIPCacheAsync()
	ipCacheMu.RLock()
	defer ipCacheMu.RUnlock()
	return ipCacheV4, ipCacheV6, ipCacheReady
}

func renderNetworkStack() string {
	v4, v6, ready := getCachedNetworkStack()
	if !ready {
		return ValSysStyle.Render("检测中...")
	}
	var parts []string
	if v4 != "" {
		parts = append(parts, "ipv4("+v4+")")
	}
	if v6 != "" {
		parts = append(parts, "ipv6("+v6+")")
	}
	if len(parts) == 0 {
		return ValSysStyle.Render("未检测到")
	}
	return ValSysStyle.Render(strings.Join(parts, " | "))
}

func renderDashboardInfoLines(stats derived.DashboardStats, width int, serviceText string) []string {
	lineStyle := lipgloss.NewStyle().Width(width)
	return []string{
		lineStyle.Render(fmt.Sprintf(" %s %s | %s",
			LabelStyle.Render("系统:"),
			ValSysStyle.Render(runtime.GOOS),
			ValSysStyle.Render(displayArch()),
		)),
		lineStyle.Render(fmt.Sprintf(" %s %s",
			LabelStyle.Render("网络栈:"),
			renderNetworkStack(),
		)),
		lineStyle.Render(fmt.Sprintf(" %s %s",
			LabelStyle.Render("协议:"),
			ValProtoStyle.Render(stats.Protocols),
		)),
		lineStyle.Render(fmt.Sprintf(" %s %s",
			LabelStyle.Render("用户:"),
			FormatUserCount(stats.UserCount),
		)),
		lineStyle.Render(fmt.Sprintf(" %s %s",
			LabelStyle.Render("分流:"),
			ValRuleStyle.Render(fmt.Sprintf("%d条规则", stats.RouteCount)),
		)),
		lineStyle.Render(fmt.Sprintf(" %s %s",
			LabelStyle.Render("服务:"),
			serviceText,
		)),
	}
}

// RenderDashboard returns a lipgloss-styled dashboard string.
func RenderDashboard(stats derived.DashboardStats, version string, width int) string {

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

	title, subtitle := renderDashboardHeader(sepWidth, "go-proxy 快捷指令：gproxy", fmt.Sprintf("作者: dhwang2 · 命令: gproxy · 版本: %s", version))
	lines := renderDashboardInfoLines(stats, sepWidth-4, renderServiceStatus())

	// Wrap status info in an inset bordered panel.
	statusContent := lipgloss.JoinVertical(lipgloss.Left, lines...)
	statusPanel := StatusPanelStyle.Width(sepWidth - 4).Render(statusContent)

	// Center the status panel.
	statusCentered := lipgloss.NewStyle().Width(sepWidth).Align(lipgloss.Center).Render(statusPanel)

	sep := SeparatorDouble(sepWidth)

	content := lipgloss.JoinVertical(lipgloss.Center,
		"",
		title,
		subtitle,
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

// checkShadowTLS checks the aggregate status of all shadow-tls-* units.
// Uses any-active semantics: reports running if at least one unit is active.
func checkShadowTLS() serviceStatusEntry {
	entry := serviceStatusEntry{name: "shadow-tls"}
	names, err := service.ShadowTLSServiceNames()
	if err != nil || len(names) == 0 {
		return entry
	}
	entry.exists = true
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, name := range names {
		out, err := exec.CommandContext(ctx, "systemctl", "is-active", name).CombinedOutput()
		if err == nil && strings.TrimSpace(string(out)) == "active" {
			entry.running = true
			return entry
		}
	}
	return entry
}

// Shared async service status cache. Both full and compact renderers
// read from the same cached entries without blocking.
var (
	svcCacheMu      sync.RWMutex
	svcCacheEntries []serviceStatusEntry
	svcCacheExpiry  time.Time
	svcRefreshing   bool
)

const serviceStatusTTL = 10 * time.Second

var dashboardServices = []string{"sing-box", "snell-v5", "shadow-tls", "caddy-sub", "proxy-watchdog"}

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
			if svc == "shadow-tls" {
				entries[i] = checkShadowTLS()
			} else {
				entries[i] = checkService(svc)
			}
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
func RenderCompactDashboard(stats derived.DashboardStats, version string, width int) string {
	title, subtitle := renderDashboardHeader(width, "go-proxy", "作者: dhwang2  版本号: "+version)
	lines := renderDashboardInfoLines(stats, width, renderCompactServiceStatus())

	sep := SeparatorDouble(width)

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		subtitle,
		sep,
		lipgloss.JoinVertical(lipgloss.Left, lines...),
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
		case "proxy-watchdog":
			return "wdog"
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
