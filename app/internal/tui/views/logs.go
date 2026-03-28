package views

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"go-proxy/internal/config"
	"go-proxy/internal/derived"
	"go-proxy/internal/logs"
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
	tui.SplitViewBase
	serviceMenu   tui.MenuModel
	viewport      viewport.Model
	viewportReady bool
	rawContent    string // original unwrapped content for re-wrap on resize
	step          logsStep
}

func NewLogsView(model *tui.Model) *LogsView {
	v := &LogsView{}
	v.Model = model
	v.Menu = tui.NewMenu("", []tui.MenuItem{
		{Key: '1', Label: "󰌱 查看 Watchdog 日志", ID: "watchdog"},
		{Key: '2', Label: "󰌱 查看服务日志", ID: "service"},
	})
	return v
}

func (v *LogsView) Name() string { return "logs" }

func (v *LogsView) setFocus(left bool) {
	v.SetFocus(left, func(l bool) {
		v.serviceMenu = v.serviceMenu.SetDim(l) // right panel: dim when left focused
	})
}

func (v *LogsView) Init() tea.Cmd {
	v.step = logsMenu
	v.Menu = v.Menu.SetActiveID("")
	v.viewportReady = false
	v.InitSplit()
	v.Split.SetMinWidths(14, 10)
	return nil
}

func (v *LogsView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.ViewResizeMsg:
		v.HandleResize(msg)
		if v.viewportReady {
			w := msg.ContentWidth
			h := msg.ContentHeight - 5
			if v.Split.Enabled() {
				w = v.Split.RightWidth()
				h = v.Split.TotalHeight()
			}
			v.viewport.Width = w
			v.viewport.Height = h
			v.viewport.SetContent(wrapPanelContent(v.rawContent, w))
		}
		return v, nil
	case tui.SubSplitResizeMsg:
		if v.viewportReady {
			v.viewport.Width = msg.RightWidth
			v.viewport.Height = msg.RightHeight
			v.viewport.SetContent(wrapPanelContent(v.rawContent, msg.RightWidth))
		}
		return v, nil
	case tui.SubSplitMouseMsg:
		// Handle mouse wheel scrolling for viewport
		if v.step == logsResult && v.viewportReady {
			if msg.Button == tea.MouseButtonWheelUp {
				v.viewport.LineUp(3)
				return v, nil
			}
			if msg.Button == tea.MouseButtonWheelDown {
				v.viewport.LineDown(3)
				return v, nil
			}
		}
		return v, v.HandleMouse(msg)
	}
	// In split mode, route keys to main menu when left-focused and not on menu step.
	if cmd, handled := v.HandleMenuNav(msg, v.step == logsMenu, true); handled {
		return v, cmd
	}
	inlineCmd, handled := v.UpdateInline(msg)
	if handled {
		return v, inlineCmd
	}
	switch msg := msg.(type) {
	case tui.MenuCursorChangeMsg:
		return v, nil
	case tui.MenuSelectMsg:
		if v.step == logsServiceSelect && !(v.Split.Enabled() && v.Split.FocusLeft()) {
			svc := msg.ID
			v.step = logsResult
			v.setFocus(false)
			return v, tea.Batch(
				v.SetInline(components.NewSpinner("加载日志...")),
				func() tea.Msg { return v.readServiceLog(svc) },
			)
		}
		v.setFocus(false)
		return v, v.triggerMenuAction(msg.ID)

	case logsContentMsg:
		// Discard stale result if user already navigated away.
		if v.step == logsMenu {
			return v, nil
		}
		w := v.Model.ContentWidth()
		h := v.Model.Height() - 5
		if v.Split.Enabled() {
			w = v.Split.RightWidth()
			h = v.Split.TotalHeight()
		}
		v.rawContent = msg.content
		v.viewport = viewport.New(w, h)
		v.viewport.SetContent(wrapPanelContent(v.rawContent, w))
		v.viewportReady = true
		v.ClearInline()
		v.step = logsResult
		v.setFocus(false)
		return v, nil

	case tui.ResultDismissedMsg:
		v.step = logsMenu
		v.Menu = v.Menu.SetActiveID("")
		v.setFocus(true)
		return v, nil

	default:
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.Type == tea.KeyEsc {
			switch v.step {
			case logsServiceSelect:
				v.step = logsMenu
				v.Menu = v.Menu.SetActiveID("")
				v.setFocus(true)
				return v, nil
			case logsResult:
				v.step = logsMenu
				v.Menu = v.Menu.SetActiveID("")
				v.viewportReady = false
				v.setFocus(true)
				return v, nil
			default:
				return v, tui.BackCmd
			}
		}
		// Left/Right arrow toggles sub-split focus.
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			if v.Split.Enabled() && v.step != logsMenu {
				if keyMsg.Type == tea.KeyLeft {
					v.setFocus(true)
					return v, nil
				}
				if keyMsg.Type == tea.KeyRight && (v.HasInline() || v.viewportReady) {
					v.setFocus(false)
					return v, nil
				}
			}
		}
		switch v.step {
		case logsMenu:
			var cmd tea.Cmd
			v.Menu, cmd = v.Menu.Update(msg)
			return v, cmd
		case logsServiceSelect:
			var cmd tea.Cmd
			if v.Split.Enabled() && v.Split.FocusLeft() {
				v.Menu, cmd = v.Menu.Update(msg)
			} else {
				v.serviceMenu, cmd = v.serviceMenu.Update(msg)
			}
			return v, cmd
		case logsResult:
			if v.viewportReady && !v.Split.FocusLeft() {
				var cmd tea.Cmd
				v.viewport, cmd = v.viewport.Update(msg)
				return v, cmd
			}
		}
	}
	return v, inlineCmd
}

func (v *LogsView) View() string {
	// Non-split fallback.
	if !v.Split.Enabled() {
		if v.HasInline() {
			return v.ViewInline()
		}
		if v.step == logsServiceSelect {
			return tui.RenderSubMenuBody(v.serviceMenu.View(), v.Model.ContentWidth())
		}
		if v.step == logsResult && v.viewportReady {
			return v.renderViewport()
		}
		return tui.RenderSubMenuBody(v.Menu.View(), v.Model.ContentWidth())
	}

	// Split mode: main menu step has no split content.
	if v.step == logsMenu {
		return tui.RenderSubMenuBody(v.Menu.View(), v.Model.ContentWidth())
	}

	// LEFT: always main menu.
	menuContent := v.Menu.View()

	// RIGHT: viewport content, spinner, service sub-menu, or empty.
	var detailContent string
	if v.step == logsResult && v.viewportReady {
		detailContent = v.renderViewport()
	} else if v.HasInline() {
		tui.InSplitPanel = true
		detailContent = v.ViewInline()
		tui.InSplitPanel = false
	} else if v.step == logsServiceSelect {
		detailContent = v.serviceMenu.View()
	} else {
		detailContent = ""
	}

	return v.Split.View(menuContent, detailContent)
}

func (v *LogsView) renderViewport() string {
	if !v.viewportReady {
		return "加载中..."
	}
	return v.viewport.View()
}

// triggerMenuAction executes the action for the given main menu item ID.
func (v *LogsView) triggerMenuAction(id string) tea.Cmd {
	v.Menu = v.Menu.SetActiveID(id)
	switch id {
	case "watchdog":
		v.step = logsResult
		v.viewportReady = false
		return tea.Batch(
			v.SetInline(components.NewSpinner("加载日志...")),
			func() tea.Msg { return v.readWatchdogLog() },
		)
	case "service":
		v.step = logsServiceSelect
		v.serviceMenu = v.buildServiceMenu()
		// Focus stays on right (set by caller)
		return nil
	}
	return nil
}

type logsContentMsg struct{ content string }

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
	inv := derived.Inventory(v.Model.Store())
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
	if v.Model.Store().SnellConf != nil && !seen["snell-v5"] {
		items = append(items, tui.MenuItem{Key: key, Label: "snell-v5", ID: "snell-v5"})
		key++
	}

	// Add shadow-tls if outbound exists.
	for _, raw := range v.Model.Store().SingBox.Outbounds {
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

// readWatchdogLog reads watchdog log from file or journalctl.
func (v *LogsView) readWatchdogLog() tea.Msg {
	content, source := logs.ReadLog(config.WatchdogLog, "proxy-watchdog", 30)
	title := "Watchdog 日志"
	if source != "" {
		title += " (" + source + ")"
	}
	return logsContentMsg{content: title + "\n\n" + colorizeLogOutput(content)}
}

// readServiceLog reads a service log with file/journalctl fallback.
func (v *LogsView) readServiceLog(svc string) tea.Msg {
	logFile, unit := logs.ServiceLogSource(svc)
	content, source := logs.ReadLog(logFile, unit, 30)
	title := svc + " 日志"
	if source != "" {
		title += " (" + source + ")"
	}
	return logsContentMsg{content: title + "\n\n" + colorizeLogOutput(content)}
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
