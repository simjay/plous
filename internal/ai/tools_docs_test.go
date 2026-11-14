package ai

import (
	"os"
	"regexp"
	"strings"
	"testing"

	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
)

func TestDocumentedToolsMatchRegisteredCapabilities(t *testing.T) {
	content, err := os.ReadFile("../../docs/reference/mcp.md")
	if err != nil {
		t.Fatal(err)
	}
	_, section, ok := strings.Cut(string(content), "## Available tools\n")
	if !ok {
		t.Fatal("MCP reference is missing the tool inventory section")
	}
	_, inventory, ok := strings.Cut(section, "| Task | Tools | Results |\n")
	if !ok {
		t.Fatal("MCP reference is missing the tool inventory table")
	}
	inventory, _, _ = strings.Cut(inventory, "\n\n")
	documented := map[string]bool{}
	for _, match := range regexp.MustCompile("`([a-z]+(?:_[a-z]+)*_(?:read|show|run))`").FindAllStringSubmatch(inventory, -1) {
		documented[match[1]] = true
	}
	registered := map[string]bool{}
	for _, tool := range ToolDefinitions(Options{
		WorkspaceID: "docs",
		Connections: []Connection{{Tool: "kubernetes", ConnectionID: "cluster", Namespace: "default"}, {Tool: "helm", ConnectionID: "cluster/helm"}},
		Debug:       kubernetes.DebugPolicy{Exec: true, Image: "fixture"},
		UI:          &testUI{},
	}) {
		registered[tool.Name] = true
		if !documented[tool.Name] {
			t.Errorf("registered tool %s is missing from the MCP reference", tool.Name)
		}
	}
	for name := range documented {
		if !registered[name] {
			t.Errorf("MCP reference advertises unavailable tool %s", name)
		}
	}
}
