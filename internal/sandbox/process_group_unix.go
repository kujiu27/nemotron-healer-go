//go:build !windows

package sandbox

import (
	"os/exec"
	"syscall"
)

// setupProcessGroup ensures subprocess and all its spawned children share a new
// process group, and registers a cancel function sending SIGKILL to the entire group
// on context timeout or cancellation, preventing orphaned runaway child processes.
func setupProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil && cmd.Process.Pid > 0 {
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
}
