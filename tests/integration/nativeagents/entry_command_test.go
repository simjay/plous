package nativeagents

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/simjay/plous/internal/ai/agent"
	"github.com/simjay/plous/internal/ai/cli"
)

func TestEntryCommandLaunchesSetupAndNativeTerminal(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []agent.Kind{agent.ClaudeCode, agent.Codex} {
		for _, resume := range []bool{false, true} {
			t.Run(string(kind)+"/resume="+strconv.FormatBool(resume), func(t *testing.T) {
				reports := t.TempDir()
				command := strconv.Quote(executable) + ` -test.run=^TestEntryCommandFixture$ -- --plous-entry-fixture ` + string(kind) + ` "team one" ` + strconv.Quote(reports)
				path, prefix, err := cli.ResolveCommand(kind, command)
				if err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
				defer cancel()
				session, err := cli.StartTerminal(ctx, cli.TerminalOptions{Kind: kind, Path: path, Args: prefix, Directory: t.TempDir(), StateDirectory: t.TempDir(), Resume: resume, Tools: &installedTools{}, Width: 80, Height: 24, Input: strings.NewReader(""), Output: io.Discard})
				if err != nil {
					t.Fatal(err)
				}
				defer session.Close()
				if err := session.Wait(); err != nil {
					t.Fatal(err)
				}
				readArgs := func(name string) []string {
					t.Helper()
					data, err := os.ReadFile(filepath.Join(reports, name+".json"))
					if err != nil {
						t.Fatal(err)
					}
					var args []string
					if err := json.Unmarshal(data, &args); err != nil {
						t.Fatal(err)
					}
					if len(args) < 3 || !reflect.DeepEqual(args[:3], []string{string(kind), "team one", reports}) {
						t.Fatal("wrapper prefix was changed", args)
					}
					return args[3:]
				}
				args := readArgs("terminal")
				if kind == agent.Codex {
					if got := readArgs("setup"); !reflect.DeepEqual(got, []string{"app-server", "--listen", "stdio://"}) {
						t.Fatal("setup did not use the entry command", got)
					}
					if !slices.Contains(args, "read-only") || !slices.Contains(args, "never") || resume && !slices.Equal(args[len(args)-2:], []string{"resume", "--last"}) {
						t.Fatal("Codex controls or resume arguments were lost", args)
					}
				} else {
					flag := "--session-id"
					if resume {
						flag = "--resume"
					}
					if !slices.Contains(args, "--strict-mcp-config") || !slices.Contains(args, "dontAsk") || args[len(args)-2] != flag {
						t.Fatal("Claude Code controls or resume arguments were lost", args)
					}
				}
			})
		}
	}
}

// The test executable acts as an enterprise wrapper, without installed CLIs or model calls.
func TestEntryCommandFixture(t *testing.T) {
	i := slices.Index(os.Args, "--plous-entry-fixture")
	if i < 0 {
		return
	}
	args := os.Args[i+1:]
	if len(args) < 4 {
		os.Exit(2)
	}
	phase := "terminal"
	if args[3] == "app-server" {
		phase = "setup"
	}
	data, _ := json.Marshal(args)
	if os.WriteFile(filepath.Join(args[2], phase+".json"), data, 0600) != nil {
		os.Exit(3)
	}
	if phase == "setup" {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			var request struct {
				ID     int    `json:"id"`
				Method string `json:"method"`
			}
			if json.Unmarshal(scanner.Bytes(), &request) != nil {
				os.Exit(4)
			}
			if request.ID == 0 {
				continue
			}
			result := map[string]any{}
			if request.Method == "config/read" {
				result["config"] = map[string]any{"mcp_servers": map[string]any{}}
			}
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"id": request.ID, "result": result})
		}
	}
	os.Exit(0)
}
