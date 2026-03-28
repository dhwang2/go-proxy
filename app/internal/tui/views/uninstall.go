package views

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"go-proxy/internal/config"
	"go-proxy/internal/service"
	"go-proxy/internal/tui"
	"go-proxy/internal/tui/components"
)

type UninstallView struct {
	tui.InlineState
	model         *tui.Model
	step          uninstallStep
	confirmPrompt string
	tableHeader   string
	previewBody   string
	viewport      viewport.Model
	ready         bool
}

type uninstallStep int

const (
	uninstallConfirm uninstallStep = iota
	uninstallResult
)

func NewUninstallView(model *tui.Model) *UninstallView {
	return &UninstallView{model: model}
}

func (v *UninstallView) Name() string { return "uninstall" }

func (v *UninstallView) Init() tea.Cmd {
	v.ClearInline()
	v.step = uninstallConfirm
	v.confirmPrompt = "确认卸载以上内容?"
	width := v.model.ContentWidth()
	v.rebuildPreview(width)
	height := v.availableViewportHeight(v.model.Height() - 2)
	if height < 1 {
		height = 1
	}
	v.viewport = viewport.New(width, height)
	v.viewport.SetContent(v.previewBody)
	v.ready = true
	return nil
}

func (v *UninstallView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.ViewResizeMsg:
		height := v.availableViewportHeight(msg.ContentHeight)
		if height < 1 {
			height = 1
		}
		if !v.ready {
			v.viewport = viewport.New(msg.ContentWidth, height)
			v.ready = true
		}
		v.rebuildPreview(msg.ContentWidth)
		v.viewport.Width = msg.ContentWidth
		v.viewport.Height = height
		v.viewport.SetContent(wrapPanelContent(v.previewBody, msg.ContentWidth))
		return v, nil
	case tui.SubSplitMouseMsg:
		if v.step == uninstallConfirm && v.ready {
			if msg.Button == tea.MouseButtonWheelUp {
				v.viewport.LineUp(3)
				return v, nil
			}
			if msg.Button == tea.MouseButtonWheelDown {
				v.viewport.LineDown(3)
				return v, nil
			}
		}
		return v, nil
	}

	inlineCmd, handled := v.UpdateInline(msg)
	if handled {
		return v, inlineCmd
	}
	switch msg := msg.(type) {
	case uninstallDoneMsg:
		v.step = uninstallResult
		if msg.success {
			v.model.SetExitMessage("卸载完成，所有服务和配置已移除")
		}
		return v, v.SetInline(components.NewResult(msg.result))

	case tui.ResultDismissedMsg:
		return v, tea.Quit
	case tea.KeyMsg:
		if v.step == uninstallConfirm {
			switch {
			case msg.Type == tea.KeyEnter:
				initCmd := v.SetInline(components.NewSpinner("正在卸载..."))
				return v, tea.Batch(initCmd, v.doUninstall)
			case msg.Type == tea.KeyEsc:
				return v, tui.BackCmd
			}
			if v.ready {
				var cmd tea.Cmd
				v.viewport, cmd = v.viewport.Update(msg)
				return v, cmd
			}
			return v, nil
		}
		if msg.Type == tea.KeyEsc {
			return v, tui.BackCmd
		}
	}
	return v, inlineCmd
}

func (v *UninstallView) View() string {
	if v.HasInline() {
		return v.ViewInline()
	}
	if v.step == uninstallConfirm {
		header := "  将卸载以下服务、文件和配置 | " + v.confirmPrompt
		dividerWidth := max(12, v.viewport.Width-2)
		divider := "  " + strings.Repeat("─", dividerWidth)
		if !v.ready {
			return header + "\n" + divider + "\n" + v.tableHeader + "\n" + v.previewBody
		}
		return header + "\n" + divider + "\n" + v.tableHeader + "\n" + v.viewport.View()
	}
	return ""
}

func (v *UninstallView) availableViewportHeight(contentHeight int) int {
	// Right panel already spends 3 lines on title/breadcrumb/separator.
	// This view spends 4 lines on fixed header, divider, and table header.
	return contentHeight - 7
}

func (v *UninstallView) rebuildPreview(width int) {
	sections := v.renderPreview(width)
	v.tableHeader = sections.Header
	v.previewBody = sections.Body
}

func (v *UninstallView) renderPreview(width int) tableSections {
	rows := v.previewRows()
	return renderTableSections(
		[]string{"类型", "名称", "路径"},
		rows,
		width,
		true,
	)
}

func (v *UninstallView) previewRows() [][]string {
	rows := [][]string{
		{"服务", "sing-box", config.SingBoxService},
		{"服务", "snell-v5", config.SnellService},
		{"服务", "shadow-tls", config.ShadowTLSService},
		{"服务", "caddy-sub", config.CaddySubService},
		{"服务", "watchdog", config.WatchdogService},
		{"二进制", "sing-box", config.SingBoxBin},
		{"二进制", "snell-server", config.SnellBin},
		{"二进制", "shadow-tls", config.ShadowTLSBin},
		{"二进制", "caddy", config.CaddyBin},
		{"配置", "sing-box.json", config.SingBoxConfig},
		{"配置", "snell-v5.conf", config.SnellConfigFile},
		{"配置", "user-management.json", config.UserMetaFile},
		{"配置", "user-route-rules.json", config.UserRouteFile},
		{"配置", "user-route-templates.json", config.UserTemplateFile},
		{"配置", "firewall-ports.json", config.FirewallConfigFile},
		{"配置", "subscription.txt", config.SubscriptionFile},
		{"配置", "Caddyfile", config.CaddyFile},
		{"配置", ".domain", config.DomainFile},
		{"日志", "sing-box.service.log", config.SingBoxLog},
		{"日志", "snell-v5.service.log", config.SnellLog},
		{"日志", "shadow-tls.service.log", config.ShadowTLSLog},
		{"日志", "caddy-sub.service.log", config.CaddySubLog},
		{"日志", "proxy-watchdog.log", config.WatchdogLog},
		{"目录", "工作目录", config.WorkDir},
		{"目录", "二进制目录", config.BinDir},
		{"目录", "配置目录", config.ConfDir},
		{"目录", "日志目录", config.LogDir},
		{"目录", "证书目录", config.CaddyCertDir},
	}

	if execPath, err := os.Executable(); err == nil {
		rows = append([][]string{{"程序", filepath.Base(execPath), execPath}}, rows...)
	}

	if serviceNames, err := service.ShadowTLSServiceNames(); err == nil {
		unitDir := filepath.Dir(config.ShadowTLSService)
		for _, name := range serviceNames {
			rows = append(rows, []string{"服务", name, filepath.Join(unitDir, name+".service")})
			rows = append(rows, []string{"日志", name + ".service.log", filepath.Join(config.LogDir, name+".service.log")})
		}
	}

	return rows
}

type uninstallDoneMsg struct {
	result  string
	success bool
}

func (v *UninstallView) doUninstall() tea.Msg {
	ctx := context.Background()
	var errs []string

	// Stop and disable all services.
	if err := service.Uninstall(ctx); err != nil {
		errs = append(errs, err.Error())
	}

	// Remove the proxy binary itself.
	if execPath, err := os.Executable(); err == nil {
		if err := os.Remove(execPath); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Sprintf("remove binary: %v", err))
		}
	}

	if len(errs) > 0 {
		return uninstallDoneMsg{
			result:  "卸载完成（部分错误）:\n" + strings.Join(errs, "\n"),
			success: true,
		}
	}
	return uninstallDoneMsg{result: "卸载完成，所有服务和配置已移除", success: true}
}
