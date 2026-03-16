package views

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"go-proxy/internal/network"
	"go-proxy/internal/tui"
	"go-proxy/internal/tui/components"
)

type networkStep int

const (
	networkMenu networkStep = iota
	networkConfirm
	networkResult
)

type NetworkView struct {
	model *tui.Model
	menu  components.MenuModel
	step  networkStep
}

func NewNetworkView(model *tui.Model) *NetworkView {
	v := &NetworkView{model: model}
	v.menu = components.NewMenu("网络管理", []components.MenuItem{
		{Key: '1', Label: "BBR 网络优化", ID: "bbr"},
		{Key: '2', Label: "服务器防火墙收敛", ID: "firewall"},
		{Key: '0', Label: "返回", ID: "back"},
	})
	return v
}

func (v *NetworkView) Name() string { return "network" }

func (v *NetworkView) Init() tea.Cmd {
	v.step = networkMenu
	return nil
}

func (v *NetworkView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case components.MenuSelectMsg:
		switch msg.ID {
		case "back":
			return v, tui.BackCmd
		case "bbr":
			return v, v.doBBRStatus
		case "firewall":
			return v, v.doFirewall
		}

	case networkActionDoneMsg:
		if msg.needConfirm {
			v.step = networkConfirm
			return v, func() tea.Msg {
				return tui.ShowOverlayMsg{
					Overlay: components.NewConfirm(msg.result),
				}
			}
		}
		v.step = networkResult
		return v, func() tea.Msg {
			return tui.ShowOverlayMsg{
				Overlay: components.NewResult(msg.result),
			}
		}

	case tui.ConfirmResultMsg:
		if msg.Confirmed {
			return v, v.doEnableBBR
		}
		v.step = networkMenu
		return v, nil

	case tui.ResultDismissedMsg:
		v.step = networkMenu
		return v, nil

	default:
		if v.step == networkMenu {
			var cmd tea.Cmd
			v.menu, cmd = v.menu.Update(msg)
			return v, cmd
		}
	}
	return v, nil
}

func (v *NetworkView) View() string { return v.menu.View() }

type networkActionDoneMsg struct {
	result      string
	needConfirm bool
}

func (v *NetworkView) doBBRStatus() tea.Msg {
	enabled, current, err := network.BBRStatus()
	if err != nil {
		return networkActionDoneMsg{result: fmt.Sprintf("BBR 状态\n\n读取失败: %s", err)}
	}
	if enabled {
		return networkActionDoneMsg{result: fmt.Sprintf("BBR 状态\n\n当前拥塞控制: %s\nBBR 已启用", current)}
	}
	return networkActionDoneMsg{
		result:      fmt.Sprintf("当前拥塞控制: %s\nBBR 未启用，是否启用？", current),
		needConfirm: true,
	}
}

func (v *NetworkView) doEnableBBR() tea.Msg {
	if err := network.EnableBBR(); err != nil {
		return networkActionDoneMsg{result: "启用 BBR 失败: " + err.Error()}
	}
	return networkActionDoneMsg{result: "BBR 已成功启用"}
}

func (v *NetworkView) doFirewall() tea.Msg {
	output, err := network.ListOpenPorts()
	if err != nil {
		return networkActionDoneMsg{result: "读取防火墙规则失败: " + err.Error()}
	}
	if output == "" {
		output = "暂无防火墙规则"
	}
	return networkActionDoneMsg{result: "防火墙规则\n\n" + output}
}
