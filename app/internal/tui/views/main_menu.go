package views

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-proxy/internal/tui"
	"go-proxy/internal/tui/components"
)

type MainMenuView struct {
	model *tui.Model
	menu  components.MenuModel
}

func NewMainMenuView(model *tui.Model) *MainMenuView {
	v := &MainMenuView{model: model}
	v.menu = components.NewMenu("", []components.MenuItem{
		{Key: '1', Label: "󰒍 安装协议", ID: "protocol-install"},
		{Key: '2', Label: "󰆴 卸载协议", ID: "protocol-remove"},
		{Key: '3', Label: "󰁥 用户管理", ID: "user"},
		{Key: '4', Label: "󰛳 分流管理", ID: "routing"},
		{Key: '5', Label: "󰒓 协议管理", ID: "service"},
		{Key: '6', Label: "󰑫 订阅管理", ID: "subscription"},
		{Key: '7', Label: "󰈔 查看配置", ID: "config"},
		{Key: '8', Label: "󰌱 运行日志", ID: "logs"},
		{Key: '9', Label: "󰚗 内核管理", ID: "core"},
		{Key: 'a', Label: "󰀂 网络管理", ID: "network"},
		{Key: 'b', Label: "󰁪 脚本更新", ID: "self-update"},
		{Key: 'c', Label: "󰩺 卸载服务", ID: "uninstall"},
		{Key: '0', Label: "󰗼 完全退出", ID: "quit"},
	})
	return v
}

func (v *MainMenuView) Name() string { return "main-menu" }

func (v *MainMenuView) Init() tea.Cmd { return nil }

func (v *MainMenuView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case components.MenuSelectMsg:
		if msg.ID == "quit" {
			return v, tea.Quit
		}
		name := msg.ID
		return v, func() tea.Msg {
			return tui.NavigateMsg{Name: name}
		}
	default:
		var cmd tea.Cmd
		v.menu, cmd = v.menu.Update(msg)
		return v, cmd
	}
}

func (v *MainMenuView) View() string {
	w := v.model.Width()
	dashboard := tui.RenderDashboard(v.model.Store(), v.model.Version(), w)

	sepWidth := tui.SeparatorWidth
	if sepWidth > w {
		sepWidth = w
	}
	sep := tui.SeparatorDouble(sepWidth)
	hint := tui.FooterHintStyle.Width(sepWidth).Render("退出(esc) | 选择(↑↓) | 确认(enter)")

	body := lipgloss.JoinVertical(lipgloss.Center,
		dashboard,
		v.menu.View(),
		sep,
		hint,
		sep,
	)

	return lipgloss.NewStyle().Width(w).Align(lipgloss.Center).Render(body)
}
