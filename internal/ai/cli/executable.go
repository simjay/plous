// Package cli hosts installed interactive agents with scoped Plous tools.
package cli

import (
	"errors"
	"fmt"
	"os/exec"

	"github.com/simjay/plous/internal/ai/agent"
)

func Executable(kind agent.Kind) (string, error) {
	name := ""
	switch kind {
	case agent.ClaudeCode:
		name = "claude"
	case agent.Codex:
		name = "codex"
	default:
		return "", errors.New("unknown external agent")
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s is unavailable: install %s and make it available on PATH", kind.Label(), name)
	}
	return path, nil
}
