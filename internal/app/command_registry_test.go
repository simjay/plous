package app

import (
	"github.com/simjay/plous/internal/app/monitor"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestCommandAliasesAreSingleLettersAndDoNotCollide(t *testing.T) {
	commands := append([]commandDefinition(nil), applicationCommands...)
	for _, action := range monitor.ResourceActionDefinitions {
		commands = append(commands, commandDefinition{action.Alias, action.Command})
	}
	keymap, err := os.ReadFile("../../docs/reference/keymap.md")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, command := range commands {
		if len(command.key) != 1 || !strings.ContainsAny(command.key, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") {
			t.Errorf("%s must have one case-sensitive letter, got %q", command.name, command.key)
		}
		for _, spelling := range []string{command.key, command.name} {
			if previous := seen[spelling]; previous != "" {
				t.Errorf("%s and %s both claim %q", previous, command.name, spelling)
			}
			seen[spelling] = command.name
			if resource := monitor.ResourceCommands[spelling]; resource != "" {
				t.Errorf("%s shadows resource %s", spelling, resource)
			}
			if !strings.Contains(string(keymap), "`:"+spelling+"`") {
				t.Errorf("keymap does not document :%s", spelling)
			}
		}
	}
	for _, command := range applicationCommands {
		for _, suffix := range []string{"", " shop"} {
			if got := canonicalCommand(command.key + suffix); got != command.name+suffix || canonicalCommand(got) != got {
				t.Errorf("alias %q was not resolved exactly: %q", command.key+suffix, got)
			}
		}
	}
}

func TestNamespaceAndStorageResourceCommandsKeepTheirKubernetesMeaning(t *testing.T) {
	for command, resource := range map[string]string{"ns": "namespaces", "sc": "storageclasses.storage.k8s.io"} {
		t.Run(command, func(t *testing.T) {
			m := exploreModel(t)
			m.execute(command)
			if m.explore.Preference.Resource != resource || m.explore.Picker != "" {
				t.Fatalf(":%s did not open %s", command, resource)
			}
		})
	}
}

func TestDocumentCommandOwnership(t *testing.T) {
	for _, command := range []string{"u", "undo", "U", "redo", "v", "review"} {
		t.Run(command, func(t *testing.T) {
			m, a := documentModel(t)
			a.Document.Paste("# draft\n")
			changed := a.Document.Text()
			m.focus = 2
			m.execute(command)
			if a.Document.Text() != changed || a.Pending != "" || m.explore.Action != a {
				t.Fatal("a command from another panel changed the open document")
			}
			m.focus = 1
			m.execute("u")
			if a.Document.Text() == changed {
				t.Fatal("editor :u did not undo")
			}
			m.execute("U")
			if a.Document.Text() != changed {
				t.Fatal("editor :U did not restore the draft")
			}
			m.execute("v")
			if a.Pending != "validate" || a.SaveRequested || a.CloseAfterApply {
				t.Fatal("optional review must validate without requesting a save")
			}
		})
	}
}

// Check the text users see against the same registries that dispatch commands.
func TestDisplayedColonCommandsAreRegistered(t *testing.T) {
	known := map[string]bool{"1": true, "2": true, "3": true, "wq": true, "q!": true, "undo": true, "redo": true, "U": true}
	for _, command := range applicationCommands {
		known[command.key], known[command.name] = true, true
	}
	for _, action := range monitor.ResourceActionDefinitions {
		known[action.Alias], known[action.Command] = true, true
	}
	for resource := range monitor.ResourceCommands {
		known[resource] = true
	}
	controlPattern := regexp.MustCompile(`(?i)\bctrl\+((?:[a-z]+\+)*(?:[a-z0-9]+|[^\s]))`)
	controlKeys := map[string]bool{"a": true, "c": true, "e": true, "h": true, "j": true, "u": true, "]": true, "shift+c": true, "shift+v": true}
	check := func(file, text string) {
		for _, command := range displayedColonCommands(text) {
			if command == "widgets.example.io" { // Documented API-qualified resource example.
				continue
			}
			if !known[command] {
				t.Errorf("%s advertises unregistered command :%s", file, command)
			}
		}
		for _, match := range controlPattern.FindAllStringSubmatch(text, -1) {
			if !controlKeys[strings.ToLower(match[1])] {
				t.Errorf("%s contains unapproved control binding %s", file, match[0])
			}
		}
	}
	fset := token.NewFileSet()
	err := filepath.WalkDir("..", func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if literal, ok := node.(*ast.BasicLit); ok && literal.Kind == token.STRING {
				text, err := strconv.Unquote(literal.Value)
				if err == nil {
					check(fset.Position(literal.Pos()).String(), text)
				}
			}
			return true
		})
		for _, comment := range file.Comments {
			check(fset.Position(comment.Pos()).String(), comment.Text())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	docs := []string{"../../README.md"}
	err = filepath.WalkDir("../../docs", func(path string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && strings.HasSuffix(path, ".md") {
			docs = append(docs, path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range docs {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		check(path, string(data))
	}
}

var displayedCommandPattern = regexp.MustCompile("(?:^|[\\s\\[(/`]):([A-Za-z0-9][A-Za-z0-9.!_-]*)")

func displayedColonCommands(text string) []string {
	var commands []string
	for _, match := range displayedCommandPattern.FindAllStringSubmatchIndex(text, -1) {
		end := match[3]
		if end < len(text) && text[end] == '/' && (end+1 == len(text) || text[end+1] != ':') {
			continue
		}
		commands = append(commands, strings.TrimRight(text[match[2]:end], "."))
	}
	return commands
}

func TestDisplayedCommandScannerDistinguishesPaths(t *testing.T) {
	for _, tc := range []struct {
		text string
		want []string
	}{
		{"Use :s, :settings and :pods. See [:H].", []string{"s", "settings", "pods", "H"}},
		{":refs/plous/source", nil},
		{":refs/heads/main :s :q!", []string{"s", "q!"}},
		{":s/:settings", []string{"s", "settings"}},
		{":widgets.example.io", []string{"widgets.example.io"}},
	} {
		if got := displayedColonCommands(tc.text); !slices.Equal(got, tc.want) {
			t.Errorf("%q: got %v, want %v", tc.text, got, tc.want)
		}
	}
}
