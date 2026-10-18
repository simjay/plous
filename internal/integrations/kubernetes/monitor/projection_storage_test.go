package monitor

import (
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"github.com/simjay/plous/internal/integrations/observation"
	"testing"
)

func TestVolumeCapacityUsesOwningResourceField(t *testing.T) {
	for _, name := range []string{"persistentvolumes", "persistentvolumeclaims"} {
		object := map[string]any{"spec": map[string]any{"capacity": map[string]any{"storage": "20Gi"}}, "status": map[string]any{"capacity": map[string]any{"storage": "10Gi"}}}
		facts := map[string]observation.Value{}
		if err := projectVolumes(kubecommon.Resource{Name: name, Version: "v1"}, object, facts); err != nil {
			t.Fatal(err)
		}
		want := "10Gi"
		if name == "persistentvolumes" {
			want = "20Gi"
		}
		if facts["capacity"].String() != want {
			t.Fatal(name, facts)
		}
	}
}
