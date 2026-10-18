package ai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func nativeOptions(t *testing.T, o *Options) {
	t.Helper()
	c := DefaultRuntimeConfig()
	o.Runtime = &c
}

func TestRuntimeConfigRejectsRemovedSettings(t *testing.T) {
	o, _ := fixture(t)
	path := filepath.Join(t.TempDir(), "agent.yaml")
	t.Setenv("PLOUS_AGENT_CONFIG", path)
	for _, old := range []string{"history_mode: rlm\n", "history_mode: direct\n", "rlm:\n  max_depth: 2\n", "tools_allow: tools.allow\n"} {
		if err := os.WriteFile(path, []byte(old), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadRuntimeConfig(o.Store, nil); err == nil || !strings.Contains(err.Error(), "supported settings") {
			t.Fatal("removed setting was silently accepted", old, err)
		}
	}
	if err := os.WriteFile(path, []byte("max_calls: 12\nmax_tokens: 250000\ntimeout_seconds: 120\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := loadRuntimeConfig(o.Store, nil)
	if err != nil || c.MaxSteps != 12 || c.MaxTokens != 250000 || c.TimeoutSeconds != 120 {
		t.Fatal(c, err)
	}
}
