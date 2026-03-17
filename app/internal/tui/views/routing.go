package views

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"go-proxy/internal/derived"
	"go-proxy/internal/routing"
	"go-proxy/internal/store"
	"go-proxy/internal/tui"
	"go-proxy/internal/tui/components"
)

type routingStep int

const (
	routingMenu routingStep = iota
	// Chain proxy flow
	routingChainMenu
	routingChainAddInput
	routingChainDeleteSelect
	// Configure routing flow
	routingConfigUser
	routingConfigPreset
	routingConfigOutbound
	// Direct outbound flow
	routingDirect
	// Test routing flow
	routingTestUser
	routingTestDomain
	// Result
	routingResult
)

type RoutingView struct {
	model   *tui.Model
	menu    components.MenuModel
	subMenu components.MenuModel
	step    routingStep
	// State for multi-step flows
	selectedUser    string
	selectedPreset  routing.Preset
	pendingChainTag string
}

func NewRoutingView(model *tui.Model) *RoutingView {
	v := &RoutingView{model: model}
	v.menu = components.NewMenu("󰛳  分流管理", []components.MenuItem{
		{Key: '1', Label: "  链式代理", ID: "chain"},
		{Key: '2', Label: "  配置分流", ID: "config"},
		{Key: '3', Label: "  直连出口", ID: "direct"},
		{Key: '4', Label: "  测试分流", ID: "test"},
		{Key: '0', Label: "  返回", ID: "back"},
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
		return v.handleMenuSelect(msg)

	case tui.InputResultMsg:
		return v.handleInput(msg)

	case routingActionDoneMsg:
		v.step = routingResult
		return v, func() tea.Msg {
			return tui.ShowOverlayMsg{Overlay: components.NewResult(msg.result)}
		}

	case tui.ResultDismissedMsg:
		v.step = routingMenu
		return v, nil

	default:
		return v.handleDefault(msg)
	}
}

func (v *RoutingView) handleMenuSelect(msg components.MenuSelectMsg) (tui.View, tea.Cmd) {
	switch v.step {
	case routingMenu:
		switch msg.ID {
		case "back":
			return v, tui.BackCmd
		case "chain":
			v.step = routingChainMenu
			v.subMenu = components.NewMenu("链式代理", []components.MenuItem{
				{Key: '1', Label: "添加节点", ID: "add"},
				{Key: '2', Label: "删除节点", ID: "delete"},
				{Key: '3', Label: "查看节点", ID: "view"},
				{Key: '0', Label: "返回", ID: "back"},
			})
			return v, nil
		case "config":
			return v, v.showUserMenu(routingConfigUser)
		case "direct":
			v.step = routingDirect
			v.subMenu = components.NewMenu("直连出口", []components.MenuItem{
				{Key: '1', Label: "仅 IPv4", ID: "ipv4_only"},
				{Key: '2', Label: "仅 IPv6", ID: "ipv6_only"},
				{Key: '3', Label: "优先 IPv4", ID: "prefer_ipv4"},
				{Key: '4', Label: "优先 IPv6", ID: "prefer_ipv6"},
				{Key: '5', Label: "AsIs (系统默认)", ID: ""},
				{Key: '0', Label: "返回", ID: "back"},
			})
			return v, nil
		case "test":
			return v, v.showUserMenu(routingTestUser)
		}

	case routingChainMenu:
		switch msg.ID {
		case "back":
			v.step = routingMenu
			return v, nil
		case "add":
			v.step = routingChainAddInput
			return v, func() tea.Msg {
				return tui.ShowOverlayMsg{
					Overlay: components.NewTextInput("链式代理地址 (host:port):", ""),
				}
			}
		case "delete":
			return v, v.showChainDeleteMenu()
		case "view":
			return v, v.doChainView
		}

	case routingChainDeleteSelect:
		if msg.ID == "back" {
			v.step = routingChainMenu
			return v, nil
		}
		tag := msg.ID
		return v, func() tea.Msg { return v.doChainRemove(tag) }

	case routingConfigUser:
		if msg.ID == "back" {
			v.step = routingMenu
			return v, nil
		}
		v.selectedUser = msg.ID
		return v, v.showPresetMenu()

	case routingConfigPreset:
		if msg.ID == "back" {
			v.step = routingConfigUser
			return v, v.showUserMenu(routingConfigUser)
		}
		presets := routing.BuiltinPresets()
		idx, _ := strconv.Atoi(msg.ID)
		if idx >= 0 && idx < len(presets) {
			v.selectedPreset = presets[idx]
		}
		return v, v.showOutboundMenu()

	case routingConfigOutbound:
		if msg.ID == "back" {
			return v, v.showPresetMenu()
		}
		outbound := msg.ID
		return v, func() tea.Msg {
			return v.doConfigRoute(v.selectedUser, v.selectedPreset, outbound)
		}

	case routingDirect:
		if msg.ID == "back" {
			v.step = routingMenu
			return v, nil
		}
		strategy := msg.ID
		return v, func() tea.Msg { return v.doDirectOutbound(strategy) }

	case routingTestUser:
		if msg.ID == "back" {
			v.step = routingMenu
			return v, nil
		}
		v.selectedUser = msg.ID
		v.step = routingTestDomain
		return v, func() tea.Msg {
			return tui.ShowOverlayMsg{
				Overlay: components.NewTextInput("测试域名:", "google.com"),
			}
		}
	}

	return v, nil
}

func (v *RoutingView) handleInput(msg tui.InputResultMsg) (tui.View, tea.Cmd) {
	if msg.Cancelled {
		// Return to appropriate parent step.
		switch v.step {
		case routingChainAddInput:
			v.step = routingChainMenu
		case routingTestDomain:
			v.step = routingMenu
		default:
			v.step = routingMenu
		}
		return v, nil
	}

	switch v.step {
	case routingChainAddInput:
		parts := strings.SplitN(msg.Value, ":", 2)
		if len(parts) != 2 {
			return v, v.showError("格式错误，请使用 host:port")
		}
		server := parts[0]
		port, err := strconv.Atoi(parts[1])
		if err != nil || port <= 0 || port > 65535 {
			return v, v.showError("端口号无效")
		}
		display := msg.Value
		return v, func() tea.Msg { return v.doChainAdd(server, port, display) }

	case routingTestDomain:
		domain := msg.Value
		user := v.selectedUser
		return v, func() tea.Msg { return v.doTestDomain(user, domain) }
	}

	return v, nil
}

func (v *RoutingView) handleDefault(msg tea.Msg) (tui.View, tea.Cmd) {
	var cmd tea.Cmd
	switch v.step {
	case routingMenu:
		v.menu, cmd = v.menu.Update(msg)
	case routingChainMenu, routingChainDeleteSelect, routingConfigUser,
		routingConfigPreset, routingConfigOutbound, routingDirect, routingTestUser:
		v.subMenu, cmd = v.subMenu.Update(msg)
	}
	return v, cmd
}

func (v *RoutingView) View() string {
	switch v.step {
	case routingChainMenu, routingChainDeleteSelect, routingConfigUser,
		routingConfigPreset, routingConfigOutbound, routingDirect, routingTestUser:
		return v.subMenu.View()
	default:
		return v.menu.View()
	}
}

// --- Helper methods to build dynamic menus ---

func (v *RoutingView) showUserMenu(nextStep routingStep) tea.Cmd {
	users := derived.UserNames(v.model.Store())
	if len(users) == 0 {
		return func() tea.Msg {
			return routingActionDoneMsg{result: "暂无用户"}
		}
	}
	items := make([]components.MenuItem, 0, len(users)+1)
	for i, u := range users {
		key := rune('1' + i)
		if i >= 9 {
			key = rune('a' + i - 9)
		}
		items = append(items, components.MenuItem{Key: key, Label: u, ID: u})
	}
	items = append(items, components.MenuItem{Key: '0', Label: "返回", ID: "back"})
	v.step = nextStep
	v.subMenu = components.NewMenu("选择用户", items)
	return nil
}

func (v *RoutingView) showPresetMenu() tea.Cmd {
	presets := routing.BuiltinPresets()
	items := make([]components.MenuItem, 0, len(presets)+1)
	for i, p := range presets {
		key := rune('1' + i)
		if i >= 9 {
			key = rune('a' + i - 9)
		}
		items = append(items, components.MenuItem{Key: key, Label: p.Name, ID: strconv.Itoa(i)})
	}
	items = append(items, components.MenuItem{Key: '0', Label: "返回", ID: "back"})
	v.step = routingConfigPreset
	v.subMenu = components.NewMenu("选择预设", items)
	return nil
}

func (v *RoutingView) showOutboundMenu() tea.Cmd {
	var items []components.MenuItem
	key := '1'
	for _, raw := range v.model.Store().SingBox.Outbounds {
		h, err := store.ParseOutboundHeader(raw)
		if err != nil {
			continue
		}
		items = append(items, components.MenuItem{
			Key:   rune(key),
			Label: fmt.Sprintf("%s (%s)", h.Tag, h.Type),
			ID:    h.Tag,
		})
		key++
	}
	items = append(items, components.MenuItem{Key: '0', Label: "返回", ID: "back"})
	v.step = routingConfigOutbound
	v.subMenu = components.NewMenu("选择出站", items)
	return nil
}

func (v *RoutingView) showChainDeleteMenu() tea.Cmd {
	chains := v.listChains()
	if len(chains) == 0 {
		return func() tea.Msg {
			return routingActionDoneMsg{result: "暂无链式代理节点"}
		}
	}
	items := make([]components.MenuItem, 0, len(chains)+1)
	for i, c := range chains {
		key := rune('1' + i)
		items = append(items, components.MenuItem{
			Key:   key,
			Label: fmt.Sprintf("%s → %s:%d", c.Tag, c.Server, c.ServerPort),
			ID:    c.Tag,
		})
	}
	items = append(items, components.MenuItem{Key: '0', Label: "返回", ID: "back"})
	v.step = routingChainDeleteSelect
	v.subMenu = components.NewMenu("删除节点", items)
	return nil
}

func (v *RoutingView) showError(msg string) tea.Cmd {
	v.step = routingResult
	return func() tea.Msg {
		return tui.ShowOverlayMsg{Overlay: components.NewResult(msg)}
	}
}

// --- Action methods ---

type routingActionDoneMsg struct{ result string }

func (v *RoutingView) listChains() []routing.ChainOutbound {
	var chains []routing.ChainOutbound
	for _, raw := range v.model.Store().SingBox.Outbounds {
		h, err := store.ParseOutboundHeader(raw)
		if err != nil || h.Type != "socks" {
			continue
		}
		var co routing.ChainOutbound
		if err := json.Unmarshal(raw, &co); err == nil {
			chains = append(chains, co)
		}
	}
	return chains
}

func (v *RoutingView) doChainView() tea.Msg {
	chains := v.listChains()
	if len(chains) == 0 {
		return routingActionDoneMsg{result: "暂无链式代理节点"}
	}
	var sb strings.Builder
	sb.WriteString("链式代理节点\n\n")
	for _, c := range chains {
		sb.WriteString(fmt.Sprintf("  %s → %s:%d\n", c.Tag, c.Server, c.ServerPort))
	}
	return routingActionDoneMsg{result: sb.String()}
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

func (v *RoutingView) doChainRemove(tag string) tea.Msg {
	if err := routing.RemoveChain(v.model.Store(), tag); err != nil {
		return routingActionDoneMsg{result: "删除失败: " + err.Error()}
	}
	routing.SyncDNS(v.model.Store(), nil, "ipv4_only")
	if err := v.model.Store().Apply(); err != nil {
		return routingActionDoneMsg{result: "保存失败: " + err.Error()}
	}
	return routingActionDoneMsg{result: fmt.Sprintf("链式代理已删除: %s", tag)}
}

func (v *RoutingView) doConfigRoute(userName string, preset routing.Preset, outbound string) tea.Msg {
	s := v.model.Store()
	rule := routing.PresetToRule(preset, userName, outbound)
	if err := routing.SetRule(s, userName, rule); err != nil {
		return routingActionDoneMsg{result: "添加规则失败: " + err.Error()}
	}
	routing.SyncDNS(s, nil, "ipv4_only")
	routing.SyncRouteRules(s)
	if err := s.Apply(); err != nil {
		return routingActionDoneMsg{result: "保存失败: " + err.Error()}
	}
	return routingActionDoneMsg{
		result: fmt.Sprintf("已添加分流规则\n\n  用户: %s\n  预设: %s\n  出站: %s",
			userName, preset.Name, outbound),
	}
}

func (v *RoutingView) doDirectOutbound(strategy string) tea.Msg {
	s := v.model.Store()
	routing.SyncDNS(s, nil, strategy)
	routing.SyncRouteRules(s)
	if err := s.Apply(); err != nil {
		return routingActionDoneMsg{result: "保存失败: " + err.Error()}
	}
	label := strategy
	if label == "" {
		label = "AsIs (系统默认)"
	}
	return routingActionDoneMsg{result: fmt.Sprintf("直连出口已设置: %s", label)}
}

func (v *RoutingView) doTestDomain(userName, domain string) tea.Msg {
	result := routing.TestDomain(v.model.Store(), userName, domain)
	if len(result.MatchedRules) == 0 {
		return routingActionDoneMsg{
			result: fmt.Sprintf("测试分流: %s @ %s\n\n无匹配规则 (将走默认出站)", domain, userName),
		}
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("测试分流: %s @ %s\n\n", domain, userName))
	for _, r := range result.MatchedRules {
		sb.WriteString(fmt.Sprintf("  → %s  [%s: %s]\n", r.Outbound, r.MatchBy, r.Value))
	}
	return routingActionDoneMsg{result: sb.String()}
}
