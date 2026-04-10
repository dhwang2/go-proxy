package views

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"go-proxy/internal/store"
	"go-proxy/internal/tui"
)

func TestParseCustomPortsSupportsMixedProtocols(t *testing.T) {
	got, err := parseCustomPorts("443, 53/udp, 3478/tcp+udp")
	if err != nil {
		t.Fatalf("parseCustomPorts() error = %v", err)
	}
	want := []store.FirewallPort{
		{Proto: "tcp", Port: 443},
		{Proto: "tcp", Port: 3478},
		{Proto: "udp", Port: 3478},
		{Proto: "udp", Port: 53},
	}
	cfg := &store.FirewallConfig{Ports: want}
	cfg.Normalize()
	want = cfg.Ports
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseCustomPorts() = %#v, want %#v", got, want)
	}
}

func TestFormatCustomPortsMergesProtocolsByPort(t *testing.T) {
	got := formatCustomPorts([]store.FirewallPort{
		{Proto: "tcp", Port: 443},
		{Proto: "udp", Port: 53},
		{Proto: "tcp", Port: 3478},
		{Proto: "udp", Port: 3478},
	})
	want := "53/udp, 443/tcp, 3478/tcp+udp"
	if got != want {
		t.Fatalf("formatCustomPorts() = %q, want %q", got, want)
	}
}

func TestNetworkMenuUsesFirewallConvergenceLabel(t *testing.T) {
	model := tui.NewModel(&store.Store{}, "dev")
	view := NewNetworkView(&model)

	menu := view.Menu.View()
	if !strings.Contains(menu, "防火墙收敛") {
		t.Fatalf("menu = %q, want 防火墙收敛", menu)
	}
	if strings.Contains(menu, "服务器防火墙收敛") {
		t.Fatalf("menu = %q, want no 服务器防火墙收敛", menu)
	}
}

func TestFirewallSubMenuMergesApplyAndView(t *testing.T) {
	model := tui.NewModel(&store.Store{}, "dev")
	view := NewNetworkView(&model)

	_ = view.triggerMenuAction("firewall")

	menu := view.subMenu.View()
	if !strings.Contains(menu, "应用防火墙收敛") || !strings.Contains(menu, "设置自定义端口") || !strings.Contains(menu, "查看当前规则") {
		t.Fatalf("subMenu = %q, want firewall entries", menu)
	}
	if strings.Contains(menu, "查看目标端口") {
		t.Fatalf("subMenu = %q, want no 查看目标端口", menu)
	}
}

func TestFirewallApplyShowsPreviewAndActions(t *testing.T) {
	model := tui.NewModel(&store.Store{
		SingBox:      &store.SingBoxConfig{},
		UserMeta:     store.NewUserManagement(),
		UserTemplate: &store.UserRouteTemplates{Templates: map[string][]store.TemplateRule{}},
	}, "dev")
	view := NewNetworkView(&model)
	view.Init()

	_ = view.triggerMenuAction("firewall")
	_ = view.handleFirewallMenu("apply")

	if view.step != networkFirewallPreview {
		t.Fatalf("step = %v, want networkFirewallPreview", view.step)
	}
	if !view.viewportReady {
		t.Fatal("expected preview viewport to be ready")
	}
	content := view.viewport.View()
	if !strings.Contains(content, "将要收敛以下目标端口") {
		t.Fatalf("content = %q, want preview title", content)
	}
	if !strings.Contains(content, "收敛以上端口") || !strings.Contains(content, "释放以上端口") {
		t.Fatalf("content = %q, want preview actions", content)
	}
	if !strings.Contains(content, "端口") || !strings.Contains(content, "协议") {
		t.Fatalf("content = %q, want desired ports table", content)
	}
}

func TestFirewallPreviewEscRestoresFirewallMenu(t *testing.T) {
	model := tui.NewModel(&store.Store{
		SingBox:      &store.SingBoxConfig{},
		UserMeta:     store.NewUserManagement(),
		UserTemplate: &store.UserRouteTemplates{Templates: map[string][]store.TemplateRule{}},
	}, "dev")
	view := NewNetworkView(&model)
	view.Init()

	_ = view.triggerMenuAction("firewall")
	_ = view.handleFirewallMenu("apply")
	updated, _ := view.Update(tea.KeyMsg{Type: tea.KeyEsc})
	view = updated.(*NetworkView)

	if view.step != networkFirewallMenu {
		t.Fatalf("step = %v, want networkFirewallMenu", view.step)
	}
	menu := view.subMenu.View()
	if !strings.Contains(menu, "应用防火墙收敛") || !strings.Contains(menu, "设置自定义端口") || !strings.Contains(menu, "查看当前规则") {
		t.Fatalf("menu = %q, want restored firewall menu", menu)
	}
	if strings.Contains(menu, "收敛以上端口") || strings.Contains(menu, "释放以上端口") {
		t.Fatalf("menu = %q, want preview actions cleared", menu)
	}
}

func TestFirewallResultDismissedRestoresFirewallMenu(t *testing.T) {
	model := tui.NewModel(&store.Store{
		SingBox:      &store.SingBoxConfig{},
		UserMeta:     store.NewUserManagement(),
		UserTemplate: &store.UserRouteTemplates{Templates: map[string][]store.TemplateRule{}},
	}, "dev")
	view := NewNetworkView(&model)
	view.Init()
	view.pendingAction = "firewall"
	view.step = networkResult
	view.viewportReady = true
	view.detailBuilder = func(int) string { return "ok" }

	updated, _ := view.Update(tui.ResultDismissedMsg{})
	view = updated.(*NetworkView)

	if view.step != networkFirewallMenu {
		t.Fatalf("step = %v, want networkFirewallMenu", view.step)
	}
	menu := view.subMenu.View()
	if !strings.Contains(menu, "应用防火墙收敛") || strings.Contains(menu, "收敛以上端口") {
		t.Fatalf("menu = %q, want restored firewall menu", menu)
	}
}
