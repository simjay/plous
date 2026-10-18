package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	helmcommon "github.com/simjay/plous/internal/integrations/helm/common"
	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestExploreInventoryInvalidation(t *testing.T) {
	e := &monitor.ExploreState{Resource: kubecommon.Resource{Name: "pods", Version: "v1", Namespaced: true}, Collection: kubernetes.Collection{Objects: []unstructured.Unstructured{exploreObject("first", "a"), exploreObject("second", "b")}}}
	now := time.Now()
	first := e.PrepareInventory(now)
	if e.PrepareInventory(now.Add(time.Millisecond)) != first {
		t.Fatal("unchanged inventory rebuilt")
	}
	e.Selected = "b"
	if e.PrepareInventory(now) != first {
		t.Fatal("selection rebuilt inventory")
	}
	e.Preference.Filter = "second"
	filtered := e.PrepareInventory(now)
	if filtered == first || len(filtered.Rows) != 1 || filtered.Rows[0].Object.GetName() != "second" {
		t.Fatal("filter cache stale")
	}
	e.Preference.Filter = ""
	e.Preference.Descending = true
	if rows := e.PrepareInventory(now).Rows; rows[0].Object.GetName() != "second" {
		t.Fatal("sort cache stale")
	}
	cached := e.PrepareInventory(now)
	if e.PrepareInventory(now.Add(time.Second)) == cached {
		t.Fatal("age and usage freshness never expire")
	}
	e.Collection.Objects = []unstructured.Unstructured{exploreObject("replacement", "c")}
	if rows := e.PrepareInventory(now).Rows; len(rows) != 1 || rows[0].Object.GetName() != "replacement" {
		t.Fatal("snapshot cache stale")
	}
	e.TableColumns(100)
	e.TableColumns(28)
	if e.Inventory.Width != 28 || !e.Inventory.Compact {
		t.Fatal("resize did not recalculate columns")
	}
}

func TestHelmInventoryRefreshAndSelection(t *testing.T) {
	h := &monitor.HelmState{Preference: monitor.HelmPreference{Section: "Charts"}, Releases: []helmtool.Release{{Name: "one", Chart: "a"}, {Name: "two", Chart: "b"}}}
	h.ChartGroups()
	initial := h.Inventory
	h.Cursor = 1
	h.Preference.Selected = "two"
	if len(h.ChartGroups()) != 2 || h.Inventory != initial {
		t.Fatal("selection rebuilt release inventory")
	}
	h.Preference.Filter = "two"
	if groups := h.ChartGroups(); len(groups) != 1 || groups[0].Key != "b" {
		t.Fatal("filter did not rebuild groups")
	}
	h.Preference.Filter = ""
	h.Releases = []helmtool.Release{{Name: "one", Chart: "changed"}}
	if groups := h.ChartGroups(); len(groups) != 1 || groups[0].Key != "changed" {
		t.Fatal("refresh did not rebuild groups")
	}
	snapshot := func(name string) *helmtool.Snapshot {
		return &helmtool.Snapshot{Name: "shop", Namespace: "apps", Version: 3, Manifest: fmt.Sprintf("apiVersion: v1\nkind: ConfigMap\nmetadata: {name: %s}\n", name)}
	}
	h.ApplyInspectorSnapshot(snapshot("before"))
	h.InspectorResources()
	cache := h.Resources
	h.InspectorResources()
	if h.Resources != cache {
		t.Fatal("unchanged snapshot parsed again")
	}
	h.ApplyInspectorSnapshot(snapshot("after"))
	resources, err := h.InspectorResources()
	if err != nil || len(resources) != 1 || resources[0].Name != "after" {
		t.Fatal("same revision refresh reused stale manifest", err)
	}
}

func TestHelmLargeCardGridKeepsSelectedGroupAndRelease(t *testing.T) {
	h := &monitor.HelmState{Preference: monitor.HelmPreference{Section: "Charts"}}
	for i := 0; i < 1000; i++ {
		h.Releases = append(h.Releases, helmtool.Release{Name: fmt.Sprintf("release-%04d", i), Chart: fmt.Sprintf("chart-%02d", i/100), Namespace: "apps", Status: "deployed"})
	}
	for _, width := range []int{28, 68, 100, 150} {
		for _, cursor := range []int{0, 99, 100, 501, 999} {
			h.Cursor = cursor
			view := ansi.Strip(monitor.HelmChartCards(h, width, 20))
			if !strings.Contains(view, fmt.Sprintf("release-%04d", cursor)) || !strings.Contains(view, fmt.Sprintf("chart-%02d", cursor/100)) {
				t.Fatalf("selection missing at %d/%d: %s", width, cursor, view)
			}
		}
	}
}

func TestHelmTimelineReusesAdjacentReadsUntilRefresh(t *testing.T) {
	snapshot := func(revision int) *helmtool.Snapshot {
		return &helmtool.Snapshot{Name: "shop", Namespace: "apps", Version: revision, Chart: helmcommon.Chart{Metadata: map[string]any{"name": "demo"}}, History: []helmtool.Revision{{Revision: 3}, {Revision: 2}, {Revision: 1}}}
	}
	calls := 0
	h := &monitor.HelmState{Client: &helmtool.Client{Config: helmtool.Config{Context: "test", Namespace: "apps"}}}
	h.Client.Runner = helmExperienceRunner(func(_ context.Context, _ helmtool.Config, args []string, _ string) ([]byte, error) {
		calls++
		for i, arg := range args {
			if arg == "--revision" {
				n, _ := strconv.Atoi(args[i+1])
				return json.Marshal(snapshot(n))
			}
		}
		t.Fatal("missing exact revision", args)
		return nil, nil
	})
	h.OpenTimeline(snapshot(3))
	for range 3 {
		cmd := h.SyncTimeline()
		if cmd == nil {
			t.Fatal("comparison not scheduled")
		}
		result := cmd().(monitor.HelmResult)
		if result.Err != nil {
			t.Fatal(result.Err)
		}
		h.ApplyTimelineChanges(result)
	}
	if calls != 2 {
		t.Fatalf("adjacent records read %d times, want 2", calls)
	}
	h.OpenTimeline(snapshot(3))
	result := h.SyncTimeline()().(monitor.HelmResult)
	h.ApplyTimelineChanges(result)
	if calls != 3 {
		t.Fatal("refresh reused an old retained record")
	}
}

func TestHelmWrappedChartHeadingKeepsCardVisible(t *testing.T) {
	h := &monitor.HelmState{Preference: monitor.HelmPreference{Section: "Charts"}, Releases: []helmtool.Release{{Name: "selected", Chart: strings.Repeat("chart-name-", 12), Namespace: "apps"}}}
	for _, height := range []int{6, 10, 18} {
		view := ansi.Strip(monitor.HelmChartCards(h, 28, height))
		if !strings.Contains(view, "selected") {
			t.Fatalf("wrapped heading hid selection at height %d: %s", height, view)
		}
	}
}
