// Package cli hosts installed interactive agents with scoped Plous tools.
package cli

import (
	"errors"
	"fmt"
	"os/exec"

	"github.com/simjay/plous/internal/ai/agent"
)

func Executable(kind agent.Kind) (string, error) {
	path, _, err := ResolveCommand(kind, (agent.Commands{}).For(kind))
	return path, err
}

func ResolveCommand(kind agent.Kind, command string) (string, []string, error) {
	if kind != agent.ClaudeCode && kind != agent.Codex {
		return "", nil, errors.New("unknown external agent")
	}
	args, err := agent.CommandArgs(command)
	if err != nil {
		return "", nil, fmt.Errorf("%s entry command: %w", kind.Label(), err)
	}
	path, err := exec.LookPath(args[0])
	if err != nil {
		return "", nil, fmt.Errorf("%s is unavailable: make %s available on PATH or update its Entry command in Settings", kind.Label(), args[0])
	}
	return path, args[1:], nil
}
