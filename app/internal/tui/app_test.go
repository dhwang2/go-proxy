package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dhwang2/go-proxy/internal/service"
	"github.com/dhwang2/go-proxy/internal/store"
)

func TestNormalizeKeyEnterCompat(t *testing.T) {
	// Test type-based normalization: KeyEnter type should always produce "enter"
	enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
	if got := normalizeKey(enterMsg); got != "enter" {
		t.Fatalf("normalizeKey(KeyEnter) = %q, want %q", got, "enter")
	}

	// KeyEnter with Alt modifier should still produce "enter"
	altEnterMsg := tea.KeyMsg{Type: tea.KeyEnter, Alt: true}
	if got := normalizeKey(altEnterMsg); got != "enter" {
		t.Fatalf("normalizeKey(Alt+KeyEnter) = %q, want %q", got, "enter")
	}

	// String-based fallback cases
	cases := map[tea.KeyType]string{
		10: "enter", // keyLF -> String() = "ctrl+j" -> normalized to "enter"
	}
	for keyType, want := range cases {
		msg := tea.KeyMsg{Type: keyType}
		if got := normalizeKey(msg); got != want {
			t.Fatalf("normalizeKey(type=%d, str=%q) = %q, want %q", keyType, msg.String(), got, want)
		}
	}

	// Regular keys should pass through
	qMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	if got := normalizeKey(qMsg); got != "q" {
		t.Fatalf("normalizeKey('q') = %q, want %q", got, "q")
	}
}

func TestUpdateMainMenuEnterCompat(t *testing.T) {
	enterVariants := []tea.KeyMsg{
		{Type: tea.KeyEnter},
		{Type: tea.KeyEnter, Alt: true},
		{Type: 10}, // keyLF
	}
	for _, msg := range enterVariants {
		model := newTestAppModel()
		if ok := model.mainMenu.SelectByKey("5"); !ok {
			t.Fatalf("failed to select service management menu item")
		}

		next, _ := model.updateMainMenu(normalizeKey(msg), nil)
		got := next.(appModel)
		if got.state != StateServiceManagement {
			t.Fatalf("key %q (type=%d) did not enter service menu, got state %v", msg.String(), msg.Type, got.state)
		}
	}
}

func TestUpdateServiceMenuEnterCompat(t *testing.T) {
	enterVariants := []tea.KeyMsg{
		{Type: tea.KeyEnter},
		{Type: tea.KeyEnter, Alt: true},
		{Type: 10}, // keyLF
	}
	for _, msg := range enterVariants {
		model := newTestAppModel()
		model.state = StateServiceManagement
		if ok := model.serviceMenu.SelectByKey("0"); !ok {
			t.Fatalf("failed to select back menu item")
		}

		next, _ := model.updateServiceMenu(normalizeKey(msg), nil)
		got := next.(appModel)
		if got.state != StateMainMenu {
			t.Fatalf("key %q (type=%d) did not return to main menu, got state %v", msg.String(), msg.Type, got.state)
		}
	}
}

func TestFullUpdatePipelineEnter(t *testing.T) {
	model := newTestAppModel()
	model.loading = false

	if ok := model.mainMenu.SelectByKey("5"); !ok {
		t.Fatalf("failed to select key 5")
	}

	enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
	next, _ := model.Update(enterMsg)
	got := next.(appModel)
	if got.state != StateServiceManagement {
		t.Fatalf("Enter via full Update pipeline did not enter service menu, got state %v, toast=%q", got.state, got.toast)
	}

	// Alt+Enter should also work (some SSH terminals send ESC+CR)
	model2 := newTestAppModel()
	model2.loading = false
	model2.mainMenu.SelectByKey("5")
	altEnterMsg := tea.KeyMsg{Type: tea.KeyEnter, Alt: true}
	next2, _ := model2.Update(altEnterMsg)
	got2 := next2.(appModel)
	if got2.state != StateServiceManagement {
		t.Fatalf("Alt+Enter via full Update pipeline did not enter service menu, got state %v, toast=%q", got2.state, got2.toast)
	}
}

func TestMainMenuItemCount(t *testing.T) {
	// 12 menu items + exit = 13 total
	if len(MainMenuItems) != 13 {
		t.Fatalf("main menu count = %d, want 13", len(MainMenuItems))
	}
}

func TestMainMenuMatchesShellProxyFeatures(t *testing.T) {
	want := []string{
		"安装协议",
		"卸载协议",
		"用户管理",
		"分流管理",
		"协议管理",
		"订阅管理",
		"查看配置",
		"运行日志",
		"内核管理",
		"网络管理",
		"脚本更新",
		"卸载服务",
		"完全退出",
	}
	if len(MainMenuItems) != len(want) {
		t.Fatalf("main menu count = %d, want %d", len(MainMenuItems), len(want))
	}
	for i, item := range MainMenuItems {
		if item.Title != want[i] {
			t.Fatalf("main menu item %d = %q, want %q", i, item.Title, want[i])
		}
	}
}

func TestMainMenuRenderHasCursorAndExit(t *testing.T) {
	model := newTestAppModel()
	model.mainMenu.Cursor = 0
	view := model.mainMenu.View()
	// Should have cursor indicator on first item
	if !strings.Contains(view, "▸") {
		t.Fatalf("main menu render missing cursor indicator: %q", view)
	}
	// Should have exit item
	if !strings.Contains(view, "完全退出") {
		t.Fatalf("main menu render missing exit row: %q", view)
	}
}

func TestMainMenuOpensSubmenus(t *testing.T) {
	cases := map[string]MenuState{
		"3":  StateUserMenu,
		"4":  StateRoutingMenu,
		"5":  StateServiceManagement,
		"7":  StateConfigMenu,
		"9":  StateCoreMenu,
		"10": StateNetworkMenu,
	}
	for key, want := range cases {
		model := newTestAppModel()
		if ok := model.mainMenu.SelectByKey(key); !ok {
			t.Fatalf("failed to select key %s", key)
		}
		next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		got := next.(appModel)
		if got.state != want {
			t.Fatalf("key %s opened state %v, want %v", key, got.state, want)
		}
	}
}

func TestStateSubtitlesExist(t *testing.T) {
	states := []MenuState{
		StateMainMenu,
		StateProtocolInstallMenu,
		StateUserMenu,
		StateRoutingMenu,
		StateServiceManagement,
		StateConfigMenu,
		StateCoreMenu,
		StateNetworkMenu,
	}
	for _, s := range states {
		sub := stateSubtitle(s)
		if sub == "" {
			t.Fatalf("state %d has no subtitle", s)
		}
	}
}

func newTestAppModel() appModel {
	st := &store.Store{
		Config:   &store.SingboxConfig{},
		UserMeta: store.DefaultUserMeta(),
	}
	svc := service.NewManager(service.Options{})
	return newAppModel(st, svc, "test")
}
