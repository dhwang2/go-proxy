package views

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-proxy/internal/store"
	"go-proxy/internal/tui"
	"go-proxy/internal/tui/components"
)

type configStep int

const (
	configMenu configStep = iota
	configViewport
)

type ConfigView struct {
	model    *tui.Model
	menu     components.MenuModel
	viewport viewport.Model
	step     configStep
	ready    bool
	title    string
}

func NewConfigView(model *tui.Model) *ConfigView {
	v := &ConfigView{model: model}
	v.menu = components.NewMenu("󰈔 配置详情", []components.MenuItem{
		{Key: '1', Label: "󰈔 sing-box", ID: "singbox"},
		{Key: '2', Label: "󰈔 snell-v5", ID: "snell"},
		{Key: '3', Label: "󰈔 shadow-tls", ID: "shadowtls"},
		{Key: '0', Label: "󰌍 返回", ID: "back"},
	})
	return v
}

func (v *ConfigView) Name() string { return "config" }

func (v *ConfigView) Init() tea.Cmd {
	v.step = configMenu
	v.ready = false
	return nil
}

func (v *ConfigView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
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

	case components.MenuSelectMsg:
		switch msg.ID {
		case "back":
			if v.step == configViewport {
				v.step = configMenu
				v.ready = false
				return v, nil
			}
			return v, tui.BackCmd
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
		if v.step == configMenu {
			var cmd tea.Cmd
			v.menu, cmd = v.menu.Update(msg)
			return v, cmd
		}
		if v.step == configViewport {
			switch msg := msg.(type) {
			case tea.KeyMsg:
				if msg.Type == tea.KeyEsc {
					v.step = configMenu
					v.ready = false
					return v, nil
				}
			}
			if v.ready {
				var cmd tea.Cmd
				v.viewport, cmd = v.viewport.Update(msg)
				return v, cmd
			}
		}
	}
	return v, nil
}

func (v *ConfigView) View() string {
	if v.step == configMenu {
		return v.menu.View()
	}

	titleStyle := lipgloss.NewStyle().
		Foreground(tui.ColorPrimary).
		Bold(true).
		PaddingLeft(1)

	header := titleStyle.Render(v.title)

	footerStyle := lipgloss.NewStyle().
		Foreground(tui.ColorMuted).
		PaddingLeft(1)
	footer := footerStyle.Render("滚动  返回")

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
	data, err := json.MarshalIndent(v.model.Store().SingBox, "", "  ")
	if err != nil {
		return "渲染配置失败: " + err.Error()
	}
	return string(data)
}

func (v *ConfigView) renderSnell() string {
	conf := v.model.Store().SnellConf
	if conf == nil {
		return "无 snell-v5 配置"
	}
	return string(conf.MarshalSnellConfig())
}

func (v *ConfigView) renderShadowTLS() string {
	var sb strings.Builder
	found := false
	for _, raw := range v.model.Store().SingBox.Outbounds {
		h, err := store.ParseOutboundHeader(raw)
		if err != nil || h.Type != "shadowtls" {
			continue
		}
		found = true
		// Parse full outbound for details.
		var ob struct {
			Type       string `json:"type"`
			Tag        string `json:"tag"`
			Server     string `json:"server"`
			ServerPort int    `json:"server_port"`
			TLS        *struct {
				ServerName string `json:"server_name"`
			} `json:"tls,omitempty"`
		}
		if err := json.Unmarshal(raw, &ob); err != nil {
			sb.WriteString(fmt.Sprintf("  %s: 解析失败\n", h.Tag))
			continue
		}
		sb.WriteString(fmt.Sprintf("  标签: %s\n", ob.Tag))
		sb.WriteString(fmt.Sprintf("  地址: %s:%d\n", ob.Server, ob.ServerPort))
		if ob.TLS != nil && ob.TLS.ServerName != "" {
			sb.WriteString(fmt.Sprintf("  SNI:  %s\n", ob.TLS.ServerName))
		}
		sb.WriteString("\n")
	}
	if !found {
		return "无 shadow-tls 配置"
	}
	return sb.String()
}
