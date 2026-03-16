package views

import (
	tea "github.com/charmbracelet/bubbletea"

	"go-proxy/internal/routing"
	"go-proxy/internal/tui"
	"go-proxy/internal/tui/components"
)

type RoutingView struct {
	model *tui.Model
	menu  components.MenuModel
}

func NewRoutingView(model *tui.Model) *RoutingView {
	v := &RoutingView{model: model}
	v.menu = components.NewMenu("Routing Management", []components.MenuItem{
		{Key: '1', Label: "设置规则  Set Rules", ID: "set"},
		{Key: '2', Label: "清除规则  Clear Rules", ID: "clear"},
		{Key: '3', Label: "链式代理  Chain Proxy", ID: "chain"},
		{Key: '4', Label: "测试规则  Test Rules", ID: "test"},
		{Key: '5', Label: "同步 DNS  Sync DNS", ID: "sync-dns"},
		{Key: '0', Label: "返回  Back", ID: "back"},
	})
	return v
}

func (v *RoutingView) Name() string { return "routing" }

func (v *RoutingView) Init() tea.Cmd { return nil }

func (v *RoutingView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case components.MenuSelectMsg:
		switch msg.ID {
		case "back":
			return v, tui.BackCmd
		case "sync-dns":
			return v, v.syncDNS
		default:
			return v, func() tea.Msg {
				return tui.ShowOverlayMsg{
					Overlay: components.NewResult("Not implemented yet"),
				}
			}
		}
	case syncDNSDoneMsg:
		return v, func() tea.Msg {
			return tui.ShowOverlayMsg{
				Overlay: components.NewResult(msg.result),
			}
		}
	case tui.ResultDismissedMsg:
		return v, nil
	default:
		var cmd tea.Cmd
		v.menu, cmd = v.menu.Update(msg)
		return v, cmd
	}
}

func (v *RoutingView) View() string { return v.menu.View() }

type syncDNSDoneMsg struct{ result string }

func (v *RoutingView) syncDNS() tea.Msg {
	routing.SyncDNS(v.model.Store(), nil, "ipv4_only")
	routing.SyncRouteRules(v.model.Store())
	if err := v.model.Store().Apply(); err != nil {
		return syncDNSDoneMsg{result: "Failed to save: " + err.Error()}
	}
	return syncDNSDoneMsg{result: "DNS and route rules synced"}
}
