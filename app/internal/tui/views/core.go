package views

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-proxy/internal/config"
	"go-proxy/internal/core"
	"go-proxy/internal/service"
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
	tui.SplitViewBase
	step    coreStep
	pending []core.UpdateCheck
}

func NewCoreView(model *tui.Model) *CoreView {
	v := &CoreView{}
	v.Model = model
	v.Menu = tui.NewMenu("", []tui.MenuItem{
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
	v.InitSplit()
	return nil
}

func (v *CoreView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.ViewResizeMsg:
		v.HandleResize(msg)
		return v, nil
	case tui.SubSplitMouseMsg:
		return v, v.HandleMouse(msg)
	}
	// In split mode, intercept up/down for menu navigation even when content is showing.
	if cmd, handled := v.HandleMenuNav(msg, v.step == coreMenu, false); handled {
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
		v.SetFocus(true)
		return v, nil

	case coreUpdateDoneMsg:
		v.step = coreResult
		return v, v.SetInline(components.NewResult(msg.result))

	case tui.ResultDismissedMsg:
		v.step = coreMenu
		v.SetFocus(true)
		return v, nil

	default:
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.Type == tea.KeyEsc {
			return v, tui.BackCmd
		}
		// Left/Right arrow toggles sub-split focus.
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			if v.HandleSplitArrows(keyMsg, v.step == coreMenu, v.HasInline()) {
				return v, nil
			}
		}
		if v.step == coreMenu {
			var cmd tea.Cmd
			v.Menu, cmd = v.Menu.Update(msg)
			return v, cmd
		}
	}
	return v, inlineCmd
}

func (v *CoreView) View() string {
	if v.step == coreMenu || !v.Split.Enabled() {
		if v.HasInline() {
			return v.ViewInline()
		}
		return v.Menu.View()
	}

	menuContent := v.Menu.View()
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

	return v.Split.View(menuContent, detailContent)
}

// triggerMenuAction executes the action for the given menu item ID.
func (v *CoreView) triggerMenuAction(id string) tea.Cmd {
	switch id {
	case "versions":
		v.step = coreWorking
		return tea.Batch(
			v.SetInline(components.NewSpinner("正在检测版本...")),
			v.doVersions,
		)
	case "check":
		v.step = coreWorking
		return tea.Batch(
			v.SetInline(components.NewSpinner("正在检查更新...")),
			func() tea.Msg { return v.doCheckUpdates(false) },
		)
	case "update":
		v.step = coreWorking
		return tea.Batch(
			v.SetInline(components.NewSpinner("正在检查更新...")),
			func() tea.Msg { return v.doCheckUpdates(true) },
		)
	}
	return nil
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
	width := v.Model.ContentWidth()
	rows := make([][]string, 0, len(core.AllComponents()))
	for _, comp := range core.AllComponents() {
		bp := binPath(comp)
		if bp == "" {
			rows = append(rows, []string{string(comp), "未配置", "-"})
			continue
		}
		info := core.DetectVersion(ctx, bp, comp)
		if info.Installed {
			rows = append(rows, []string{string(comp), "已安装", info.Version})
			continue
		}
		rows = append(rows, []string{string(comp), "未安装", "-"})
	}
	return coreVersionsDoneMsg{result: renderTable([]string{"组件", "状态", "版本"}, rows, width, false)}
}

func (v *CoreView) doCheckUpdates(forUpdate bool) tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	var avail []core.UpdateCheck
	width := v.Model.ContentWidth()
	rows := make([][]string, 0, len(core.AllComponents()))

	for _, comp := range core.UpdatableComponents() {
		bp := binPath(comp)
		if bp == "" {
			continue
		}
		check, err := core.CheckUpdate(ctx, comp, bp)
		if err != nil {
			rows = append(rows, []string{string(comp), "-", "-", "检查失败: " + err.Error()})
			continue
		}
		if check.CurrentVersion == "" {
			rows = append(rows, []string{string(comp), "-", check.LatestVersion, "未安装"})
			continue
		}
		if !check.UpdateAvail {
			rows = append(rows, []string{string(comp), check.CurrentVersion, check.LatestVersion, "已最新"})
			continue
		}
		rows = append(rows, []string{string(comp), check.CurrentVersion, check.LatestVersion, "可更新"})
		avail = append(avail, *check)
	}

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
			rows = append(rows, []string{string(comp), info.Version, "-", "手动管理"})
			continue
		}
		rows = append(rows, []string{string(comp), "-", "-", "未安装(手动管理)"})
	}

	return coreCheckDoneMsg{
		result:    renderTable([]string{"组件", "当前版本", "最新版本", "状态"}, rows, width, false),
		updates:   avail,
		forUpdate: forUpdate,
	}
}

func (v *CoreView) doUpdate(updates []core.UpdateCheck) tea.Msg {
	ctx := context.Background()
	width := v.Model.ContentWidth()
	rows := make([][]string, 0, len(updates))

	for _, u := range updates {
		if u.DownloadURL == "" {
			rows = append(rows, []string{string(u.Component), u.CurrentVersion, u.LatestVersion, "无下载地址，已跳过"})
			continue
		}
		bp := binPath(u.Component)
		if err := core.DownloadBinary(ctx, u.DownloadURL, bp); err != nil {
			rows = append(rows, []string{string(u.Component), u.CurrentVersion, u.LatestVersion, "下载失败: " + err.Error()})
			continue
		}
		rows = append(rows, []string{string(u.Component), u.CurrentVersion, u.LatestVersion, "更新成功"})
		svc := componentService(u.Component)
		if svc != "" {
			service.Restart(ctx, service.Name(svc))
		}
	}
	return coreUpdateDoneMsg{result: renderTable([]string{"组件", "当前版本", "目标版本", "结果"}, rows, width, false)}
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
