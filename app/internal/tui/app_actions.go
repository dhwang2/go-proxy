package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dhwang2/go-proxy/internal/config"
	corepkg "github.com/dhwang2/go-proxy/internal/core"
	netpkg "github.com/dhwang2/go-proxy/internal/network"
	protopkg "github.com/dhwang2/go-proxy/internal/protocol"
	routingpkg "github.com/dhwang2/go-proxy/internal/routing"
	"github.com/dhwang2/go-proxy/internal/service"
	"github.com/dhwang2/go-proxy/internal/store"
	subpkg "github.com/dhwang2/go-proxy/internal/subscription"
	"github.com/dhwang2/go-proxy/internal/tui/components"
	updatepkg "github.com/dhwang2/go-proxy/internal/update"
	userpkg "github.com/dhwang2/go-proxy/internal/user"
)

const (
	selectionProtocolRemove = "protocol-remove"
	selectionUserRename     = "user-rename"
	selectionUserDelete     = "user-delete"
	selectionRouteSetUser   = "route-set-user"
	selectionRouteSetOut    = "route-set-outbound"
	selectionRouteClearUser = "route-clear-user"
	selectionRouteTestUser  = "route-test-user"
	selectionRouteDirect    = "route-direct-user"
	selectionChainRemove    = "chain-remove"
)

const (
	inputUserAdd       = "user-add"
	inputUserRename    = "user-rename"
	inputRouteRuleSets = "route-rule-sets"
	inputChainTag      = "chain-tag"
	inputChainServer   = "chain-server"
	inputChainPort     = "chain-port"
	inputProtocolSNI   = "protocol-sni"
	inputProtoPort     = "proto-port"
	inputProtoSNI      = "proto-sni"
	inputProtoPassword = "proto-password"
)

const (
	selectionSSMethod  = "ss-method"
	selectionSnellObfs = "snell-obfs"
)

const (
	confirmSnellIPv6 = "confirm-snell-ipv6"
	confirmSnellUDP  = "confirm-snell-udp"
)

const (
	confirmProtocolRemove = "confirm-protocol-remove"
	confirmUserDelete     = "confirm-user-delete"
	confirmChainRemove    = "confirm-chain-remove"
	confirmCoreUpdate     = "confirm-core-update"
	confirmSelfUpdate     = "confirm-release-update"
	confirmUninstall      = "confirm-uninstall"
)

type selectionChoice struct {
	Label string
	Value string
}

func newMenuList(items []MenuItem) components.MenuList {
	out := make([]components.Item, 0, len(items))
	for _, item := range items {
		out = append(out, components.Item{
			Key:         item.Key,
			Title:       item.Title,
			Description: item.Description,
			Value:       item.Title,
		})
	}
	return components.NewMenuList(out)
}

func (m appModel) isMenuState() bool {
	return m.currentMenu() != nil
}

func (m *appModel) currentMenu() *components.MenuList {
	switch m.state {
	case StateMainMenu:
		return &m.mainMenu
	case StateProtocolInstallMenu:
		return &m.protocolInstallMenu
	case StateUserMenu:
		return &m.userMenu
	case StateRoutingMenu:
		return &m.routingMenu
	case StateRoutingRulesMenu:
		return &m.routingRulesMenu
	case StateRoutingChainMenu:
		return &m.routingChainMenu
	case StateServiceManagement:
		return &m.serviceMenu
	case StateLogsMenu:
		return &m.logsMenu
	case StateConfigMenu:
		return &m.configMenu
	case StateCoreMenu:
		return &m.coreMenu
	case StateNetworkMenu:
		return &m.networkMenu
	case StateBBRMenu:
		return &m.bbrMenu
	case StateFirewallMenu:
		return &m.firewallMenu
	case StateSelectionMenu:
		return &m.selectionMenu
	default:
		return nil
	}
}

func (m appModel) updateMenuState(key string, spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	menu := m.currentMenu()
	if menu == nil {
		return m, spinCmd
	}
	switch key {
	case "up", "k":
		menu.MoveUp()
		return m, spinCmd
	case "down", "j":
		menu.MoveDown()
		return m, spinCmd
	case "enter":
		return m.selectMenu(menu.Selected().Key, spinCmd)
	case "esc":
		return m.goBack(spinCmd)
	case "r":
		m.loading = true
		m.spinner.SetMessage("刷新服务状态")
		return m, tea.Batch(spinCmd, m.loadStatusCmd("刷新服务状态"))
	}
	if key == "q" {
		if m.state == StateMainMenu {
			return m, tea.Quit
		}
		return m.goBack(spinCmd)
	}
	return m, spinCmd
}

func (m appModel) selectMenu(key string, spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	switch m.state {
	case StateMainMenu:
		return m.selectMainMenuAction(key, spinCmd)
	case StateProtocolInstallMenu:
		return m.selectProtocolInstallAction(key, spinCmd)
	case StateUserMenu:
		return m.selectUserMenuAction(key, spinCmd)
	case StateRoutingMenu:
		return m.selectRoutingMenuAction(key, spinCmd)
	case StateRoutingRulesMenu:
		return m.selectRoutingRulesAction(key, spinCmd)
	case StateRoutingChainMenu:
		return m.selectRoutingChainAction(key, spinCmd)
	case StateServiceManagement:
		return m.selectServiceMenuAction(key, spinCmd)
	case StateLogsMenu:
		return m.selectLogsAction(key, spinCmd)
	case StateConfigMenu:
		return m.selectConfigAction(key, spinCmd)
	case StateCoreMenu:
		return m.selectCoreAction(key, spinCmd)
	case StateNetworkMenu:
		return m.selectNetworkAction(key, spinCmd)
	case StateBBRMenu:
		return m.selectBBRAction(key, spinCmd)
	case StateFirewallMenu:
		return m.selectFirewallAction(key, spinCmd)
	case StateSelectionMenu:
		return m.selectSelectionAction(key, spinCmd)
	default:
		return m, spinCmd
	}
}

func (m appModel) goBack(spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	switch m.state {
	case StateMainMenu:
		return m, tea.Quit
	case StateProtocolInstallMenu, StateUserMenu, StateRoutingMenu, StateConfigMenu, StateLogsMenu, StateCoreMenu, StateNetworkMenu:
		m.state = StateMainMenu
	case StateRoutingRulesMenu, StateRoutingChainMenu:
		m.state = StateRoutingMenu
	case StateServiceManagement:
		m.state = StateMainMenu
	case StateBBRMenu, StateFirewallMenu:
		m.state = StateNetworkMenu
	case StateSelectionMenu, StateInputPrompt, StateConfirmPrompt, StateTextView, StateConfigView, StateLogsView:
		m.state = m.returnState
	default:
		m.state = StateMainMenu
	}
	return m, spinCmd
}

func (m appModel) updateTextState(key string, spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	if key == "q" || key == "esc" || key == "0" {
		m.state = m.returnState
		return m, spinCmd
	}
	return m, spinCmd
}

func (m appModel) updateInputState(msg tea.KeyMsg, spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	m.input = m.input.Update(msg)
	if m.input.Cancelled() {
		m.state = m.returnState
		toastCmd := (&m).setToast("已取消", false)
		return m, batchCmd(spinCmd, toastCmd)
	}
	if m.input.Submitted() {
		return m.handleInputSubmit(spinCmd)
	}
	return m, spinCmd
}

func (m appModel) updateConfirmState(key string, spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	next, answered, result := m.confirm.Update(key)
	m.confirm = next
	if !answered {
		if key == "esc" || key == "q" || key == "0" {
			m.state = m.returnState
			toastCmd := (&m).setToast("已取消", false)
			return m, batchCmd(spinCmd, toastCmd)
		}
		return m, spinCmd
	}
	// For option-style confirms (snell IPv6/UDP), both Yes and No proceed.
	if m.isOptionConfirm() {
		return m.handleConfirmSubmit(spinCmd)
	}
	if !result {
		m.state = m.returnState
		toastCmd := (&m).setToast("已取消", false)
		return m, batchCmd(spinCmd, toastCmd)
	}
	return m.handleConfirmSubmit(spinCmd)
}

func (m appModel) isOptionConfirm() bool {
	return m.confirmMode == confirmSnellIPv6 || m.confirmMode == confirmSnellUDP
}

func (m appModel) selectMainMenuAction(key string, spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	switch key {
	case "0":
		return m, tea.Quit
	case "1":
		m.state = StateProtocolInstallMenu
	case "2":
		return m.openProtocolRemoveSelection(spinCmd)
	case "3":
		m.state = StateUserMenu
	case "4":
		m.state = StateRoutingMenu
	case "5":
		m.state = StateServiceManagement
	case "6":
		return m.showSubscriptions(subpkg.FormatAll, "", spinCmd)
	case "7":
		m.state = StateConfigMenu
	case "8":
		m.state = StateLogsMenu
	case "9":
		m.state = StateCoreMenu
	case "10":
		m.state = StateNetworkMenu
	case "11":
		m = m.openConfirm("脚本更新", "检查并应用脚本更新?", confirmSelfUpdate, StateMainMenu)
	case "12":
		m = m.openConfirm("卸载服务", "确认彻底卸载所有组件?", confirmUninstall, StateMainMenu)
	}
	return m, spinCmd
}

func (m appModel) selectProtocolInstallAction(key string, spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	if key == "0" {
		m.state = StateMainMenu
		return m, spinCmd
	}
	item := m.protocolInstallMenu.Selected()
	protoName := strings.ToLower(item.Value)

	// Start the install prompt chain: first ask for port.
	m.actionContext["protocol"] = protoName
	defaultPort := protopkg.DefaultPort(protoName)
	m = m.openInput("安装 "+protoName,
		fmt.Sprintf("监听端口 [默认: %d]", defaultPort),
		"回车使用默认端口",
		inputProtoPort, StateProtocolInstallMenu)
	return m, spinCmd
}

func (m appModel) executeProtocolInstall(protoName, sni string, spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	m.loading = true
	m.spinner = components.NewSpinner("正在安装 " + protoName + "...")

	// Build install spec from collected actionContext.
	spec := protopkg.InstallSpec{Protocol: protoName, SNI: sni}
	if portStr := m.actionContext["port"]; portStr != "" {
		spec.Port, _ = strconv.Atoi(portStr)
	}
	if method := m.actionContext["method"]; method != "" {
		spec.Method = method
	}
	if password := m.actionContext["password"]; password != "" {
		spec.Secret = password
	}

	// Snell-specific options.
	snellOpts := map[string]string{}
	if v := m.actionContext["ipv6"]; v != "" {
		snellOpts["ipv6"] = v
	}
	if v := m.actionContext["udp"]; v != "" {
		snellOpts["udp"] = v
	}
	if v := m.actionContext["obfs"]; v != "" {
		snellOpts["obfs"] = v
	}

	installCmd := m.protocolInstallCmd(protoName, spec, snellOpts)
	return m, tea.Batch(spinCmd, m.spinner.Init(), installCmd)
}

func (m appModel) protocolInstallCmd(protoName string, spec protopkg.InstallSpec, snellOpts map[string]string) tea.Cmd {
	st := m.store
	svc := m.services
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		var lines []string

		// Step 1: Ensure required binaries are present.
		binResults := corepkg.EnsureProtocolBinaries(ctx, "/etc/go-proxy", protoName)
		for _, br := range binResults {
			if br.Err != nil {
				return protocolInstallDoneMsg{proto: protoName, err: br.Err}
			}
			if br.Downloaded {
				lines = append(lines, fmt.Sprintf("✓ %s 已下载", br.Name))
			}
		}

		// Step 2: Apply snell-specific options before install.
		if protoName == "snell" && st.SnellConf != nil {
			for k, v := range snellOpts {
				if v != "" {
					st.SnellConf.Values[k] = v
				}
			}
		}

		// Step 3: Install protocol config.
		result, err := protopkg.Install(st, spec)
		if err != nil {
			return protocolInstallDoneMsg{proto: protoName, err: err}
		}

		// Step 4: Apply changes (provision services, write config, restart).
		if result.Changed() {
			svc.EnsureServiceFiles(ctx)
			ops := svc.ApplyStore(ctx, st)
			for _, op := range ops {
				status := "✓"
				if op.Status == "failed" {
					status = "✗"
				}
				msg := op.Name + " " + op.Status
				if op.Message != "" {
					msg += " " + op.Message
				}
				lines = append(lines, status+" "+msg)
			}
		}

		// Step 5: Build detailed summary.
		lines = append(lines, "")
		lines = append(lines, protocolInstallSummary(st, protoName)...)

		body := strings.Join(lines, "\n")
		return protocolInstallDoneMsg{proto: protoName, body: body}
	}
}

func protocolInstallSummary(st *store.Store, proto string) []string {
	var lines []string
	rows := protopkg.List(st)
	for _, row := range rows {
		if row.Protocol != proto {
			continue
		}
		lines = append(lines, fmt.Sprintf("协议: %s", row.Protocol))
		lines = append(lines, fmt.Sprintf("标签: %s", row.Tag))
		lines = append(lines, fmt.Sprintf("端口: %d", row.Port))
		lines = append(lines, fmt.Sprintf("用户: %d", row.Users))
	}
	if len(lines) == 0 {
		lines = append(lines, "协议: "+proto)
	}
	return lines
}

func (m appModel) openProtocolRemoveSelection(spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	rows := protopkg.List(m.store)
	if len(rows) == 0 {
		toastCmd := (&m).setToast("无已安装协议", false)
		return m, batchCmd(spinCmd, toastCmd)
	}
	choices := make([]selectionChoice, 0, len(rows))
	for _, row := range rows {
		value := row.Tag
		if row.Protocol == "snell" {
			value = "snell"
		}
		label := fmt.Sprintf("%s · %s · %d", row.Protocol, row.Tag, row.Port)
		choices = append(choices, selectionChoice{Label: label, Value: value})
	}
	m = m.openSelection("卸载协议", "选择要卸载的协议", choices, selectionProtocolRemove, StateMainMenu)
	return m, spinCmd
}

func (m appModel) selectUserMenuAction(key string, spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	switch key {
	case "0":
		m.state = StateMainMenu
	case "1":
		rows := userpkg.ListGroups(m.store)
		m = m.openTextView("用户列表", renderUserTable(rows), StateUserMenu)
	case "2":
		m = m.openInput("添加用户", "用户名", "", inputUserAdd, StateUserMenu)
	case "3":
		return m.openUserSelection("重置用户", "选择要重置的用户", selectionUserRename, StateUserMenu)
	case "4":
		return m.openUserSelection("删除用户", "选择要删除的用户", selectionUserDelete, StateUserMenu)
	}
	return m, spinCmd
}

func (m appModel) selectRoutingMenuAction(key string, spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	switch key {
	case "0":
		m.state = StateMainMenu
	case "1":
		m.state = StateRoutingChainMenu
	case "2":
		m.state = StateRoutingRulesMenu
	case "3":
		return m.openUserSelection("直连出口", "选择用户", selectionRouteDirect, StateRoutingMenu)
	case "4":
		return m.openUserSelection("测试分流", "选择要测试的用户", selectionRouteTestUser, StateRoutingMenu)
	}
	return m, spinCmd
}

func (m appModel) selectRoutingRulesAction(key string, spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	switch key {
	case "0":
		m.state = StateRoutingMenu
	case "1":
		return m.openUserSelection("添加分流规则", "选择用户", selectionRouteSetUser, StateRoutingRulesMenu)
	case "2":
		return m.openUserSelection("删除分流规则", "选择用户", selectionRouteClearUser, StateRoutingRulesMenu)
	case "3":
		return m.openUserSelection("修改分流规则", "选择用户", selectionRouteSetUser, StateRoutingRulesMenu)
	}
	return m, spinCmd
}

func (m appModel) selectRoutingChainAction(key string, spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	switch key {
	case "0":
		m.state = StateRoutingMenu
	case "1":
		m.actionContext = map[string]string{}
		m = m.openInput("添加节点", "节点标签", "", inputChainTag, StateRoutingChainMenu)
	case "2":
		nodes := routingpkg.ListChainNodes(m.store)
		if len(nodes) == 0 {
			toastCmd := (&m).setToast("无链式代理节点", false)
			return m, batchCmd(spinCmd, toastCmd)
		}
		choices := make([]selectionChoice, 0, len(nodes))
		for _, node := range nodes {
			choices = append(choices, selectionChoice{
				Label: fmt.Sprintf("%s · %s · %d", node.Tag, node.Server, node.Port),
				Value: node.Tag,
			})
		}
		m = m.openSelection("删除节点", "选择要删除的节点", choices, selectionChainRemove, StateRoutingChainMenu)
	case "3":
		nodes := routingpkg.ListChainNodes(m.store)
		m = m.openTextView("链式代理", renderChainTable(nodes), StateRoutingChainMenu)
	}
	return m, spinCmd
}

func (m appModel) selectServiceMenuAction(key string, spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	switch key {
	case "0":
		m.state = StateMainMenu
		return m, spinCmd
	case "4":
		rows, err := m.services.AllStatuses(context.Background())
		if err != nil {
			toastCmd := (&m).setToast(err.Error(), true)
			return m, batchCmd(spinCmd, toastCmd)
		}
		m = m.openTextView("服务状态", renderServiceStatusTable(rows), StateServiceManagement)
		return m, spinCmd
	case "1", "2", "3":
		action := map[string]string{"1": "restart", "2": "stop", "3": "start"}[key]
		m.loading = true
		m.spinner.SetMessage("正在执行 " + action)
		return m, tea.Batch(spinCmd, m.serviceActionCmd(action))
	default:
		return m, spinCmd
	}
}

const selectionLogService = "log-service"

func (m appModel) selectLogsAction(key string, spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	switch key {
	case "0":
		m.state = StateMainMenu
	case "1":
		return m.showLogs(50, "proxy", "脚本日志", spinCmd)
	case "2":
		return m.showLogs(50, "proxy-watchdog", "Watchdog 日志", spinCmd)
	case "3":
		return m.openServiceLogSelection(spinCmd)
	}
	return m, spinCmd
}

func (m appModel) openServiceLogSelection(spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	statuses, err := m.services.AllStatuses(context.Background())
	if err != nil {
		toastCmd := (&m).setToast(err.Error(), true)
		return m, batchCmd(spinCmd, toastCmd)
	}
	if len(statuses) == 0 {
		toastCmd := (&m).setToast("无可用服务", false)
		return m, batchCmd(spinCmd, toastCmd)
	}
	choices := make([]selectionChoice, 0, len(statuses))
	for _, s := range statuses {
		label := s.Name
		if s.Version != "" && s.Version != "-" {
			label += " (" + s.Version + ")"
		}
		choices = append(choices, selectionChoice{Label: label, Value: s.Name})
	}
	m = m.openSelection("服务日志", "选择要查看的服务", choices, selectionLogService, StateLogsMenu)
	return m, spinCmd
}

func (m appModel) selectConfigAction(key string, spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	switch key {
	case "0":
		m.state = StateMainMenu
	case "1":
		body, err := config.Pretty(m.store.Config)
		if err != nil {
			toastCmd := (&m).setToast(err.Error(), true)
			return m, batchCmd(spinCmd, toastCmd)
		}
		m = m.openTextView("sing-box 配置", body, StateConfigMenu)
	case "2":
		body := "empty"
		if m.store.SnellConf != nil && len(m.store.SnellConf.Values) > 0 {
			body = string(m.store.SnellConf.MarshalText())
		}
		m = m.openTextView("snell-v5 配置", body, StateConfigMenu)
	case "3":
		m = m.openTextView("shadow-tls 配置", renderShadowTLSConfig(m.services), StateConfigMenu)
	}
	return m, spinCmd
}

func (m appModel) selectCoreAction(key string, spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	switch key {
	case "0":
		m.state = StateMainMenu
	case "1":
		rows, err := corepkg.CheckVersion(context.Background(), corepkg.VersionOptions{WorkDir: "/etc/go-proxy"})
		if err != nil {
			toastCmd := (&m).setToast(err.Error(), true)
			return m, batchCmd(spinCmd, toastCmd)
		}
		m = m.openTextView("组件版本", renderComponentVersions(rows), StateCoreMenu)
	case "2":
		rows, err := corepkg.CheckUpdates(context.Background(), corepkg.UpdateOptions{WorkDir: "/etc/go-proxy", Component: "all", OnlyCheck: true})
		if err != nil {
			toastCmd := (&m).setToast(err.Error(), true)
			return m, batchCmd(spinCmd, toastCmd)
		}
		m = m.openTextView("检查更新", renderComponentUpdates(rows), StateCoreMenu)
	case "3":
		m = m.openConfirm("执行更新", "应用核心组件更新?", confirmCoreUpdate, StateCoreMenu)
	}
	return m, spinCmd
}

func (m appModel) selectNetworkAction(key string, spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	switch key {
	case "0":
		m.state = StateMainMenu
	case "1":
		m.state = StateBBRMenu
	case "2":
		m.state = StateFirewallMenu
	}
	return m, spinCmd
}

func (m appModel) selectBBRAction(key string, spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	switch key {
	case "0":
		m.state = StateNetworkMenu
	case "1":
		status, err := netpkg.EnableBBR(context.Background())
		if err != nil {
			toastCmd := (&m).setToast(err.Error(), true)
			return m, batchCmd(spinCmd, toastCmd)
		}
		m = m.openTextView("BBR 状态", renderBBRStatus(status), StateBBRMenu)
	case "2":
		status, err := netpkg.DisableBBR(context.Background())
		if err != nil {
			toastCmd := (&m).setToast(err.Error(), true)
			return m, batchCmd(spinCmd, toastCmd)
		}
		m = m.openTextView("BBR 状态", renderBBRStatus(status), StateBBRMenu)
	}
	return m, spinCmd
}

func (m appModel) selectFirewallAction(key string, spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	switch key {
	case "0":
		m.state = StateNetworkMenu
	case "1":
		status, err := netpkg.FirewallStatusInfo(context.Background(), m.store)
		if err != nil {
			toastCmd := (&m).setToast(err.Error(), true)
			return m, batchCmd(spinCmd, toastCmd)
		}
		m = m.openTextView("防火墙状态", renderFirewallStatus(status), StateFirewallMenu)
	case "2":
		status, err := netpkg.ApplyFirewall(context.Background(), m.store)
		if err != nil {
			toastCmd := (&m).setToast(err.Error(), true)
			return m, batchCmd(spinCmd, toastCmd)
		}
		m = m.openTextView("防火墙状态", renderFirewallStatus(status), StateFirewallMenu)
	case "3":
		body, err := netpkg.ShowFirewallRules(context.Background())
		if err != nil {
			toastCmd := (&m).setToast(err.Error(), true)
			return m, batchCmd(spinCmd, toastCmd)
		}
		m = m.openTextView("防火墙规则", body, StateFirewallMenu)
	}
	return m, spinCmd
}

func (m appModel) selectSelectionAction(key string, spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	if key == "0" {
		m.state = m.returnState
		return m, spinCmd
	}
	item := m.selectionMenu.Selected()
	value := item.Value
	if value == "" {
		value = item.Title
	}
	switch m.selectionMode {
	case selectionProtocolRemove:
		m.actionContext["target"] = value
		m.actionContext["label"] = item.Title
		m = m.openConfirm("卸载协议", "确认卸载选中的协议?", confirmProtocolRemove, m.returnState)
	case selectionUserRename:
		m.actionContext["old"] = value
		m = m.openInput("重置用户", "新用户名", "", inputUserRename, m.returnState)
	case selectionUserDelete:
		m.actionContext["target"] = value
		m = m.openConfirm("删除用户", "确认删除选中的用户?", confirmUserDelete, m.returnState)
	case selectionRouteSetUser:
		m.actionContext["user"] = value
		return m.openOutboundSelection(spinCmd)
	case selectionRouteSetOut:
		m.actionContext["outbound"] = value
		m = m.openInput("配置分流", "规则集", "逗号分隔", inputRouteRuleSets, m.returnState)
	case selectionRouteClearUser:
		body, err := m.runRoutingClear(value)
		if err != nil {
			toastCmd := (&m).setToast(err.Error(), true)
			return m, batchCmd(spinCmd, toastCmd)
		}
		m = m.openTextView("删除分流规则", body, m.returnState)
	case selectionRouteTestUser:
		body, err := m.runRoutingTest(value)
		if err != nil {
			toastCmd := (&m).setToast(err.Error(), true)
			return m, batchCmd(spinCmd, toastCmd)
		}
		m = m.openTextView("测试分流", body, m.returnState)
	case selectionRouteDirect:
		body, err := m.runDirectOutbound(value)
		if err != nil {
			toastCmd := (&m).setToast(err.Error(), true)
			return m, batchCmd(spinCmd, toastCmd)
		}
		m = m.openTextView("直连出口", body, m.returnState)
	case selectionChainRemove:
		m.actionContext["target"] = value
		m = m.openConfirm("删除节点", "确认删除选中的节点?", confirmChainRemove, m.returnState)
	case selectionLogService:
		return m.showLogs(50, value, value+" 日志", spinCmd)
	case selectionSSMethod:
		m.actionContext["method"] = value
		return m.executeProtocolInstall("ss", "", spinCmd)
	case selectionSnellObfs:
		m.actionContext["obfs"] = value
		return m.executeProtocolInstall("snell", "", spinCmd)
	}
	return m, spinCmd
}

func (m appModel) handleInputSubmit(spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	value := strings.TrimSpace(m.input.Value())
	switch m.inputMode {
	case inputUserAdd:
		if value == "" {
			toastCmd := (&m).setToast("用户名不能为空", true)
			return m, batchCmd(spinCmd, toastCmd)
		}
		body, err := m.runUserAdd(value)
		if err != nil {
			toastCmd := (&m).setToast(err.Error(), true)
			return m, batchCmd(spinCmd, toastCmd)
		}
		m = m.openTextView("添加用户", body, StateUserMenu)
	case inputUserRename:
		if value == "" {
			toastCmd := (&m).setToast("用户名不能为空", true)
			return m, batchCmd(spinCmd, toastCmd)
		}
		body, err := m.runUserRename(m.actionContext["old"], value)
		if err != nil {
			toastCmd := (&m).setToast(err.Error(), true)
			return m, batchCmd(spinCmd, toastCmd)
		}
		m = m.openTextView("重命名用户", body, StateUserMenu)
	case inputRouteRuleSets:
		body, err := m.runRoutingSet(m.actionContext["user"], m.actionContext["outbound"], value)
		if err != nil {
			toastCmd := (&m).setToast(err.Error(), true)
			return m, batchCmd(spinCmd, toastCmd)
		}
		m = m.openTextView("配置分流", body, StateRoutingRulesMenu)
	case inputChainTag:
		m.actionContext["tag"] = value
		m = m.openInput("添加节点", "服务器", "", inputChainServer, StateRoutingChainMenu)
	case inputChainServer:
		m.actionContext["server"] = value
		m = m.openInput("添加节点", "端口", "", inputChainPort, StateRoutingChainMenu)
	case inputChainPort:
		body, err := m.runChainAdd(m.actionContext["tag"], m.actionContext["server"], value)
		if err != nil {
			toastCmd := (&m).setToast(err.Error(), true)
			return m, batchCmd(spinCmd, toastCmd)
		}
		m = m.openTextView("添加节点", body, StateRoutingChainMenu)
	case inputProtocolSNI:
		if value == "" {
			toastCmd := (&m).setToast("此协议需要 SNI", true)
			return m, batchCmd(spinCmd, toastCmd)
		}
		return m.executeProtocolInstall(m.actionContext["protocol"], value, spinCmd)
	case inputProtoPort:
		return m.handleProtoPortInput(value, spinCmd)
	case inputProtoSNI:
		return m.handleProtoSNIInput(value, spinCmd)
	case inputProtoPassword:
		return m.handleProtoPasswordInput(value, spinCmd)
	}
	return m, spinCmd
}

// handleProtoPortInput processes the port input and advances to the next prompt.
func (m appModel) handleProtoPortInput(value string, spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	proto := m.actionContext["protocol"]

	if value == "" {
		m.actionContext["port"] = ""
	} else {
		port, err := strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			toastCmd := (&m).setToast("端口无效，请输入 1-65535", true)
			return m, batchCmd(spinCmd, toastCmd)
		}
		m.actionContext["port"] = value
	}

	// Next step depends on protocol type.
	switch proto {
	case "ss":
		// SS needs cipher method selection.
		choices := []selectionChoice{
			{Label: "2022-blake3-aes-128-gcm (默认)", Value: "2022-blake3-aes-128-gcm"},
			{Label: "2022-blake3-aes-256-gcm", Value: "2022-blake3-aes-256-gcm"},
			{Label: "2022-blake3-chacha20-poly1305", Value: "2022-blake3-chacha20-poly1305"},
		}
		m = m.openSelection("安装 ss", "选择加密方式", choices, selectionSSMethod, StateProtocolInstallMenu)
		return m, spinCmd

	case "snell":
		// Snell needs IPv6 confirm.
		m = m.openConfirm("安装 snell", "开启双栈 (IPv6)?", confirmSnellIPv6, StateProtocolInstallMenu)
		return m, spinCmd

	case "trojan", "tuic", "anytls":
		// TLS protocols need SNI if no existing TLS config.
		if _, err := protopkg.EnsureTLS(m.store.Config, proto, ""); err != nil {
			m = m.openInput("安装 "+proto, "域名 (SNI)", "例如 example.com", inputProtoSNI, StateProtocolInstallMenu)
			return m, spinCmd
		}
		return m.executeProtocolInstall(proto, "", spinCmd)

	case "vless":
		// VLESS Reality needs a decoy SNI.
		if _, err := protopkg.EnsureTLS(m.store.Config, proto, ""); err != nil {
			m = m.openInput("安装 vless", "伪装域名 (SNI)", "例如 www.google.com", inputProtoSNI, StateProtocolInstallMenu)
			return m, spinCmd
		}
		return m.executeProtocolInstall(proto, "", spinCmd)

	default:
		return m.executeProtocolInstall(proto, "", spinCmd)
	}
}

// handleProtoSNIInput processes the SNI input and executes install.
func (m appModel) handleProtoSNIInput(value string, spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	proto := m.actionContext["protocol"]
	if value == "" {
		toastCmd := (&m).setToast("此协议需要域名 (SNI)", true)
		return m, batchCmd(spinCmd, toastCmd)
	}
	m.actionContext["sni"] = value
	return m.executeProtocolInstall(proto, value, spinCmd)
}

// handleProtoPasswordInput stores password and continues chain.
func (m appModel) handleProtoPasswordInput(value string, spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	m.actionContext["password"] = value
	proto := m.actionContext["protocol"]
	return m.executeProtocolInstall(proto, m.actionContext["sni"], spinCmd)
}

func (m appModel) handleConfirmSubmit(spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	switch m.confirmMode {
	case confirmProtocolRemove:
		body, err := m.runProtocolRemove(m.actionContext["target"])
		if err != nil {
			toastCmd := (&m).setToast(err.Error(), true)
			return m, batchCmd(spinCmd, toastCmd)
		}
		m = m.openTextView("卸载协议", body, StateMainMenu)
	case confirmUserDelete:
		body, err := m.runUserDelete(m.actionContext["target"])
		if err != nil {
			toastCmd := (&m).setToast(err.Error(), true)
			return m, batchCmd(spinCmd, toastCmd)
		}
		m = m.openTextView("删除用户", body, StateUserMenu)
	case confirmChainRemove:
		body, err := m.runChainRemove(m.actionContext["target"])
		if err != nil {
			toastCmd := (&m).setToast(err.Error(), true)
			return m, batchCmd(spinCmd, toastCmd)
		}
		m = m.openTextView("删除节点", body, StateRoutingChainMenu)
	case confirmCoreUpdate:
		body, err := m.runCoreUpdate()
		if err != nil {
			toastCmd := (&m).setToast(err.Error(), true)
			return m, batchCmd(spinCmd, toastCmd)
		}
		m = m.openTextView("执行更新", body, StateCoreMenu)
	case confirmSelfUpdate:
		return m.runSelfUpdate(spinCmd)
	case confirmUninstall:
		body, err := m.runUninstall()
		if err != nil {
			toastCmd := (&m).setToast(err.Error(), true)
			return m, batchCmd(spinCmd, toastCmd)
		}
		m = m.openTextView("卸载服务", body, StateMainMenu)
	case confirmSnellIPv6:
		if m.confirm.Result() {
			m.actionContext["ipv6"] = "true"
		} else {
			m.actionContext["ipv6"] = "false"
		}
		m = m.openConfirm("安装 snell", "开启 UDP?", confirmSnellUDP, StateProtocolInstallMenu)
	case confirmSnellUDP:
		if m.confirm.Result() {
			m.actionContext["udp"] = "true"
		} else {
			m.actionContext["udp"] = "false"
		}
		choices := []selectionChoice{
			{Label: "off (默认)", Value: "off"},
			{Label: "http", Value: "http"},
			{Label: "tls", Value: "tls"},
		}
		m = m.openSelection("安装 snell", "选择混淆模式", choices, selectionSnellObfs, StateProtocolInstallMenu)
	}
	return m, spinCmd
}

func (m appModel) viewMenuBody() string {
	menu := m.currentMenu()
	if menu == nil {
		return ""
	}
	w := minInt(76, m.width-4)
	if w < 40 {
		w = 72
	}
	menu.Width = w
	// Main menu has no title/subtitle
	if m.state == StateMainMenu {
		return menu.View()
	}
	subtitle := stateSubtitle(m.state)
	if m.state == StateSelectionMenu && m.textHint != "" {
		subtitle = m.textHint
	}
	var lines []string
	if subtitle != "" {
		lines = append(lines, "  "+subtitleStyle.Render(subtitle))
	}
	if m.state == StateProtocolInstallMenu {
		lines = append(lines, "  "+subtitleStyle.Render(m.portInfoLine()))
	}
	if len(lines) > 0 {
		lines = append(lines, "")
	}
	lines = append(lines, menu.View())
	return strings.Join(lines, "\n")
}

func (m appModel) viewTextBody() string {
	body := strings.TrimSpace(m.textBody)
	if body == "" {
		body = "empty"
	}
	lines := []string{body}
	if m.textHint != "" {
		lines = append(lines, "", mutedStyle.Render(m.textHint))
	}
	return strings.Join(lines, "\n")
}

func (m appModel) viewInputBody() string {
	var lines []string
	if m.inputHint != "" {
		lines = append(lines, mutedStyle.Render(m.inputHint))
	}
	lines = append(lines, m.input.View())
	return strings.Join(lines, "\n")
}

func (m appModel) viewConfirmBody() string {
	return m.confirm.View()
}

func (m appModel) openTextView(title, body string, returnState MenuState) appModel {
	m.state = StateTextView
	m.returnState = returnState
	m.textTitle = title
	m.textBody = strings.TrimSpace(body)
	m.textHint = "按 0、esc 或 q 返回"
	return m
}

func (m appModel) openSelection(title, hint string, choices []selectionChoice, mode string, returnState MenuState) appModel {
	items := make([]components.Item, 0, len(choices)+1)
	for i, choice := range choices {
		items = append(items, components.Item{
			Key:   strconv.Itoa(i + 1),
			Title: choice.Label,
			Value: choice.Value,
		})
	}
	items = append(items, components.Item{Key: "0", Title: "返回", Value: "back"})
	m.selectionMenu = components.NewMenuList(items)
	m.selectionTitle = title
	m.textHint = hint
	m.selectionMode = mode
	m.returnState = returnState
	m.state = StateSelectionMenu
	return m
}

func (m appModel) openInput(title, prompt, hint, mode string, returnState MenuState) appModel {
	m.inputTitle = title
	m.inputHint = hint
	m.input = components.NewTextInput(prompt)
	m.inputMode = mode
	m.returnState = returnState
	m.state = StateInputPrompt
	return m
}

func (m appModel) openConfirm(title, question, mode string, returnState MenuState) appModel {
	m.confirmTitle = title
	m.confirm = components.NewConfirm(question)
	m.confirmMode = mode
	m.returnState = returnState
	m.state = StateConfirmPrompt
	return m
}

func (m appModel) openUserSelection(title, hint, mode string, returnState MenuState) (tea.Model, tea.Cmd) {
	rows := userpkg.ListGroups(m.store)
	if len(rows) == 0 {
		toastCmd := (&m).setToast("无用户", false)
		return m, toastCmd
	}
	choices := make([]selectionChoice, 0, len(rows))
	for _, row := range rows {
		choices = append(choices, selectionChoice{Label: row.Name, Value: row.Name})
	}
	m = m.openSelection(title, hint, choices, mode, returnState)
	return m, nil
}

func (m appModel) openOutboundSelection(spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	choices := make([]selectionChoice, 0)
	seen := map[string]struct{}{
		"direct": {},
		"block":  {},
	}
	choices = append(choices,
		selectionChoice{Label: "direct", Value: "direct"},
		selectionChoice{Label: "block", Value: "block"},
	)
	for _, ob := range m.store.Config.Outbounds {
		tag := strings.TrimSpace(ob.Tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		choices = append(choices, selectionChoice{Label: tag, Value: tag})
	}
	sort.Slice(choices[2:], func(i, j int) bool {
		return choices[i+2].Label < choices[j+2].Label
	})
	m = m.openSelection("配置分流", "选择出站", choices, selectionRouteSetOut, StateRoutingRulesMenu)
	return m, spinCmd
}

func (m *appModel) refreshStatusSync() {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if rows, err := m.services.AllStatuses(ctx); err == nil {
		m.statusRows = rows
	}
}

func (m appModel) portInfoLine() string {
	used := protopkg.UsedPorts(m.store)
	if len(used) == 0 {
		return "ports: (0 occupied | random: 10000-65000)"
	}
	ports := make([]int, 0, len(used))
	for p := range used {
		ports = append(ports, p)
	}
	sort.Ints(ports)
	portStrs := make([]string, 0, len(ports))
	for _, p := range ports {
		portStrs = append(portStrs, strconv.Itoa(p))
	}
	return fmt.Sprintf("ports: (%d occupied: [%s] | random: 10000-65000)",
		len(ports), strings.Join(portStrs, ","))
}

func (m appModel) applyProtocolMutation(ctx context.Context, result protopkg.MutationResult, action string) string {
	if !result.Changed() {
		return action + ": no changes"
	}
	ops := m.services.ApplyStore(ctx, m.store)
	return strings.TrimSpace(strings.Join([]string{
		fmt.Sprintf("%s completed", action),
		fmt.Sprintf("added inbounds: %d", result.AddedInbounds),
		fmt.Sprintf("removed inbounds: %d", result.RemovedInbounds),
		fmt.Sprintf("meta updates: %d", result.UpdatedMetaRows),
		renderOperationResults(ops),
	}, "\n"))
}

func (m appModel) applyUserMutation(ctx context.Context, result userpkg.MutationResult, action string) string {
	if !result.Changed() {
		return action + ": no changes"
	}
	ops := m.services.ApplyStore(ctx, m.store)
	return strings.TrimSpace(strings.Join([]string{
		fmt.Sprintf("%s completed", action),
		fmt.Sprintf("users: %d", result.AffectedUsers),
		fmt.Sprintf("route rows: %d", result.AffectedRouteRows),
		fmt.Sprintf("dns rows: %d", result.AffectedDNSRows),
		renderOperationResults(ops),
	}, "\n"))
}

func (m appModel) applyRoutingMutation(ctx context.Context, result routingpkg.MutationResult, action string) string {
	if !result.Changed() {
		return action + ": no changes"
	}
	ops := m.services.ApplyStore(ctx, m.store)
	return strings.TrimSpace(strings.Join([]string{
		fmt.Sprintf("%s completed", action),
		fmt.Sprintf("route rows: %d", result.RouteChanged),
		fmt.Sprintf("dns rows: %d", result.DNSChanged),
		renderOperationResults(ops),
	}, "\n"))
}

func renderOperationResults(ops []service.OperationResult) string {
	lines := make([]string, 0, len(ops))
	for _, op := range ops {
		line := fmt.Sprintf("%s  %s", op.Name, op.Status)
		if op.Message != "" {
			line += "  " + op.Message
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func renderUserTable(rows []userpkg.GroupStats) string {
	if len(rows) == 0 {
		return "no users"
	}
	table := components.NewTable([]string{"user", "active", "disabled", "protocols"})
	for _, row := range rows {
		table.AddRow([]string{
			row.Name,
			strconv.Itoa(row.Active),
			strconv.Itoa(row.Disabled),
			strconv.Itoa(row.Protocols),
		})
	}
	return table.View()
}

func renderRouteTable(rows []routingpkg.RouteRow) string {
	if len(rows) == 0 {
		return "no routing rules"
	}
	table := components.NewTable([]string{"outbound", "user", "rule set"})
	for _, row := range rows {
		table.AddRow([]string{
			row.Outbound,
			strings.Join(row.AuthUser, ","),
			strings.Join(row.RuleSet, ","),
		})
	}
	return table.View()
}

func renderChainTable(rows []routingpkg.ChainNode) string {
	if len(rows) == 0 {
		return "no chain nodes"
	}
	table := components.NewTable([]string{"tag", "type", "server", "port"})
	for _, row := range rows {
		table.AddRow([]string{row.Tag, row.Type, row.Server, strconv.Itoa(row.Port)})
	}
	return table.View()
}

func renderProtocolTable(rows []protopkg.InventoryRow) string {
	if len(rows) == 0 {
		return "no protocols"
	}
	table := components.NewTable([]string{"protocol", "tag", "port", "users", "source"})
	for _, row := range rows {
		table.AddRow([]string{row.Protocol, row.Tag, strconv.Itoa(row.Port), strconv.Itoa(row.Users), row.Source})
	}
	return table.View()
}

func renderComponentVersions(rows []corepkg.ComponentVersion) string {
	table := components.NewTable([]string{"component", "version", "note"})
	for _, row := range rows {
		if !row.Installed {
			continue
		}
		note := ""
		if row.Err != nil {
			note = row.Err.Error()
		}
		table.AddRow([]string{row.Name, nonEmpty(row.Version, "-"), note})
	}
	return table.View()
}

func renderComponentUpdates(rows []corepkg.ComponentUpdate) string {
	table := components.NewTable([]string{"component", "update", "current", "latest", "applied"})
	for _, row := range rows {
		if !row.Installed {
			continue
		}
		updateText := "no"
		if row.NeedsUpdate {
			updateText = "yes"
		}
		table.AddRow([]string{
			row.Name,
			updateText,
			nonEmpty(row.Current, "-"),
			nonEmpty(row.Latest, "-"),
			strconv.FormatBool(row.Applied),
		})
	}
	return table.View()
}

func renderTargetSet(target subpkg.TargetSet) string {
	lines := []string{fmt.Sprintf("preferred  %s  %s", target.Preferred.Family, target.Preferred.Host)}
	for _, row := range target.Targets {
		lines = append(lines, fmt.Sprintf("%s  %s", row.Family, row.Host))
	}
	return strings.Join(lines, "\n")
}

func renderBBRStatus(status netpkg.BBRStatus) string {
	lines := []string{
		fmt.Sprintf("kernel             %s", status.Kernel),
		fmt.Sprintf("kernel supported   %t", status.KernelSupported),
		fmt.Sprintf("congestion         %s", status.Congestion),
		fmt.Sprintf("qdisc              %s", status.Qdisc),
		fmt.Sprintf("available          %s", status.Available),
		fmt.Sprintf("enabled            %t", status.Enabled),
	}
	return strings.Join(lines, "\n")
}

func renderFirewallStatus(status netpkg.FirewallStatus) string {
	lines := []string{
		fmt.Sprintf("backend  %s", status.Backend),
		fmt.Sprintf("tcp      %s", joinInts(status.TCP)),
		fmt.Sprintf("udp      %s", joinInts(status.UDP)),
	}
	return strings.Join(lines, "\n")
}

func renderServiceStatusTable(rows []service.ServiceStatus) string {
	table := components.NewTable([]string{"service", "state", "version"})
	for _, row := range rows {
		version := row.Version
		if version == "" {
			version = "-"
		}
		table.AddRow([]string{row.Name, row.State, version})
	}
	return table.View()
}

func renderShadowTLSConfig(svc *service.Manager) string {
	rows, err := svc.AllStatuses(context.Background())
	if err != nil {
		return err.Error()
	}
	lines := make([]string, 0)
	for _, row := range rows {
		if !strings.Contains(row.Name, "shadow-tls") {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s  %s  %s", row.Name, row.State, nonEmpty(row.Version, "-")))
	}
	if len(lines) == 0 {
		return "not configured"
	}
	return strings.Join(lines, "\n")
}

func joinInts(items []int) string {
	if len(items) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, strconv.Itoa(item))
	}
	return strings.Join(parts, ", ")
}

func nonEmpty(v, fallback string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback
	}
	return v
}

func (m appModel) runUserAdd(name string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := userpkg.AddGroup(m.store, name)
	if err != nil {
		return "", err
	}
	body := m.applyUserMutation(ctx, result, "add user")
	m.refreshStatusSync()
	return body, nil
}

func (m appModel) runUserRename(oldName, newName string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := userpkg.RenameGroup(m.store, oldName, newName)
	if err != nil {
		return "", err
	}
	body := m.applyUserMutation(ctx, result, "reset user")
	m.refreshStatusSync()
	return body, nil
}

func (m appModel) runUserDelete(name string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := userpkg.DeleteGroup(m.store, name)
	if err != nil {
		return "", err
	}
	body := m.applyUserMutation(ctx, result, "delete user")
	m.refreshStatusSync()
	return body, nil
}

func (m appModel) runProtocolRemove(target string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := protopkg.Remove(m.store, target)
	if err != nil {
		return "", err
	}
	body := m.applyProtocolMutation(ctx, result, "remove protocol")
	m.refreshStatusSync()
	return body, nil
}

func (m appModel) runRoutingSet(user, outbound, ruleSets string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := routingpkg.UpsertUserRule(m.store, user, outbound, []string{ruleSets})
	if err != nil {
		return "", err
	}
	body := m.applyRoutingMutation(ctx, result, "set rule")
	m.refreshStatusSync()
	return body, nil
}

func (m appModel) runRoutingClear(user string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := routingpkg.ClearUserRule(m.store, user)
	if err != nil {
		return "", err
	}
	body := m.applyRoutingMutation(ctx, result, "clear rule")
	m.refreshStatusSync()
	return body, nil
}

func (m appModel) runRoutingSyncDNS() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := routingpkg.SyncDNS(m.store)
	if err != nil {
		return "", err
	}
	body := m.applyRoutingMutation(ctx, result, "sync dns")
	m.refreshStatusSync()
	return body, nil
}

func (m appModel) runRoutingReconcile() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := routingpkg.Reconcile(m.store)
	if err != nil {
		return "", err
	}
	body := m.applyRoutingMutation(ctx, result, "reconcile")
	m.refreshStatusSync()
	return body, nil
}

func (m appModel) runRoutingTest(user string) (string, error) {
	byOutbound, total, err := routingpkg.TestUser(m.store, user)
	if err != nil {
		return "", err
	}
	lines := []string{
		fmt.Sprintf("user        %s", user),
		fmt.Sprintf("total rules %d", total),
	}
	if total > 0 {
		outbounds := make([]string, 0, len(byOutbound))
		for name := range byOutbound {
			outbounds = append(outbounds, name)
		}
		sort.Strings(outbounds)
		for _, name := range outbounds {
			lines = append(lines, fmt.Sprintf("%s  %d", name, byOutbound[name]))
		}
	}
	return strings.Join(lines, "\n"), nil
}

func (m appModel) runDirectOutbound(user string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := routingpkg.SetDirectMode(m.store, user)
	if err != nil {
		return "", err
	}
	body := m.applyRoutingMutation(ctx, result, "direct outbound")
	m.refreshStatusSync()
	return body, nil
}

func (m appModel) runChainAdd(tag, server, portText string) (string, error) {
	port, err := strconv.Atoi(strings.TrimSpace(portText))
	if err != nil {
		return "", fmt.Errorf("invalid port: %s", portText)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := routingpkg.AddChainNode(m.store, tag, server, port)
	if err != nil {
		return "", err
	}
	body := m.applyRoutingMutation(ctx, result, "add node")
	m.refreshStatusSync()
	return body, nil
}

func (m appModel) runChainRemove(tag string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := routingpkg.RemoveChainNode(m.store, tag)
	if err != nil {
		return "", err
	}
	body := m.applyRoutingMutation(ctx, result, "delete node")
	m.refreshStatusSync()
	return body, nil
}

func (m appModel) showSubscriptions(format subpkg.OutputFormat, host string, spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	result, err := subpkg.Render(m.store, subpkg.RenderOptions{Format: format, Host: host})
	if err != nil {
		toastCmd := (&m).setToast(err.Error(), true)
		return m, batchCmd(spinCmd, toastCmd)
	}
	body := renderSubscriptions(result, format)
	m = m.openTextView("订阅管理", body, StateMainMenu)
	return m, spinCmd
}

func renderSubscriptions(result subpkg.RenderResult, format subpkg.OutputFormat) string {
	if len(result.Users) == 0 {
		return "no subscription links"
	}
	lines := []string{renderTargetSet(result.Target), ""}
	for _, user := range result.Users {
		lines = append(lines, user)
		links := result.ByUser[user]
		if format == subpkg.FormatAll || format == subpkg.FormatSingbox {
			for _, line := range links.Singbox {
				lines = append(lines, line)
			}
		}
		if format == subpkg.FormatAll || format == subpkg.FormatSurge {
			for _, line := range links.Surge {
				lines = append(lines, line)
			}
		}
		lines = append(lines, "")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

func (m appModel) showLogs(lines int, name, title string, spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var body string
	var err error

	// "proxy" is not a systemd service — read the script log file or syslog.
	if name == "proxy" {
		body, err = m.collectScriptLogs(ctx, lines)
	} else {
		body, err = m.services.CollectLogsFor(ctx, lines, name)
	}
	if err != nil {
		toastCmd := (&m).setToast(err.Error(), true)
		return m, batchCmd(spinCmd, toastCmd)
	}
	if strings.TrimSpace(body) == "" {
		body = "无日志记录"
	}
	m = m.openTextView(title, body, StateLogsMenu)
	return m, spinCmd
}

func (m appModel) collectScriptLogs(ctx context.Context, lines int) (string, error) {
	logFile := "/etc/go-proxy/log/proxy.log"
	if data, err := os.ReadFile(logFile); err == nil && len(data) > 0 {
		all := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		start := 0
		if len(all) > lines {
			start = len(all) - lines
		}
		return strings.Join(all[start:], "\n"), nil
	}
	// Fallback to syslog/journalctl
	cmd := exec.CommandContext(ctx, "journalctl", "-t", "proxy", "--no-pager", "-n", fmt.Sprintf("%d", lines))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("无法读取脚本日志: %v", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (m appModel) runCoreUpdate() (string, error) {
	rows, err := corepkg.ApplyUpdates(context.Background(), corepkg.UpdateOptions{WorkDir: "/etc/go-proxy", Component: "all"})
	if err != nil {
		return "", err
	}
	for _, row := range rows {
		if row.Applied {
			_ = m.services.OperateAll(context.Background(), "restart")
			break
		}
	}
	m.refreshStatusSync()
	return renderComponentUpdates(rows), nil
}

func (m appModel) runSelfUpdate(spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	opts := updatepkg.Options{
		WorkDir: "/etc/go-proxy",
	}
	result, err := updatepkg.Run(context.Background(), opts)
	if err != nil {
		toastCmd := (&m).setToast(err.Error(), true)
		return m, batchCmd(spinCmd, toastCmd)
	}
	if result.Updated {
		// Binary replaced on disk — must exit so user relaunches with new version.
		fmt.Printf("\n更新完成，请重新启动 proxy menu\n")
		fmt.Printf("  remote  %s\n", nonEmpty(result.RemoteRef, "-"))
		return m, tea.Quit
	}
	body := strings.Join([]string{
		fmt.Sprintf("current       %s", nonEmpty(result.CurrentRef, "-")),
		fmt.Sprintf("remote        %s", nonEmpty(result.RemoteRef, "-")),
		fmt.Sprintf("needs update  %t", result.NeedsUpdate),
		fmt.Sprintf("message       %s", nonEmpty(result.Message, "-")),
	}, "\n")
	m = m.openTextView("脚本更新", body, StateMainMenu)
	return m, spinCmd
}

func (m appModel) runUninstall() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := service.Uninstall(ctx, "/etc/go-proxy"); err != nil {
		return "", err
	}
	return "uninstall completed", nil
}
