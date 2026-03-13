package tui

type MenuState int

const (
	StateMainMenu MenuState = iota
	StateProtocolInstallMenu
	StateProtocolRemoveMenu
	StateUserMenu
	StateRoutingMenu
	StateRoutingRulesMenu
	StateRoutingChainMenu
	StateServiceManagement
	StateSubscriptionMenu
	StateConfigMenu
	StateConfigView
	StateLogsMenu
	StateLogsView
	StateCoreMenu
	StateNetworkMenu
	StateBBRMenu
	StateFirewallMenu
	StateUpdateMenu
	StateSelectionMenu
	StateInputPrompt
	StateConfirmPrompt
	StateTextView
	StateUninstallMenu
)

type MenuItem struct {
	Key         string
	Title       string
	Description string
	Target      MenuState
}

var MainMenuItems = []MenuItem{
	{Key: "1", Title: "安装协议", Target: StateProtocolInstallMenu},
	{Key: "2", Title: "卸载协议", Target: StateProtocolRemoveMenu},
	{Key: "3", Title: "用户管理", Target: StateUserMenu},
	{Key: "4", Title: "分流管理", Target: StateRoutingMenu},
	{Key: "5", Title: "协议管理", Target: StateServiceManagement},
	{Key: "6", Title: "订阅管理", Target: StateSubscriptionMenu},
	{Key: "7", Title: "查看配置", Target: StateConfigMenu},
	{Key: "8", Title: "运行日志", Target: StateLogsMenu},
	{Key: "9", Title: "内核管理", Target: StateCoreMenu},
	{Key: "10", Title: "网络管理", Target: StateNetworkMenu},
	{Key: "11", Title: "脚本更新", Target: StateUpdateMenu},
	{Key: "12", Title: "卸载服务", Target: StateUninstallMenu},
	{Key: "0", Title: "完全退出", Target: StateMainMenu},
}

var ProtocolInstallMenuItems = []MenuItem{
	{Key: "1", Title: "trojan", Target: StateProtocolInstallMenu},
	{Key: "2", Title: "vless", Target: StateProtocolInstallMenu},
	{Key: "3", Title: "tuic", Target: StateProtocolInstallMenu},
	{Key: "4", Title: "ss", Target: StateProtocolInstallMenu},
	{Key: "5", Title: "anytls", Target: StateProtocolInstallMenu},
	{Key: "6", Title: "snell-v5", Target: StateProtocolInstallMenu},
	{Key: "0", Title: "返回", Target: StateMainMenu},
}

var UserMenuItems = []MenuItem{
	{Key: "1", Title: "用户列表", Target: StateUserMenu},
	{Key: "2", Title: "添加用户", Target: StateUserMenu},
	{Key: "3", Title: "重命名用户", Target: StateUserMenu},
	{Key: "4", Title: "删除用户", Target: StateUserMenu},
	{Key: "0", Title: "返回", Target: StateMainMenu},
}

var RoutingMenuItems = []MenuItem{
	{Key: "1", Title: "链式代理", Target: StateRoutingChainMenu},
	{Key: "2", Title: "配置分流", Target: StateRoutingRulesMenu},
	{Key: "3", Title: "直连出口", Target: StateRoutingMenu},
	{Key: "4", Title: "测试分流", Target: StateRoutingMenu},
	{Key: "0", Title: "返回", Target: StateMainMenu},
}

var RoutingRulesMenuItems = []MenuItem{
	{Key: "1", Title: "添加分流规则", Target: StateRoutingRulesMenu},
	{Key: "2", Title: "删除分流规则", Target: StateRoutingRulesMenu},
	{Key: "3", Title: "修改分流规则", Target: StateRoutingRulesMenu},
	{Key: "0", Title: "返回", Target: StateRoutingMenu},
}

var RoutingChainMenuItems = []MenuItem{
	{Key: "1", Title: "添加节点", Target: StateRoutingChainMenu},
	{Key: "2", Title: "删除节点", Target: StateRoutingChainMenu},
	{Key: "3", Title: "查看节点", Target: StateRoutingChainMenu},
	{Key: "0", Title: "返回", Target: StateRoutingMenu},
}

var ServiceMenuItems = []MenuItem{
	{Key: "1", Title: "重启所有服务", Target: StateServiceManagement},
	{Key: "2", Title: "停止所有服务", Target: StateServiceManagement},
	{Key: "3", Title: "启动所有服务", Target: StateServiceManagement},
	{Key: "4", Title: "查看服务状态", Target: StateServiceManagement},
	{Key: "0", Title: "返回", Target: StateMainMenu},
}

var LogsMenuItems = []MenuItem{
	{Key: "1", Title: "查看脚本日志 (最近50行)", Target: StateLogsMenu},
	{Key: "2", Title: "查看 Watchdog 日志 (最近50行)", Target: StateLogsMenu},
	{Key: "3", Title: "查看服务日志 (按协议选择)", Target: StateLogsMenu},
	{Key: "0", Title: "返回", Target: StateMainMenu},
}

var ConfigMenuItems = []MenuItem{
	{Key: "1", Title: "sing-box", Target: StateConfigMenu},
	{Key: "2", Title: "snell-v5", Target: StateConfigMenu},
	{Key: "3", Title: "shadow-tls", Target: StateConfigMenu},
	{Key: "0", Title: "返回", Target: StateMainMenu},
}

var CoreMenuItems = []MenuItem{
	{Key: "1", Title: "查看版本", Target: StateCoreMenu},
	{Key: "2", Title: "检查更新", Target: StateCoreMenu},
	{Key: "3", Title: "执行更新", Target: StateCoreMenu},
	{Key: "0", Title: "返回", Target: StateMainMenu},
}

var NetworkMenuItems = []MenuItem{
	{Key: "1", Title: "BBR 网络优化", Target: StateBBRMenu},
	{Key: "2", Title: "服务器防火墙收敛", Target: StateFirewallMenu},
	{Key: "0", Title: "返回", Target: StateMainMenu},
}

var BBRMenuItems = []MenuItem{
	{Key: "1", Title: "开启优化", Target: StateBBRMenu},
	{Key: "2", Title: "卸载优化", Target: StateBBRMenu},
	{Key: "0", Title: "返回", Target: StateNetworkMenu},
}

var FirewallMenuItems = []MenuItem{
	{Key: "1", Title: "查看状态", Target: StateFirewallMenu},
	{Key: "2", Title: "应用规则", Target: StateFirewallMenu},
	{Key: "3", Title: "显示规则", Target: StateFirewallMenu},
	{Key: "0", Title: "返回", Target: StateNetworkMenu},
}

func stateTitle(s MenuState) string {
	switch s {
	case StateMainMenu:
		return "主菜单"
	case StateProtocolInstallMenu:
		return "安装协议"
	case StateProtocolRemoveMenu:
		return "卸载协议"
	case StateUserMenu:
		return "用户管理"
	case StateRoutingMenu:
		return "分流管理"
	case StateRoutingRulesMenu:
		return "配置分流"
	case StateRoutingChainMenu:
		return "链式代理"
	case StateServiceManagement:
		return "协议管理"
	case StateSubscriptionMenu:
		return "订阅管理"
	case StateConfigMenu:
		return "查看配置"
	case StateConfigView:
		return "配置详情"
	case StateLogsMenu, StateLogsView:
		return "运行日志"
	case StateCoreMenu:
		return "内核管理"
	case StateNetworkMenu:
		return "网络管理"
	case StateBBRMenu:
		return "BBR 网络优化"
	case StateFirewallMenu:
		return "防火墙管理"
	case StateUpdateMenu:
		return "脚本更新"
	case StateSelectionMenu:
		return "选择"
	case StateInputPrompt:
		return "输入"
	case StateConfirmPrompt:
		return "确认"
	case StateTextView:
		return "详情"
	case StateUninstallMenu:
		return "卸载服务"
	default:
		return "菜单"
	}
}

func stateSubtitle(s MenuState) string {
	switch s {
	case StateMainMenu:
		return "退出时统一生效"
	case StateProtocolInstallMenu:
		return "退出时统一生效"
	case StateUserMenu:
		return "用户组管理"
	case StateRoutingMenu:
		return "链式代理、分流规则、直连出口"
	case StateRoutingRulesMenu:
		return "按用户配置分流规则"
	case StateRoutingChainMenu:
		return "Socks 链式代理节点管理"
	case StateServiceManagement:
		return "启动、停止、重启代理服务"
	case StateConfigMenu:
		return "查看运行中的配置文件"
	case StateCoreMenu:
		return "核心组件版本与更新"
	case StateNetworkMenu:
		return "BBR 优化与防火墙管理"
	case StateBBRMenu:
		return "TCP BBR 拥塞控制"
	case StateFirewallMenu:
		return "端口规则与防火墙后端"
	default:
		return ""
	}
}
