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
	viewport   viewport.Model
	step       configStep
	ready      bool
	selectedID string
	rawContent string
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
		if v.ready {
			v.rebuildViewport(msg.ContentWidth, msg.ContentHeight)
		}
		return v, nil
	case tui.SubSplitResizeMsg:
		if v.ready {
			v.rebuildViewport(msg.RightWidth, msg.RightHeight+3)
		}
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
		v.selectedID = msg.id
		w, h := v.viewportSize()
		v.viewport = viewport.New(w, h)
		v.rawContent = v.renderSelectedContent(w)
		v.viewport.SetContent(v.rawContent)
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
			return v.Menu.View()
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
		return func() tea.Msg { return configContentMsg{id: id} }
	case "snell":
		v.step = configViewport
		v.ready = false
		return func() tea.Msg { return configContentMsg{id: id} }
	case "shadowtls":
		v.step = configViewport
		v.ready = false
		return func() tea.Msg { return configContentMsg{id: id} }
	}
	return nil
}

type configContentMsg struct{ id string }

func (v *ConfigView) viewportSize() (int, int) {
	w := v.Model.ContentWidth()
	h := v.Model.Height() - 5
	if v.Split.Enabled() {
		w = v.Split.RightWidth()
		h = v.Split.TotalHeight()
	}
	if h < 1 {
		h = 1
	}
	return w, h
}

func (v *ConfigView) rebuildViewport(contentWidth, contentHeight int) {
	w := contentWidth
	h := contentHeight - 5
	if v.Split.Enabled() {
		w = v.Split.RightWidth()
		h = v.Split.TotalHeight()
	}
	if h < 1 {
		h = 1
	}
	v.viewport.Width = w
	v.viewport.Height = h
	v.rawContent = v.renderSelectedContent(w)
	v.viewport.SetContent(v.rawContent)
}

func (v *ConfigView) renderSelectedContent(width int) string {
	switch v.selectedID {
	case "singbox":
		return v.renderSingBox(width)
	case "snell":
		return v.renderSnell(width)
	case "shadowtls":
		return v.renderShadowTLS(width)
	default:
		return ""
	}
}

func (v *ConfigView) renderSingBox(width int) string {
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

	return wrapPanelContent(sb.String(), width)
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

func (v *ConfigView) renderSnell(width int) string {
	conf := v.Model.Store().SnellConf
	if conf == nil {
		return "  snell-v5 未安装\n\n  配置文件: " + config.SnellConfigFile
	}

	type kv struct{ k, v string }
	boolStr := func(b bool) string {
		if b {
			return "true"
		}
		return "false"
	}
	rows := []kv{
		{"监听地址", conf.Listen},
		{"PSK", conf.PSK},
		{"IPv6", boolStr(conf.IPv6)},
		{"UDP", boolStr(conf.UDP)},
		{"Obfs", conf.Obfs},
		{"配置路径", config.SnellConfigFile},
	}
	tableRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		tableRows = append(tableRows, []string{r.k, r.v})
	}

	var sb strings.Builder
	sb.WriteString(lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true).Render("  snell-v5 配置"))
	sb.WriteString("\n\n")
	sb.WriteString(renderTable([]string{"属性", "值"}, tableRows, width, false))
	return sb.String()
}

func (v *ConfigView) renderShadowTLS(width int) string {
	bindings, err := service.ListShadowTLSBindings(v.Model.Store())
	if err != nil {
		return "  读取 shadow-tls 配置失败\n\n  " + err.Error()
	}
	if len(bindings) == 0 {
		return "  无 shadow-tls 配置"
	}

	headers := []string{"#", "实例", "监听端口", "后端类型", "后端端口", "SNI", "密码"}
	rows := make([][]string, 0, len(bindings))
	for i, b := range bindings {
		rows = append(rows, []string{
			fmt.Sprintf("%d", i+1),
			b.ServiceName,
			fmt.Sprintf("%d", b.ListenPort),
			b.BackendProto,
			fmt.Sprintf("%d", b.BackendPort),
			b.SNI,
			b.Password,
		})
	}

	var sb strings.Builder
	sb.WriteString(lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true).Render("  shadow-tls 配置"))
	sb.WriteString("\n\n")
	sb.WriteString(renderTable(headers, rows, width, false))
	return sb.String()
}
