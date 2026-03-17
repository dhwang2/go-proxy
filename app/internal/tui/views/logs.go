package views

import (
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

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
	model       *tui.Model
	menu        components.MenuModel
	serviceMenu components.MenuModel
	step        logsStep
}

func NewLogsView(model *tui.Model) *LogsView {
	v := &LogsView{model: model}
	v.menu = components.NewMenu("󰌱 运行日志", []components.MenuItem{
		{Key: '1', Label: "󰌱 查看脚本日志", ID: "script"},
		{Key: '2', Label: "󰌱 查看 Watchdog 日志", ID: "watchdog"},
		{Key: '3', Label: "󰌱 查看服务日志", ID: "service"},
		{Key: '0', Label: "󰌍 返回", ID: "back"},
	})
	return v
}

func (v *LogsView) Name() string { return "logs" }

func (v *LogsView) Init() tea.Cmd {
	v.step = logsMenu
	return nil
}

func (v *LogsView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case components.MenuSelectMsg:
		if v.step == logsServiceSelect {
			if msg.ID == "back" {
				v.step = logsMenu
				return v, nil
			}
			svc := msg.ID
			v.step = logsResult
			return v, func() tea.Msg { return v.readJournalctl(svc) }
		}

		switch msg.ID {
		case "back":
			return v, tui.BackCmd
		case "script":
			v.step = logsResult
			return v, func() tea.Msg { return v.readJournalctl("proxy-script") }
		case "watchdog":
			v.step = logsResult
			return v, func() tea.Msg { return v.readJournalctl("proxy-watchdog") }
		case "service":
			v.step = logsServiceSelect
			v.serviceMenu = v.buildServiceMenu()
			return v, nil
		}

	case logsActionDoneMsg:
		v.step = logsResult
		return v, func() tea.Msg {
			return tui.ShowOverlayMsg{
				Overlay: components.NewResult(msg.result),
			}
		}

	case tui.ResultDismissedMsg:
		v.step = logsMenu
		return v, nil

	default:
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
	return v, nil
}

func (v *LogsView) View() string {
	if v.step == logsServiceSelect {
		return v.serviceMenu.View()
	}
	return v.menu.View()
}

type logsActionDoneMsg struct{ result string }

func (v *LogsView) buildServiceMenu() components.MenuModel {
	// Map protocol types to systemd service names.
	serviceMap := map[string]string{
		"vless":       "sing-box",
		"trojan":      "sing-box",
		"shadowsocks": "sing-box",
		"tuic":        "sing-box",
		"anytls":      "sing-box",
	}

	seen := make(map[string]bool)
	var items []components.MenuItem
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
		items = append(items, components.MenuItem{Key: key, Label: svc, ID: svc})
		key++
	}

	// Add snell-v5 if config exists.
	if v.model.Store().SnellConf != nil && !seen["snell-v5"] {
		items = append(items, components.MenuItem{Key: key, Label: "snell-v5", ID: "snell-v5"})
		key++
	}

	// Add shadow-tls if outbound exists.
	for _, raw := range v.model.Store().SingBox.Outbounds {
		h, err := store.ParseOutboundHeader(raw)
		if err == nil && h.Type == "shadowtls" && !seen["shadow-tls"] {
			items = append(items, components.MenuItem{Key: key, Label: "shadow-tls", ID: "shadow-tls"})
			key++
			seen["shadow-tls"] = true
			break
		}
	}

	// Add caddy-sub if not seen.
	if !seen["caddy-sub"] {
		items = append(items, components.MenuItem{Key: key, Label: "caddy-sub", ID: "caddy-sub"})
		key++
	}

	items = append(items, components.MenuItem{Key: '0', Label: "󰌍 返回", ID: "back"})
	return components.NewMenu("查看服务日志", items)
}

func (v *LogsView) readJournalctl(unit string) tea.Msg {
	out, err := exec.Command("journalctl", "-u", unit, "-n", "50", "--no-pager").CombinedOutput()
	if err != nil {
		return logsActionDoneMsg{result: fmt.Sprintf("%s 日志\n\n读取失败: %s\n%s", unit, err, string(out))}
	}
	result := strings.TrimSpace(string(out))
	if result == "" {
		result = "暂无日志"
	}
	return logsActionDoneMsg{result: fmt.Sprintf("%s 日志\n\n%s", unit, result)}
}
