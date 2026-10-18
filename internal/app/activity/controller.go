package activity

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"github.com/simjay/plous/internal/ui"
	"github.com/simjay/plous/internal/workspace"
	"sort"
)

type Controller struct {
	Workspace  string
	Changes    []workspace.Change
	Operations []workspace.Operation
	Pending    bool
	Load       *ActivityLoad
	Error      string
	Inventory  *ActivityInventory
	Filter     Filter
	Positions  map[string]int
}

func (a *Controller) Stop() {
	if a.Load != nil {
		a.Load.Cancel()
		a.Load = nil
	}
}

func (a *Controller) Reload(workspaceID string, available bool) {
	a.Stop()
	if a.Workspace != workspaceID {
		a.Changes, a.Operations = nil, nil
		a.Workspace = workspaceID
		a.Filter = All
		a.Positions = nil
	}
	a.Inventory, a.Error = nil, ""
	a.Pending = available
}

func (a *Controller) Start(store *workspace.Store) tea.Cmd {
	if !a.Pending || a.Load != nil || store == nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := &ActivityLoad{Workspace: a.Workspace, Ctx: ctx, Cancel: cancel}
	a.Pending, a.Load = false, run
	return func() tea.Msg { return ReadActivity(store, run) }
}

func (a *Controller) Matches(result ActivityResult, workspaceID string) bool {
	return result.Load != nil && result.Load == a.Load && result.Load.Workspace == workspaceID
}

func (a *Controller) Apply(result ActivityResult) bool {
	if result.Load == nil || result.Load != a.Load {
		return false
	}
	a.Load = nil
	if result.Err != nil {
		a.Error = "Activity unavailable: " + result.Err.Error()
		return false
	}
	a.Changes, a.Operations, a.Inventory = result.Changes, result.Operations, nil
	a.Error = ""
	return true
}

func (a *Controller) Entries(workspaceID, tool, connection, query string) []ActivityEntry {
	key := ActivityInventoryKey{Workspace: workspaceID, Tool: tool, Connection: connection, Query: query, Filter: a.Filter, ChangeCount: len(a.Changes), OperationCount: len(a.Operations)}
	if len(a.Changes) > 0 {
		key.Change = &a.Changes[0]
	}
	if len(a.Operations) > 0 {
		key.Operation = &a.Operations[0]
	}
	if a.Inventory != nil && a.Inventory.Key == key {
		return a.Inventory.Entries
	}
	var entries []ActivityEntry
	for i := range a.Changes {
		change := &a.Changes[i]
		if a.Filter == All || a.Filter == WorkspaceChanges {
			entries = append(entries, ActivityEntry{Change: change, At: change.Time})
		}
	}
	groups := map[string]int{}
	for i := range a.Operations {
		operation := &a.Operations[i]
		if (tool == "" || operation.Tool == tool || tool == "kubernetes" && operation.Tool == "helm") && (connection == "" || operation.ConnectionID == connection) {
			entry := operationEntry(operation)
			if a.Filter != All && a.Filter != entry.Category() {
				continue
			}
			if operation.Kind == "revision" {
				if index, ok := groups[entry.Identity()]; ok {
					entries[index].Observations = append(entries[index].Observations, operation)
					continue
				}
				groups[entry.Identity()] = len(entries)
			}
			entries = append(entries, entry)
		}
	}
	for i := range entries {
		entries[i].prepareObservations()
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].At.Equal(entries[j].At) && entries[i].Operation != nil && entries[j].Operation != nil {
			return entries[i].Identity() < entries[j].Identity()
		}
		return entries[i].At.After(entries[j].At)
	})
	if query != "" {
		filtered := []ActivityEntry{}
		for _, entry := range entries {
			if ui.SearchContains(entry.SearchText(), query) {
				filtered = append(filtered, entry)
			}
		}
		entries = filtered
	}
	a.Inventory = &ActivityInventory{Key: key, Entries: entries}
	return entries
}
