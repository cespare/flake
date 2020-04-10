package main

import (
	"os"

	"golang.org/x/sys/unix"
)

var stdoutIsTTY bool

func init() {
	_, err := unix.IoctlGetTermios(int(os.Stdout.Fd()), ioctlReadTermios)
	stdoutIsTTY = err == nil
}
