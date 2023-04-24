//go:build unix

package main

import (
	"context"
	"os/exec"

	"golang.org/x/sys/unix"
)

func commandContext(ctx context.Context, command string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.SysProcAttr = &unix.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return unix.Kill(-cmd.Process.Pid, unix.SIGKILL)
	}
	return cmd
}
