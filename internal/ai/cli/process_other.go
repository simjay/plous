//go:build !unix

package cli

import "os/exec"

func configureProcess(cmd *exec.Cmd) {}
