package chat

import (
	"github.com/simjay/plous/internal/ai/history"
	"github.com/simjay/plous/internal/integrations/observation"
	"strings"
	"testing"
	"time"
)

func TestSourcesIdentifyProjectedScope(t *testing.T) {
	for _, tool := range []string{"kubernetes_resources_read", "helm_releases_read"} {
		r := observation.New("fixture", "get", time.Now().UTC())
		r.Connection, r.Namespace = "captured", "*"
		r.Partial("only a selected page")
		title, scope, observed := SourceSummary(history.ReasoningEntry{Tool: tool, Projection: &r})
		if !strings.Contains(title, "captured") || !strings.Contains(scope, "all namespaces") || !strings.Contains(scope, "partial coverage") || !observed.Equal(r.ObservedAt) {
			t.Fatal(title, scope, observed)
		}
	}
}
