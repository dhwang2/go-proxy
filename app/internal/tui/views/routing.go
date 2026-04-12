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
	routingChainMenu
	routingChainAddInput
	routingChainDeleteSelect
	routingConfigMenu
	routingConfigAddUser
	routingConfigAddPresetInput
	routingConfigAddOutbound
	routingConfigDeleteUser
	routingConfigDeleteInput
	routingConfigModifyUser
	routingConfigModifyInput
	routingConfigModifyOutbound
	routingDirect
	routingTestUser
	routingTestDomain
	routingResult
)

type RoutingView struct {
	tui.SplitViewBase
	subMenu tui.MenuModel
	step    routingStep

	activeMenu     string
	selectedUser   string
	selectedPreset []routing.Preset
	selectedIndex  []int
}

func NewRoutingView(model *tui.Model) *RoutingView {
	v := &RoutingView{}
	v.Model = model
	v.Menu = tui.NewMenu("", []tui.MenuItem{
		{Key: '1', Label: "󰌘 链式代理", ID: "chain"},
		{Key: '2', Label: "󰒓 配置分流", ID: "config"},
		{Key: '3', Label: "󰩟 直连出口", ID: "direct"},
		{Key: '4', Label: "󰙨 测试分流", ID: "test"},
	})
	return v
}

func (v *RoutingView) Name() string { return "routing" }

func (v *RoutingView) setFocus(left bool) {
	v.SetFocus(left, func(l bool) {
		v.subMenu = v.subMenu.SetDim(l)
	})
}

func (v *RoutingView) Init() tea.Cmd {
	v.step = routingMenu
	v.Menu = v.Menu.SetActiveID("")
	v.activeMenu = ""
	v.selectedUser = ""
	v.selectedPreset = nil
	v.selectedIndex = nil
	v.InitSplit()
	v.Split.SetMinWidths(14, 10)
	return nil
}

func (v *RoutingView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.ViewResizeMsg:
		v.HandleResize(msg)
		return v, nil
	case tui.SubSplitMouseMsg:
		return v, v.HandleMouse(msg)
	}
	if cmd, handled := v.HandleMenuNav(msg, v.step == routingMenu, false); handled {
		return v, cmd
	}
	inlineCmd, handled := v.UpdateInline(msg)
	if handled {
		return v, inlineCmd
	}
	switch msg := msg.(type) {
	case tui.MenuCursorChangeMsg:
		return v, nil
	case tui.MenuSelectMsg:
		if v.step == routingMenu || (v.Split.Enabled() && v.Split.FocusLeft()) {
			v.setFocus(false)
			return v, v.triggerMenuAction(msg.ID)
		}
		return v.handleMenuSelect(msg)
	case tui.InputResultMsg:
		return v.handleInput(msg)
	case routingActionDoneMsg:
		v.step = routingResult
		v.setFocus(false)
		return v, v.SetInline(components.NewResult(msg.result))
	case tui.ResultDismissedMsg:
		if v.activeMenu == "config" {
			v.step = routingConfigMenu
			return v, nil
		}
		v.step = routingMenu
		v.Menu = v.Menu.SetActiveID("")
		v.activeMenu = ""
		v.setFocus(true)
		return v, nil
	default:
		return v.handleDefault(msg)
	}
}

func (v *RoutingView) triggerMenuAction(id string) tea.Cmd {
	v.Menu = v.Menu.SetActiveID(id)
	v.activeMenu = id
	switch id {
	case "chain":
		v.step = routingChainMenu
		v.subMenu = tui.NewMenu("󰌘 链式代理", []tui.MenuItem{
			{Key: '1', Label: "󰐕 添加节点", ID: "add"},
			{Key: '2', Label: "󰍷 删除节点", ID: "delete"},
			{Key: '3', Label: "󰋼 查看节点", ID: "view"},
		})
		return nil
	case "config":
		v.step = routingConfigMenu
		v.subMenu = tui.NewMenu("󰒓 配置分流", []tui.MenuItem{
			{Key: '1', Label: "󰐕 一次性添加多条规则", ID: "add"},
			{Key: '2', Label: "󰍷 一次性删除多条规则", ID: "delete"},
			{Key: '3', Label: "󰏫 一次性修改多条规则", ID: "modify"},
			{Key: '4', Label: "󰍉 查看当前规则", ID: "view"},
		})
		return nil
	case "direct":
		v.step = routingDirect
		v.subMenu = tui.NewMenu("󰩟 直连出口", []tui.MenuItem{
			{Key: '1', Label: "󰩟 仅 IPv4", ID: "ipv4_only"},
			{Key: '2', Label: "󰩟 仅 IPv6", ID: "ipv6_only"},
			{Key: '3', Label: "󰩟 优先 IPv4", ID: "prefer_ipv4"},
			{Key: '4', Label: "󰩟 优先 IPv6", ID: "prefer_ipv6"},
			{Key: '5', Label: "󰩟 AsIs (系统默认)", ID: ""},
		})
		return nil
	case "test":
		return v.showUserMenu(routingTestUser)
	}
	return nil
}

func (v *RoutingView) handleMenuSelect(msg tui.MenuSelectMsg) (tui.View, tea.Cmd) {
	switch v.step {
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
	case routingConfigMenu:
		switch msg.ID {
		case "add":
			return v, v.showUserMenu(routingConfigAddUser)
		case "delete":
			return v, v.showUserMenu(routingConfigDeleteUser)
		case "modify":
			return v, v.showUserMenu(routingConfigModifyUser)
		case "view":
			return v, v.doConfigViewAll
		}
	case routingConfigAddUser:
		v.selectedUser = msg.ID
		v.step = routingConfigAddPresetInput
		return v, v.SetInline(components.NewTextInput(v.presetSelectionPrompt(), "1,2"))
	case routingConfigAddOutbound:
		outbound := msg.ID
		return v, func() tea.Msg { return v.doConfigRouteBatch(v.selectedUser, v.selectedPreset, outbound) }
	case routingConfigDeleteUser:
		v.selectedUser = msg.ID
		if len(derived.UserRoutes(v.Model.Store(), msg.ID)) == 0 {
			return v, func() tea.Msg { return routingActionDoneMsg{result: "暂无分流规则"} }
		}
		v.step = routingConfigDeleteInput
		return v, v.SetInline(components.NewTextInput(v.ruleSelectionPrompt(msg.ID), "1,2"))
	case routingConfigModifyUser:
		v.selectedUser = msg.ID
		if len(derived.UserRoutes(v.Model.Store(), msg.ID)) == 0 {
			return v, func() tea.Msg { return routingActionDoneMsg{result: "暂无分流规则"} }
		}
		v.step = routingConfigModifyInput
		return v, v.SetInline(components.NewTextInput(v.ruleSelectionPrompt(msg.ID), "1,2"))
	case routingConfigModifyOutbound:
		outbound := msg.ID
		return v, func() tea.Msg { return v.doModifyRoutes(v.selectedUser, v.selectedIndex, outbound) }
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
		switch v.step {
		case routingChainAddInput:
			v.step = routingChainMenu
		case routingConfigAddPresetInput:
			v.step = routingConfigMenu
		case routingConfigDeleteInput, routingConfigModifyInput:
			v.step = routingConfigMenu
		default:
			v.step = routingMenu
			v.Menu = v.Menu.SetActiveID("")
			v.activeMenu = ""
			v.setFocus(true)
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
	case routingConfigAddPresetInput:
		presets, err := parsePresetSelection(msg.Value)
		if err != nil {
			return v, v.showError(err.Error())
		}
		v.selectedPreset = presets
		return v, v.showOutboundMenu(routingConfigAddOutbound, "选择出站")
	case routingConfigDeleteInput:
		indexes, err := parseRuleSelection(msg.Value, len(derived.UserRoutes(v.Model.Store(), v.selectedUser)))
		if err != nil {
			return v, v.showError(err.Error())
		}
		return v, func() tea.Msg { return v.doDeleteRoutes(v.selectedUser, indexes) }
	case routingConfigModifyInput:
		indexes, err := parseRuleSelection(msg.Value, len(derived.UserRoutes(v.Model.Store(), v.selectedUser)))
		if err != nil {
			return v, v.showError(err.Error())
		}
		v.selectedIndex = indexes
		return v, v.showOutboundMenu(routingConfigModifyOutbound, "选择新的出站")
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
		case routingChainMenu, routingConfigMenu, routingDirect, routingTestUser:
			v.step = routingMenu
			v.Menu = v.Menu.SetActiveID("")
			v.activeMenu = ""
			v.setFocus(true)
			return v, nil
		case routingChainDeleteSelect:
			v.step = routingChainMenu
			return v, nil
		case routingConfigAddUser, routingConfigDeleteUser, routingConfigModifyUser, routingConfigAddOutbound, routingConfigModifyOutbound:
			v.step = routingConfigMenu
			return v, nil
		default:
			return v, tui.BackCmd
		}
	}
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if v.Split.Enabled() && v.step != routingMenu {
			if keyMsg.Type == tea.KeyLeft {
				v.setFocus(true)
				return v, nil
			}
			if keyMsg.Type == tea.KeyRight && (v.HasInline() || v.step != routingResult) {
				v.setFocus(false)
				return v, nil
			}
		}
	}
	var cmd tea.Cmd
	switch v.step {
	case routingMenu:
		v.Menu, cmd = v.Menu.Update(msg)
	default:
		if v.Split.Enabled() && v.Split.FocusLeft() {
			v.Menu, cmd = v.Menu.Update(msg)
		} else {
			v.subMenu, cmd = v.subMenu.Update(msg)
		}
	}
	return v, cmd
}

func (v *RoutingView) View() string {
	if !v.Split.Enabled() {
		if v.HasInline() {
			return v.ViewInline()
		}
		switch v.step {
		case routingChainMenu, routingChainDeleteSelect, routingConfigMenu, routingConfigAddUser,
			routingConfigAddOutbound, routingConfigDeleteUser, routingConfigModifyUser,
			routingConfigModifyOutbound, routingDirect, routingTestUser:
			return v.subMenu.View()
		default:
			return v.Menu.View()
		}
	}

	if v.step == routingMenu {
		return v.Menu.View()
	}

	menuContent := v.Menu.View()
	var detailContent string
	if v.HasInline() {
		tui.InSplitPanel = true
		detailContent = v.ViewInline()
		tui.InSplitPanel = false
	} else {
		switch v.step {
		case routingChainMenu, routingChainDeleteSelect, routingConfigMenu, routingConfigAddUser,
			routingConfigAddOutbound, routingConfigDeleteUser, routingConfigModifyUser,
			routingConfigModifyOutbound, routingDirect, routingTestUser:
			detailContent = v.subMenu.View()
		}
	}
	return v.Split.View(menuContent, detailContent)
}

func (v *RoutingView) showUserMenu(nextStep routingStep) tea.Cmd {
	users := derived.UserNames(v.Model.Store())
	if len(users) == 0 {
		return func() tea.Msg { return routingActionDoneMsg{result: "暂无用户"} }
	}
	items := make([]tui.MenuItem, 0, len(users))
	for i, userName := range users {
		key := rune('1' + i)
		if i >= 9 {
			key = rune('a' + i - 9)
		}
		items = append(items, tui.MenuItem{Key: key, Label: userName, ID: userName})
	}
	v.step = nextStep
	v.subMenu = tui.NewMenu("选择用户", items)
	return nil
}

func (v *RoutingView) showOutboundMenu(nextStep routingStep, title string) tea.Cmd {
	var items []tui.MenuItem
	key := '1'
	for _, raw := range v.Model.Store().SingBox.Outbounds {
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
	v.step = nextStep
	v.subMenu = tui.NewMenu(title, items)
	return nil
}

func (v *RoutingView) showChainDeleteMenu() tea.Cmd {
	chains := v.listChains()
	if len(chains) == 0 {
		return func() tea.Msg { return routingActionDoneMsg{result: "暂无链式代理节点"} }
	}
	items := make([]tui.MenuItem, 0, len(chains))
	for i, chain := range chains {
		items = append(items, tui.MenuItem{
			Key:   rune('1' + i),
			Label: fmt.Sprintf("%s → %s:%d", chain.Tag, chain.Server, chain.ServerPort),
			ID:    chain.Tag,
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

type routingActionDoneMsg struct{ result string }

func (v *RoutingView) listChains() []routing.ChainOutbound {
	return routing.ListChains(v.Model.Store())
}

func (v *RoutingView) doChainView() tea.Msg {
	chains := v.listChains()
	if len(chains) == 0 {
		return routingActionDoneMsg{result: "暂无链式代理节点"}
	}
	var sb strings.Builder
	sb.WriteString("链式代理节点\n\n")
	for _, chain := range chains {
		sb.WriteString(fmt.Sprintf("  %s → %s:%d\n", chain.Tag, chain.Server, chain.ServerPort))
	}
	return routingActionDoneMsg{result: sb.String()}
}

func (v *RoutingView) doChainAdd(server string, port int, username, password, display string) tea.Msg {
	tag, err := routing.AddChain(v.Model.Store(), server, port, username, password)
	if err != nil {
		return routingActionDoneMsg{result: "添加失败: " + err.Error()}
	}
	routing.SyncDNS(v.Model.Store(), nil, "ipv4_only")
	if err := v.Model.Store().Apply(); err != nil {
		return routingActionDoneMsg{result: "保存失败: " + err.Error()}
	}
	return routingActionDoneMsg{result: fmt.Sprintf("链式代理已添加: %s (%s)", tag, display)}
}

func (v *RoutingView) doChainRemove(tag string) tea.Msg {
	if err := routing.RemoveChain(v.Model.Store(), tag); err != nil {
		return routingActionDoneMsg{result: "删除失败: " + err.Error()}
	}
	routing.SyncDNS(v.Model.Store(), nil, "ipv4_only")
	if err := v.Model.Store().Apply(); err != nil {
		return routingActionDoneMsg{result: "保存失败: " + err.Error()}
	}
	return routingActionDoneMsg{result: fmt.Sprintf("链式代理已删除: %s", tag)}
}

func (v *RoutingView) doConfigViewAll() tea.Msg {
	var sb strings.Builder
	sb.WriteString("当前分流规则\n\n")
	users := derived.AllRoutedUsers(v.Model.Store())
	if len(users) == 0 {
		sb.WriteString("暂无分流规则")
		return routingActionDoneMsg{result: sb.String()}
	}
	for _, userName := range users {
		sb.WriteString(v.renderUserRules(userName))
		sb.WriteString("\n")
	}
	return routingActionDoneMsg{result: strings.TrimSpace(sb.String())}
}

func (v *RoutingView) doConfigRouteBatch(userName string, presets []routing.Preset, outbound string) tea.Msg {
	s := v.Model.Store()
	rules := make([]store.UserRouteRule, 0, len(presets))
	names := make([]string, 0, len(presets))
	for _, preset := range presets {
		rules = append(rules, routing.PresetToRule(preset, userName, outbound))
		names = append(names, preset.Name)
	}
	count, err := routing.SetRules(s, userName, rules)
	if err != nil {
		return routingActionDoneMsg{result: "添加规则失败: " + err.Error()}
	}
	routing.SyncDNS(s, nil, "ipv4_only")
	routing.SyncRouteRules(s)
	if err := s.Apply(); err != nil {
		return routingActionDoneMsg{result: "保存失败: " + err.Error()}
	}
	return routingActionDoneMsg{
		result: fmt.Sprintf("已添加 %d 条分流规则\n\n  用户: %s\n  规则: %s\n  出站: %s", count, userName, strings.Join(names, ", "), outbound),
	}
}

func (v *RoutingView) doDeleteRoutes(userName string, indexes []int) tea.Msg {
	s := v.Model.Store()
	count, err := routing.DeleteUserRulesByIndex(s, userName, indexes)
	if err != nil {
		return routingActionDoneMsg{result: "删除规则失败: " + err.Error()}
	}
	routing.SyncDNS(s, nil, "ipv4_only")
	routing.SyncRouteRules(s)
	if err := s.Apply(); err != nil {
		return routingActionDoneMsg{result: "保存失败: " + err.Error()}
	}
	return routingActionDoneMsg{result: fmt.Sprintf("已删除 %d 条分流规则\n\n用户: %s", count, userName)}
}

func (v *RoutingView) doModifyRoutes(userName string, indexes []int, outbound string) tea.Msg {
	s := v.Model.Store()
	count, err := routing.ReplaceUserRuleOutbounds(s, userName, indexes, outbound)
	if err != nil {
		return routingActionDoneMsg{result: "修改规则失败: " + err.Error()}
	}
	routing.SyncDNS(s, nil, "ipv4_only")
	routing.SyncRouteRules(s)
	if err := s.Apply(); err != nil {
		return routingActionDoneMsg{result: "保存失败: " + err.Error()}
	}
	return routingActionDoneMsg{result: fmt.Sprintf("已修改 %d 条分流规则\n\n用户: %s\n出站: %s", count, userName, outbound)}
}

func (v *RoutingView) doDirectOutbound(strategy string) tea.Msg {
	s := v.Model.Store()
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
	result := routing.TestDomain(v.Model.Store(), userName, domain)
	if len(result.MatchedRules) == 0 {
		return routingActionDoneMsg{result: fmt.Sprintf("测试分流: %s @ %s\n\n无匹配规则 (将走默认出站)", domain, userName)}
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("测试分流: %s @ %s\n\n", domain, userName))
	for _, rule := range result.MatchedRules {
		sb.WriteString(fmt.Sprintf("  → %s  [%s: %s]\n", rule.Outbound, rule.MatchBy, rule.Value))
	}
	return routingActionDoneMsg{result: sb.String()}
}

func (v *RoutingView) presetSelectionPrompt() string {
	var sb strings.Builder
	sb.WriteString("输入规则编号，支持逗号分隔\n")
	for _, option := range routing.AddMenuPresetOptions() {
		sb.WriteString(fmt.Sprintf("%s. %s\n", option.Key, option.Preset.Label))
	}
	return strings.TrimSpace(sb.String())
}

func (v *RoutingView) ruleSelectionPrompt(userName string) string {
	return "输入规则编号，支持 1,2 或 all\n\n" + v.renderUserRules(userName)
}

func (v *RoutingView) renderUserRules(userName string) string {
	rules := derived.UserRoutes(v.Model.Store(), userName)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("用户: %s\n", userName))
	if len(rules) == 0 {
		sb.WriteString("  暂无分流规则")
		return sb.String()
	}
	for i, rule := range rules {
		sb.WriteString(fmt.Sprintf("  %d. %s -> %s\n", i+1, routing.UserRouteLabel(rule), routing.OutboundLabel(rule.Outbound)))
	}
	return strings.TrimRight(sb.String(), "\n")
}

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

func parsePresetSelection(input string) ([]routing.Preset, error) {
	tokens := splitMultiInput(input)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("请输入至少一条规则")
	}
	options := routing.AddMenuPresetOptions()
	byKey := make(map[string]routing.Preset, len(options))
	for _, option := range options {
		byKey[option.Key] = option.Preset
	}
	seen := make(map[string]bool, len(tokens))
	result := make([]routing.Preset, 0, len(tokens))
	for _, token := range tokens {
		preset, ok := byKey[token]
		if !ok {
			return nil, fmt.Errorf("无效规则编号: %s", token)
		}
		if seen[token] {
			continue
		}
		seen[token] = true
		result = append(result, preset)
	}
	return result, nil
}

func parseRuleSelection(input string, total int) ([]int, error) {
	tokens := splitMultiInput(input)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("请输入规则编号")
	}
	if len(tokens) == 1 && (tokens[0] == "all" || tokens[0] == "*") {
		return routing.ExpandRuleIndexes(total, nil, true), nil
	}
	seen := make(map[int]bool, len(tokens))
	indexes := make([]int, 0, len(tokens))
	for _, token := range tokens {
		idx, err := strconv.Atoi(token)
		if err != nil || idx < 1 || idx > total {
			return nil, fmt.Errorf("无效规则编号: %s", token)
		}
		if seen[idx] {
			continue
		}
		seen[idx] = true
		indexes = append(indexes, idx)
	}
	return routing.ExpandRuleIndexes(total, indexes, false), nil
}

func splitMultiInput(input string) []string {
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" {
		return nil
	}
	return strings.FieldsFunc(input, func(r rune) bool {
		return r == ',' || r == ' ' || r == '，' || r == '、'
	})
}
