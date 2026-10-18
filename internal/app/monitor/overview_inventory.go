package monitor

import (
	"fmt"
	"sort"
	"strings"

	gitops "github.com/simjay/plous/internal/integrations/gitops/common"
	helm "github.com/simjay/plous/internal/integrations/helm/monitor"
)

// Counter drilldowns use the same observed objects as the displayed counts.
func (s *OverviewState) OpenInventory() bool {
	if s.Pane != 3 || s.InventoryTool != "" || s.Counter == 0 {
		return false
	}
	rows := s.Filtered(3)
	if len(rows) == 0 {
		return false
	}
	row := rows[min(s.Cursor[3], len(rows)-1)]
	if row.Target.Tool == "kubernetes" || s.Counter > len(row.Counters) {
		return false
	}
	s.InventoryTool, s.InventoryCounter = row.Target.Tool, row.Counters[s.Counter-1].Label
	s.InventoryCursor, s.InventoryScroll, s.InventoryQuery = s.Cursor[3], s.Scroll[3], s.Query
	s.Cursor[3], s.Scroll[3], s.Query = 0, 0, ""
	return true
}

func (s *OverviewState) CloseInventory() bool {
	if s.InventoryTool == "" {
		return false
	}
	s.InventoryTool, s.InventoryCounter = "", ""
	s.Cursor[3], s.Scroll[3], s.Query = s.InventoryCursor, s.InventoryScroll, s.InventoryQuery
	return true
}

func (s *OverviewState) inventoryRows() []OverviewRow {
	rows := []OverviewRow{}
	label := s.InventoryCounter
	if s.InventoryTool == "helm" && s.Data.Helm != nil {
		for _, release := range s.Data.Helm.Releases {
			match := overviewHelmCounter(label, release)
			if !match {
				continue
			}
			rows = append(rows, OverviewRow{ID: release.Namespace + "/" + release.Name, Title: release.Name, Status: release.Status, Source: "Helm", At: release.Updated, Detail: fmt.Sprintf("Storage revision %d", release.Revision), Target: OverviewTarget{Tool: "helm", Name: release.Name, Namespace: release.Namespace, Revision: release.Revision}})
		}
	}
	snapshots := s.Data.Flux
	if s.InventoryTool == "argocd" {
		snapshots = nil
		if s.Data.Argo != nil {
			snapshots = []gitops.Snapshot{*s.Data.Argo}
		}
	}
	if s.InventoryTool != "helm" {
		for _, snapshot := range snapshots {
			for _, item := range snapshot.Items {
				match := overviewGitOpsCounter(label, item)
				if !match {
					continue
				}
				source, status := "Flux", item.Status
				if s.InventoryTool == "argocd" {
					source, status = "Argo CD", item.Sync+" / "+item.Health
				}
				rows = append(rows, OverviewRow{ID: item.Ref.Key(), Title: item.Ref.Name, Status: status, Source: source, At: snapshot.ReadAt, Detail: item.Reason, Target: OverviewTarget{Tool: s.InventoryTool, Name: item.Ref.Name, Namespace: item.Ref.Namespace, UID: item.UID, Ref: item.Ref}})
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows
}

func overviewHelmCounter(label string, release helm.OverviewRelease) bool {
	switch label {
	case "Releases":
		return true
	case "Deployed":
		return release.Status == "deployed"
	case "Pending":
		return strings.HasPrefix(release.Status, "pending-")
	case "Failed":
		return release.Status == "failed"
	}
	return false
}

func overviewGitOpsCounter(label string, item gitops.Item) bool {
	switch label {
	case "Applications":
		return true
	case "Synced":
		return item.Sync == "Synced"
	case "OutOfSync":
		return item.Sync == "OutOfSync"
	case "Degraded":
		return item.Health == "Degraded"
	case "Kustomizations":
		return item.Ref.Kind == "Kustomization"
	case "HelmReleases":
		return item.Ref.Kind == "HelmRelease"
	case "Sources":
		return item.Ref.Kind != "Kustomization" && item.Ref.Kind != "HelmRelease"
	case "NotReady":
		return item.Status == "Not ready" || item.Status == "Stalled"
	}
	return false
}

func (s *OverviewState) inventoryCoverage() string {
	for _, row := range s.Rows[3] {
		if row.Target.Tool != s.InventoryTool {
			continue
		}
		for _, counter := range row.Counters {
			if counter.Label != s.InventoryCounter {
				continue
			}
			if counter.Value == "?" || counter.Value == "Unavailable" {
				return "Unavailable"
			}
			if strings.Contains(counter.Value, "+") {
				return "Partial"
			}
			return "Observed"
		}
	}
	return "Unavailable"
}
