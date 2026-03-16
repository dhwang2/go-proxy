package views

import (
	"go-proxy/internal/protocol"
	"go-proxy/internal/tui/components"
)

// ProtocolInstallMenuItems returns the protocol type selection menu.
func ProtocolInstallMenuItems() []components.MenuItem {
	items := make([]components.MenuItem, 0, len(protocol.AllTypes()))
	for i, t := range protocol.AllTypes() {
		spec := protocol.Specs()[t]
		key := string(rune('1' + i))
		if i >= 9 {
			key = string(rune('a' + i - 9))
		}
		items = append(items, components.MenuItem{
			Key:   key,
			Label: spec.DisplayName,
		})
	}
	items = append(items, components.MenuItem{Key: "0", Label: "返回  Back"})
	return items
}

// ProtocolRemoveMenuItems returns a menu of installed protocols for removal.
func ProtocolRemoveMenuItems(tags []string) []components.MenuItem {
	items := make([]components.MenuItem, 0, len(tags)+1)
	for i, tag := range tags {
		key := string(rune('1' + i))
		if i >= 9 {
			key = string(rune('a' + i - 9))
		}
		items = append(items, components.MenuItem{Key: key, Label: tag})
	}
	items = append(items, components.MenuItem{Key: "0", Label: "返回  Back"})
	return items
}
