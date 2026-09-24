package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-proxy/internal/config"
)

func TestRoutingNetworkRejectInvalidInputBeforeRuntime(t *testing.T) {
	cases := [][]string{
		{"route", "rule", "add", "--user", "alice", "--rules", "unknown", "--out", "direct"},
		{"route", "rule", "remove", "--user", "alice", "--rules", "0", "--confirm"},
		{"route", "rule", "remove", "--user", "alice", "--rules", "1"},
		{"route", "rule", "remove", "--user", "alice", "--rules", "1", "--all", "--confirm"},
		{"route", "rule", "remove"},
		{"route", "direct", "set", "unknown"},
		{"route", "direct", "set"},
		{"route", "chain", "add", "relay", "--parameter", "example.com:0"},
		{"route", "chain", "add", "relay", "--parameter", "example.com:1080:private-user"},
		{"route", "chain", "add", "relay", "--parameter", "2001:db8::1:1080:private-user:private-password"},
		{"route", "chain", "add", "relay", "--parameter", "example.com:notaport:private-user:private-password"},
		{"network", "firewall", "add", "65536/both"},
		{"network", "firewall", "add", "80/icmp"},
		{"network", "firewall", "add", "80"},
		{"network", "firewall", "add", "80/tcp", "--transport", "tcp"},
		{"network", "firewall", "remove", "80/tcp"},
		{"network", "firewall", "release"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out, stderr bytes.Buffer
			r := New("test", "test", strings.NewReader(`{"username":"private-user","password":"private-password","unknown":true}`), &out, &stderr)
			r.App.LockDir = filepath.Join(t.TempDir(), "uninitialized")
			code := r.Run(context.Background(), append(args, "--json"))
			if code != 2 {
				t.Fatalf("exit %d, want 2; stdout=%s stderr=%s", code, out.String(), stderr.String())
			}
			var response struct {
				OK      bool `json:"ok"`
				Changed bool `json:"changed"`
			}
			if err := json.Unmarshal(out.Bytes(), &response); err != nil || response.OK || response.Changed {
				t.Fatalf("invalid error envelope: %s (%v)", out.String(), err)
			}
			if strings.Contains(out.String()+stderr.String(), "private-") {
				t.Fatal("credentials leaked through invalid argument output")
			}
		})
	}
}

// --parameter carries the whole endpoint. An IPv6 host is bracketed, the
// credentials are optional and go together, and everything after the third
// colon is the password.
func TestChainParameterParsesEveryForm(t *testing.T) {
	type parsed struct {
		host           string
		port           int
		username, pass string
	}
	valid := map[string]parsed{
		"198.51.100.7:1080":              {"198.51.100.7", 1080, "", ""},
		"198.51.100.7:1080:alice:s3cr3t": {"198.51.100.7", 1080, "alice", "s3cr3t"},
		"proxy.example.net:1080:a:b:c:d": {"proxy.example.net", 1080, "a", "b:c:d"},
		"[2001:db8::1]:1080":             {"2001:db8::1", 1080, "", ""},
		"[2001:db8::1]:1080:alice:pw":    {"2001:db8::1", 1080, "alice", "pw"},
		" 198.51.100.7:1080:alice:pw ":   {"198.51.100.7", 1080, "alice", "pw"},
	}
	for value, want := range valid {
		host, port, username, password, err := parseChainParameter(value)
		if err != nil || (parsed{host, port, username, password}) != want {
			t.Errorf("%q = %q %d %q %q %v, want %+v", value, host, port, username, password, err, want)
		}
	}
	for _, value := range []string{
		"", "198.51.100.7", "198.51.100.7:", ":1080", "198.51.100.7:port",
		"198.51.100.7:1080:alice", "198.51.100.7:1080::pw", "198.51.100.7:1080:alice:",
		"[2001:db8::1:1080", "[2001:db8::1]1080", "2001:db8::1:1080",
	} {
		if _, _, _, _, err := parseChainParameter(value); err == nil {
			t.Errorf("%q was accepted", value)
		} else if strings.Contains(err.Error(), value) && value != "" {
			t.Errorf("%q was echoed in the error: %v", value, err)
		}
	}
}

// A malformed --parameter is an argument error: refused before the operation
// starts, so no progress line precedes it.
func TestChainParameterRefusedBeforeTheOperationStarts(t *testing.T) {
	for _, args := range [][]string{
		{"route", "chain", "add", "relay", "--parameter", "198.51.100.7:1080:private-user"},
		{"route", "chain", "modify", "relay", "--parameter", "198.51.100.7:port"},
	} {
		var out, stderr bytes.Buffer
		r := New("test", "test", strings.NewReader(""), &out, &stderr)
		r.App.LockDir = filepath.Join(t.TempDir(), "uninitialized")
		if code := r.Run(context.Background(), args); code != 2 {
			t.Fatalf("%v: exit %d", args, code)
		}
		if r.executed {
			t.Fatalf("%v: refused after a progress line:\n%s", args, stderr.String())
		}
	}
}

// rule add --rules takes menu indexes, several at once, never a name; an
// unknown one is refused before the operation starts.
func TestPresetIndexesResolveBeforeTheOperation(t *testing.T) {
	var out, stderr bytes.Buffer
	r := New("test", "test", strings.NewReader(""), &out, &stderr)
	r.App.LockDir = filepath.Join(t.TempDir(), "uninitialized")
	code := r.Run(context.Background(), []string{"route", "rule", "add", "--user", "alice", "--rules", "1,x,a", "--out", "direct"})
	// An error with no commands to show after it keeps its error line.
	if code != 2 || r.executed || !strings.HasPrefix(stderr.String(), "error: unknown preset x") {
		t.Fatalf("exit %d:\n%s", code, stderr.String())
	}
}

// --preset is gone, not aliased: rule add, modify and remove all take --rules.
func TestRuleAddHasNoPresetFlag(t *testing.T) {
	var out, stderr bytes.Buffer
	r := New("test", "test", strings.NewReader(""), &out, &stderr)
	r.App.LockDir = filepath.Join(t.TempDir(), "uninitialized")
	code := r.Run(context.Background(), []string{"route", "rule", "add", "--user", "alice", "--preset", "1", "--out", "direct"})
	if code != 2 || r.executed || !strings.Contains(stderr.String(), "unknown flag: --preset") {
		t.Fatalf("exit %d:\n%s", code, stderr.String())
	}
}

// chain remove on a chain rules still select prints those rules as route rule
// list does, then the commands that take each user's rules off it, selectors
// filled in; nothing is removed and the exit is a failure.
func TestChainRemoveListsTheRulesInTheWay(t *testing.T) {
	dir := t.TempDir()
	for target, name := range map[*string]string{
		&config.SingBoxConfig: "sing-box.json", &config.UserMetaFile: "users.json", &config.UserRouteFile: "routes.json",
		&config.UserTemplateFile: "templates.json", &config.FirewallConfigFile: "firewall.json", &config.SnellConfigFile: "snell.conf",
		&config.SingBoxBin: "sing-box", &config.DomainFile: "domain", &config.CaddyFile: "Caddyfile",
	} {
		original := *target
		*target = filepath.Join(dir, name)
		t.Cleanup(func() { *target = original })
	}
	for path, data := range map[string]string{
		config.SingBoxConfig: `{"dns":{"strategy":"prefer_ipv6"},"outbounds":[{"type":"direct","tag":"direct"}]}`,
		config.UserMetaFile:  `{"schema":3,"groups":{"all":["alice","bob"]}}`,
		config.SingBoxBin:    "#!/bin/sh\nexit 0\n",
	} {
		if err := os.WriteFile(path, []byte(data), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	run := func(args ...string) (int, string, string) {
		var out, stderr bytes.Buffer
		r := New("test", "test", strings.NewReader(""), &out, &stderr)
		r.App.LockDir = filepath.Join(dir, "locks")
		r.App.RequireRoot = false
		code := r.Run(context.Background(), args)
		return code, out.String(), stderr.String()
	}
	for _, args := range [][]string{
		{"route", "chain", "add", "res1", "--parameter", "192.0.2.10:1080"},
		{"route", "rule", "add", "--user", "alice", "--rules", "b,1", "--out", "res1"},
		{"route", "rule", "add", "--user", "bob", "--rules", "9", "--out", "res1"},
		{"route", "final", "set", "res1"},
	} {
		if code, _, stderr := run(args...); code != 0 {
			t.Fatalf("%v: exit %d\n%s", args, code, stderr)
		}
	}
	code, out, stderr := run("route", "chain", "remove", "res1", "--confirm")
	want := "alice:\n1.OpenAI/ChatGPT  -> res1: 192.0.2.10:1080\nb.Netflix         -> res1: 192.0.2.10:1080\n\n" +
		"bob:\n9.GitHub          -> res1: 192.0.2.10:1080\n\n" +
		"gproxy route rule modify --user alice --rules 1,b --out direct\n" +
		"gproxy route rule remove --user alice --rules 1,b --confirm\n" +
		"gproxy route rule modify --user bob --rules 9 --out direct\n" +
		"gproxy route rule remove --user bob --rules 9 --confirm\n" +
		"gproxy route final set direct   (the chain is also the route final)\n"
	if code != 1 || out != "" || stderr != want {
		t.Fatalf("exit %d, stdout %q, stderr:\n%s\nwant:\n%s", code, out, stderr, want)
	}
	// --json keeps one envelope with the same rules as data.
	code, out, _ = run("route", "chain", "remove", "res1", "--confirm", "--json")
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Rules      []map[string]any `json:"rules"`
			RouteFinal bool             `json:"route_final"`
		} `json:"data"`
		Error struct{ Code string } `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil || code != 1 || envelope.OK || envelope.Error.Code != "conflict" || len(envelope.Data.Rules) != 3 || !envelope.Data.RouteFinal {
		t.Fatalf("json exit %d: %v\n%s", code, err, out)
	}
	if _, list, _ := run("route", "chain", "list"); !strings.Contains(list, "res1") {
		t.Fatalf("the refused chain is gone:\n%s", list)
	}
}
