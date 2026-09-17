package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func guidanceRunner(t *testing.T, out, stderr *bytes.Buffer) *Runner {
	t.Helper()
	r := New("test", "test", strings.NewReader(""), out, stderr)
	r.App.RequireRoot = false
	r.App.LockDir = filepath.Join(t.TempDir(), "absent")
	return r
}

// Guidance must be indistinguishable from any other usage error to a caller:
// stderr, exit 2, nothing on stdout, and no runtime state created. Otherwise an
// agent could read an example command as the command's result.
func TestGuidanceIsAUsageErrorNotAResult(t *testing.T) {
	for _, args := range [][]string{
		{"protocol", "add"},
		{"protocol", "add", "vless"},
		{"server", "restart"},
		{"server", "start"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out, stderr bytes.Buffer
			r := guidanceRunner(t, &out, &stderr)
			if code := r.Run(context.Background(), args); code != 2 {
				t.Fatalf("exit=%d, want 2; stderr=%s", code, stderr.String())
			}
			if out.Len() != 0 {
				t.Fatalf("guidance reached stdout: %q", out.String())
			}
			if !strings.Contains(stderr.String(), "error: ") {
				t.Fatalf("guidance did not read as an error: %q", stderr.String())
			}
			if strings.Contains(stderr.String(), "starting ") {
				t.Fatalf("guidance claimed the operation started: %q", stderr.String())
			}
		})
	}
}

func TestGuidanceCarriesChoicesUnderJSON(t *testing.T) {
	var out, stderr bytes.Buffer
	r := guidanceRunner(t, &out, &stderr)
	if code := r.Run(context.Background(), []string{"protocol", "add", "--json"}); code != 2 {
		t.Fatalf("exit=%d, want 2", code)
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Protocols []map[string]string `json:"protocols"`
		} `json:"data"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("guidance envelope did not parse: %v: %s", err, out.String())
	}
	if envelope.OK || envelope.Error.Code != "invalid_argument" {
		t.Fatalf("guidance was not an invalid_argument failure: %s", out.String())
	}
	if len(envelope.Data.Protocols) != 5 {
		t.Fatalf("machine readers got %d protocols, want 5: %s", len(envelope.Data.Protocols), out.String())
	}
	if bytes.Contains(out.Bytes(), []byte{0x1b}) {
		t.Fatalf("guidance JSON carried an escape sequence: %q", out.String())
	}
}

func TestProtocolGuidanceNamesEveryInstallableProtocol(t *testing.T) {
	var out, stderr bytes.Buffer
	r := guidanceRunner(t, &out, &stderr)
	r.Run(context.Background(), []string{"protocol", "add"})
	for _, name := range []string{"vless", "ss", "tuic", "anytls", "snell"} {
		if !strings.Contains(stderr.String(), name) {
			t.Fatalf("guidance omitted %q:\n%s", name, stderr.String())
		}
	}
}

// The examples are the whole point of the guidance: if one stops being a
// runnable command the guidance is actively misleading.
func TestInstallExamplesNameTheirOwnProtocolAndRequiredFlags(t *testing.T) {
	for _, entry := range protocolCatalogue {
		for _, example := range entry.Examples {
			command := example[1]
			if !strings.HasPrefix(command, "gproxy protocol add "+entry.Name+" ") {
				t.Fatalf("%s example does not invoke its own protocol: %q", entry.Name, command)
			}
			for _, required := range []string{"--user ", "--port "} {
				if !strings.Contains(command, required) {
					t.Fatalf("%s example omits %s: %q", entry.Name, required, command)
				}
			}
		}
	}
}

func TestServerGuidanceListsSelectableServices(t *testing.T) {
	var out, stderr bytes.Buffer
	r := guidanceRunner(t, &out, &stderr)
	r.Run(context.Background(), []string{"server", "stop"})
	for _, name := range []string{"sing-box", "proxy-watchdog", "--all"} {
		if !strings.Contains(stderr.String(), name) {
			t.Fatalf("guidance omitted %q:\n%s", name, stderr.String())
		}
	}
}

func TestServiceVerbsAreNoLongerTopLevel(t *testing.T) {
	for _, verb := range []string{"start", "stop", "restart"} {
		var out, stderr bytes.Buffer
		r := guidanceRunner(t, &out, &stderr)
		if code := r.Run(context.Background(), []string{verb, "--all"}); code != 2 {
			t.Fatalf("%s is still reachable at the top level: exit=%d", verb, code)
		}
	}
}
