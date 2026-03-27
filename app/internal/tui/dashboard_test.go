package tui

import (
	"regexp"
	"strings"
	"testing"

	"go-proxy/internal/derived"
)

func TestRenderCompactDashboardIncludesRouteCount(t *testing.T) {
	view := RenderCompactDashboard(derived.DashboardStats{
		UserCount:  2,
		RouteCount: 5,
		Protocols:  "ss, snell",
	}, "dev", 40)

	if !strings.Contains(view, "分流:") || !strings.Contains(view, "5条规则") {
		t.Fatalf("RenderCompactDashboard() missing route count: %q", view)
	}

	plain := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(view, "")
	lines := strings.Split(plain, "\n")
	if len(lines) < 2 {
		t.Fatalf("RenderCompactDashboard() lines = %d, want >= 2", len(lines))
	}
	if !strings.HasPrefix(lines[1], " ") {
		t.Fatalf("subtitle is not centered: %q", lines[1])
	}
}
