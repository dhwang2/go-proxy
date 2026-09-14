package views

import (
	"strings"
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
					Type: "anytls",
					Tag:  "anytls_443",
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

	if !userHasSelectedProtocol(s, protocol.AnyTLS, "u1") {
		t.Fatal("expected u1 to already have anytls")
	}
	if userHasSelectedProtocol(s, protocol.AnyTLS, "u2") {
		t.Fatal("expected u2 not to have anytls")
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

func TestProtocolInstallMenuDoesNotContainTink(t *testing.T) {
	model := tui.NewModel(&store.Store{
		SingBox:      &store.SingBoxConfig{},
		UserMeta:     store.NewUserManagement(),
		UserTemplate: &store.UserRouteTemplates{Templates: map[string][]store.TemplateRule{}},
	}, "dev")
	view := NewProtocolInstallView(&model)

	view.resetMenuState(44, 20)

	menu := strings.ToLower(view.Menu.View())
	if strings.Contains(menu, "tink") {
		t.Fatalf("menu = %q, want no tink entry", menu)
	}
}

func TestVLESSRealityChoicePrecedesInboundReuse(t *testing.T) {
	for _, existingReality := range []bool{false, true} {
		for _, chooseReality := range []bool{false, true} {
			s := &store.Store{
				SingBox: &store.SingBoxConfig{Inbounds: []store.Inbound{{
					Type: "vless", Tag: "vless_existing", ListenPort: 443,
					Users: []store.User{{Name: "u1", UUID: "uuid"}},
					TLS:   &store.TLSConfig{Enabled: true, Reality: &store.RealityConfig{Enabled: existingReality}},
				}}},
				UserMeta: store.NewUserManagement(),
			}
			model := tui.NewModel(s, "dev")
			view := NewProtocolInstallView(&model)
			view.resetMenuState(80, 24)
			if strings.Contains(strings.ToLower(view.Menu.View()), "trojan") {
				t.Fatal("removed protocol remains in install menu")
			}
			view.triggerMenuAction("vless")
			if view.step != protoInstallReality || view.pendingUser != "" {
				t.Fatal("Reality choice must precede user selection and inbound lookup")
			}
			view.Update(tui.ConfirmResultMsg{Confirmed: chooseReality})
			wantType := protocol.VLESS
			if chooseReality {
				wantType = protocol.VLESSReality
			}
			if view.pendingType != wantType {
				t.Fatalf("type = %s, want %s", view.pendingType, wantType)
			}
			if existingReality == chooseReality {
				if view.step != protoInstallResult {
					t.Fatal("matching variant must reuse the existing inbound")
				}
				continue
			}
			if view.step != protoInstallPort {
				t.Fatal("different variant must request a new port")
			}
			view.ClearInline()
			view.handlePortInput("24443")
			if chooseReality {
				if view.step != protoInstallRealitySNI {
					t.Fatal("Reality must request handshake SNI instead of a certificate")
				}
				view.Update(tui.InputResultMsg{Value: "https://invalid"})
				if view.pendingSNI != "" || view.step != protoInstallRealitySNI {
					t.Fatal("invalid handshake SNI was accepted")
				}
				view.Update(tui.InputResultMsg{Value: "www.microsoft.com"})
				if view.pendingSNI != "www.microsoft.com" || view.pendingDomain != "" {
					t.Fatal("Reality SNI must be passed without certificate configuration")
				}
			} else if view.step != protoInstallDomain {
				t.Fatal("ordinary VLESS must retain the TLS certificate flow")
			}
		}
	}
}
