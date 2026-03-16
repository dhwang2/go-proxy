package views

import (
	tea "github.com/charmbracelet/bubbletea"

	"go-proxy/internal/tui"
	"go-proxy/internal/tui/components"
)

type SelfUpdateView struct {
	model *tui.Model
}

func NewSelfUpdateView(model *tui.Model) *SelfUpdateView {
	return &SelfUpdateView{model: model}
}

func (v *SelfUpdateView) Name() string { return "self-update" }

func (v *SelfUpdateView) Init() tea.Cmd {
	return func() tea.Msg {
		return tui.ShowOverlayMsg{
			Overlay: components.NewResult("Self-update not yet implemented"),
		}
	}
}

func (v *SelfUpdateView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg.(type) {
	case tui.ResultDismissedMsg:
		return v, func() tea.Msg { return tui.BackMsg{} }
	}
	return v, nil
}

func (v *SelfUpdateView) View() string { return "" }
