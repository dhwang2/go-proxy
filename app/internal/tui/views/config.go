package views

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-proxy/internal/config"
	"go-proxy/internal/service"
	"go-proxy/internal/store"
	"go-proxy/internal/tui"
)

type configStep int

const (
	configMenu configStep = iota
	configViewport
)

type ConfigView struct {
	tui.InlineState
	model    *tui.Model
	menu     tui.MenuModel
	split    tui.SubSplitModel
	viewport viewport.Model
	step     configStep
	ready    bool
}

func NewConfigView(model *tui.Model) *ConfigView {
	v := &ConfigView{model: model}
	v.menu = tui.NewMenu("", []tui.MenuItem{
		{Key: '1', Label: "󰈔 sing-box", ID: "singbox"},
		{Key: '2', Label: "󰈔 snell-v5", ID: "snell"},
		{Key: '3', Label: "󰈔 shadow-tls", ID: "shadowtls"},
	})
	return v
}

func (v *ConfigView) Name() string { return "config" }

func (v *ConfigView) setFocus(left bool) {
	v.split.SetFocusLeft(left)
	v.menu = v.menu.SetDim(!left)
}

func (v *ConfigView) Init() tea.Cmd {
	v.step = configMenu
	v.ready = false
	v.split.SetFocusLeft(true)
	v.split.SetSize(v.model.ContentWidth(), v.model.Height()-5)
	return nil
}

func (v *ConfigView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.ViewResizeMsg:
		v.split.SetSize(msg.ContentWidth, msg.ContentHeight-5)
		return v, nil
	case tui.SubSplitMouseMsg:
		// Handle mouse wheel scrolling for viewport
		if v.step == configViewport && v.ready {
			if msg.Button == tea.MouseButtonWheelUp {
				v.viewport.LineUp(3)
				return v, nil
			}
			if msg.Button == tea.MouseButtonWheelDown {
				v.viewport.LineDown(3)
				return v, nil
			}
		}
		var cmd tea.Cmd
		v.split, cmd = v.split.Update(msg.MouseMsg)
		return v, cmd
	}
	// In split mode, intercept up/down for menu navigation even when content is showing.
	if v.split.Enabled() && v.step != configMenu && v.split.FocusLeft() {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			if keyMsg.Type == tea.KeyUp || keyMsg.Type == tea.KeyDown {
				var cmd tea.Cmd
				v.menu, cmd = v.menu.Update(msg)
				return v, cmd
			}
		}
	}
	inlineCmd, handled := v.UpdateInline(msg)
	if handled {
		return v, inlineCmd
	}
	switch msg := msg.(type) {
	case tui.MenuCursorChangeMsg:
		return v, nil
	case tui.MenuSelectMsg:
		v.setFocus(false)
		return v, v.triggerMenuAction(msg.ID)

	case configContentMsg:
		w := v.model.ContentWidth()
		h := v.model.Height() - 5
		if v.split.Enabled() {
			w = v.split.RightWidth()
			h = v.split.TotalHeight()
		}
		v.viewport = viewport.New(w, h)
		v.viewport.SetContent(msg.content)
		v.ready = true
		return v, nil

	default:
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.Type == tea.KeyEsc {
			if v.step == configViewport {
				v.step = configMenu
				v.ready = false
				v.setFocus(true)
				return v, nil
			}
			return v, tui.BackCmd
		}
		// Left/Right arrow toggles sub-split focus.
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			if v.split.Enabled() && v.step != configMenu {
				if keyMsg.Type == tea.KeyLeft {
					v.setFocus(true)
					return v, nil
				}
				if keyMsg.Type == tea.KeyRight && v.HasInline() {
					v.setFocus(false)
					return v, nil
				}
			}
		}
		if v.step == configMenu {
			var cmd tea.Cmd
			v.menu, cmd = v.menu.Update(msg)
			return v, cmd
		}
		if v.step == configViewport && v.ready {
			var cmd tea.Cmd
			v.viewport, cmd = v.viewport.Update(msg)
			return v, cmd
		}
	}
	return v, inlineCmd
}

func (v *ConfigView) IsSubSplitRightFocused() bool {
	return v.split.Enabled() && !v.split.FocusLeft()
}

func (v *ConfigView) View() string {
	if v.step == configMenu || !v.split.Enabled() {
		if v.HasInline() {
			return v.ViewInline()
		}
		if v.step == configMenu {
			return tui.RenderSubMenuBody(v.menu.View(), v.model.ContentWidth())
		}
		return v.renderViewport()
	}

	menuContent := v.menu.View()
	detailContent := v.renderViewport()

	return v.split.View(menuContent, detailContent)
}

func (v *ConfigView) renderViewport() string {
	if !v.ready {
		return "加载中..."
	}
	return v.viewport.View()
}

// triggerMenuAction executes the action for the given menu item ID.
func (v *ConfigView) triggerMenuAction(id string) tea.Cmd {
	switch id {
	case "singbox":
		v.step = configViewport
		v.ready = false
		content := v.renderSingBox()
		return func() tea.Msg { return configContentMsg{content: content} }
	case "snell":
		v.step = configViewport
		v.ready = false
		content := v.renderSnell()
		return func() tea.Msg { return configContentMsg{content: content} }
	case "shadowtls":
		v.step = configViewport
		v.ready = false
		content := v.renderShadowTLS()
		return func() tea.Msg { return configContentMsg{content: content} }
	}
	return nil
}

type configContentMsg struct{ content string }

func (v *ConfigView) renderSingBox() string {
	s := v.model.Store()
	var sb strings.Builder

	titleStyle := lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(tui.ColorLabel).Bold(true)
	valStyle := lipgloss.NewStyle().Foreground(tui.ColorValSys)
	sepStyle := lipgloss.NewStyle().Foreground(tui.ColorMuted)
	sep := sepStyle.Render(strings.Repeat("─", 42))

	sb.WriteString(titleStyle.Render("sing-box 配置"))
	sb.WriteString("\n")
	sb.WriteString(sep)
	sb.WriteString("\n")

	// 入站数量
	sb.WriteString(fmt.Sprintf("%-12s %s\n",
		labelStyle.Render("入站数量:"),
		valStyle.Render(fmt.Sprintf("%d", len(s.SingBox.Inbounds)))))

	// 全局出口
	globalFinal := "🐸 direct"
	if s.SingBox.Route != nil && s.SingBox.Route.Final != "" {
		globalFinal = s.SingBox.Route.Final
	}
	sb.WriteString(fmt.Sprintf("%-12s %s\n",
		labelStyle.Render("全局出口:"),
		valStyle.Render(globalFinal)))

	// dns策略 / dns出口
	if s.SingBox.DNS != nil {
		sb.WriteString(fmt.Sprintf("%-12s %s\n",
			labelStyle.Render("dns策略:"),
			valStyle.Render(s.SingBox.DNS.Strategy)))
		sb.WriteString(fmt.Sprintf("%-12s %s\n",
			labelStyle.Render("dns出口:"),
			valStyle.Render(s.SingBox.DNS.Final)))
	}

	sb.WriteString(sep)
	sb.WriteString("\n")
	sb.WriteString(renderOrderedSingBoxJSON(s.SingBox))

	return sb.String()
}

func renderOrderedSingBoxJSON(c *store.SingBoxConfig) string {
	type entry struct {
		key  string
		data json.RawMessage
	}
	var entries []entry

	add := func(key string, v interface{}) {
		data, err := json.MarshalIndent(v, "  ", "  ")
		if err != nil {
			return
		}
		entries = append(entries, entry{key, data})
	}

	if c.Log != nil {
		add("log", c.Log)
	}
	if len(c.Experimental) > 0 {
		add("experimental", c.Experimental)
	}
	if c.DNS != nil {
		add("dns", c.DNS)
	}
	if len(c.Inbounds) > 0 {
		add("inbounds", c.Inbounds)
	}
	if len(c.Outbounds) > 0 {
		add("outbounds", c.Outbounds)
	}
	if c.Route != nil {
		add("route", c.Route)
	}

	var parts []string
	for _, e := range entries {
		parts = append(parts, fmt.Sprintf("  %q: %s", e.key, string(e.data)))
	}
	return "{\n" + strings.Join(parts, ",\n") + "\n}"
}

func (v *ConfigView) renderSnell() string {
	conf := v.model.Store().SnellConf
	if conf == nil {
		return "  snell-v5 未安装\n\n  配置文件: " + config.SnellConfigFile
	}

	titleStyle := lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(tui.ColorLabel).Bold(true)
	valStyle := lipgloss.NewStyle().Foreground(tui.ColorValSys)
	sepStyle := lipgloss.NewStyle().Foreground(tui.ColorMuted)

	var sb strings.Builder
	sb.WriteString(titleStyle.Render("  snell-v5 配置"))
	sb.WriteString("\n\n")
	sb.WriteString(labelStyle.Render("  概览"))
	sb.WriteString("\n")
	sb.WriteString(sepStyle.Render("  " + strings.Repeat("─", 40)))
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  %-12s %s\n",
		labelStyle.Render("监听地址:"),
		valStyle.Render(conf.Listen)))
	sb.WriteString(fmt.Sprintf("  %-12s %s\n",
		labelStyle.Render("PSK:"),
		valStyle.Render(conf.PSK)))
	sb.WriteString(fmt.Sprintf("  %-12s %s\n",
		labelStyle.Render("配置路径:"),
		valStyle.Render(config.SnellConfigFile)))

	// Service status
	status := checkServiceActive("snell-v5")
	sb.WriteString(fmt.Sprintf("  %-12s %s\n",
		labelStyle.Render("服务状态:"),
		status))

	sb.WriteString("\n")
	sb.WriteString(labelStyle.Render("  原始配置"))
	sb.WriteString("\n")
	sb.WriteString(sepStyle.Render("  " + strings.Repeat("─", 40)))
	sb.WriteString("\n")
	sb.WriteString(string(conf.MarshalSnellConfig()))

	return sb.String()
}

func (v *ConfigView) renderShadowTLS() string {
	titleStyle := lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(tui.ColorLabel).Bold(true)
	valStyle := lipgloss.NewStyle().Foreground(tui.ColorValSys)
	sepStyle := lipgloss.NewStyle().Foreground(tui.ColorMuted)

	var sb strings.Builder
	sb.WriteString(titleStyle.Render("  shadow-tls 配置"))
	sb.WriteString("\n\n")
	found := false

	for _, raw := range v.model.Store().SingBox.Outbounds {
		h, err := store.ParseOutboundHeader(raw)
		if err != nil || h.Type != "shadowtls" {
			continue
		}
		found = true

		var ob struct {
			Type       string `json:"type"`
			Tag        string `json:"tag"`
			Server     string `json:"server"`
			ServerPort int    `json:"server_port"`
			Version    int    `json:"version"`
			Password   string `json:"password"`
			TLS        *struct {
				Enabled    bool   `json:"enabled"`
				ServerName string `json:"server_name"`
			} `json:"tls,omitempty"`
		}
		if err := json.Unmarshal(raw, &ob); err != nil {
			sb.WriteString(fmt.Sprintf("  %s: 解析失败\n\n", h.Tag))
			continue
		}

		sb.WriteString(labelStyle.Render(fmt.Sprintf("  实例: %s", ob.Tag)))
		sb.WriteString("\n")
		sb.WriteString(sepStyle.Render("  " + strings.Repeat("─", 40)))
		sb.WriteString("\n")

		sb.WriteString(fmt.Sprintf("  %-12s %s\n",
			labelStyle.Render("地址:"),
			valStyle.Render(fmt.Sprintf("%s:%d", ob.Server, ob.ServerPort))))

		if ob.Version > 0 {
			sb.WriteString(fmt.Sprintf("  %-12s %s\n",
				labelStyle.Render("版本:"),
				valStyle.Render(fmt.Sprintf("v%d", ob.Version))))
		}

		if ob.TLS != nil && ob.TLS.ServerName != "" {
			sb.WriteString(fmt.Sprintf("  %-12s %s\n",
				labelStyle.Render("SNI:"),
				valStyle.Render(ob.TLS.ServerName)))
		}

		sb.WriteString("\n")
	}

	// Service status
	status := checkServiceActive("shadow-tls")
	sb.WriteString(labelStyle.Render("  服务状态"))
	sb.WriteString("\n")
	sb.WriteString(sepStyle.Render("  " + strings.Repeat("─", 40)))
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  %-12s %s\n",
		labelStyle.Render("shadow-tls:"),
		status))
	sb.WriteString(fmt.Sprintf("  %-12s %s\n",
		labelStyle.Render("二进制:"),
		valStyle.Render(config.ShadowTLSBin)))

	if !found {
		return "  无 shadow-tls outbound 配置\n\n" + sb.String()
	}

	return sb.String()
}

func checkServiceActive(name string) string {
	greenStyle := lipgloss.NewStyle().Foreground(tui.ColorSuccess).Bold(true)
	redStyle := lipgloss.NewStyle().Foreground(tui.ColorError).Bold(true)
	grayStyle := lipgloss.NewStyle().Foreground(tui.ColorMuted)

	ctx := context.Background()
	st, _ := service.GetStatus(ctx, service.Name(name))
	if st == nil {
		return grayStyle.Render("● 未安装")
	}
	if st.Running {
		return greenStyle.Render("● 运行中")
	}
	return redStyle.Render("● 已停止")
}
