//go:build windows

package sandbox

import (
	"os/exec"
)

// setupProcessGroup on Windows registers process termination on context cancellation.
func setupProcessGroup(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
}
