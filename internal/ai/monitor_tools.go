package ai

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/simjay/plous/internal/integrations/observation"
)

// MonitorReader projects saved monitor records for a captured Kubernetes target.
type MonitorReader interface {
	Description() string
	Schema() map[string]any
	Read(fingerprint, connectionID string, args map[string]json.RawMessage) (observation.Result, error)
}

func (op *operation) readMonitor(ctx context.Context, connection Connection, args map[string]json.RawMessage) (observation.Result, error) {
	if err := ctx.Err(); err != nil {
		return observation.Result{}, err
	}
	if connection.Target == nil || connection.Target.CheckIdentity() != nil {
		return observation.Result{}, denyRead(errors.New("the captured Kubernetes target is unavailable; reselect it before retrying"))
	}
	result, err := op.o.Monitor.Read(connection.Target.Fingerprint, connection.ConnectionID, args)
	if err != nil {
		return observation.Result{}, denyRead(err)
	}
	return result, nil
}
