package views

import (
	"go-proxy/internal/tui/components"
)

// MainMenuItems returns the 12 main menu items + exit.
func MainMenuItems() []components.MenuItem {
	return []components.MenuItem{
		{Key: "1", Label: "安装协议  Install Protocol"},
		{Key: "2", Label: "卸载协议  Remove Protocol"},
		{Key: "3", Label: "用户管理  User Management"},
		{Key: "4", Label: "分流管理  Routing Management"},
		{Key: "5", Label: "协议管理  Service Management"},
		{Key: "6", Label: "订阅管理  Subscription"},
		{Key: "7", Label: "查看配置  View Configuration"},
		{Key: "8", Label: "运行日志  Runtime Logs"},
		{Key: "9", Label: "内核管理  Core Management"},
		{Key: "a", Label: "网络管理  Network Management"},
		{Key: "b", Label: "脚本更新  Self Update"},
		{Key: "c", Label: "卸载服务  Uninstall"},
		{Key: "0", Label: "退出  Exit"},
	}
}

// MainMenuViewID maps menu indices to view IDs.
// Returns -1 for exit.
func MainMenuViewID(index int) int {
	switch index {
	case 0:
		return 1 // ProtocolInstall
	case 1:
		return 2 // ProtocolRemove
	case 2:
		return 3 // UserMenu
	case 3:
		return 4 // RoutingMenu
	case 4:
		return 5 // ServiceMenu
	case 5:
		return 6 // Subscription
	case 6:
		return 7 // Config
	case 7:
		return 8 // Logs
	case 8:
		return 9 // Core
	case 9:
		return 10 // Network
	case 10:
		return 11 // SelfUpdate
	case 11:
		return 12 // Uninstall
	case 12:
		return -1 // Exit
	default:
		return 0
	}
}
