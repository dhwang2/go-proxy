package views

import "go-proxy/internal/tui/components"

// NetworkMenuItems returns the network management submenu.
func NetworkMenuItems() []components.MenuItem {
	return []components.MenuItem{
		{Key: "1", Label: "BBR 状态  BBR Status"},
		{Key: "2", Label: "启用 BBR  Enable BBR"},
		{Key: "3", Label: "防火墙  Firewall Rules"},
		{Key: "0", Label: "返回  Back"},
	}
}
