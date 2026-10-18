package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	helmcommon "github.com/simjay/plous/internal/integrations/helm/common"
	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
)

type helmExperienceRunner func(context.Context, helmtool.Config, []string, string) ([]byte, error)

func (f helmExperienceRunner) Run(c context.Context, cfg helmtool.Config, args []string, in string) ([]byte, error) {
	return f(c, cfg, args, in)
}

func TestHelmTimelineOrdersLoadsAndReturnsFromExactRevision(t *testing.T) {
	m, h := helmModel(t)
	h.Releases = []helmtool.Release{{Name: "shop", Namespace: "apps", Revision: 4, Chart: "demo"}}
	m.updateHelm("enter")
	if h.Pending != "timeline" {
		t.Fatal("Releases did not open history")
	}
	m.updateHelmResult(monitor.HelmResult{State: h, Run: h.Run, Kind: "timeline", Snapshot: &helmtool.Snapshot{Name: "shop", Namespace: "apps", History: []helmtool.Revision{{Revision: 1}, {Revision: 4}, {Revision: 3}}}})
	h.Pending = ""
	timeline := h.Timeline
	if timeline.Selected() != 4 || timeline.Revisions[2].Revision != 1 {
		t.Fatal("history not newest first")
	}
	m.updateHelm("down")
	timeline.Scroll = 2
	m.updateHelm("enter")
	if h.Pending != "detail" || h.JobRevision != 3 || h.RequestedRelease.Name != "shop" {
		t.Fatal("wrong inspector target")
	}
	m.updateHelmResult(monitor.HelmResult{State: h, Run: h.Run, Kind: "detail", Snapshot: &helmtool.Snapshot{Name: "shop", Namespace: "apps", Version: 3}})
	h.Pending = ""
	if h.Tab != 5 {
		t.Fatal("inspector does not start with resources")
	}
	m.updateHelm("esc")
	if h.Timeline != timeline || timeline.Cursor != 1 || timeline.Scroll != 2 || h.Detail != nil {
		t.Fatal("back lost timeline state")
	}
	m.updateHelm("B")
	if h.JobRevision != 3 || h.Pending != "rollback" {
		t.Fatal("timeline rollback target changed")
	}
	h.Pending = ""
	h.BeginWorkflow()
	m.leaveHelmWorkflow(h, true)
	if h.Timeline != timeline || timeline.Selected() != 3 {
		t.Fatal("workflow lost timeline origin")
	}
	m.updateHelm("esc")
	if h.Timeline != nil || h.Cursor != 0 {
		t.Fatal("timeline did not return to selected release")
	}
}

func TestHelmTimelineUnavailableBaselineStaleResultsAndReadScope(t *testing.T) {
	m, h := helmModel(t)
	h.OpenTimeline(&helmtool.Snapshot{Name: "shop", Namespace: "apps", History: []helmtool.Revision{{Revision: 7}, {Revision: 1}}})
	var calls []string
	h.Client.Runner = helmExperienceRunner(func(_ context.Context, _ helmtool.Config, args []string, _ string) ([]byte, error) {
		calls = append(calls, strings.Join(args, " "))
		if args[0] != "status" || args[1] != "shop" || !strings.Contains(strings.Join(args, " "), "--namespace apps") {
			t.Fatal("unexpected read", args)
		}
		return json.Marshal(&helmtool.Snapshot{Name: "shop", Namespace: "apps", Version: 1, Chart: helmcommon.Chart{Metadata: map[string]any{"name": "demo"}}})
	})
	cmd := h.SyncTimeline()
	if cmd == nil || !strings.Contains(h.Timeline.Entries[7].Err, "Baseline revision 6 unavailable") {
		t.Fatal("missing predecessor was silently substituted")
	}
	result := cmd().(monitor.HelmResult)
	if len(calls) != 1 || strings.Contains(calls[0], "--revision 7") {
		t.Fatal("wrong baseline read", calls)
	}
	old := h.Timeline
	m.chooseHelmNamespace(h, "production")
	m.updateHelmResult(result)
	if h.Timeline != nil || len(old.Entries) != 1 {
		t.Fatal("stale result restored another namespace")
	}
}

func TestHelmInspectorSelectorsKeepIdentityAndLatestRelease(t *testing.T) {
	m, h := helmModel(t)
	h.Preference.Filter = "first"
	h.Releases = []helmtool.Release{{Name: "first", Namespace: "apps", Chart: "demo", Revision: 4}, {Name: "second", Namespace: "apps", Chart: "demo", Revision: 7}, {Name: "unrelated", Namespace: "apps", Chart: "other", Revision: 1}}
	h.Detail = &helmtool.Snapshot{Name: "first", Namespace: "apps", Version: 3, Chart: helmcommon.Chart{Metadata: map[string]any{"name": "demo"}}, History: []helmtool.Revision{{Revision: 3}, {Revision: 4}}}
	m.updateHelm("J")
	if len(h.Menu) != 2 {
		t.Fatal("release selector followed text filter or included unrelated charts", h.Menu)
	}
	m.updateHelm("down")
	m.updateHelm("enter")
	if h.RequestedRelease.Name != "second" || h.RequestedRelease.Revision != 0 || h.Pending != "detail" {
		t.Fatal("release switch did not request latest")
	}
	h.Pending = ""
	h.RequestedRelease = nil
	m.updateHelm("F")
	if !strings.HasPrefix(h.Menu[0], "Revision 4") || h.Choice != 1 {
		t.Fatal("revision selector lost historical selection")
	}
	m.updateHelm("home")
	m.updateHelm("enter")
	if h.JobRevision != 4 {
		t.Fatal("revision selector ignored exact revision")
	}
}

func TestHelmChartsAndInspectorFitAndSeparateHooks(t *testing.T) {
	m, h := helmModel(t)
	m.chooseHelmSection("Charts")
	for i := 0; i < 12; i++ {
		h.Releases = append(h.Releases, helmtool.Release{Name: fmt.Sprintf("release-%02d", i), Namespace: "apps", Chart: fmt.Sprintf("chart-%d", i/4), Version: fmt.Sprint(i%2 + 1), Revision: i + 1, Status: "deployed"})
	}
	for _, size := range [][2]int{{28, 10}, {50, 16}, {90, 26}, {150, 40}} {
		for _, cursor := range []int{0, 5, 11} {
			h.Cursor = cursor
			view := m.helmBody(size[0], size[1])
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] || !strings.Contains(ansi.Strip(view), fmt.Sprintf("release-%02d", cursor)) {
				t.Fatalf("card selection clipped at %v: %s", size, ansi.Strip(view))
			}
		}
	}
	snapshot := &helmtool.Snapshot{Name: "shop", Namespace: "apps", Version: 1, Manifest: "# Source: demo/templates/config.yaml\napiVersion: v1\nkind: ConfigMap\nmetadata: {name: config}\ndata: {password: hidden}\n---\napiVersion: apps/v1\nkind: Deployment\nmetadata: {name: web}\nspec: {replicas: 2}\n"}
	hookData, _ := json.Marshal(map[string]any{"hooks": []map[string]any{{"name": "preflight", "events": []string{"pre-upgrade"}, "manifest": "apiVersion: batch/v1\nkind: Job\nmetadata: {name: preflight}\n"}}})
	if err := json.Unmarshal(hookData, snapshot); err != nil {
		t.Fatal(err)
	}
	h.Detail = snapshot
	h.Tab = 5
	resources, err := monitor.HelmResources(snapshot)
	if err != nil || resources[0].Kind != "Deployment" || resources[1].Group != "Configuration" || len(resources) != 3 || resources[2].Group != "Hooks" {
		t.Fatal("inventory not grouped by purpose", err, resources)
	}
	for _, size := range [][2]int{{28, 10}, {50, 16}, {100, 26}, {150, 40}} {
		view := m.helmBody(size[0], size[1])
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] || !strings.Contains(ansi.Strip(view), "web") {
			t.Fatalf("inspector selection clipped at %v: %s", size, ansi.Strip(view))
		}
	}
	h.Subcursor = 1
	m.updateHelm("enter")
	m.updateHelm("enter")
	if h.Document == nil || strings.Contains(h.Document.Text(), "hidden") || !strings.Contains(h.Document.Text(), "config") {
		t.Fatal("resource selection or masking lost after grouping")
	}
}

func TestHelmChartSectionReturnPreservesCardAndTimelineCancellation(t *testing.T) {
	m, h := helmModel(t)
	m.chooseHelmSection("Charts")
	h.Cursor = 4
	m.chooseHelmSection("Releases")
	h.Cursor = 2
	m.chooseHelmSection("Charts")
	if h.Cursor != 4 {
		t.Fatal("return to Charts lost the card position")
	}
	h.OpenTimeline(&helmtool.Snapshot{Name: "shop", Namespace: "apps", History: []helmtool.Revision{{Revision: 3}, {Revision: 2}, {Revision: 1}}})
	cancelled := false
	job := &monitor.HelmTimelineLoad{Revision: 3, Cancel: func() { cancelled = true }}
	h.Timeline.Load = job
	m.helmTimelineKey(h, "down")
	if !cancelled || h.Timeline.Load != nil {
		t.Fatal("a slow previous comparison blocked the newly selected revision")
	}
	m.updateHelmResult(monitor.HelmResult{State: h, Kind: "revision-diff", TimelineLoad: job, Changes: &helmtool.RevisionChanges{Added: 42}})
	if len(h.Timeline.Entries) != 0 {
		t.Fatal("cancelled comparison was accepted")
	}
}
