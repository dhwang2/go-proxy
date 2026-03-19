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
	"go-proxy/internal/derived"
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
	title    string
}

func NewConfigView(model *tui.Model) *ConfigView {
	v := &ConfigView{model: model}
	v.menu = tui.NewMenu("󰈔 配置详情", []tui.MenuItem{
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
	v.split.SetFocusLeft(true)
	v.split.SetSize(v.model.ContentWidth(), v.model.Height()-6)
	return nil
}

func (v *ConfigView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
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
	case tea.WindowSizeMsg:
		if v.step == configViewport {
			headerHeight := 2
			footerHeight := 1
			if !v.ready {
				v.viewport = viewport.New(msg.Width, msg.Height-headerHeight-footerHeight)
				v.ready = true
			} else {
				v.viewport.Width = msg.Width
				v.viewport.Height = msg.Height - headerHeight - footerHeight
			}
		}
		return v, nil

	case tui.MenuSelectMsg:
		v.split.SetFocusLeft(false)
		switch msg.ID {
		case "singbox":
			v.step = configViewport
			v.title = "sing-box 配置"
			v.ready = false
			content := v.renderSingBox()
			return v, func() tea.Msg {
				return configContentMsg{content: content}
			}
		case "snell":
			v.step = configViewport
			v.title = "snell-v5 配置"
			v.ready = false
			content := v.renderSnell()
			return v, func() tea.Msg {
				return configContentMsg{content: content}
			}
		case "shadowtls":
			v.step = configViewport
			v.title = "shadow-tls 配置"
			v.ready = false
			content := v.renderShadowTLS()
			return v, func() tea.Msg {
				return configContentMsg{content: content}
			}
		}

	case configContentMsg:
		v.viewport = viewport.New(v.model.Width(), v.model.Height()-3)
		v.viewport.SetContent(msg.content)
		v.ready = true
		return v, nil

	default:
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.Type == tea.KeyEsc {
			if v.step == configViewport {
				v.step = configMenu
				v.ready = false
				v.split.SetFocusLeft(true)
				return v, nil
			}
			return v, tui.BackCmd
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
	titleStyle := lipgloss.NewStyle().
		Foreground(tui.ColorPrimary).
		Bold(true).
		PaddingLeft(1)

	header := titleStyle.Render(v.title)

	footerStyle := lipgloss.NewStyle().
		Foreground(tui.ColorMuted).
		PaddingLeft(1)
	footer := footerStyle.Render("↑↓ 滚动 | esc 返回")

	if !v.ready {
		return lipgloss.JoinVertical(lipgloss.Left, header, "加载中...", footer)
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		v.viewport.View(),
		footer,
	)
}

type configContentMsg struct{ content string }

func (v *ConfigView) renderSingBox() string {
	s := v.model.Store()
	var sb strings.Builder

	labelStyle := lipgloss.NewStyle().Foreground(tui.ColorLabel).Bold(true)
	valStyle := lipgloss.NewStyle().Foreground(tui.ColorValSys)
	sepStyle := lipgloss.NewStyle().Foreground(tui.ColorMuted)

	// Summary stats
	sb.WriteString(labelStyle.Render("  概览"))
	sb.WriteString("\n")
	sb.WriteString(sepStyle.Render("  " + strings.Repeat("─", 40)))
	sb.WriteString("\n")

	// Inbound count
	sb.WriteString(fmt.Sprintf("  %-16s %s\n",
		labelStyle.Render("Inbound 数量:"),
		valStyle.Render(fmt.Sprintf("%d", len(s.SingBox.Inbounds)))))

	// Protocol types
	inv := derived.Inventory(s)
	if len(inv) > 0 {
		var types []string
		for _, info := range inv {
			label := info.Type
			if info.HasReality {
				label += "+reality"
			}
			types = append(types, fmt.Sprintf("%s/%d", label, info.Port))
		}
		sb.WriteString(fmt.Sprintf("  %-16s %s\n",
			labelStyle.Render("协议/端口:"),
			valStyle.Render(strings.Join(types, ", "))))
	}

	// Outbound count
	sb.WriteString(fmt.Sprintf("  %-16s %s\n",
		labelStyle.Render("Outbound 数量:"),
		valStyle.Render(fmt.Sprintf("%d", len(s.SingBox.Outbounds)))))

	// Global outbound types
	if len(s.SingBox.Outbounds) > 0 {
		var obTypes []string
		for _, raw := range s.SingBox.Outbounds {
			h, err := store.ParseOutboundHeader(raw)
			if err == nil {
				obTypes = append(obTypes, h.Type)
			}
		}
		if len(obTypes) > 0 {
			sb.WriteString(fmt.Sprintf("  %-16s %s\n",
				labelStyle.Render("Outbound 类型:"),
				valStyle.Render(strings.Join(obTypes, ", "))))
		}
	}

	// DNS info
	if s.SingBox.DNS != nil {
		sb.WriteString(fmt.Sprintf("  %-16s %s\n",
			labelStyle.Render("DNS 服务器:"),
			valStyle.Render(fmt.Sprintf("%d", len(s.SingBox.DNS.Servers)))))
		sb.WriteString(fmt.Sprintf("  %-16s %s\n",
			labelStyle.Render("DNS 规则:"),
			valStyle.Render(fmt.Sprintf("%d", len(s.SingBox.DNS.Rules)))))
	}

	// Route rules
	if s.SingBox.Route != nil {
		sb.WriteString(fmt.Sprintf("  %-16s %s\n",
			labelStyle.Render("路由规则:"),
			valStyle.Render(fmt.Sprintf("%d", len(s.SingBox.Route.Rules)))))
		sb.WriteString(fmt.Sprintf("  %-16s %s\n",
			labelStyle.Render("规则集:"),
			valStyle.Render(fmt.Sprintf("%d", len(s.SingBox.Route.RuleSet)))))
	}

	// User count
	names := derived.UserNames(s)
	sb.WriteString(fmt.Sprintf("  %-16s %s\n",
		labelStyle.Render("用户数量:"),
		valStyle.Render(fmt.Sprintf("%d", len(names)))))

	// Full JSON
	sb.WriteString("\n")
	sb.WriteString(labelStyle.Render("  完整配置"))
	sb.WriteString("\n")
	sb.WriteString(sepStyle.Render("  " + strings.Repeat("─", 40)))
	sb.WriteString("\n")

	data, err := json.MarshalIndent(s.SingBox, "", "  ")
	if err != nil {
		sb.WriteString("  渲染配置失败: " + err.Error())
	} else {
		sb.WriteString(string(data))
	}

	return sb.String()
}

func (v *ConfigView) renderSnell() string {
	conf := v.model.Store().SnellConf
	if conf == nil {
		return "  snell-v5 未安装\n\n  配置文件: " + config.SnellConfigFile
	}

	labelStyle := lipgloss.NewStyle().Foreground(tui.ColorLabel).Bold(true)
	valStyle := lipgloss.NewStyle().Foreground(tui.ColorValSys)
	sepStyle := lipgloss.NewStyle().Foreground(tui.ColorMuted)

	var sb strings.Builder
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
	labelStyle := lipgloss.NewStyle().Foreground(tui.ColorLabel).Bold(true)
	valStyle := lipgloss.NewStyle().Foreground(tui.ColorValSys)
	sepStyle := lipgloss.NewStyle().Foreground(tui.ColorMuted)

	var sb strings.Builder
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
