package monitor

import (
	"context"
	"fmt"
	"sort"
	"time"

	tea "charm.land/bubbletea/v2"
	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
)

type HelmTimeline struct {
	Release        helmtool.Release
	Revisions      []helmtool.Revision
	Cursor, Scroll int
	Entries        map[int]HelmTimelineEntry
	Load           *HelmTimelineLoad
	Snapshots      map[int]*helmtool.Snapshot
}

type HelmTimelineEntry struct {
	Changes *helmtool.RevisionChanges
	Err     string
}

type HelmTimelineLoad struct {
	Revision int
	Cancel   context.CancelFunc
}

func (h *HelmState) OpenTimeline(s *helmtool.Snapshot) {
	selected, scroll := 0, 0
	if h.Timeline != nil {
		selected, scroll = h.Timeline.Selected(), h.Timeline.Scroll
	}
	h.StopTimelineLoad()
	t := &HelmTimeline{Release: helmtool.Release{Name: s.Name, Namespace: s.Namespace}, Revisions: append([]helmtool.Revision(nil), s.History...), Entries: map[int]HelmTimelineEntry{}, Snapshots: map[int]*helmtool.Snapshot{s.Version: s}}
	sort.Slice(t.Revisions, func(i, j int) bool { return t.Revisions[i].Revision > t.Revisions[j].Revision })
	if len(t.Revisions) > 0 {
		t.Release.Revision = t.Revisions[0].Revision
	}
	for i, r := range t.Revisions {
		if r.Revision == selected {
			t.Cursor, t.Scroll = i, scroll
		}
	}
	h.Timeline, h.Detail = t, nil
	h.Notice, h.Err = "", ""
	if h.Find != nil {
		h.RefreshFind()
	}
}

func (t *HelmTimeline) Selected() int {
	if t == nil || len(t.Revisions) == 0 {
		return 0
	}
	return t.Revisions[min(t.Cursor, len(t.Revisions)-1)].Revision
}

func (h *HelmState) StopTimelineLoad() {
	if h.Timeline != nil && h.Timeline.Load != nil {
		h.Timeline.Load.Cancel()
		h.Timeline.Load = nil
	}
}

func (h *HelmState) SyncTimeline() tea.Cmd {
	t := h.Timeline
	if t == nil || h.Detail != nil || h.WorkflowVisible() || h.Busy || h.Pending != "" || h.Namespaces != nil || t.Load != nil || h.Client == nil {
		return nil
	}
	// Load the selected block and two following blocks. Older history stays lazy.
	for i := t.Cursor; i < min(len(t.Revisions), t.Cursor+3); i++ {
		revision := t.Revisions[i].Revision
		if _, ok := t.Entries[revision]; ok {
			continue
		}
		baseline := revision == 1
		for _, r := range t.Revisions {
			if r.Revision == revision-1 {
				baseline = true
				break
			}
		}
		if !baseline {
			t.Entries[revision] = HelmTimelineEntry{Err: fmt.Sprintf("Baseline revision %d unavailable. Earlier history may have been pruned.", revision-1)}
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		job := &HelmTimelineLoad{Revision: revision, Cancel: cancel}
		t.Load = job
		client, release := h.Client, t.Release
		current, before := t.Snapshots[revision], t.Snapshots[revision-1]
		return func() tea.Msg {
			defer cancel()
			result := HelmResult{State: h, Kind: "revision-diff", TimelineLoad: job}
			var err error
			if current == nil {
				current, err = client.RecordedSnapshot(ctx, release.Name, release.Namespace, revision)
			}
			if err != nil {
				result.Err = fmt.Errorf("revision %d unavailable: %w", revision, err)
				return result
			}
			result.Snapshots = append(result.Snapshots, current)
			if revision > 1 && before == nil {
				before, err = client.RecordedSnapshot(ctx, release.Name, release.Namespace, revision-1)
				if err != nil {
					result.Err = fmt.Errorf("baseline revision %d unavailable: %w", revision-1, err)
					return result
				}
			}
			if before != nil {
				result.Snapshots = append(result.Snapshots, before)
			}
			result.Changes, result.Err = helmtool.CompareRevisions(before, current)
			return result
		}
	}
	return nil
}

func (h *HelmState) ApplyTimelineChanges(r HelmResult) {
	if h == nil || h.Timeline == nil || r.TimelineLoad == nil || h.Timeline.Load != r.TimelineLoad {
		return
	}
	t := h.Timeline
	t.Load = nil
	if t.Snapshots == nil {
		t.Snapshots = map[int]*helmtool.Snapshot{}
	}
	for _, snapshot := range r.Snapshots {
		t.Snapshots[snapshot.Version] = snapshot
	}
	// Comparisons share adjacent records only within this timeline. Refresh creates
	// a new cache; writes and rollback validation always read independently.
	for len(t.Snapshots) > 7 {
		furthest, distance := 0, -1
		for revision := range t.Snapshots {
			d := revision - t.Selected()
			if d < 0 {
				d = -d
			}
			if d > distance {
				furthest, distance = revision, d
			}
		}
		delete(t.Snapshots, furthest)
	}
	entry := HelmTimelineEntry{Changes: r.Changes}
	if r.Err != nil {
		entry.Err = helmtool.SafeText(r.Err.Error())
	}
	t.Entries[r.TimelineLoad.Revision] = entry
	// Bound retained comparison bodies; returning to an evicted block reloads it.
	for len(t.Entries) > 6 {
		furthest, distance := 0, -1
		for revision := range t.Entries {
			d := revision - t.Selected()
			if d < 0 {
				d = -d
			}
			if d > distance {
				furthest, distance = revision, d
			}
		}
		delete(t.Entries, furthest)
	}
	if h.Find != nil {
		h.RefreshFind()
	}
}
