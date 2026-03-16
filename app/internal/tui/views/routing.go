package views

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"go-proxy/internal/derived"
	"go-proxy/internal/routing"
	"go-proxy/internal/tui"
	"go-proxy/internal/tui/components"
)

type RoutingView struct {
	model *tui.Model
	menu  components.MenuModel
	step  routingStep
	// chain proxy state
	pendingChainServer string
}

type routingStep int

const (
	routingMenu routingStep = iota
	routingChainServer
	routingChainPort
	routingSyncDNS
	routingResult
)

func NewRoutingView(model *tui.Model) *RoutingView {
	v := &RoutingView{model: model}
	v.menu = components.NewMenu("分流管理", []components.MenuItem{
		{Key: '1', Label: "同步 DNS 和路由规则", ID: "sync"},
		{Key: '2', Label: "添加链式代理", ID: "chain-add"},
		{Key: '3', Label: "删除链式代理", ID: "chain-remove"},
		{Key: '4', Label: "查看路由规则", ID: "view-rules"},
		{Key: '0', Label: "返回", ID: "back"},
	})
	return v
}

func (v *RoutingView) Name() string { return "routing" }

func (v *RoutingView) Init() tea.Cmd {
	v.step = routingMenu
	return nil
}

func (v *RoutingView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case components.MenuSelectMsg:
		switch msg.ID {
		case "back":
			return v, tui.BackCmd
		case "sync":
			v.step = routingSyncDNS
			return v, v.doSyncDNS
		case "chain-add":
			v.step = routingChainServer
			return v, func() tea.Msg {
				return tui.ShowOverlayMsg{
					Overlay: components.NewTextInput("链式代理地址 (host:port):", ""),
				}
			}
		case "chain-remove":
			return v, v.doChainRemove
		case "view-rules":
			return v, v.doViewRules
		}

	case tui.InputResultMsg:
		if msg.Cancelled {
			v.step = routingMenu
			return v, nil
		}
		switch v.step {
		case routingChainServer:
			parts := strings.SplitN(msg.Value, ":", 2)
			if len(parts) != 2 {
				v.step = routingResult
				return v, func() tea.Msg {
					return tui.ShowOverlayMsg{
						Overlay: components.NewResult("格式错误，请使用 host:port"),
					}
				}
			}
			server := parts[0]
			port, err := strconv.Atoi(parts[1])
			if err != nil || port <= 0 || port > 65535 {
				v.step = routingResult
				return v, func() tea.Msg {
					return tui.ShowOverlayMsg{
						Overlay: components.NewResult("端口号无效"),
					}
				}
			}
			val := msg.Value
			return v, func() tea.Msg { return v.doChainAdd(server, port, val) }
		}

	case routingActionDoneMsg:
		v.step = routingResult
		return v, func() tea.Msg {
			return tui.ShowOverlayMsg{
				Overlay: components.NewResult(msg.result),
			}
		}

	case tui.ResultDismissedMsg:
		v.step = routingMenu
		return v, nil

	default:
		if v.step == routingMenu {
			var cmd tea.Cmd
			v.menu, cmd = v.menu.Update(msg)
			return v, cmd
		}
	}
	return v, nil
}

func (v *RoutingView) View() string { return v.menu.View() }

type routingActionDoneMsg struct{ result string }

func (v *RoutingView) doSyncDNS() tea.Msg {
	routing.SyncDNS(v.model.Store(), nil, "ipv4_only")
	routing.SyncRouteRules(v.model.Store())
	if err := v.model.Store().Apply(); err != nil {
		return routingActionDoneMsg{result: "同步失败: " + err.Error()}
	}
	return routingActionDoneMsg{result: "DNS 和路由规则已同步"}
}

func (v *RoutingView) doChainAdd(server string, port int, display string) tea.Msg {
	tag := "res-socks"
	if err := routing.SetChain(v.model.Store(), tag, server, port); err != nil {
		return routingActionDoneMsg{result: "添加失败: " + err.Error()}
	}
	routing.SyncDNS(v.model.Store(), nil, "ipv4_only")
	if err := v.model.Store().Apply(); err != nil {
		return routingActionDoneMsg{result: "保存失败: " + err.Error()}
	}
	return routingActionDoneMsg{result: fmt.Sprintf("链式代理已添加: %s", display)}
}

func (v *RoutingView) doChainRemove() tea.Msg {
	tag := "res-socks"
	if err := routing.RemoveChain(v.model.Store(), tag); err != nil {
		return routingActionDoneMsg{result: "删除失败: " + err.Error()}
	}
	routing.SyncDNS(v.model.Store(), nil, "ipv4_only")
	if err := v.model.Store().Apply(); err != nil {
		return routingActionDoneMsg{result: "保存失败: " + err.Error()}
	}
	return routingActionDoneMsg{result: "链式代理已删除"}
}

func (v *RoutingView) doViewRules() tea.Msg {
	s := v.model.Store()
	users := derived.AllRoutedUsers(s)
	if len(users) == 0 {
		return routingActionDoneMsg{result: "暂无路由规则"}
	}

	var sb strings.Builder
	sb.WriteString("路由规则\n\n")
	for _, userName := range users {
		rules := derived.UserRoutes(s, userName)
		sb.WriteString(fmt.Sprintf("  %s (%d 条规则)\n", userName, len(rules)))
		for _, r := range rules {
			var targets []string
			targets = append(targets, r.Domain...)
			targets = append(targets, r.DomainSuffix...)
			targets = append(targets, r.DomainKeyword...)
			targets = append(targets, r.RuleSet...)
			targets = append(targets, r.IPCIDR...)
			summary := strings.Join(targets, ", ")
			if len(summary) > 60 {
				summary = summary[:57] + "..."
			}
			sb.WriteString(fmt.Sprintf("    → %s: %s\n", r.Outbound, summary))
		}
	}
	return routingActionDoneMsg{result: sb.String()}
}
