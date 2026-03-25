package views

import (
	"testing"

	"go-proxy/internal/protocol"
	"go-proxy/internal/store"
	"go-proxy/internal/tui"
)

func TestUserHasSelectedProtocol(t *testing.T) {
	s := &store.Store{
		SingBox: &store.SingBoxConfig{
			Inbounds: []store.Inbound{
				{
					Type: "trojan",
					Tag:  "trojan_443",
					Users: []store.User{
						{Name: "u1", Password: "pw1"},
					},
				},
			},
		},
		UserMeta:  store.NewUserManagement(),
		SnellConf: &store.SnellConfig{Listen: "0.0.0.0:1443", PSK: "snell-psk"},
	}
	s.UserMeta.Name[store.UserKey("snell", store.SnellTag, "snell-psk")] = "u1"

	if !userHasSelectedProtocol(s, protocol.Trojan, "u1") {
		t.Fatal("expected u1 to already have trojan")
	}
	if userHasSelectedProtocol(s, protocol.Trojan, "u2") {
		t.Fatal("expected u2 not to have trojan")
	}
	if !userHasSelectedProtocol(s, protocol.Snell, "u1") {
		t.Fatal("expected u1 to already have snell")
	}
	if userHasSelectedProtocol(s, protocol.Snell, "u2") {
		t.Fatal("expected u2 not to have snell")
	}
}

func TestResetMenuStateRestoresMenuFocus(t *testing.T) {
	model := tui.NewModel(&store.Store{
		SingBox:      &store.SingBoxConfig{},
		UserMeta:     store.NewUserManagement(),
		UserTemplate: &store.UserRouteTemplates{Templates: map[string][]store.TemplateRule{}},
	}, "dev")
	view := NewProtocolInstallView(&model)
	view.Menu = view.Menu.SetDim(true)

	view.resetMenuState(44, 20)

	if view.Menu.IsDimmed() {
		t.Fatal("menu should not remain dimmed after resetMenuState")
	}
	if !view.Split.FocusLeft() {
		t.Fatal("split should return focus to the left menu after resetMenuState")
	}
}
