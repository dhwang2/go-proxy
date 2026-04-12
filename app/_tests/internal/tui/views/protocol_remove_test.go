package views

import (
	"testing"

	"go-proxy/internal/store"
	"go-proxy/internal/tui"
)

func TestProtocolRemoveInitClearsStaleTableWhenInventoryEmpty(t *testing.T) {
	model := tui.NewModel(&store.Store{
		SingBox:      &store.SingBoxConfig{},
		UserMeta:     store.NewUserManagement(),
		UserTemplate: &store.UserRouteTemplates{Templates: map[string][]store.TemplateRule{}},
	}, "dev")

	view := NewProtocolRemoveView(&model)
	view.rows = []protocolRemoveRow{{Protocol: "shadowsocks"}}
	view.tableHeader = "stale"

	view.Init()

	if len(view.rows) != 0 {
		t.Fatalf("rows len = %d, want 0", len(view.rows))
	}
	if view.tableHeader != "" {
		t.Fatalf("tableHeader = %q, want empty", view.tableHeader)
	}
	if !view.emptyResult {
		t.Fatal("expected emptyResult to be true")
	}
	if view.step != protoRemoveResult {
		t.Fatalf("step = %v, want %v", view.step, protoRemoveResult)
	}
}
