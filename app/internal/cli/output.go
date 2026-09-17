package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"time"
)

type contextWriter struct {
	ctx    context.Context
	writer io.Writer
}

func (w contextWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	if f, ok := w.writer.(*os.File); ok {
		deadline, hasDeadline := w.ctx.Deadline()
		if !hasDeadline {
			deadline = time.Now().Add(24 * time.Hour)
		}
		if f.SetWriteDeadline(deadline) == nil {
			finished := make(chan struct{})
			stop := context.AfterFunc(w.ctx, func() { _ = f.SetWriteDeadline(time.Now()); close(finished) })
			n, err := f.Write(p)
			if !stop() {
				<-finished
			}
			_ = f.SetWriteDeadline(time.Time{})
			if cause := w.ctx.Err(); cause != nil {
				return n, cause
			}
			return n, err
		}
	}
	switch w.writer.(type) {
	case *bytes.Buffer, *strings.Builder:
		return w.writer.Write(p)
	}
	data := append([]byte(nil), p...)
	type response struct {
		n   int
		err error
	}
	done := make(chan response, 1)
	go func() { n, err := w.writer.Write(data); done <- response{n, err} }()
	select {
	case result := <-done:
		return result.n, result.err
	case <-w.ctx.Done():
		if closer, ok := w.writer.(io.Closer); ok {
			go closer.Close()
		}
		return 0, w.ctx.Err()
	}
}
