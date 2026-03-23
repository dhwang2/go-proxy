package derived

import (
	"testing"

	"go-proxy/internal/store"
)

func TestInventoryIncludesSnell(t *testing.T) {
	s := &store.Store{
		SingBox:   &store.SingBoxConfig{},
		UserMeta:  store.NewUserManagement(),
		SnellConf: &store.SnellConfig{Listen: "0.0.0.0:8443", PSK: "secret"},
	}

	inv := Inventory(s)
	if len(inv) != 1 {
		t.Fatalf("Inventory() len = %d, want 1", len(inv))
	}
	if inv[0].Type != store.SnellTag {
		t.Fatalf("Inventory()[0].Type = %q, want %q", inv[0].Type, store.SnellTag)
	}
	if inv[0].Tag != store.SnellTag {
		t.Fatalf("Inventory()[0].Tag = %q, want %q", inv[0].Tag, store.SnellTag)
	}
}

func TestMembershipIncludesSnellMappedUserName(t *testing.T) {
	s := &store.Store{
		SingBox:   &store.SingBoxConfig{},
		UserMeta:  store.NewUserManagement(),
		SnellConf: &store.SnellConfig{Listen: "0.0.0.0:8443", PSK: "secret"},
	}
	s.UserMeta.Name[store.UserKey("snell", store.SnellTag, "secret")] = "alice"

	m := Membership(s)
	entries := m["alice"]
	if len(entries) != 1 {
		t.Fatalf("Membership()[alice] len = %d, want 1", len(entries))
	}
	if entries[0].Proto != store.SnellTag {
		t.Fatalf("Membership()[alice][0].Proto = %q, want %q", entries[0].Proto, store.SnellTag)
	}
}

func TestMembershipFallsBackToSingleActiveInboundUserForSnell(t *testing.T) {
	s := &store.Store{
		SingBox: &store.SingBoxConfig{
			Inbounds: []store.Inbound{
				{
					Type: "trojan",
					Tag:  "trojan_2053",
					Users: []store.User{
						{Name: "alice", Password: "secret"},
					},
				},
			},
		},
		UserMeta:  store.NewUserManagement(),
		SnellConf: &store.SnellConfig{Listen: "0.0.0.0:8443", PSK: "secret"},
	}
	s.UserMeta.Groups["~/.groups"] = []string{"alice", "bob"}

	m := Membership(s)
	entries := m["alice"]
	if len(entries) != 2 {
		t.Fatalf("Membership()[alice] len = %d, want 2", len(entries))
	}
	if entries[1].Proto != store.SnellTag {
		t.Fatalf("Membership()[alice][1].Proto = %q, want %q", entries[1].Proto, store.SnellTag)
	}
}
