//go:build !unix

package main

import (
	"context"
	"os/exec"
)

func commandContext(ctx context.Context, command string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, command, args...)
}
