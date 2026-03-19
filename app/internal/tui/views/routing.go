package views

import (
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
	tui.InlineState
	model   *tui.Model
	menu    tui.MenuModel
	subMenu tui.MenuModel
	step    routingStep
	// State for multi-step flows
	selectedUser    string
	selectedPreset  routing.Preset
	pendingChainTag string
}

func NewRoutingView(model *tui.Model) *RoutingView {
	v := &RoutingView{model: model}
	v.menu = tui.NewMenu("󰛳 分流管理", []tui.MenuItem{
		{Key: '1', Label: "󰌘 链式代理", ID: "chain"},
		{Key: '2', Label: "󰒓 配置分流", ID: "config"},
		{Key: '3', Label: "󰩟 直连出口", ID: "direct"},
		{Key: '4', Label: "󰙨 测试分流", ID: "test"},
	})
	return v
}

func (v *RoutingView) Name() string { return "routing" }

func (v *RoutingView) Init() tea.Cmd {
	v.step = routingMenu
	return nil
}

func (v *RoutingView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	inlineCmd, handled := v.UpdateInline(msg)
	if handled {
		return v, inlineCmd
	}
	switch msg := msg.(type) {
	case tui.MenuSelectMsg:
		return v.handleMenuSelect(msg)

	case tui.InputResultMsg:
		return v.handleInput(msg)

	case routingActionDoneMsg:
		v.step = routingResult
		return v, v.SetInline(components.NewResult(msg.result))

	case tui.ResultDismissedMsg:
		v.step = routingMenu
		return v, nil

	default:
		return v.handleDefault(msg)
	}
}

func (v *RoutingView) handleMenuSelect(msg tui.MenuSelectMsg) (tui.View, tea.Cmd) {
	switch v.step {
	case routingMenu:
		switch msg.ID {
		case "chain":
			v.step = routingChainMenu
			v.subMenu = tui.NewMenu("󰌘 链式代理", []tui.MenuItem{
				{Key: '1', Label: "󰐕 添加节点", ID: "add"},
				{Key: '2', Label: "󰍷 删除节点", ID: "delete"},
				{Key: '3', Label: "󰋼 查看节点", ID: "view"},
			})
			return v, nil
		case "config":
			return v, v.showUserMenu(routingConfigUser)
		case "direct":
			v.step = routingDirect
			v.subMenu = tui.NewMenu("󰩟 直连出口", []tui.MenuItem{
				{Key: '1', Label: "󰩟 仅 IPv4", ID: "ipv4_only"},
				{Key: '2', Label: "󰩟 仅 IPv6", ID: "ipv6_only"},
				{Key: '3', Label: "󰩟 优先 IPv4", ID: "prefer_ipv4"},
				{Key: '4', Label: "󰩟 优先 IPv6", ID: "prefer_ipv6"},
				{Key: '5', Label: "󰩟 AsIs (系统默认)", ID: ""},
			})
			return v, nil
		case "test":
			return v, v.showUserMenu(routingTestUser)
		}

	case routingChainMenu:
		switch msg.ID {
		case "add":
			v.step = routingChainAddInput
			return v, v.SetInline(components.NewTextInput("链式代理 (地址:端口:用户:密码):", ""))
		case "delete":
			return v, v.showChainDeleteMenu()
		case "view":
			return v, v.doChainView
		}

	case routingChainDeleteSelect:
		tag := msg.ID
		return v, func() tea.Msg { return v.doChainRemove(tag) }

	case routingConfigUser:
		v.selectedUser = msg.ID
		return v, v.showPresetMenu()

	case routingConfigPreset:
		presets := routing.BuiltinPresets()
		idx, _ := strconv.Atoi(msg.ID)
		if idx >= 0 && idx < len(presets) {
			v.selectedPreset = presets[idx]
		}
		return v, v.showOutboundMenu()

	case routingConfigOutbound:
		outbound := msg.ID
		return v, func() tea.Msg {
			return v.doConfigRoute(v.selectedUser, v.selectedPreset, outbound)
		}

	case routingDirect:
		strategy := msg.ID
		return v, func() tea.Msg { return v.doDirectOutbound(strategy) }

	case routingTestUser:
		v.selectedUser = msg.ID
		v.step = routingTestDomain
		return v, v.SetInline(components.NewTextInput("测试域名:", "google.com"))
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
		server, port, username, password, err := parseChainInput(msg.Value)
		if err != nil {
			return v, v.showError(err.Error())
		}
		display := msg.Value
		return v, func() tea.Msg { return v.doChainAdd(server, port, username, password, display) }

	case routingTestDomain:
		domain := msg.Value
		user := v.selectedUser
		return v, func() tea.Msg { return v.doTestDomain(user, domain) }
	}

	return v, nil
}

func (v *RoutingView) handleDefault(msg tea.Msg) (tui.View, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.Type == tea.KeyEsc {
		switch v.step {
		case routingChainMenu:
			v.step = routingMenu
			return v, nil
		case routingChainDeleteSelect:
			v.step = routingChainMenu
			return v, nil
		case routingConfigUser:
			v.step = routingMenu
			return v, nil
		case routingConfigPreset:
			return v, v.showUserMenu(routingConfigUser)
		case routingConfigOutbound:
			return v, v.showPresetMenu()
		case routingDirect:
			v.step = routingMenu
			return v, nil
		case routingTestUser:
			v.step = routingMenu
			return v, nil
		default:
			return v, tui.BackCmd
		}
	}
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
	if v.HasInline() {
		return v.ViewInline()
	}
	switch v.step {
	case routingChainMenu, routingChainDeleteSelect, routingConfigUser,
		routingConfigPreset, routingConfigOutbound, routingDirect, routingTestUser:
		return tui.RenderSubMenuBody(v.subMenu.View(), v.model.ContentWidth())
	default:
		return tui.RenderSubMenuBody(v.menu.View(), v.model.ContentWidth())
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
	items := make([]tui.MenuItem, 0, len(users)+1)
	for i, u := range users {
		key := rune('1' + i)
		if i >= 9 {
			key = rune('a' + i - 9)
		}
		items = append(items, tui.MenuItem{Key: key, Label: u, ID: u})
	}
	v.step = nextStep
	v.subMenu = tui.NewMenu("选择用户", items)
	return nil
}

func (v *RoutingView) showPresetMenu() tea.Cmd {
	presets := routing.BuiltinPresets()
	items := make([]tui.MenuItem, 0, len(presets)+1)
	for i, p := range presets {
		key := rune('1' + i)
		if i >= 9 {
			key = rune('a' + i - 9)
		}
		items = append(items, tui.MenuItem{Key: key, Label: p.Label, ID: strconv.Itoa(i)})
	}
	v.step = routingConfigPreset
	v.subMenu = tui.NewMenu("选择预设", items)
	return nil
}

func (v *RoutingView) showOutboundMenu() tea.Cmd {
	var items []tui.MenuItem
	key := '1'
	for _, raw := range v.model.Store().SingBox.Outbounds {
		h, err := store.ParseOutboundHeader(raw)
		if err != nil {
			continue
		}
		items = append(items, tui.MenuItem{
			Key:   rune(key),
			Label: fmt.Sprintf("%s (%s)", h.Tag, h.Type),
			ID:    h.Tag,
		})
		key++
	}
	v.step = routingConfigOutbound
	v.subMenu = tui.NewMenu("选择出站", items)
	return nil
}

func (v *RoutingView) showChainDeleteMenu() tea.Cmd {
	chains := v.listChains()
	if len(chains) == 0 {
		return func() tea.Msg {
			return routingActionDoneMsg{result: "暂无链式代理节点"}
		}
	}
	items := make([]tui.MenuItem, 0, len(chains)+1)
	for i, c := range chains {
		key := rune('1' + i)
		items = append(items, tui.MenuItem{
			Key:   key,
			Label: fmt.Sprintf("%s → %s:%d", c.Tag, c.Server, c.ServerPort),
			ID:    c.Tag,
		})
	}
	v.step = routingChainDeleteSelect
	v.subMenu = tui.NewMenu("删除节点", items)
	return nil
}

func (v *RoutingView) showError(msg string) tea.Cmd {
	v.step = routingResult
	return v.SetInline(components.NewResult(msg))
}

// --- Action methods ---

type routingActionDoneMsg struct{ result string }

func (v *RoutingView) listChains() []routing.ChainOutbound {
	return routing.ListChains(v.model.Store())
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

func (v *RoutingView) doChainAdd(server string, port int, username, password, display string) tea.Msg {
	tag, err := routing.AddChain(v.model.Store(), server, port, username, password)
	if err != nil {
		return routingActionDoneMsg{result: "添加失败: " + err.Error()}
	}
	routing.SyncDNS(v.model.Store(), nil, "ipv4_only")
	if err := v.model.Store().Apply(); err != nil {
		return routingActionDoneMsg{result: "保存失败: " + err.Error()}
	}
	return routingActionDoneMsg{result: fmt.Sprintf("链式代理已添加: %s (%s)", tag, display)}
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

// parseChainInput parses the chain proxy input in format: address:port:username:password
// Username and password are optional. Minimum format: address:port.
func parseChainInput(input string) (server string, port int, username, password string, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", 0, "", "", fmt.Errorf("输入不能为空")
	}

	parts := strings.SplitN(input, ":", 4)
	if len(parts) < 2 {
		return "", 0, "", "", fmt.Errorf("格式错误，请使用 地址:端口[:用户:密码]")
	}

	server = parts[0]
	port, convErr := strconv.Atoi(parts[1])
	if convErr != nil || port <= 0 || port > 65535 {
		return "", 0, "", "", fmt.Errorf("端口号无效")
	}

	if len(parts) >= 3 {
		username = parts[2]
	}
	if len(parts) >= 4 {
		password = parts[3]
	}

	return server, port, username, password, nil
}
