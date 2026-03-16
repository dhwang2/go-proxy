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
	model   *tui.Model
	step    selfUpdateStep
	check   *update.SelfUpdateCheck
	status  string
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
	switch msg := msg.(type) {
	case selfUpdateCheckDoneMsg:
		if msg.err != nil {
			v.step = selfUpdateResult
			return v, func() tea.Msg {
				return tui.ShowOverlayMsg{
					Overlay: components.NewResult("检查更新失败: " + msg.err.Error()),
				}
			}
		}
		v.check = msg.check
		if !msg.check.UpdateAvail {
			v.step = selfUpdateResult
			result := fmt.Sprintf("当前版本: %s\n已是最新版本", msg.check.CurrentVersion)
			return v, func() tea.Msg {
				return tui.ShowOverlayMsg{
					Overlay: components.NewResult(result),
				}
			}
		}
		v.step = selfUpdateConfirm
		prompt := fmt.Sprintf("发现新版本: %s → %s\n是否更新?",
			msg.check.CurrentVersion, msg.check.LatestVersion)
		return v, func() tea.Msg {
			return tui.ShowOverlayMsg{
				Overlay: components.NewConfirm(prompt),
			}
		}

	case tui.ConfirmResultMsg:
		if !msg.Confirmed {
			return v, tui.BackCmd
		}
		v.step = selfUpdateUpdating
		v.status = "正在下载更新..."
		return v, v.doUpdate

	case selfUpdateDoneMsg:
		v.step = selfUpdateResult
		var result string
		if msg.err != nil {
			result = "更新失败: " + msg.err.Error()
		} else {
			result = fmt.Sprintf("已更新到 %s\n请重启程序", v.check.LatestVersion)
		}
		return v, func() tea.Msg {
			return tui.ShowOverlayMsg{
				Overlay: components.NewResult(result),
			}
		}

	case tui.ResultDismissedMsg:
		return v, tui.BackCmd
	}
	return v, nil
}

func (v *SelfUpdateView) View() string {
	if v.step == selfUpdateChecking || v.step == selfUpdateUpdating {
		return "\n  " + v.status + "\n"
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
