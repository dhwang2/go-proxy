package main

import (
	"context"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"go-proxy/internal/cli"
)

var version = "dev"
var revision = ""

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	var interrupted atomic.Int32
	go func() {
		select {
		case sig := <-signals:
			code := int32(130)
			if sig == syscall.SIGTERM {
				code = 143
			}
			interrupted.Store(code)
			cancel()
		case <-ctx.Done():
		}
	}()
	runner := cli.New(version, revision, os.Stdin, os.Stdout, os.Stderr)
	code := runner.Run(ctx, os.Args[1:])
	if signalCode := interrupted.Load(); signalCode != 0 {
		code = int(signalCode)
	}
	os.Exit(code)
}
