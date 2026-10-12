package application

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"go-proxy/internal/config"
	"go-proxy/internal/derived"
	"go-proxy/internal/routing"
	"go-proxy/internal/store"
)

// installV038State replaces the fixture's state with what v0.3.8 wrote.
func installV038State(t *testing.T) *App {
	t.Helper()
	a := routingApplicationFixture(t)
	for path, data := range map[string]string{
		config.SingBoxConfig:   compatV038SingBox,
		config.UserMetaFile:    compatV038UserMeta,
		config.UserRouteFile:   compatV038UserRoutes,
		config.SnellConfigFile: compatV038Snell,
	} {
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return a
}

// From v0.3.8 on, a later version must keep a running host as it is. Loading
// v0.3.8 state, recompiling every route and DNS rule and saving has to write
// back exactly what v0.3.8 wrote: a difference would reach sing-box on the
// operator's next change, and restart it with a configuration nobody asked
// for. A change that has to alter this needs a migration that keeps the
// running behaviour, its own test, and a release note; it is never fixed by
// regenerating the fixture from a newer build.
func TestV038StateRecompilesUnchanged(t *testing.T) {
	installV038State(t)
	s, err := store.Load()
	if err != nil {
		t.Fatalf("v0.3.8 state no longer loads: %v", err)
	}
	routing.Sync(s)
	for _, file := range []string{store.FileSingBox, store.FileUserMeta, store.FileUserRoutes, store.FileSnellConf} {
		s.MarkDirty(file)
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		config.SingBoxConfig:   compatV038SingBox,
		config.UserMetaFile:    compatV038UserMeta,
		config.UserRouteFile:   compatV038UserRoutes,
		config.SnellConfigFile: compatV038Snell,
	} {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Errorf("%s is no longer what v0.3.8 wrote:\n%s", path, firstDifference(want, string(got)))
		}
	}
}

// The read commands see v0.3.8 state as v0.3.8 did: the same users and
// memberships, every rule numbered by its preset, and each rule's chain.
func TestV038StateReadsAsItWasWritten(t *testing.T) {
	a := installV038State(t)
	ctx := context.Background()
	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	membership := derived.Membership(snapshot.Store)
	if len(membership["alice"]) != 4 || len(membership["bob"]) != 3 {
		t.Fatalf("memberships: alice %d, bob %d", len(membership["alice"]), len(membership["bob"]))
	}
	listed, err := a.RoutingList(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	selectors := map[string][]string{}
	for _, entry := range listed.Data.(map[string]any)["rules"].([]RouteEntry) {
		selectors[entry.User] = append(selectors[entry.User], entry.Selector+">"+entry.Outbound)
	}
	if got := strings.Join(selectors["alice"], " "); got != "1>res1 2>res1 3>res1 j>res1 b>direct r>direct" {
		t.Fatalf("alice's rules: %s", got)
	}
	if got := strings.Join(selectors["bob"], " "); got != "1>res1 2>res1 5>res6 a>res6" {
		t.Fatalf("bob's rules: %s", got)
	}
	chains, err := a.RoutingChains(ctx)
	if err != nil {
		t.Fatal(err)
	}
	views := chains.Data.(map[string]any)["chains"].([]ChainView)
	if len(views) != 2 || views[0].Address != "198.51.100.7:1080" || views[1].Address != "[2001:db8::7]:1080" ||
		views[0].DomainStrategy != "ipv4_only" || views[1].DomainStrategy != "ipv6_only" {
		t.Fatalf("chains: %+v", views)
	}
}

// firstDifference shows the first line that differs, so a failure says what
// changed rather than printing two whole files.
func firstDifference(want, got string) string {
	wantLines, gotLines := strings.Split(want, "\n"), strings.Split(got, "\n")
	for index := 0; index < len(wantLines) || index < len(gotLines); index++ {
		var w, g string
		if index < len(wantLines) {
			w = wantLines[index]
		}
		if index < len(gotLines) {
			g = gotLines[index]
		}
		if w != g {
			return fmt.Sprintf("line %d:\n  v0.3.8: %s\n  now:    %s", index+1, w, g)
		}
	}
	return "files differ only in length"
}
