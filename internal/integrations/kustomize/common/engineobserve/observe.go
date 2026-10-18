// Package plousobserve is a pinned adapter to Krusty's builtin factory registry.
// Its module path permits access to the engine's internal registry without
// copying or modifying upstream code. Callers must serialize all Krusty builds.
package plousobserve

import (
	"sigs.k8s.io/kustomize/api/internal/plugins/builtinhelpers"
	"sigs.k8s.io/kustomize/api/resmap"
)

// Capture runs around each builtin invocation in the engine's actual order.
// Configuration and scope belong to this invocation, including nested bases.
type Capture func(scope, kind string, configuration []byte, resources resmap.ResMap, before bool)

func Install(capture Capture) func() {
	old := make(map[builtinhelpers.BuiltinPluginType]func() resmap.TransformerPlugin)
	for kind, factory := range builtinhelpers.TransformerFactories {
		old[kind] = factory
		builtinhelpers.TransformerFactories[kind] = func() resmap.TransformerPlugin {
			return &transformer{TransformerPlugin: factory(), kind: kind.String(), capture: capture}
		}
	}
	return func() {
		for kind, factory := range old {
			builtinhelpers.TransformerFactories[kind] = factory
		}
	}
}

type transformer struct {
	resmap.TransformerPlugin
	scope, kind   string
	configuration []byte
	capture       Capture
}

func (t *transformer) Config(h *resmap.PluginHelpers, data []byte) error {
	t.scope, t.configuration = h.Loader().Root(), append([]byte(nil), data...)
	return t.TransformerPlugin.Config(h, data)
}
func (t *transformer) Transform(m resmap.ResMap) error {
	t.capture(t.scope, t.kind, t.configuration, m, true)
	if err := t.TransformerPlugin.Transform(m); err != nil {
		return err
	}
	t.capture(t.scope, t.kind, t.configuration, m, false)
	return nil
}
