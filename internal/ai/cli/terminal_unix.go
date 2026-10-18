//go:build unix

package cli

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func configureTerminalProcess(cmd *exec.Cmd) {
	// pty.StartWithSize creates the session and controlling terminal.
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}

func killTerminalGroup(cmd *exec.Cmd) { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
