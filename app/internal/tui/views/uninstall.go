package views

import (
	"context"

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
			Overlay: components.NewConfirm("Uninstall all services and configuration?"),
		}
	}
}

func (v *UninstallView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.ConfirmResultMsg:
		if !msg.Confirmed {
			return v, tui.BackCmd
		}
		return v, v.doUninstall

	case uninstallDoneMsg:
		v.step = uninstallResult
		return v, func() tea.Msg {
			return tui.ShowOverlayMsg{
				Overlay: components.NewResult(msg.result),
			}
		}

	case tui.ResultDismissedMsg:
		return v, tea.Quit
	}
	return v, nil
}

func (v *UninstallView) View() string { return "" }

type uninstallDoneMsg struct{ result string }

func (v *UninstallView) doUninstall() tea.Msg {
	ctx := context.Background()
	if err := service.Uninstall(ctx); err != nil {
		return uninstallDoneMsg{result: "Uninstall error: " + err.Error()}
	}
	return uninstallDoneMsg{result: "Uninstalled successfully"}
}
