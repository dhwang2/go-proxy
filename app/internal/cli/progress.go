package cli

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// A long operation says what it is doing as it goes. Certificate issuance can
// take minutes, and a line that has not changed in two minutes is
// indistinguishable from a hung program, so on a terminal the current step
// animates in place with the time it has taken.
//
// Only on a terminal. A pipe, a file, a captured log and an agent get the same
// steps as plain lines, written once and never rewritten, because a carriage
// return in a captured stream is contamination -- the same rule colour follows,
// checked against the same writer.
type progressLine struct {
	out     io.Writer
	animate bool

	mu      sync.Mutex
	label   string
	started time.Time
	frame   int
	drawn   bool
	stop    chan struct{}
	stopped chan struct{}
}

// spinnerFrames is one braille cell rotating. It occupies one column in every
// terminal that can render it, so the label never shifts.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

const spinnerInterval = 120 * time.Millisecond

func newProgressLine(out io.Writer, animate bool) *progressLine {
	return &progressLine{out: out, animate: animate}
}

// show reports a new step. The step that was running is finalised as an
// ordinary line first, so the transcript a terminal ends up with is the same
// one a pipe gets.
func (p *progressLine) show(message string) {
	if p == nil {
		return
	}
	if !p.animate {
		fmt.Fprintln(p.out, message)
		return
	}
	p.mu.Lock()
	p.commitLocked()
	p.label, p.started, p.frame = message, time.Now(), 0
	p.drawLocked()
	p.mu.Unlock()
	p.start()
}

// finish ends the animation, leaving the last step on screen as an ordinary
// line. Whatever the command has to say next -- a result, or the reason it
// failed -- follows it.
func (p *progressLine) finish() {
	if p == nil || !p.animate {
		return
	}
	p.mu.Lock()
	running := p.stop
	p.stop, p.stopped = nil, nil
	p.mu.Unlock()
	if running != nil {
		close(running)
	}
	p.mu.Lock()
	p.commitLocked()
	p.mu.Unlock()
}

func (p *progressLine) start() {
	p.mu.Lock()
	if p.stop != nil || p.label == "" {
		p.mu.Unlock()
		return
	}
	stop, stopped := make(chan struct{}), make(chan struct{})
	p.stop, p.stopped = stop, stopped
	p.mu.Unlock()
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(spinnerInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				p.mu.Lock()
				p.frame++
				p.drawLocked()
				p.mu.Unlock()
			}
		}
	}()
}

// drawLocked rewrites the animated line in place.
func (p *progressLine) drawLocked() {
	if p.label == "" {
		return
	}
	elapsed := ""
	// Below a second there is nothing to report but the motion itself.
	if waited := time.Since(p.started); waited >= time.Second {
		elapsed = " " + waited.Round(time.Second).String()
	}
	fmt.Fprintf(p.out, "\r\x1b[K%s %s%s", spinnerFrames[p.frame%len(spinnerFrames)], p.label, elapsed)
	p.drawn = true
}

// commitLocked turns the animated line into an ordinary one: the same text a
// pipe would have received, with the spinner and the clock removed.
func (p *progressLine) commitLocked() {
	if !p.drawn {
		return
	}
	fmt.Fprintf(p.out, "\r\x1b[K%s\n", p.label)
	p.drawn = false
	p.label = ""
}
