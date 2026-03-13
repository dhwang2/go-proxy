package protocol

import (
	"strings"
	"testing"

	"github.com/dhwang2/go-proxy/internal/store"
)

func TestInstallShadowsocksAddsInboundAndMeta(t *testing.T) {
	st := newProtocolTestStore()
	stubPortAvailable(t, true)

	result, err := Install(st, InstallSpec{Protocol: "ss", Group: "Alpha Team", Secret: "pass-1"})
	if err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	if !result.ConfigChanged || !result.MetaChanged {
		t.Fatalf("expected config+meta changes, got %+v", result)
	}
	if result.AddedInbounds != 1 {
		t.Fatalf("expected AddedInbounds=1, got %d", result.AddedInbounds)
	}
	if len(st.Config.Inbounds) != 2 {
		t.Fatalf("expected one new inbound, got %d", len(st.Config.Inbounds))
	}
	added := st.Config.Inbounds[1]
	if normalizeProtocolType(added.Type) != "ss" {
		t.Fatalf("expected ss inbound, got %s", added.Type)
	}
	if added.ListenPort <= 0 {
		t.Fatalf("expected assigned port")
	}
	if len(added.Users) != 1 {
		t.Fatalf("expected one user")
	}
	key := userMetaKey("ss", added.Tag, added.Users[0].Key())
	if st.UserMeta.Name[key] != "alpha-team" {
		t.Fatalf("expected mapped group alpha-team, got %q", st.UserMeta.Name[key])
	}
}

func TestInstallSnellUpdatesSnellAndMeta(t *testing.T) {
	st := newProtocolTestStore()
	stubPortAvailable(t, true)

	result, err := Install(st, InstallSpec{Protocol: "snell", Group: "beta", Secret: "snell-psk-1"})
	if err != nil {
		t.Fatalf("Install snell returned error: %v", err)
	}
	if !result.SnellChanged || !result.MetaChanged {
		t.Fatalf("expected snell+meta changes, got %+v", result)
	}
	if !strings.HasPrefix(st.SnellConf.Get("listen"), "0.0.0.0:") {
		t.Fatalf("expected listen configured, got %q", st.SnellConf.Get("listen"))
	}
	if st.SnellConf.Get("psk") != "snell-psk-1" {
		t.Fatalf("expected psk saved")
	}
	key := userMetaKey("snell", "snell-v5", "snell-psk-1")
	if st.UserMeta.Name[key] != "beta" {
		t.Fatalf("expected snell meta mapping")
	}
}

func TestRemoveByProtocolCleansInboundAndMeta(t *testing.T) {
	st := newProtocolTestStore()
	stubPortAvailable(t, true)
	_, err := Install(st, InstallSpec{Protocol: "ss", Group: "alpha", Secret: "pass-2", Tag: "ss-test", Port: 18388})
	if err != nil {
		t.Fatalf("install setup failed: %v", err)
	}
	if len(st.Config.Inbounds) != 2 {
		t.Fatalf("setup expected 2 inbounds")
	}

	result, err := Remove(st, "ss")
	if err != nil {
		t.Fatalf("remove failed: %v", err)
	}
	if !result.ConfigChanged {
		t.Fatalf("expected config changed")
	}
	if result.RemovedInbounds != 2 {
		t.Fatalf("expected RemovedInbounds=2, got %d", result.RemovedInbounds)
	}
	if len(st.Config.Inbounds) != 0 {
		t.Fatalf("expected all ss inbounds removed")
	}
	for key := range st.UserMeta.Name {
		if strings.HasPrefix(key, "ss|") {
			t.Fatalf("expected ss meta rows removed, found %s", key)
		}
	}
}

func TestListIncludesSnell(t *testing.T) {
	st := newProtocolTestStore()
	rows := List(st)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	foundSS := false
	foundSnell := false
	for _, row := range rows {
		if row.Protocol == "ss" {
			foundSS = true
		}
		if row.Protocol == "snell" && row.Tag == "snell-v5" {
			foundSnell = true
		}
	}
	if !foundSS || !foundSnell {
		t.Fatalf("missing expected protocol rows: %#v", rows)
	}
}

func stubPortAvailable(t *testing.T, available bool) {
	t.Helper()
	old := portAvailableFn
	portAvailableFn = func(int) bool { return available }
	t.Cleanup(func() { portAvailableFn = old })
}

func newProtocolTestStore() *store.Store {
	meta := store.DefaultUserMeta()
	meta.Name["ss|ss-old|old-pass"] = "legacy"
	meta.Groups["legacy"] = store.Group{}
	return &store.Store{
		Config: &store.SingboxConfig{
			Inbounds: []store.Inbound{
				{
					Type:       "shadowsocks",
					Tag:        "ss-old",
					ListenPort: 8388,
					Users: []store.User{{
						Name:     "legacy",
						Password: "old-pass",
						Method:   "2022-blake3-aes-128-gcm",
					}},
				},
			},
		},
		UserMeta: meta,
		SnellConf: &store.SnellConfig{Values: map[string]string{
			"listen": "0.0.0.0:8444",
			"psk":    "snell-legacy",
		}},
	}
}
