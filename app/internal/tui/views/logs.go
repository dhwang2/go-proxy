package views

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-proxy/internal/config"
	"go-proxy/internal/derived"
	"go-proxy/internal/store"
	"go-proxy/internal/tui"
	"go-proxy/internal/tui/components"
)

type logsStep int

const (
	logsMenu logsStep = iota
	logsServiceSelect
	logsResult
)

type LogsView struct {
	tui.InlineState
	model       *tui.Model
	menu        tui.MenuModel
	serviceMenu tui.MenuModel
	split       tui.SubSplitModel
	step        logsStep
}

func NewLogsView(model *tui.Model) *LogsView {
	v := &LogsView{model: model}
	v.menu = tui.NewMenu("󰌱 运行日志", []tui.MenuItem{
		{Key: '1', Label: "󰌱 查看脚本日志 (最近30行)", ID: "script"},
		{Key: '2', Label: "󰌱 查看 Watchdog 日志 (最近30行)", ID: "watchdog"},
		{Key: '3', Label: "󰌱 查看服务日志 (按服务选择)", ID: "service"},
	})
	return v
}

func (v *LogsView) Name() string { return "logs" }

func (v *LogsView) Init() tea.Cmd {
	v.step = logsMenu
	v.split.SetFocusLeft(true)
	v.split.SetSize(v.model.ContentWidth(), v.model.Height()-6)
	return nil
}

func (v *LogsView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.ViewResizeMsg:
		v.split.SetSize(msg.ContentWidth, msg.ContentHeight-6)
		return v, nil
	case tui.SubSplitMouseMsg:
		var cmd tea.Cmd
		v.split, cmd = v.split.Update(msg.MouseMsg)
		return v, cmd
	}
	inlineCmd, handled := v.UpdateInline(msg)
	if handled {
		return v, inlineCmd
	}
	switch msg := msg.(type) {
	case tui.MenuSelectMsg:
		if v.step == logsServiceSelect {
			svc := msg.ID
			v.step = logsResult
			v.split.SetFocusLeft(false)
			return v, tea.Batch(
				v.SetInline(components.NewSpinner("加载日志...")),
				func() tea.Msg { return v.readServiceLog(svc) },
			)
		}

		v.split.SetFocusLeft(false)
		switch msg.ID {
		case "script":
			v.step = logsResult
			return v, tea.Batch(
				v.SetInline(components.NewSpinner("加载日志...")),
				func() tea.Msg { return v.readScriptLog() },
			)
		case "watchdog":
			v.step = logsResult
			return v, tea.Batch(
				v.SetInline(components.NewSpinner("加载日志...")),
				func() tea.Msg { return v.readWatchdogLog() },
			)
		case "service":
			v.step = logsServiceSelect
			v.split.SetFocusLeft(true)
			return v, nil
		}

	case logsActionDoneMsg:
		v.step = logsResult
		v.split.SetFocusLeft(false)
		return v, v.SetInline(components.NewResult(msg.result))

	case tui.ResultDismissedMsg:
		v.step = logsMenu
		v.split.SetFocusLeft(true)
		return v, nil

	default:
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.Type == tea.KeyEsc {
			switch v.step {
			case logsServiceSelect:
				v.step = logsMenu
				v.split.SetFocusLeft(true)
				return v, nil
			default:
				return v, tui.BackCmd
			}
		}
		switch v.step {
		case logsMenu:
			var cmd tea.Cmd
			v.menu, cmd = v.menu.Update(msg)
			return v, cmd
		case logsServiceSelect:
			var cmd tea.Cmd
			v.serviceMenu, cmd = v.serviceMenu.Update(msg)
			return v, cmd
		}
	}
	return v, inlineCmd
}

func (v *LogsView) View() string {
	if v.step == logsMenu || !v.split.Enabled() {
		if v.HasInline() {
			return v.ViewInline()
		}
		if v.step == logsServiceSelect {
			return tui.RenderSubMenuBody(v.serviceMenu.View(), v.model.ContentWidth())
		}
		return tui.RenderSubMenuBody(v.menu.View(), v.model.ContentWidth())
	}

	var menuContent string
	if v.step == logsServiceSelect {
		menuContent = v.serviceMenu.View()
	} else {
		menuContent = v.menu.View()
	}

	var detailContent string
	if v.HasInline() {
		tui.InSplitPanel = true
		detailContent = v.ViewInline()
		tui.InSplitPanel = false
	} else {
		detailContent = lipgloss.NewStyle().
			Foreground(tui.ColorMuted).
			Render("加载中...")
	}

	return v.split.View(menuContent, detailContent)
}

type logsActionDoneMsg struct{ result string }

func (v *LogsView) buildServiceMenu() tui.MenuModel {
	// Map protocol types to systemd service names.
	serviceMap := map[string]string{
		"vless":       "sing-box",
		"trojan":      "sing-box",
		"shadowsocks": "sing-box",
		"tuic":        "sing-box",
		"anytls":      "sing-box",
	}

	seen := make(map[string]bool)
	var items []tui.MenuItem
	key := '1'

	// Always include sing-box if any inbound exists.
	inv := derived.Inventory(v.model.Store())
	for _, p := range inv {
		svc := serviceMap[p.Type]
		if svc == "" {
			svc = p.Type
		}
		if seen[svc] {
			continue
		}
		seen[svc] = true
		items = append(items, tui.MenuItem{Key: key, Label: svc, ID: svc})
		key++
	}

	// Add snell-v5 if config exists.
	if v.model.Store().SnellConf != nil && !seen["snell-v5"] {
		items = append(items, tui.MenuItem{Key: key, Label: "snell-v5", ID: "snell-v5"})
		key++
	}

	// Add shadow-tls if outbound exists.
	for _, raw := range v.model.Store().SingBox.Outbounds {
		h, err := store.ParseOutboundHeader(raw)
		if err == nil && h.Type == "shadowtls" && !seen["shadow-tls"] {
			items = append(items, tui.MenuItem{Key: key, Label: "shadow-tls", ID: "shadow-tls"})
			key++
			seen["shadow-tls"] = true
			break
		}
	}

	// Add caddy-sub if not seen.
	if !seen["caddy-sub"] {
		items = append(items, tui.MenuItem{Key: key, Label: "caddy-sub", ID: "caddy-sub"})
		key++
	}

	return tui.NewMenu("查看服务日志", items)
}

// readScriptLog reads the script log file directly.
func (v *LogsView) readScriptLog() tea.Msg {
	content, source := readLogFileOrJournalctl(config.ScriptLog, "proxy-script", 30)
	title := "脚本日志"
	if source != "" {
		title += " (" + source + ")"
	}
	return logsActionDoneMsg{result: title + "\n\n" + colorizeLogOutput(content)}
}

// readWatchdogLog reads watchdog log from file or journalctl.
func (v *LogsView) readWatchdogLog() tea.Msg {
	content, source := readLogFileOrJournalctl(config.WatchdogLog, "proxy-watchdog", 30)
	title := "Watchdog 日志"
	if source != "" {
		title += " (" + source + ")"
	}
	return logsActionDoneMsg{result: title + "\n\n" + colorizeLogOutput(content)}
}

// readServiceLog reads a service log with file/journalctl fallback.
func (v *LogsView) readServiceLog(svc string) tea.Msg {
	logFile, unit := serviceLogSource(svc)
	content, source := readLogFileOrJournalctl(logFile, unit, 30)
	title := svc + " 日志"
	if source != "" {
		title += " (" + source + ")"
	}
	return logsActionDoneMsg{result: title + "\n\n" + colorizeLogOutput(content)}
}

// serviceLogSource returns the log file path and systemd unit for a service.
func serviceLogSource(svc string) (logFile, unit string) {
	switch svc {
	case "sing-box":
		return config.SingBoxLog, "sing-box"
	case "snell-v5":
		return config.SnellLog, "snell-v5"
	case "shadow-tls":
		return config.ShadowTLSLog, "shadow-tls"
	case "caddy-sub":
		return config.CaddySubLog, "caddy-sub"
	default:
		return "", svc
	}
}

// readLogFileOrJournalctl tries to read a log file, falls back to journalctl.
// Returns (content, source_note).
func readLogFileOrJournalctl(logFile, unit string, lines int) (string, string) {
	// Try log file first.
	if logFile != "" {
		if info, err := os.Stat(logFile); err == nil && info.Size() > 0 {
			content := tailFile(logFile, lines)
			if content != "" {
				return content, logFile
			}
		}
	}

	// Fall back to journalctl.
	if unit != "" {
		out, err := exec.Command("journalctl", "-u", unit, "-n", fmt.Sprintf("%d", lines), "--no-pager").CombinedOutput()
		if err == nil {
			result := strings.TrimSpace(string(out))
			if result != "" {
				return result, "journalctl -u " + unit
			}
		}
	}

	return "暂无日志", ""
}

// tailFile reads the last N lines of a file.
func tailFile(path string, n int) string {
	out, err := exec.Command("tail", "-n", fmt.Sprintf("%d", n), path).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// colorizeLogOutput adds ANSI color codes to log output for display.
// ERROR/FATAL -> red, WARN -> yellow, INFO -> cyan.
func colorizeLogOutput(content string) string {
	if content == "" || content == "暂无日志" {
		return content
	}

	lines := strings.Split(content, "\n")
	var result strings.Builder
	for i, line := range lines {
		if i > 0 {
			result.WriteString("\n")
		}
		result.WriteString(colorizeLine(line))
	}
	return result.String()
}

func colorizeLine(line string) string {
	upper := strings.ToUpper(line)
	switch {
	case strings.Contains(upper, "FATAL") || strings.Contains(upper, "ERROR") || strings.Contains(upper, "FAILED"):
		return "\033[31;1m" + line + "\033[0m"
	case strings.Contains(upper, "WARN") || strings.Contains(upper, "WARNING"):
		return "\033[33;1m" + line + "\033[0m"
	case strings.Contains(upper, "INFO"):
		return "\033[36;1m" + line + "\033[0m"
	default:
		return line
	}
}
