package monitor

import (
	"context"
	"strings"
	"testing"
	"time"

	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestNamespaceLoadLifetimeRejectsCancelledAndRepeatedResults(t *testing.T) {
	for _, timeout := range []time.Duration{0, time.Minute} {
		t.Run(timeout.String(), func(t *testing.T) {
			p := &NamespacePicker{}
			var oldContext, currentContext context.Context
			old := p.Load(timeout, func(ctx context.Context) (kubernetes.Collection, error) {
				oldContext = ctx
				return kubernetes.Collection{}, ctx.Err()
			})
			if duplicate := p.Load(timeout, nil); duplicate != nil {
				t.Fatal("a second load replaced an active request")
			}
			p.Cancel()
			current := p.Load(timeout, func(ctx context.Context) (kubernetes.Collection, error) {
				currentContext = ctx
				if _, hasDeadline := ctx.Deadline(); hasDeadline != (timeout > 0) {
					t.Fatal("namespace load changed the caller's timeout policy")
				}
				return kubernetes.Collection{Objects: []unstructured.Unstructured{
					{Object: map[string]any{"metadata": map[string]any{"name": "zeta"}}},
					{Object: map[string]any{"metadata": map[string]any{"name": "alpha"}}},
				}}, nil
			})
			if p.Apply(old()) || oldContext.Err() != context.Canceled || p.Loaded {
				t.Fatal("a cancelled request changed the replacement picker")
			}
			result := current()
			if !p.Apply(result) || p.Run != nil || !p.Loaded || p.Err != "" || strings.Join(p.Names, ",") != "alpha,zeta" || currentContext.Err() != context.Canceled {
				t.Fatalf("current namespace result was not committed and released: %#v", p)
			}
			result.Names = []string{"duplicate"}
			if p.Apply(result) || strings.Join(p.Names, ",") != "alpha,zeta" {
				t.Fatal("a repeated completion changed the loaded namespaces")
			}
		})
	}
}
