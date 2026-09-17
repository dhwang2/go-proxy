package network

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

type commandOutput struct {
	data     []byte
	command  *exec.Cmd
	overflow bool
}

func (b *commandOutput) Write(p []byte) (int, error) {
	if len(b.data)+len(p) > 1<<20 {
		b.overflow = true
		_ = b.command.Cancel()
		return 0, errors.New("network command output exceeds 1 mib")
	}
	b.data = append(b.data, p...)
	return len(p), nil
}

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	command.WaitDelay = 250 * time.Millisecond
	output := &commandOutput{command: command}
	command.Stdout = output
	command.Stderr = output
	err := command.Run()
	if output.overflow {
		return nil, errors.New("network command output exceeds 1 mib")
	}
	if ctx.Err() != nil {
		return output.data, ctx.Err()
	}
	return output.data, err
}
