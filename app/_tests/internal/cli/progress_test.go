package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// A stream that cannot be rewritten must never be rewritten. This is the same
// guarantee colour makes, on the same writer, and it is what keeps a captured
// log, a pipe and an agent reading the steps as plain lines.
func TestProgressWritesNoControlCodesWithoutATerminal(t *testing.T) {
	var out bytes.Buffer
	line := newProgressLine(&out, false)
	line.show("downloading sing-box")
	line.show("waiting for certificate issuance")
	line.finish()
	if bytes.ContainsAny(out.Bytes(), "\x1b\r") {
		t.Fatalf("plain progress carries terminal control: %q", out.String())
	}
	if out.String() != "downloading sing-box\nwaiting for certificate issuance\n" {
		t.Fatalf("plain progress is not one line per step: %q", out.String())
	}
}

// On a terminal the current step animates in place, and what is left behind
// when it ends is the same transcript a pipe would have received.
func TestAnimatedProgressLeavesThePlainTranscript(t *testing.T) {
	var out bytes.Buffer
	line := newProgressLine(&out, true)
	line.show("preparing caddy")
	line.show("waiting for certificate issuance")
	line.finish()

	if !strings.Contains(out.String(), "\x1b[K") {
		t.Fatalf("an animated line never erased itself: %q", out.String())
	}
	// Every step survives as its own committed line, in order, with no spinner
	// frame left on it.
	committed := []string{}
	for _, chunk := range strings.Split(out.String(), "\n") {
		if index := strings.LastIndex(chunk, "\x1b[K"); index >= 0 {
			chunk = chunk[index+len("\x1b[K"):]
		}
		if chunk != "" {
			committed = append(committed, chunk)
		}
	}
	if strings.Join(committed, "|") != "preparing caddy|waiting for certificate issuance" {
		t.Fatalf("committed transcript is %q", committed)
	}
	for _, frame := range spinnerFrames {
		if strings.Contains(strings.Join(committed, ""), frame) {
			t.Fatalf("a spinner frame outlived the animation: %q", committed)
		}
	}
}

// The animation reports how long the step has taken, because a certificate can
// take minutes and the question a reader has is whether anything is happening.
func TestAnimatedProgressReportsElapsedTime(t *testing.T) {
	var out bytes.Buffer
	line := newProgressLine(&out, true)
	line.show("waiting for certificate issuance")
	line.mu.Lock()
	line.started = time.Now().Add(-83 * time.Second)
	line.drawLocked()
	line.mu.Unlock()
	line.finish()
	if !strings.Contains(out.String(), "1m23s") {
		t.Fatalf("no elapsed time in %q", out.String())
	}
	// Not on the line that outlives it.
	if strings.Contains(out.String(), "issuance 1m23s\n") {
		t.Fatalf("the committed line kept the clock: %q", out.String())
	}
}

// finish is called on the way out of every mutation and again after the handler
// returns, so it has to be harmless the second time.
func TestFinishIsSafeTwiceAndWithoutAStep(t *testing.T) {
	var out bytes.Buffer
	line := newProgressLine(&out, true)
	line.finish()
	line.show("starting gproxy protocol add")
	line.finish()
	before := out.Len()
	line.finish()
	if out.Len() != before {
		t.Fatalf("a second finish wrote more: %q", out.String())
	}
	var nilLine *progressLine
	nilLine.show("no writer")
	nilLine.finish()
}
