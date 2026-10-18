package code

import (
	"path/filepath"
	"strings"

	kustomizecommon "github.com/simjay/plous/internal/integrations/kustomize/common"
	manifest "github.com/simjay/plous/internal/integrations/manifest"
	"sigs.k8s.io/kustomize/api/resmap"
	"sigs.k8s.io/kustomize/api/resource"
	"sigs.k8s.io/yaml"
)

type observer struct {
	root        string
	observation *kustomizecommon.Observation
	ids         map[*resource.Resource]int
	before      map[int]manifest.Resource
	bytes       int
}

func newObserver(root string) *observer {
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	return &observer{root: root, observation: &kustomizecommon.Observation{FinalIDs: map[string]int{}}, ids: map[*resource.Resource]int{}}
}
func (o *observer) snapshot(m resmap.ResMap) map[int]manifest.Resource {
	out := map[int]manifest.Resource{}
	for _, r := range m.Resources() {
		id := o.ids[r]
		if id == 0 {
			id = len(o.ids) + 1
			o.ids[r] = id
		}
		copy := r.DeepCopy()
		copy.RemoveBuildAnnotations()
		object, err := copy.Map()
		if err != nil {
			o.observation.Truncated = true
			continue
		}
		object, _ = manifest.Redact(object).(map[string]any)
		out[id] = manifest.Resource{APIVersion: r.GetApiVersion(), Kind: r.GetKind(), Namespace: r.GetNamespace(), Name: r.GetName(), Object: object}
	}
	return out
}
func (o *observer) capture(scope, kind string, configuration []byte, m resmap.ResMap, before bool) {
	if o.observation.Truncated {
		return
	}
	snapshot := o.snapshot(m)
	if before {
		o.before = snapshot
		return
	}
	changed := len(snapshot) != len(o.before)
	for id, r := range snapshot {
		if manifest.JSON(r.Object) != manifest.JSON(o.before[id].Object) {
			changed = true
			break
		}
	}
	if !changed {
		return
	}
	size := 0
	for _, set := range []map[int]manifest.Resource{o.before, snapshot} {
		for _, r := range set {
			size += len(manifest.JSON(r.Object))
		}
	}
	if len(o.observation.Stages) >= 256 || o.bytes+size > 32<<20 {
		o.observation.Truncated = true
		return
	}
	o.bytes += size
	relative, err := filepath.Rel(o.root, scope)
	if err != nil || strings.HasPrefix(relative, "..") {
		o.observation.Truncated = true
		return
	}
	var config map[string]any
	_ = yaml.Unmarshal(configuration, &config)
	file, _ := config["path"].(string)
	configuredKind, _ := config["kind"].(string)
	metadata, _ := config["metadata"].(map[string]any)
	name, _ := metadata["name"].(string)
	o.observation.Stages = append(o.observation.Stages, kustomizecommon.Stage{Scope: filepath.ToSlash(relative), Kind: kind, Path: file, ConfigurationName: name, External: configuredKind != "", Before: o.before, After: snapshot})
}
func (o *observer) finish(m resmap.ResMap) {
	for _, r := range m.Resources() {
		if id := o.ids[r]; id > 0 {
			key := manifest.Resource{APIVersion: r.GetApiVersion(), Kind: r.GetKind(), Namespace: r.GetNamespace(), Name: r.GetName()}.Key()
			o.observation.FinalIDs[key] = id
		}
	}
}
