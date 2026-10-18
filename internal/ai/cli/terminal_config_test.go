package cli

import (
	"encoding/json"
	"github.com/simjay/plous/internal/ai/agent"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestNativeCodexRequiresExplicitServerInventory(t *testing.T) {
	for _, raw := range []string{`{}`, `{"config":{}}`, `{"config":null}`, `{"config":{"mcp_servers":null}}`, `{"config":{"mcp_servers":[]}}`} {
		if _, err := codexServers(json.RawMessage(raw)); err == nil {
			t.Errorf("unknown server inventory accepted: %s", raw)
		}
	}
	for _, raw := range []string{`{"config":{"mcp_servers":{}}}`, `{"config":{"mcp_servers":{"plous":{"url":"http://127.0.0.1:1"},"name.with.dots":{"command":"fixture"}}}}`} {
		if _, err := codexServers(json.RawMessage(raw)); err != nil {
			t.Errorf("explicit server inventory rejected: %v", err)
		}
	}
}

func TestClaudeSessionIdentitySurvivesResumeAndSeparatesScopes(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	id, err := claudeSessionID(first, false)
	if err != nil || !sessionUUID.MatchString(id) {
		t.Fatalf("new session: %q %v", id, err)
	}
	resumed, err := claudeSessionID(first, true)
	if err != nil || resumed != id {
		t.Fatalf("resume: %q %v", resumed, err)
	}
	other, err := claudeSessionID(second, false)
	if err != nil || other == id {
		t.Fatalf("other scope: %q %v", other, err)
	}
	fresh, err := claudeSessionID(first, false)
	if err != nil || fresh == id {
		t.Fatalf("new conversation: %q %v", fresh, err)
	}
	for _, resume := range []bool{false, true} {
		flag := "--session-id"
		if resume {
			flag = "--resume"
		}
		args, err := terminalResumeArgs(TerminalOptions{Kind: agent.ClaudeCode, Resume: resume, sessionID: fresh}, []string{"--tools", ""})
		if err != nil || !slices.Equal(args, []string{"--tools", "", flag, fresh}) {
			t.Fatalf("unbound conversation launch: %v %v", args, err)
		}
	}
	if err := os.WriteFile(filepath.Join(first, "session-id"), []byte("some other session"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := claudeSessionID(first, true); err == nil {
		t.Fatal("accepted corrupt session identity")
	}
}

func TestNativeResumeRequiresAgentSpecificPolicy(t *testing.T) {
	for _, kind := range []agent.Kind{agent.Plous, "opencode", "unknown", ""} {
		for _, resume := range []bool{false, true} {
			args, err := terminalResumeArgs(TerminalOptions{Kind: kind, Resume: resume}, []string{"--fixture"})
			if err == nil || args != nil {
				t.Fatalf("agent %q inherited another agent's launch policy: %v %v", kind, args, err)
			}
		}
	}
	for _, resume := range []bool{false, true} {
		want := []string{"--fixture"}
		if resume {
			want = append(want, "resume", "--last")
		}
		args, err := terminalResumeArgs(TerminalOptions{Kind: agent.Codex, Resume: resume}, []string{"--fixture"})
		if err != nil || !slices.Equal(args, want) {
			t.Fatalf("Codex resume changed: %v %v", args, err)
		}
	}
}
