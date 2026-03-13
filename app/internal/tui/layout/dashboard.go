package layout

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/dhwang2/go-proxy/internal/derived"
	"github.com/dhwang2/go-proxy/internal/service"
	"github.com/dhwang2/go-proxy/internal/store"
)

var (
	dashLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#6c6c6c"))
	dashValueStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#bcbcbc"))
	dashAccent     = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff87d7")).Bold(true)
	dashGreen      = lipgloss.NewStyle().Foreground(lipgloss.Color("#5fff87")).Bold(true)
	dashRed        = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5f5f")).Bold(true)
	dashYellow     = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffd75f")).Bold(true)
	dashMuted      = lipgloss.NewStyle().Foreground(lipgloss.Color("#6c6c6c"))
	dashTitle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#5fd7ff")).Bold(true)
	dashBorder     = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#444444")).
			Padding(0, 1)
)

// Dashboard renders a bordered status panel.
func Dashboard(st *store.Store, statuses []service.ServiceStatus, width int) string {
	if st == nil {
		return ""
	}
	if width < 40 {
		width = 40
	}

	stack := installedStack(statuses)
	statusLine := overallStatus(statuses)

	protocols := derived.InstalledProtocols(st.Config)
	protoText := strings.Join(protocols, "+")
	if protoText == "" {
		protoText = "—"
	}

	ports := derived.ListeningPorts(st.Config)
	portText := derived.FormatPorts(ports)
	if portText == "" {
		portText = "—"
	}

	rules := derived.RouteRuleCount(st.Config)

	lines := []string{
		dashTitle.Render("服务端管理"),
		kv("系统", fmt.Sprintf("%s | 架构: %s", st.Platform.OS, stack)),
		kvRaw("状态", statusLine),
		kv("协议", protoText),
		kvAccent("端口", portText),
		kv("分流", fmt.Sprintf("%d条规则", rules)),
	}

	content := strings.Join(lines, "\n")
	return dashBorder.Width(width).Render(content)
}

func kv(key, value string) string {
	return fmt.Sprintf(" %s %s",
		dashLabelStyle.Render(key+"："),
		dashValueStyle.Render(value))
}

func kvAccent(key, value string) string {
	return fmt.Sprintf(" %s %s",
		dashLabelStyle.Render(key+"："),
		dashAccent.Render(value))
}

func kvRaw(key, styledValue string) string {
	return fmt.Sprintf(" %s %s",
		dashLabelStyle.Render(key+"："),
		styledValue)
}

func overallStatus(statuses []service.ServiceStatus) string {
	if len(statuses) == 0 {
		return dashMuted.Render("… 加载中")
	}
	active, failed := 0, 0
	for _, s := range statuses {
		switch strings.ToLower(s.State) {
		case "active":
			active++
		case "failed":
			failed++
		}
	}
	if failed > 0 {
		return dashRed.Render(fmt.Sprintf("● 异常 (%d failed)", failed))
	}
	if active == len(statuses) {
		return dashGreen.Render("● 运行中")
	}
	if active > 0 {
		return dashYellow.Render(fmt.Sprintf("◐ 部分运行 (%d/%d)", active, len(statuses)))
	}
	return dashMuted.Render("○ 已停止")
}

func installedStack(statuses []service.ServiceStatus) string {
	var names []string
	for _, s := range statuses {
		if s.Version != "" && s.Version != "-" {
			names = append(names, s.Name)
		}
	}
	if len(names) == 0 {
		return "—"
	}
	return strings.Join(names, "+")
}
