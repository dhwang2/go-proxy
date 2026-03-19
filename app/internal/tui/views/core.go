package views

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"go-proxy/internal/config"
	"go-proxy/internal/core"
	"go-proxy/internal/tui"
	"go-proxy/internal/tui/components"
)

type coreStep int

const (
	coreMenu coreStep = iota
	coreWorking
	coreConfirm
	coreResult
)

type CoreView struct {
	tui.InlineState
	model   *tui.Model
	menu    tui.MenuModel
	step    coreStep
	pending []core.UpdateCheck
}

func NewCoreView(model *tui.Model) *CoreView {
	v := &CoreView{model: model}
	v.menu = tui.NewMenu("󰚗 内核管理", []tui.MenuItem{
		{Key: '1', Label: "󰋼 查看版本", ID: "versions"},
		{Key: '2', Label: "󰁪 检查更新", ID: "check"},
		{Key: '3', Label: "󰏗 执行更新", ID: "update"},
	})
	return v
}

func (v *CoreView) Name() string { return "core" }

func (v *CoreView) Init() tea.Cmd {
	v.step = coreMenu
	v.pending = nil
	return nil
}

func (v *CoreView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	inlineCmd, handled := v.UpdateInline(msg)
	if handled {
		return v, inlineCmd
	}
	switch msg := msg.(type) {
	case tui.MenuSelectMsg:
		switch msg.ID {
		case "versions":
			v.step = coreWorking
			return v, tea.Batch(
				v.SetInline(components.NewSpinner("正在检测版本...")),
				v.doVersions,
			)
		case "check":
			v.step = coreWorking
			return v, tea.Batch(
				v.SetInline(components.NewSpinner("正在检查更新...")),
				func() tea.Msg { return v.doCheckUpdates(false) },
			)
		case "update":
			v.step = coreWorking
			return v, tea.Batch(
				v.SetInline(components.NewSpinner("正在检查更新...")),
				func() tea.Msg { return v.doCheckUpdates(true) },
			)
		}

	case coreVersionsDoneMsg:
		v.step = coreResult
		return v, v.SetInline(components.NewResult(msg.result))

	case coreCheckDoneMsg:
		if msg.forUpdate && len(msg.updates) > 0 {
			v.pending = msg.updates
			v.step = coreConfirm
			return v, v.SetInline(components.NewConfirm(msg.result + "\n\n是否执行更新？"))
		}
		v.step = coreResult
		return v, v.SetInline(components.NewResult(msg.result))

	case tui.ConfirmResultMsg:
		if msg.Confirmed && len(v.pending) > 0 {
			updates := v.pending
			v.pending = nil
			v.step = coreWorking
			return v, tea.Batch(
				v.SetInline(components.NewSpinner("正在下载更新...")),
				func() tea.Msg { return v.doUpdate(updates) },
			)
		}
		v.step = coreMenu
		return v, nil

	case coreUpdateDoneMsg:
		v.step = coreResult
		return v, v.SetInline(components.NewResult(msg.result))

	case tui.ResultDismissedMsg:
		v.step = coreMenu
		return v, nil

	default:
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.Type == tea.KeyEsc {
			return v, tui.BackCmd
		}
		if v.step == coreMenu {
			var cmd tea.Cmd
			v.menu, cmd = v.menu.Update(msg)
			return v, cmd
		}
	}
	return v, inlineCmd
}

func (v *CoreView) View() string {
	if v.HasInline() {
		return v.ViewInline()
	}
	return tui.RenderSubMenuBody(v.menu.View(), v.model.ContentWidth())
}

type coreVersionsDoneMsg struct{ result string }
type coreCheckDoneMsg struct {
	result    string
	updates   []core.UpdateCheck
	forUpdate bool
}
type coreUpdateDoneMsg struct{ result string }

func binPath(comp core.Component) string {
	switch comp {
	case core.CompSingBox:
		return config.SingBoxBin
	case core.CompSnell:
		return config.SnellBin
	case core.CompShadowTLS:
		return config.ShadowTLSBin
	case core.CompCaddy:
		return config.CaddyBin
	default:
		return ""
	}
}

func (v *CoreView) doVersions() tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var sb strings.Builder
	sb.WriteString("组件版本\n\n")
	for _, comp := range core.AllComponents() {
		bp := binPath(comp)
		if bp == "" {
			sb.WriteString(fmt.Sprintf("  %s: 未配置\n", comp))
			continue
		}
		info := core.DetectVersion(ctx, bp, comp)
		if info.Installed {
			sb.WriteString(fmt.Sprintf("  %s: %s\n", comp, info.Version))
		} else {
			sb.WriteString(fmt.Sprintf("  %s: 未安装\n", comp))
		}
	}
	return coreVersionsDoneMsg{result: sb.String()}
}

func (v *CoreView) doCheckUpdates(forUpdate bool) tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	var sb strings.Builder
	var avail []core.UpdateCheck
	sb.WriteString("更新检查\n\n")

	for _, comp := range core.UpdatableComponents() {
		bp := binPath(comp)
		if bp == "" {
			continue
		}
		check, err := core.CheckUpdate(ctx, comp, bp)
		if err != nil {
			sb.WriteString(fmt.Sprintf("  %s: 检查失败 (%s)\n", comp, err))
			continue
		}
		if check.CurrentVersion == "" {
			// Not installed - skip for update purposes.
			sb.WriteString(fmt.Sprintf("  %s: 未安装 (最新 %s)\n", comp, check.LatestVersion))
			continue
		}
		if !check.UpdateAvail {
			sb.WriteString(fmt.Sprintf("  %s: %s (最新)\n",
				comp, check.CurrentVersion))
			continue
		}
		sb.WriteString(fmt.Sprintf("  %s: %s → %s (可更新)\n",
			comp, check.CurrentVersion, check.LatestVersion))
		avail = append(avail, *check)
	}

	// Show non-updatable components.
	for _, comp := range core.AllComponents() {
		if core.HasRepo(comp) {
			continue
		}
		bp := binPath(comp)
		if bp == "" {
			continue
		}
		info := core.DetectVersion(ctx, bp, comp)
		if info.Installed {
			sb.WriteString(fmt.Sprintf("  %s: %s (手动管理)\n", comp, info.Version))
		} else {
			sb.WriteString(fmt.Sprintf("  %s: 未安装 (手动管理)\n", comp))
		}
	}

	if len(avail) == 0 {
		sb.WriteString("\n所有组件已是最新版本")
	}

	return coreCheckDoneMsg{result: sb.String(), updates: avail, forUpdate: forUpdate}
}

func (v *CoreView) doUpdate(updates []core.UpdateCheck) tea.Msg {
	ctx := context.Background()
	var sb strings.Builder
	sb.WriteString("更新结果\n\n")

	for _, u := range updates {
		if u.DownloadURL == "" {
			sb.WriteString(fmt.Sprintf("  %s: 无下载地址，跳过\n", u.Component))
			continue
		}
		bp := binPath(u.Component)
		if err := core.DownloadBinary(ctx, u.DownloadURL, bp); err != nil {
			sb.WriteString(fmt.Sprintf("  %s: 下载失败 (%s)\n", u.Component, err))
			continue
		}
		sb.WriteString(fmt.Sprintf("  %s: %s → %s 更新成功\n",
			u.Component, u.CurrentVersion, u.LatestVersion))
		svc := componentService(u.Component)
		if svc != "" {
			exec.Command("systemctl", "restart", svc).Run()
		}
	}
	return coreUpdateDoneMsg{result: sb.String()}
}

func componentService(comp core.Component) string {
	switch comp {
	case core.CompSingBox:
		return "sing-box"
	case core.CompSnell:
		return "snell-v5"
	case core.CompShadowTLS:
		return "shadow-tls"
	case core.CompCaddy:
		return "caddy-sub"
	default:
		return ""
	}
}
