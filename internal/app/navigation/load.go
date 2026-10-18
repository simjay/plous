// Package navigation prepares agent-requested views without changing UI state.
package navigation

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/simjay/plous/internal/ai"
	"github.com/simjay/plous/internal/ai/agent"
	helm "github.com/simjay/plous/internal/integrations/helm/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	manifest "github.com/simjay/plous/internal/integrations/manifest"
	"github.com/simjay/plous/internal/source"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
)

type Prepared struct {
	Request             agent.ViewRequest
	Origin              string
	Client              *kubernetes.Client
	Catalog             []kubecommon.Resource
	Resource            kubecommon.Resource
	Object              *unstructured.Unstructured
	Collection          kubernetes.Collection
	Text, Before, After string
	DetailText          string
	References          []kubernetes.Reference
	Helm                *helm.Client
	Snapshot            *helm.Snapshot
}

// Load has no renderer, write, connection selection, or process-execution API.
func Load(ctx context.Context, q agent.ViewRequest, connections []ai.Connection, profile string) (*Prepared, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	p := &Prepared{Request: q, Origin: "live"}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if q.View == "evidence" {
		if len(q.Evidence) == 0 {
			return nil, errors.New("saved evidence was not resolved")
		}
		p.Text, p.Origin = string(q.Evidence), "saved"
		return p, nil
	}
	kind := "kubernetes"
	if q.View == "source" {
		kind = "source"
	}
	if q.View == "compare" {
		kind = "helm"
	}
	index := slices.IndexFunc(connections, func(c ai.Connection) bool { return c.ConnectionID == q.ConnectionID && c.Tool == kind })
	if index < 0 {
		return nil, errors.New("view is outside the captured connection")
	}
	c := connections[index]
	var err error
	switch q.View {
	case "source":
		p.Text, p.Request.Identity, err = source.DisplayFile(ctx, source.ReadScope{Root: c.Path, View: c.SourceView, ProfileRoot: profile, ConnectionID: c.ConnectionID}, q.Path, q.Identity)
		if err == nil && (q.Line < 0 || q.Line > strings.Count(p.Text, "\n")+1) {
			err = errors.New("requested line does not exist")
		}
		p.Request.Line = max(1, q.Line)
		p.Origin = "source"
	case "compare":
		p.Origin = "recorded"
		p.Helm, err = helm.New(helm.Config{Target: c.Target, TargetCaptured: c.TargetCaptured, Kubeconfig: c.Path, Context: c.Context, Namespace: q.Namespace, Driver: c.Driver})
		if err != nil {
			break
		}
		var before *helm.Snapshot
		before, err = p.Helm.RecordedSnapshot(ctx, q.Name, q.Namespace, q.CompareRevision)
		if err != nil {
			break
		}
		p.Snapshot, err = p.Helm.RecordedSnapshot(ctx, q.Name, q.Namespace, q.Revision)
		if err != nil {
			break
		}
		if q.Tab == "manifests" {
			left, e := manifest.Resources(before.Manifest)
			if e != nil {
				err = e
				break
			}
			right, e := manifest.Resources(p.Snapshot.Manifest)
			if e != nil {
				err = e
				break
			}
			a, b := map[string]any{}, map[string]any{}
			for _, r := range left {
				a[r.ScopedKey(q.Namespace)] = r.Object
			}
			for _, r := range right {
				b[r.ScopedKey(q.Namespace)] = r.Object
			}
			p.Before, p.After = manifest.MaskedComparison(a, b)
		} else {
			p.Request.Tab = "values"
			p.Before, p.After = manifest.MaskedComparison(before.Config, p.Snapshot.Config)
		}
	case "resource", "list", "logs":
		if _, err = labels.Parse(q.Labels); err != nil {
			break
		}
		if c.TargetCaptured {
			p.Client, err = kubernetes.NewTarget(c.Target, "")
		} else {
			p.Client, err = kubernetes.New(c.Path, c.Context, c.Namespace)
		}
		if err != nil {
			break
		}
		p.Catalog, _, err = p.Client.Discover(ctx)
		if err != nil {
			break
		}
		var ok bool
		p.Resource, ok = kubernetes.Find(p.Catalog, q.Resource)
		if !ok {
			err = errors.New("resource type is unavailable or ambiguous")
			break
		}
		p.Request.Resource = p.Resource.Key()
		if p.Resource.Namespaced && q.Namespace == "" {
			err = errors.New("namespace is required; use * only for lists")
			break
		}
		if !p.Resource.Namespaced && q.Namespace != "" && q.Namespace != "*" {
			err = errors.New("cluster-scoped resources have no namespace")
			break
		}
		if q.View == "list" {
			ns := q.Namespace
			if ns == "*" {
				ns = ""
			}
			p.Collection, err = p.Client.List(ctx, p.Resource, ns, q.Labels)
		} else {
			if q.Namespace == "*" {
				err = errors.New("an object requires a concrete namespace")
				break
			}
			p.Object, err = p.Client.Get(ctx, p.Resource, q.Namespace, q.Name)
			if err != nil {
				break
			}
			if q.UID != "" && string(p.Object.GetUID()) != q.UID {
				err = errors.New("resource was replaced; read the replacement before opening it")
				break
			}
			if p.Object.GetName() != q.Name || p.Object.GetNamespace() != q.Namespace {
				err = errors.New("server returned a different resource identity")
				break
			}
			p.Request.UID = string(p.Object.GetUID())
			if q.Tab == "events" {
				p.DetailText, err = p.Client.Events(ctx, *p.Object)
				if err != nil {
					break
				}
			}
			if q.Tab == "related" {
				p.References, p.DetailText = p.Client.Related(ctx, p.Resource, *p.Object, p.Catalog)
			}
			if q.View == "logs" {
				if !slices.Contains(kubernetes.Containers(*p.Object), q.Container) {
					err = errors.New("container does not exist in the requested Pod")
					break
				}
				// Opening shows a bounded snapshot. Follow remains a user-owned control.
				tooLarge := false
				err = p.Client.Logs(ctx, kubernetes.LogOptions{Namespace: q.Namespace, Pod: q.Name, UID: p.Object.GetUID(), Container: q.Container, Previous: q.Previous}, func(text string) {
					if len(p.Text)+len(text) > 2<<20 {
						tooLarge = true
						cancel()
						return
					}
					p.Text += text
				})
				if tooLarge {
					err = errors.New("log snapshot exceeds the 2 MiB display limit")
				}
			}
		}
	default:
		err = fmt.Errorf("unsupported view %q", q.View)
	}
	if err != nil {
		return nil, err
	}
	return p, ctx.Err()
}
