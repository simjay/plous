package agent

import (
	"reflect"
	"testing"
)

func TestCommandArgs(t *testing.T) {
	for _, tc := range []struct {
		command string
		want    []string
	}{
		{"celai codex", []string{"celai", "codex"}},
		{` "/opt/Company Tools/celai" claude --profile 'team one' `, []string{"/opt/Company Tools/celai", "claude", "--profile", "team one"}},
		{`celai team\ one "a\"b" ''`, []string{"celai", "team one", `a"b`, ""}},
		{"celai $HOME $(whoami) `id` ; | > ~", []string{"celai", "$HOME", "$(whoami)", "`id`", ";", "|", ">", "~"}},
	} {
		t.Run(tc.command, func(t *testing.T) {
			got, err := CommandArgs(tc.command)
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("args = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
	for _, command := range []string{"", " \t", `'' codex`, `celai "codex`, `celai \`, "celai\ncodex", "celai\x00codex"} {
		if _, err := CommandArgs(command); err == nil {
			t.Fatalf("accepted invalid command %q", command)
		}
	}
}

func TestCommandsKeepAgentDefaultsIndependent(t *testing.T) {
	c := Commands{}
	if c.For(ClaudeCode) != "claude" || c.For(Codex) != "codex" || c.For(Plous) != "" {
		t.Fatal("incorrect defaults", c)
	}
	c = c.With(Codex, " celai codex ").With(ClaudeCode, "celai claude")
	if c.For(Codex) != "celai codex" || c.For(ClaudeCode) != "celai claude" || c.Validate() != nil {
		t.Fatal("commands are not independent", c)
	}
	c = c.With(Codex, "codex").With(ClaudeCode, "  ")
	if c != (Commands{}) {
		t.Fatal("defaults did not clear overrides", c)
	}
}
