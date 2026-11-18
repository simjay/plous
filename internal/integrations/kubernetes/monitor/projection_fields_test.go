package monitor

import (
	"reflect"
	"testing"

	"github.com/simjay/plous/internal/integrations/observation"
)

func TestSelectedPathsPreserveValuesAndMaskAncestors(t *testing.T) {
	object := map[string]any{
		"metadata": map[string]any{"annotations": map[string]any{"other": "hidden"}},
		"spec": map[string]any{
			"a/b":      map[string]any{"~copy": []any{true, nil}},
			"settings": map[string]any{"httpHeaders": []any{"hidden"}},
			"optional": nil,
			"replicas": int64(0),
		},
	}
	projector := &Projector{}
	for _, test := range []struct {
		path, status string
		value        observation.Value
	}{
		{"/spec/a~1b/~0copy/0", "found", observation.Boolean(true)},
		{"/spec/a~1b/~0copy/00", "missing", observation.Unknown()},
		{"/spec/optional", "null", observation.Unknown()},
		{"/spec/replicas", "found", observation.Number(0)},
		{"/spec", "non_scalar", observation.Unknown()},
		{"/metadata/annotations/other", "redacted", observation.Unknown()},
		{"/spec/settings/httpHeaders/0", "redacted", observation.Unknown()},
	} {
		got, status := projector.pathFact(object, test.path)
		if status != test.status || !reflect.DeepEqual(got, test.value) {
			t.Errorf("%s: got %s %+v, want %s %+v", test.path, status, got, test.status, test.value)
		}
	}
}
