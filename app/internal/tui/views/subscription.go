package views

import "go-proxy/internal/tui/components"

// SubscriptionFormatItems returns the subscription format selection.
func SubscriptionFormatItems() []components.MenuItem {
	return []components.MenuItem{
		{Key: "1", Label: "Surge"},
		{Key: "2", Label: "sing-box"},
		{Key: "0", Label: "返回  Back"},
	}
}
