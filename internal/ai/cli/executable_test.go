package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/simjay/plous/internal/ai/agent"
)

func TestResolveCommandUsesWrapperAndReportsMissingCommand(t *testing.T) {
	dir := t.TempDir()
	wrapper := filepath.Join(dir, "celai")
	if err := os.WriteFile(wrapper, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	for _, kind := range []agent.Kind{agent.ClaudeCode, agent.Codex} {
		command := "celai " + (agent.Commands{}).For(kind) + ` --profile "team one"`
		path, args, err := ResolveCommand(kind, command)
		if err != nil || path != wrapper || !reflect.DeepEqual(args, []string{(agent.Commands{}).For(kind), "--profile", "team one"}) {
			t.Fatalf("resolved %s: %q %q, %v", kind, path, args, err)
		}
		_, _, err = ResolveCommand(kind, "missing-wrapper codex")
		if err == nil || !strings.Contains(err.Error(), "missing-wrapper") || !strings.Contains(err.Error(), "Settings") {
			t.Fatal("missing wrapper was not explained", err)
		}
	}
}
