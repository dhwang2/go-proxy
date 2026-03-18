package views

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"go-proxy/internal/service"
	"go-proxy/internal/tui"
	"go-proxy/internal/tui/components"
)

type UninstallView struct {
	model *tui.Model
	step  uninstallStep
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
	v.step = uninstallConfirm
	return func() tea.Msg {
		return tui.ShowOverlayMsg{
			Overlay: components.NewConfirm("确认卸载所有服务和配置?"),
		}
	}
}

func (v *UninstallView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.ConfirmResultMsg:
		if !msg.Confirmed {
			return v, tui.BackCmd
		}
		return v, tea.Sequence(
			func() tea.Msg {
				return tui.ShowOverlayMsg{Overlay: components.NewSpinner("正在卸载...")}
			},
			v.doUninstall,
		)

	case uninstallDoneMsg:
		v.step = uninstallResult
		if msg.success {
			v.model.SetExitMessage("卸载完成，所有服务和配置已移除")
		}
		return v, func() tea.Msg {
			return tui.ShowOverlayMsg{
				Overlay: components.NewResult(msg.result),
			}
		}

	case tui.ResultDismissedMsg:
		return v, tea.Quit
	case tea.KeyMsg:
		if msg.Type == tea.KeyEsc {
			return v, tui.BackCmd
		}
	}
	return v, nil
}

func (v *UninstallView) View() string { return "" }

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
