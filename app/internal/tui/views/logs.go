package views

import "go-proxy/internal/tui/components"

// LogSourceItems returns the log source selection.
func LogSourceItems() []components.MenuItem {
	return []components.MenuItem{
		{Key: "1", Label: "sing-box"},
		{Key: "2", Label: "snell-v5"},
		{Key: "3", Label: "shadow-tls"},
		{Key: "4", Label: "caddy-sub"},
		{Key: "5", Label: "proxy-script"},
		{Key: "6", Label: "proxy-watchdog"},
		{Key: "0", Label: "返回  Back"},
	}
}
