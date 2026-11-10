package agent

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// Commands keeps each native agent's entry command independent of the selection.
type Commands struct {
	ClaudeCode string `json:"claude_code,omitempty"`
	Codex      string `json:"codex,omitempty"`
}

func (c Commands) For(kind Kind) string {
	switch kind {
	case ClaudeCode:
		if strings.TrimSpace(c.ClaudeCode) != "" {
			return c.ClaudeCode
		}
		return "claude"
	case Codex:
		if strings.TrimSpace(c.Codex) != "" {
			return c.Codex
		}
		return "codex"
	}
	return ""
}

func (c Commands) With(kind Kind, command string) Commands {
	command = strings.TrimSpace(command)
	if command == (Commands{}).For(kind) {
		command = ""
	}
	switch kind {
	case ClaudeCode:
		c.ClaudeCode = command
	case Codex:
		c.Codex = command
	}
	return c
}

func (c Commands) Validate() error {
	for _, kind := range []Kind{ClaudeCode, Codex} {
		if _, err := CommandArgs(c.For(kind)); err != nil {
			return fmt.Errorf("%s entry command: %w", kind.Label(), err)
		}
	}
	return nil
}

// CommandArgs supports quoted arguments without invoking a shell or expanding values.
func CommandArgs(command string) ([]string, error) {
	var args []string
	var word strings.Builder
	var quote rune
	escaped, started := false, false
	for _, r := range command {
		if r == 0 || r == '\n' || r == '\r' {
			return nil, errors.New("use a single line without NUL characters")
		}
		switch {
		case escaped:
			if quote == '"' && r != '"' && r != '\\' && r != '$' && r != '`' {
				word.WriteRune('\\')
			}
			word.WriteRune(r)
			escaped = false
		case r == '\\' && quote != '\'':
			escaped, started = true, true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				word.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, started = r, true
		case unicode.IsSpace(r):
			if started {
				args = append(args, word.String())
				word.Reset()
				started = false
			}
		default:
			word.WriteRune(r)
			started = true
		}
	}
	if escaped || quote != 0 {
		return nil, errors.New("close quotes and finish escaped characters")
	}
	if started {
		args = append(args, word.String())
	}
	if len(args) == 0 || args[0] == "" {
		return nil, errors.New("an executable is required")
	}
	return args, nil
}
