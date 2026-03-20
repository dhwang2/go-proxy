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
	tui.InlineState
	model         *tui.Model
	step          uninstallStep
	confirmPrompt string
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
	v.confirmPrompt = "确认卸载所有服务和配置?"
	return nil
}

func (v *UninstallView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
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
		return "\n  " + v.confirmPrompt + "\n"
	}
	return ""
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
