package views

import "go-proxy/internal/tui/components"

// UserMenuItems returns the user management submenu.
func UserMenuItems() []components.MenuItem {
	return []components.MenuItem{
		{Key: "1", Label: "用户列表  List Users"},
		{Key: "2", Label: "添加用户  Add User"},
		{Key: "3", Label: "重命名用户  Rename User"},
		{Key: "4", Label: "删除用户  Delete User"},
		{Key: "0", Label: "返回  Back"},
	}
}
