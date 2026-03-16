package views

import "go-proxy/internal/tui/components"

// RoutingMenuItems returns the routing management submenu.
func RoutingMenuItems() []components.MenuItem {
	return []components.MenuItem{
		{Key: "1", Label: "设置规则  Set Rules"},
		{Key: "2", Label: "清除规则  Clear Rules"},
		{Key: "3", Label: "链式代理  Chain Proxy"},
		{Key: "4", Label: "测试规则  Test Rules"},
		{Key: "5", Label: "同步 DNS  Sync DNS"},
		{Key: "0", Label: "返回  Back"},
	}
}
