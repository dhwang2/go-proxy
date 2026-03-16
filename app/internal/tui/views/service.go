package views

import "go-proxy/internal/tui/components"

// ServiceMenuItems returns the service management submenu.
func ServiceMenuItems() []components.MenuItem {
	return []components.MenuItem{
		{Key: "1", Label: "sing-box"},
		{Key: "2", Label: "snell-v5"},
		{Key: "3", Label: "shadow-tls"},
		{Key: "4", Label: "caddy-sub"},
		{Key: "5", Label: "proxy-watchdog"},
		{Key: "0", Label: "返回  Back"},
	}
}

// ServiceActionItems returns the actions for a selected service.
func ServiceActionItems() []components.MenuItem {
	return []components.MenuItem{
		{Key: "1", Label: "启动  Start"},
		{Key: "2", Label: "停止  Stop"},
		{Key: "3", Label: "重启  Restart"},
		{Key: "4", Label: "状态  Status"},
		{Key: "0", Label: "返回  Back"},
	}
}
