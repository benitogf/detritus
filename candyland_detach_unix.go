//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// detachProcess puts the spawned sidecar in its own session so it outlives
// detritus and isn't killed by signals sent to detritus's process group.
func detachProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
