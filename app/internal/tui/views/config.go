package views

import (
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
	tui.SplitViewBase
	viewport viewport.Model
	step     configStep
	ready    bool
}

func NewConfigView(model *tui.Model) *ConfigView {
	v := &ConfigView{}
	v.Model = model
	v.Menu = tui.NewMenu("", []tui.MenuItem{
		{Key: '1', Label: "󰈔 sing-box", ID: "singbox"},
		{Key: '2', Label: "󰈔 snell-v5", ID: "snell"},
		{Key: '3', Label: "󰈔 shadow-tls", ID: "shadowtls"},
	})
	return v
}

func (v *ConfigView) Name() string { return "config" }

func (v *ConfigView) Init() tea.Cmd {
	v.step = configMenu
	v.ready = false
	v.InitSplit()
	return nil
}

func (v *ConfigView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.ViewResizeMsg:
		v.HandleResize(msg)
		return v, nil
	case tui.SubSplitMouseMsg:
		// Handle mouse wheel scrolling for viewport before delegating to split.
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
		return v, v.HandleMouse(msg)
	}
	// In split mode, intercept up/down for menu navigation even when content is showing.
	if cmd, ok := v.HandleMenuNav(msg, v.step == configMenu, false); ok {
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
		v.SetFocus(false)
		return v, v.triggerMenuAction(msg.ID)

	case configContentMsg:
		w := v.Model.ContentWidth()
		h := v.Model.Height() - 5
		if v.Split.Enabled() {
			w = v.Split.RightWidth()
			h = v.Split.TotalHeight()
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
				v.SetFocus(true)
				return v, nil
			}
			return v, tui.BackCmd
		}
		// Left/Right arrow toggles sub-split focus.
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			if v.HandleSplitArrows(keyMsg, v.step == configMenu, v.HasInline()) {
				return v, nil
			}
		}
		if v.step == configMenu {
			var cmd tea.Cmd
			v.Menu, cmd = v.Menu.Update(msg)
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

func (v *ConfigView) View() string {
	if v.step == configMenu || !v.Split.Enabled() {
		if v.HasInline() {
			return v.ViewInline()
		}
		if v.step == configMenu {
			return tui.RenderSubMenuBody(v.Menu.View(), v.Model.ContentWidth())
		}
		return v.renderViewport()
	}

	menuContent := v.Menu.View()
	detailContent := v.renderViewport()

	return v.Split.View(menuContent, detailContent)
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
	s := v.Model.Store()
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
	conf := v.Model.Store().SnellConf
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

	return sb.String()
}

func (v *ConfigView) renderShadowTLS() string {
	bindings, err := service.ListShadowTLSBindings(v.Model.Store())
	if err != nil {
		return "  读取 shadow-tls 配置失败\n\n  " + err.Error()
	}
	if len(bindings) == 0 {
		return "  无 shadow-tls 配置"
	}

	titleStyle := lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(tui.ColorLabel).Bold(true)
	valStyle := lipgloss.NewStyle().Foreground(tui.ColorValSys)

	var sb strings.Builder
	sb.WriteString(titleStyle.Render("  shadow-tls 配置"))
	sb.WriteString("\n\n")
	for i, binding := range bindings {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("  %-12s %s\n", labelStyle.Render("实例:"), valStyle.Render(binding.ServiceName)))
		sb.WriteString(fmt.Sprintf("  %-12s %s\n", labelStyle.Render("监听端口:"), valStyle.Render(fmt.Sprintf("%d", binding.ListenPort))))
		sb.WriteString(fmt.Sprintf("  %-12s %s\n", labelStyle.Render("后端类型:"), valStyle.Render(binding.BackendProto)))
		sb.WriteString(fmt.Sprintf("  %-12s %s\n", labelStyle.Render("后端端口:"), valStyle.Render(fmt.Sprintf("%d", binding.BackendPort))))
		sb.WriteString(fmt.Sprintf("  %-12s %s\n", labelStyle.Render("SNI:"), valStyle.Render(binding.SNI)))
		sb.WriteString(fmt.Sprintf("  %-12s %s\n", labelStyle.Render("密码:"), valStyle.Render(binding.Password)))
	}

	return sb.String()
}
