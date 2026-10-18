package monitor

import (
	"context"
	"crypto/sha256"
	"fmt"

	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
	"github.com/simjay/plous/internal/workspace"
)

func RecordHelmHistory(ctx context.Context, store *workspace.Store, workspaceID, connectionID string, config helmtool.Config, s *helmtool.Snapshot) error {
	ops, err := store.Operations(workspaceID)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, op := range ops {
		seen[op.ID] = true
	}
	for _, rev := range s.History {
		if err := ctx.Err(); err != nil {
			return err
		}
		id := fmt.Sprintf("helm-%x", sha256.Sum256([]byte(fmt.Sprintf("%s/%s/%s/%d/%s", config.Fingerprint(), s.Namespace, s.Name, rev.Revision, rev.Status))))
		if seen[id] {
			continue
		}
		_, err := store.RecordOperation(workspace.Operation{ID: id, WorkspaceID: workspaceID, ConnectionID: connectionID, ConnectionFingerprint: config.Fingerprint(), Tool: "helm", Context: config.Context, Namespace: s.Namespace, Release: s.Name, Kind: "revision", Status: rev.Status, Revision: rev.Revision, Chart: rev.Chart, SourceTime: rev.Updated})
		if err != nil {
			return err
		}
	}
	return nil
}
