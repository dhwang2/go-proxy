package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func rootTestRunner(t *testing.T, in io.Reader, out, stderr io.Writer) *Runner {
	t.Helper()
	r := New("test-version", "test-revision", in, out, stderr)
	r.App.RequireRoot = false
	r.App.LockDir = filepath.Join(t.TempDir(), "uninitialized")
	return r
}

func assertSingleRootJSON(t *testing.T, data []byte, ok bool) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	var result map[string]json.RawMessage
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("invalid result JSON: %v: %s", err, data)
	}
	if string(result["ok"]) != map[bool]string{true: "true", false: "false"}[ok] {
		t.Fatalf("wrong result state: %s", data)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("stdout must contain exactly one JSON value: %v", err)
	}
	if bytes.Contains(data, []byte{0x1b}) {
		t.Fatal("structured output contains terminal controls")
	}
}

func TestRootHelpDoesNotInitializeOrReadInput(t *testing.T) {
	for _, args := range [][]string{nil, {"--help"}, {"--json", "--help"}, {"network", "--help"}, {"network", "firewall", "--help"}, {"route", "chain", "--help"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			reader, writer := io.Pipe()
			defer reader.Close()
			defer writer.Close()
			r := rootTestRunner(t, reader, &stdout, &stderr)
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			result := make(chan int, 1)
			go func() { result <- r.Run(ctx, args) }()
			select {
			case code := <-result:
				if code != 0 {
					t.Fatalf("help exit %d, stderr=%s", code, stderr.String())
				}
			case <-time.After(750 * time.Millisecond):
				_ = reader.Close()
				t.Fatal("help waited for stdin or runtime initialization")
			}
			if !strings.Contains(stdout.String(), "Usage:") || !strings.Contains(stdout.String(), "gproxy") {
				t.Fatalf("help is missing: %s", stdout.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("unexpected help diagnostics: %s", stderr.String())
			}
			if _, err := os.Stat(r.App.LockDir); !os.IsNotExist(err) {
				t.Fatal("help initialized runtime state")
			}
		})
	}
}

func TestRootUsageErrorsHaveNoEffectsAndOneJSONResult(t *testing.T) {
	cases := [][]string{
		{"unknown"},
		{"network", "unknown"},
		{"network", "firewall", "unknown"},
		{"route", "chain", "unknown"},
		{"--unknown"},
		{"version", "--unknown"},
		{"network", "status", "--unknown"},
		{"user", "rename", "alice"},
		{"route", "test", "--user", "alice"},
		{"protocol", "add"},
		{"protocol", "add", "unsupported", "--user", "alice", "--port", "auto"},
		{"version", "--timeout", "0"},
		{"version", "--timeout", "-1s"},
		{"version", "--timeout", "invalid"},
	}
	for _, args := range cases {
		for _, jsonFirst := range []bool{false, true} {
			command := append([]string(nil), args...)
			if jsonFirst {
				command = append([]string{"--json"}, command...)
			} else {
				command = append(command, "--json")
			}
			t.Run(strings.Join(command, " "), func(t *testing.T) {
				var stdout, stderr bytes.Buffer
				r := rootTestRunner(t, strings.NewReader(""), &stdout, &stderr)
				if code := r.Run(context.Background(), command); code != 2 {
					t.Fatalf("usage error exit %d: %s %s", code, stdout.String(), stderr.String())
				}
				assertSingleRootJSON(t, stdout.Bytes(), false)
				var result struct {
					Changed bool `json:"changed"`
					Error   struct {
						Code string `json:"code"`
					} `json:"error"`
				}
				if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if result.Changed || result.Error.Code != "invalid_argument" {
					t.Fatalf("wrong usage error: %s", stdout.String())
				}
				if _, err := os.Stat(r.App.LockDir); !os.IsNotExist(err) {
					t.Fatal("invalid input initialized runtime state")
				}
			})
		}
	}
}

func TestRootJSONFlagsWorkBeforeAndAfterCommand(t *testing.T) {
	for _, args := range [][]string{{"--json", "version"}, {"version", "--json"}, {"--json=true", "version"}, {"version", "--json=true"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			r := rootTestRunner(t, strings.NewReader(""), &stdout, &stderr)
			if code := r.Run(context.Background(), args); code != 0 {
				t.Fatalf("version exit %d: %s", code, stderr.String())
			}
			assertSingleRootJSON(t, stdout.Bytes(), true)
			if !bytes.Contains(stdout.Bytes(), []byte(`"version":"test-version"`)) {
				t.Fatalf("missing version: %s", stdout.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("version emitted diagnostics: %s", stderr.String())
			}
			if _, err := os.Stat(r.App.LockDir); !os.IsNotExist(err) {
				t.Fatal("version initialized runtime state")
			}
		})
	}
}

func filledRootPipe(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close(); _ = reader.Close() })
	if err := writer.SetWriteDeadline(time.Now().Add(20 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	n, err := writer.Write(bytes.Repeat([]byte("x"), 1<<20))
	if n == 0 || n == 1<<20 || !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("could not establish pipe backpressure: written=%d error=%v", n, err)
	}
	if err := writer.SetWriteDeadline(time.Time{}); err != nil {
		t.Fatal(err)
	}
	return reader, writer
}

func TestContextWriterCancelsRealBlockedPipe(t *testing.T) {
	_, writer := filledRootPipe(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := (contextWriter{ctx: ctx, writer: writer}).Write([]byte("blocked")); done <- err }()
	timer := time.AfterFunc(30*time.Millisecond, cancel)
	defer timer.Stop()
	started := time.Now()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("blocked write error=%v, want cancellation", err)
		}
	case <-time.After(750 * time.Millisecond):
		_ = writer.Close()
		t.Fatal("blocked pipe ignored cancellation")
	}
	if time.Since(started) >= time.Second {
		t.Fatal("blocked write exceeded the cleanup budget")
	}
}

func TestRootStdoutBackpressureHonorsTimeout(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	var stderr bytes.Buffer
	r := rootTestRunner(t, strings.NewReader(""), writer, &stderr)
	r.Version = strings.Repeat("v", 1<<20)
	done := make(chan int, 1)
	started := time.Now()
	go func() { done <- r.Run(context.Background(), []string{"version", "--json", "--timeout", "40ms"}) }()
	select {
	case code := <-done:
		if code != 1 {
			t.Fatalf("blocked stdout exit %d, want runtime error", code)
		}
	case <-time.After(750 * time.Millisecond):
		_ = writer.Close()
		t.Fatal("command did not terminate under stdout backpressure")
	}
	if time.Since(started) >= time.Second {
		t.Fatal("stdout deadline exceeded the cleanup budget")
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(output) == 0 || json.Valid(output) {
		t.Fatal("fixture did not exercise a partial JSON write")
	}
	if bytes.Contains(output, []byte(`"error":`)) {
		t.Fatal("error JSON was appended to a partially written result")
	}
	if stderr.Len() == 0 {
		t.Fatal("partial output failure was not reported on stderr")
	}
}

func TestRootStderrBackpressureHonorsTimeout(t *testing.T) {
	_, writer := filledRootPipe(t)
	var stdout bytes.Buffer
	r := rootTestRunner(t, strings.NewReader(""), &stdout, writer)
	done := make(chan int, 1)
	started := time.Now()
	go func() {
		done <- r.Run(context.Background(), []string{"protocol", "install", "unsupported", "--user", "alice", "--port", "auto", "--json", "--timeout", "40ms"})
	}()
	select {
	case code := <-done:
		if code != 2 {
			t.Fatalf("invalid protocol exit %d: %s", code, stdout.String())
		}
	case <-time.After(750 * time.Millisecond):
		_ = writer.Close()
		t.Fatal("blocked diagnostics prevented the command from terminating")
	}
	if time.Since(started) >= time.Second {
		t.Fatal("stderr deadline exceeded the cleanup budget")
	}
	assertSingleRootJSON(t, stdout.Bytes(), false)
	if _, err := os.Stat(r.App.LockDir); !os.IsNotExist(err) {
		t.Fatal("invalid protocol initialized runtime state")
	}
}

type rootPartialWriter struct {
	data  bytes.Buffer
	calls int
}

func (w *rootPartialWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.calls == 1 {
		n := len(p) / 2
		_, _ = w.data.Write(p[:n])
		return n, io.ErrClosedPipe
	}
	return w.data.Write(p)
}

func TestRootDoesNotAppendErrorJSONAfterPartialWrite(t *testing.T) {
	var stdout rootPartialWriter
	var stderr bytes.Buffer
	r := rootTestRunner(t, strings.NewReader(""), &stdout, &stderr)
	if code := r.Run(context.Background(), []string{"version", "--json"}); code != 1 {
		t.Fatalf("partial write exit %d, want 1", code)
	}
	if stdout.calls != 1 {
		t.Fatalf("wrote %d stdout results after a partial write", stdout.calls)
	}
	if json.Valid(stdout.data.Bytes()) {
		t.Fatal("fixture did not produce partial output")
	}
	if stderr.Len() == 0 {
		t.Fatal("partial write failure lacked a diagnostic")
	}
}
