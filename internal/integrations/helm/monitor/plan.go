package monitor

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	helmcommon "github.com/simjay/plous/internal/integrations/helm/common"
	manifestutil "github.com/simjay/plous/internal/integrations/manifest"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/yaml"
)

func validRelease(name string) error {
	if len(name) > 53 || len(validation.IsDNS1123Subdomain(name)) != 0 {
		return errors.New("release name must be a DNS name of at most 53 characters")
	}
	return nil
}
func validNamespace(ns string) error {
	if len(validation.IsDNS1123Label(ns)) != 0 {
		return errors.New("select a concrete namespace")
	}
	return nil
}
func ValidateValues(text string) error {
	if len(text) > 2<<20 {
		return errors.New("values exceed 2 MiB")
	}
	var value map[string]any
	if err := yaml.UnmarshalStrict([]byte(text), &value); err != nil {
		return fmt.Errorf("invalid values: %w", err)
	}
	return nil
}

type Plan struct {
	ID, Operation, Name, Namespace, Context string
	Manifest, Before, Values, Lint          string
	CurrentRevision, TargetRevision         int
	Artifact                                *helmcommon.Artifact
	config                                  Config
	seal                                    string
	target                                  *Snapshot
	currentIdentity                         string
	targetConfigIdentity                    string
}

func (p *Plan) identity() string {
	copy := *p
	copy.ID = ""
	copy.seal = ""
	data, _ := json.Marshal(copy)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}
func (p *Plan) sealPlan() { p.ID = p.identity(); p.seal = p.ID }
func (c *Client) PrepareChart(ctx context.Context, a *helmcommon.Artifact, name, ns, values string, baseline *Snapshot) (*Plan, error) {
	if err := validRelease(name); err != nil {
		return nil, err
	}
	if err := validNamespace(ns); err != nil {
		return nil, err
	}
	if err := ValidateValues(values); err != nil {
		return nil, err
	}
	targetIdentity, err := c.captureTarget()
	if err != nil {
		return nil, err
	}
	file, cleanup, err := a.Materialize()
	if err != nil {
		return nil, err
	}
	defer cleanup()
	lint, err := c.Runner.Run(ctx, c.Config, []string{"lint", file, "--values", "-"}, values)
	if err != nil {
		return nil, err
	}
	args := []string{"template", name, file, "--namespace", ns, "--kube-context", c.Config.Context, "--values", "-", "--include-crds"}
	p := &Plan{Operation: "preview", Name: name, Namespace: ns, Context: c.Config.Context, Values: values, Lint: SafeText(string(lint)), Artifact: a, config: c.Config, targetConfigIdentity: targetIdentity}
	if baseline != nil {
		if baseline.Name != name || baseline.Namespace != ns {
			return nil, errors.New("baseline identity differs from preview target")
		}
		p.Before = baseline.Manifest
		p.CurrentRevision = baseline.Version
		args = append(args, "--is-upgrade")
	}
	rendered, err := c.Runner.Run(ctx, c.Config, args, values)
	if err != nil {
		return nil, err
	}
	p.Manifest = string(rendered)
	if _, err = manifestutil.Resources(p.Manifest); err != nil {
		return nil, err
	}
	p.sealPlan()
	return p, nil
}
func (c *Client) PrepareRollback(ctx context.Context, name, ns string, revision int) (*Plan, error) {
	targetIdentity, err := c.captureTarget()
	if err != nil {
		return nil, err
	}
	current, err := c.Metadata(ctx, name, ns, 0)
	if err != nil {
		return nil, err
	}
	if current.Revision == revision {
		return nil, errors.New("choose a different retained revision")
	}
	before, err := c.Snapshot(ctx, name, ns, current.Revision)
	if err != nil {
		return nil, err
	}
	target, err := c.Snapshot(ctx, name, ns, revision)
	if err != nil {
		return nil, err
	}
	p := &Plan{Operation: "rollback", Name: name, Namespace: ns, Context: c.Config.Context, Before: before.Manifest, Manifest: target.Manifest, CurrentRevision: current.Revision, TargetRevision: revision, Values: manifestutil.YAML(target.Config, false), config: c.Config, target: target, currentIdentity: snapshotIdentity(before), targetConfigIdentity: targetIdentity, Lint: "Uses the retained revision's manifests, values, and hooks. No source repository is required."}
	p.sealPlan()
	return p, nil
}
func (c *Client) Execute(ctx context.Context, p *Plan) (string, error) {
	if p == nil || p.ID == "" || p.ID != p.seal || p.identity() != p.seal || p.config != c.Config {
		return "not submitted", errors.New("preview or target changed; prepare a new preview")
	}
	if p.Operation != "rollback" {
		return "not submitted", errors.New("this plan is a preview only")
	}
	if err := c.recheckTarget(p); err != nil {
		return "not submitted", err
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Minute)
	defer cancel()
	latest, err := c.Metadata(ctx, p.Name, p.Namespace, 0)
	if err != nil {
		return "not submitted", err
	}
	if latest.Revision != p.CurrentRevision {
		return "not submitted", errors.New("release changed after preview; prepare a new rollback")
	}
	current, err := c.Snapshot(ctx, p.Name, p.Namespace, p.CurrentRevision)
	if err != nil {
		return "not submitted", err
	}
	if snapshotIdentity(current) != p.currentIdentity {
		return "not submitted", errors.New("current release changed after preview")
	}
	retained, err := c.Snapshot(ctx, p.Name, p.Namespace, p.TargetRevision)
	if err != nil {
		return "not submitted", err
	}
	if snapshotIdentity(retained) != snapshotIdentity(p.target) {
		return "not submitted", errors.New("retained revision changed after preview")
	}
	if err := c.recheckTarget(p); err != nil {
		return "not submitted", err
	}
	_, err = c.run(ctx, p.Namespace, "rollback", p.Name, strconv.Itoa(p.TargetRevision), "--timeout", "5m")
	return outcome(ctx, err)
}

func (c *Client) recheckTarget(p *Plan) error {
	identity, err := c.captureTarget()
	if err != nil {
		return err
	}
	if identity != p.targetConfigIdentity {
		return errors.New("kubeconfig changed after preview; prepare a new preview")
	}
	return nil
}
func outcome(ctx context.Context, err error) (string, error) {
	if ctx.Err() != nil {
		return "unknown", ctx.Err()
	}
	if errors.Is(err, helmcommon.ErrOutputLimit) {
		return "unknown", err
	}
	if err != nil {
		return "failed", err
	}
	return "completed", nil
}
func snapshotIdentity(s *Snapshot) string {
	if s == nil {
		return ""
	}
	copy := *s
	copy.History = nil
	data, _ := json.Marshal(copy)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}
func (p *Plan) Summary() string {
	resources, err := manifestutil.Resources(p.Manifest)
	if err != nil {
		return err.Error()
	}
	before, _ := manifestutil.Resources(p.Before)
	old := map[string]manifestutil.Resource{}
	for _, r := range before {
		old[r.ScopedKey(p.Namespace)] = r
	}
	lines := []string{fmt.Sprintf("%s · %s / %s", strings.ToUpper(p.Operation), p.Namespace, p.Name), "Context: " + p.Context, p.Lint, "", "Resource changes:"}
	for _, r := range resources {
		key := r.ScopedKey(p.Namespace)
		previous, exists := old[key]
		label := "+ Add "
		if exists {
			label = "= Keep "
			if manifestutil.YAML(previous.Object, false) != manifestutil.YAML(r.Object, false) {
				label = "~ Change "
			}
		}
		lines = append(lines, label+key)
		if r.ClusterScoped() {
			lines = append(lines, "  Cluster-scoped resource: affects this cluster beyond the release namespace.")
		}
		metadata, _ := r.Object["metadata"].(map[string]any)
		annotations, _ := metadata["annotations"].(map[string]any)
		if hook, _ := annotations["helm.sh/hook"].(string); hook != "" {
			lines = append(lines, "  Helm hook: "+hook)
		}
		if exists && manifestutil.YAML(previous.Object, false) != manifestutil.YAML(r.Object, false) {
			for _, field := range []string{"replicas", "selector", "template", "volumeClaimTemplates"} {
				beforeSpec, _ := previous.Object["spec"].(map[string]any)
				afterSpec, _ := r.Object["spec"].(map[string]any)
				if manifestutil.JSON(beforeSpec[field]) != manifestutil.JSON(afterSpec[field]) {
					lines = append(lines, "  Changed spec."+field)
				}
			}
		}
		delete(old, key)
	}
	for _, r := range before {
		if _, exists := old[r.ScopedKey(p.Namespace)]; exists {
			lines = append(lines, "- Remove "+r.ScopedKey(p.Namespace))
		}
	}
	if p.target != nil {
		for _, h := range p.target.Hooks {
			lines = append(lines, "Hook: "+h.Name+" ("+strings.Join(h.Events, ", ")+")")
		}
	}
	lines = append(lines, "", "Preview estimates rendered changes. Admission, hooks, lookups, controllers, and readiness can differ at execution.")
	return strings.Join(lines, "\n")
}
