package architecture

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const module = "github.com/simjay/plous/"

func TestOwnershipBoundaries(t *testing.T) {
	violations, err := ownershipViolations(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, violation := range violations {
		t.Error(violation)
	}
}

func TestNativeCLIImportBoundary(t *testing.T) {
	for _, dependency := range []string{
		"context", module + "internal/ai/agent", "github.com/creack/pty", "github.com/pelletier/go-toml/v2",
		module + "internal/ai/history", module + "internal/app/chat", "github.com/cloudwego/eino/schema",
	} {
		t.Run(dependency, func(t *testing.T) {
			root := t.TempDir()
			for _, directory := range []string{"cmd", "internal/ai/cli"} {
				if err := os.MkdirAll(filepath.Join(root, directory), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(root, "internal/ai/cli/native.go"), []byte(fmt.Sprintf("package cli\nimport _ %q\n", dependency)), 0600); err != nil {
				t.Fatal(err)
			}
			violations, err := ownershipViolations(root)
			if err != nil {
				t.Fatal(err)
			}
			forbidden := strings.Contains(dependency, "/history") || strings.Contains(dependency, "/app/") || strings.Contains(dependency, "/eino/")
			if (len(violations) > 0) != forbidden {
				t.Fatalf("unexpected boundary result: %v", violations)
			}
		})
	}
}

func ownershipViolations(root string) ([]string, error) {
	var violations []string
	checkFile := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range f.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			dependency := strings.TrimPrefix(imported, module)
			if violation := applicationBoundaryViolation(rel, dependency); violation != "" {
				violations = append(violations, violation)
			}
			if strings.HasPrefix(rel, "internal/ai/cli/") && strings.Contains(strings.Split(imported, "/")[0], ".") && !slices.Contains([]string{module + "internal/ai/agent", "github.com/creack/pty", "github.com/pelletier/go-toml/v2"}, imported) {
				violations = append(violations, fmt.Sprintf("%s imports %s outside the native process and shared agent boundary", rel, imported))
			}
			if strings.HasPrefix(rel, "internal/ai/agent/") && strings.Contains(strings.Split(imported, "/")[0], ".") {
				violations = append(violations, fmt.Sprintf("%s imports non-standard dependency %s into agent contracts", rel, imported))
			}
			if violation := toolBoundaryViolation(rel, dependency); violation != "" {
				violations = append(violations, violation)
			}
			if !strings.HasPrefix(imported, module) {
				continue
			}
			if dependency == "tests" || strings.HasPrefix(dependency, "tests/") || dependency == "benchmark" || strings.HasPrefix(dependency, "benchmark/") {
				violations = append(violations, fmt.Sprintf("%s imports test-only dependency %s", rel, dependency))
			}
			for _, rule := range []struct {
				owner     string
				forbidden []string
			}{
				{"internal/integrations/observation/", []string{"internal/ai", "internal/app", "internal/workspace", "internal/integrations/kubernetes", "internal/integrations/helm"}},
				{"internal/ui/", []string{"internal/app", "internal/integrations", "internal/workspace", "internal/ai"}},
				{"internal/integrations/", []string{"internal/app", "internal/ui", "internal/ai", "internal/workspace"}},
				{"internal/ai/model/", []string{"internal/app", "internal/ai/history", "internal/workspace"}},
				{"internal/ai/history/", []string{"internal/app", "internal/workspace", "internal/integrations"}},
			} {
				if !strings.HasPrefix(rel, rule.owner) {
					continue
				}
				for _, forbidden := range rule.forbidden {
					if rule.owner == "internal/ai/history/" && dependency == "internal/integrations/observation" {
						continue
					}
					if dependency == forbidden || strings.HasPrefix(dependency, forbidden+"/") {
						violations = append(violations, fmt.Sprintf("%s crosses ownership boundary into %s", rel, dependency))
					}
				}
			}
			if strings.HasPrefix(rel, "internal/app/") && strings.Count(rel, "/") > 2 && dependency == "internal/app" {
				violations = append(violations, fmt.Sprintf("feature %s imports application composition", rel))
			}
		}
		return nil
	}
	for _, directory := range []string{"cmd", "internal"} {
		if err := filepath.WalkDir(filepath.Join(root, directory), checkFile); err != nil {
			return nil, err
		}
	}
	return violations, nil
}

func integrationSurface(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "internal/integrations/"), "/")
	if strings.HasPrefix(path, "internal/integrations/") && len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

func applicationBoundaryViolation(owner, dependency string) string {
	if !strings.HasPrefix(owner, "internal/app/") {
		return ""
	}
	entry := filepath.ToSlash(filepath.Dir(owner)) == "internal/app"
	interactive := strings.HasPrefix(owner, "internal/app/tui/")
	if !entry && !interactive && dependency == "internal/app/tui" {
		return fmt.Sprintf("%s imports the interactive host from %s", owner, dependency)
	}
	if entry {
		for _, prefix := range []string{"charm.land/", "internal/ui", "internal/app/terminal", "internal/ai/history", "internal/workspace"} {
			if dependency == prefix || strings.HasPrefix(dependency, prefix+"/") || strings.HasSuffix(prefix, "/") && strings.HasPrefix(dependency, prefix) {
				return fmt.Sprintf("%s owns interactive lifecycle dependency %s outside tui", owner, dependency)
			}
		}
	}
	return ""
}

func TestApplicationEntryAndInteractiveHostBoundaries(t *testing.T) {
	for _, test := range []struct {
		owner, dependency string
		allowed           bool
	}{
		{"app.go", "internal/app/tui", true},
		{"app.go", "internal/app/debugging", true},
		{"app.go", "charm.land/bubbletea/v2", false},
		{"app.go", "internal/ai/history", false},
		{"app.go", "internal/workspace", false},
		{"app.go", "internal/app/terminal", false},
		{"tui/runtime.go", "internal/app/terminal", true},
		{"tui/model.go", "internal/app/chat", true},
		{"chat/controller.go", "internal/app/tui", false},
		{"navigation/load.go", "internal/app/tui", false},
		{"debugging/command.go", "internal/app/tui", false},
		{"terminal/output_unix.go", "internal/app/tui", false},
	} {
		owner := "internal/app/" + test.owner
		if allowed := applicationBoundaryViolation(owner, test.dependency) == ""; allowed != test.allowed {
			t.Errorf("%s -> %s: allowed=%v", owner, test.dependency, allowed)
		}
	}
}

func toolBoundaryViolation(owner, dependency string) string {
	surface, target := integrationSurface(owner), integrationSurface(dependency)
	if surface == "common" {
		for _, client := range []string{"rest", "dynamic", "kubernetes", "discovery", "plugin", "tools/remotecommand", "tools/portforward"} {
			prefix := "k8s.io/client-go/" + client
			if dependency == prefix || strings.HasPrefix(dependency, prefix+"/") {
				return fmt.Sprintf("%s imports live client dependency %s outside Monitor", owner, dependency)
			}
		}
	}
	forbidden := surface == "common" && target == "monitor"
	if forbidden {
		return fmt.Sprintf("%s crosses tool interface boundary into %s", owner, dependency)
	}
	return ""
}

func TestToolInterfaceBoundaries(t *testing.T) {
	for _, test := range []struct {
		owner, dependency string
		allowed           bool
	}{
		{"flux/common/declarations.go", "flux/monitor", false},
		{"argo/monitor/client.go", "gitops/monitor", true},
	} {
		owner := "internal/integrations/" + test.owner
		dependency := "internal/integrations/" + test.dependency
		if allowed := toolBoundaryViolation(owner, dependency) == ""; allowed != test.allowed {
			t.Errorf("%s -> %s: allowed=%v", owner, dependency, allowed)
		}
	}
	for _, client := range []string{"k8s.io/client-go/rest", "k8s.io/client-go/plugin/pkg/client/auth"} {
		if toolBoundaryViolation("internal/integrations/kubernetes/common/resource.go", client) == "" {
			t.Fatalf("common must not initialize live clients: %s", client)
		}
	}
}

func TestToolSurfaceLayout(t *testing.T) {
	for _, tool := range []string{"kubernetes", "helm", "argo", "flux"} {
		root := filepath.Join("..", "..", "internal", "integrations", tool)
		flat, err := filepath.Glob(filepath.Join(root, "*.go"))
		if err != nil || len(flat) != 0 {
			t.Errorf("%s has implementation outside its interfaces: %v, %v", tool, flat, err)
		}
		for _, surface := range []string{"monitor", "common"} {
			files, err := filepath.Glob(filepath.Join(root, surface, "*.go"))
			if err != nil || len(files) == 0 {
				t.Errorf("%s/%s interface is missing: %v", tool, surface, err)
			}
		}
	}
}

func TestOwnershipRejectsTestOnlyImports(t *testing.T) {
	for _, dependency := range []string{"benchmark", "benchmark/fixtures", "tests", "tests/support"} {
		t.Run(dependency, func(t *testing.T) {
			root := t.TempDir()
			for _, directory := range []string{"cmd/plous", "internal"} {
				if err := os.MkdirAll(filepath.Join(root, directory), 0700); err != nil {
					t.Fatal(err)
				}
			}
			main := "package main\nimport _ " + strconv.Quote(module+dependency) + "\n"
			if err := os.WriteFile(filepath.Join(root, "cmd", "plous", "main.go"), []byte(main), 0600); err != nil {
				t.Fatal(err)
			}
			violations, err := ownershipViolations(root)
			if err != nil {
				t.Fatal(err)
			}
			want := "cmd/plous/main.go imports test-only dependency " + dependency
			if len(violations) != 1 || violations[0] != want {
				t.Fatalf("test-only product dependency was not identified: %v", violations)
			}
		})
	}
}

func TestProductLayout(t *testing.T) {
	root := filepath.Join("..", "..")
	owners := map[string]bool{"ai": true, "app": true, "integrations": true, "ui": true, "workspace": true}
	entries, err := os.ReadDir(filepath.Join(root, "internal"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() && !owners[entry.Name()] {
			t.Errorf("unassigned internal owner %s; place related code under its existing domain", entry.Name())
		}
	}
	commands, err := os.ReadDir(filepath.Join(root, "cmd"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range commands {
		if entry.IsDir() && entry.Name() != "plous" {
			t.Errorf("unexpected product entry point cmd/%s", entry.Name())
		}
	}

	for _, required := range []string{"cmd/plous/main.go", "cmd/plous/VERSION", "internal/ai/run.go"} {
		if _, err := os.Stat(filepath.Join(root, required)); err != nil {
			t.Error(err)
		}
	}
}
