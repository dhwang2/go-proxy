package views

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"go-proxy/internal/tui"
	"go-proxy/internal/tui/components"
	"go-proxy/internal/update"
)

type SelfUpdateView struct {
	tui.InlineState
	model         *tui.Model
	step          selfUpdateStep
	check         *update.SelfUpdateCheck
	status        string
	confirmPrompt string
}

type selfUpdateStep int

const (
	selfUpdateChecking selfUpdateStep = iota
	selfUpdateConfirm
	selfUpdateUpdating
	selfUpdateResult
)

func NewSelfUpdateView(model *tui.Model) *SelfUpdateView {
	return &SelfUpdateView{model: model}
}

func (v *SelfUpdateView) Name() string { return "self-update" }

func (v *SelfUpdateView) Init() tea.Cmd {
	v.step = selfUpdateChecking
	v.check = nil
	v.status = "正在检查更新..."
	return v.doCheck
}

func (v *SelfUpdateView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	inlineCmd, handled := v.UpdateInline(msg)
	if handled {
		return v, inlineCmd
	}
	switch msg := msg.(type) {
	case selfUpdateCheckDoneMsg:
		if msg.err != nil {
			v.step = selfUpdateResult
			return v, v.SetInline(components.NewResult("检查更新失败: " + msg.err.Error()))
		}
		v.check = msg.check
		if !msg.check.UpdateAvail {
			v.step = selfUpdateResult
			result := fmt.Sprintf("当前版本: %s\n已是最新版本", msg.check.CurrentVersion)
			return v, v.SetInline(components.NewResult(result))
		}
		v.step = selfUpdateConfirm
		v.confirmPrompt = fmt.Sprintf("发现新版本: %s → %s\n是否更新?",
			msg.check.CurrentVersion, msg.check.LatestVersion)
		return v, nil

	case selfUpdateDoneMsg:
		if msg.err != nil {
			v.step = selfUpdateResult
			return v, v.SetInline(components.NewResult("更新失败: " + msg.err.Error()))
		}
		// Successful update: set exit message and quit immediately.
		v.model.SetExitMessage(fmt.Sprintf("更新完成 (%s)，请重新运行 proxy", v.check.LatestVersion))
		return v, tea.Quit

	case tui.ResultDismissedMsg:
		return v, tui.BackCmd
	case tea.KeyMsg:
		if v.step == selfUpdateConfirm {
			switch {
			case msg.Type == tea.KeyEnter:
				v.step = selfUpdateUpdating
				v.status = "正在下载更新..."
				return v, v.doUpdate
			case msg.Type == tea.KeyEsc:
				return v, tui.BackCmd
			}
			return v, nil
		}
		if msg.Type == tea.KeyEsc {
			return v, tui.BackCmd
		}
	}
	return v, inlineCmd
}

func (v *SelfUpdateView) View() string {
	if v.HasInline() {
		return v.ViewInline()
	}
	if v.step == selfUpdateConfirm {
		return "  " + v.confirmPrompt
	}
	if v.step == selfUpdateChecking || v.step == selfUpdateUpdating {
		return "  " + v.status
	}
	return ""
}

type selfUpdateCheckDoneMsg struct {
	check *update.SelfUpdateCheck
	err   error
}

type selfUpdateDoneMsg struct{ err error }

func (v *SelfUpdateView) doCheck() tea.Msg {
	ctx := context.Background()
	check, err := update.CheckSelfUpdate(ctx, v.model.Version())
	return selfUpdateCheckDoneMsg{check: check, err: err}
}

func (v *SelfUpdateView) doUpdate() tea.Msg {
	ctx := context.Background()
	err := update.SelfUpdate(ctx, v.check.DownloadURL)
	return selfUpdateDoneMsg{err: err}
}
