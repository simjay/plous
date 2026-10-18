//go:build !unix

package cli

import "os/exec"

func configureTerminalProcess(cmd *exec.Cmd) {}

func killTerminalGroup(cmd *exec.Cmd) { _ = cmd.Process.Kill() }
