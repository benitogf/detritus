//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// detachProcess starts the sidecar in a new process group so it outlives
// detritus and isn't terminated when detritus's console group receives signals.
func detachProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000200} // CREATE_NEW_PROCESS_GROUP
}
