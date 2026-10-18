package monitor

import (
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"github.com/simjay/plous/internal/integrations/observation"
)

func projectEvents(r kubecommon.Resource, o map[string]any, f map[string]observation.Value) error {
	for _, field := range []string{"reason", "message", "note", "type", "count", "firstTimestamp", "lastTimestamp", "eventTime", "action"} {
		if v := projectedValue(o, field); !v.Unknown {
			f[field] = v
		}
	}
	for _, object := range []string{"involvedObject", "regarding"} {
		for _, field := range []string{"kind", "namespace", "name", "uid"} {
			if v := projectedValue(o, object, field); !v.Unknown {
				f[object+"."+field] = v
			}
		}
	}
	return nil
}
