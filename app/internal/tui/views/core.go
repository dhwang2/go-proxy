package views

import "go-proxy/internal/tui/components"

// CoreMenuItems returns the core management submenu.
func CoreMenuItems() []components.MenuItem {
	return []components.MenuItem{
		{Key: "1", Label: "查看版本  View Versions"},
		{Key: "2", Label: "检查更新  Check Updates"},
		{Key: "3", Label: "更新内核  Update Core"},
		{Key: "0", Label: "返回  Back"},
	}
}
