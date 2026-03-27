package tui

import "testing"

func TestNavigateMsgResetsBreadcrumbStack(t *testing.T) {
	model := NewModel(nil, "dev")

	next, _ := model.Update(NavigateMsg{Name: "routing"})
	model = next.(Model)
	if got := model.nav.Breadcrumb(); len(got) != 1 || got[0] != "routing" {
		t.Fatalf("breadcrumb after first navigate = %#v", got)
	}

	next, _ = model.Update(NavigateMsg{Name: "service"})
	model = next.(Model)
	if got := model.nav.Breadcrumb(); len(got) != 1 || got[0] != "service" {
		t.Fatalf("breadcrumb after second navigate = %#v", got)
	}
}
