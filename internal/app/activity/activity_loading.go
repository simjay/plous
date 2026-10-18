package activity

import (
	"context"
	"fmt"
	"github.com/simjay/plous/internal/workspace"
	"slices"
)

type ActivityLoad struct {
	Workspace string
	Ctx       context.Context
	Cancel    context.CancelFunc
}

type ActivityResult struct {
	Load       *ActivityLoad
	Changes    []workspace.Change
	Operations []workspace.Operation
	Err        error
}

type ActivityInventoryKey struct {
	Workspace, Tool, Connection, Query string
	Filter                             Filter
	Change                             *workspace.Change
	Operation                          *workspace.Operation
	ChangeCount, OperationCount        int
}

type ActivityInventory struct {
	Key     ActivityInventoryKey
	Entries []ActivityEntry
}

func ReadActivity(store *workspace.Store, run *ActivityLoad) ActivityResult {
	defer run.Cancel()
	result := ActivityResult{Load: run}
	result.Changes, result.Err = store.HistoryContext(run.Ctx, run.Workspace)
	if result.Err == nil {
		result.Operations, result.Err = store.OperationsContext(run.Ctx, run.Workspace)
	}
	slices.Reverse(result.Changes)
	return result
}

func (e ActivityEntry) Identity() string {
	if e.Change != nil {
		return "change/" + e.Change.ID
	}
	if e.Operation.Kind == "revision" && e.Operation.ConnectionFingerprint != "" && e.Operation.Revision > 0 {
		op := e.Operation
		return fmt.Sprintf("revision/%q/%q/%q/%q/%q/%q/%d", op.WorkspaceID, op.ConnectionID, op.ConnectionFingerprint, op.Context, op.Namespace, op.Release, op.Revision)
	}
	return "operation/" + e.Operation.ID
}
