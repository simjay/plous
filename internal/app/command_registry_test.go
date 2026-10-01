package app

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestCommandAliasesAreSingleLettersAndDoNotCollide(t *testing.T) {
	commands := append([]commandDefinition(nil), applicationCommands...)
	for _, action := range resourceActionDefinitions {
		commands = append(commands, commandDefinition{action.alias, action.command})
	}
	keymap, err := os.ReadFile("../../docs/keymap.md")
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
			if resource := resourceCommands[spelling]; resource != "" {
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
			if m.explore.preference.Resource != resource || m.explore.picker != "" {
				t.Fatalf(":%s did not open %s", command, resource)
			}
		})
	}
}

func TestDocumentCommandOwnership(t *testing.T) {
	for _, command := range []string{"u", "undo", "U", "redo", "v", "review"} {
		t.Run(command, func(t *testing.T) {
			m, a := documentModel(t)
			a.document.Paste("# draft\n")
			changed := a.document.Text()
			m.focus = 2
			m.execute(command)
			if a.document.Text() != changed || a.pending != "" || m.explore.action != a {
				t.Fatal("a command from another panel changed the open document")
			}
			m.focus = 1
			m.execute("u")
			if a.document.Text() == changed {
				t.Fatal("editor :u did not undo")
			}
			m.execute("U")
			if a.document.Text() != changed {
				t.Fatal("editor :U did not restore the draft")
			}
			m.execute("v")
			if a.pending != "validate" || a.saveRequested || a.closeAfterApply {
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
	for _, action := range resourceActionDefinitions {
		known[action.alias], known[action.command] = true, true
	}
	for resource := range resourceCommands {
		known[resource] = true
	}
	commandPattern := regexp.MustCompile("(?:^|[\\s\\[(/`]):([A-Za-z0-9][A-Za-z0-9.!_-]*)")
	controlPattern := regexp.MustCompile(`(?i)\bctrl\+([a-z0-9]+|[^\s])`)
	controlKeys := map[string]bool{"a": true, "c": true, "e": true, "j": true, "u": true, "]": true}
	check := func(file, text string) {
		for _, match := range commandPattern.FindAllStringSubmatch(text, -1) {
			command := strings.TrimRight(match[1], ".")
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
	docs, err := filepath.Glob("../../docs/*.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range append(docs, "../../README.md") {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		check(path, string(data))
	}
}
