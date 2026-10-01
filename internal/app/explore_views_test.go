package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/kubernetes"
)

func TestExploreSidebarOpensNativeResourceAndKeepsScope(t *testing.T) {
	m := exploreModel(t)
	m.syncExploreTree()
	var deployment int = -1
	for i, row := range m.treeRows() {
		if row.kind == "resource" && row.resource == "deployments.apps" {
			deployment = i
		}
	}
	if deployment < 0 {
		t.Fatal("native resource missing from expanded Workloads sidebar")
	}
	m.treeCursor = deployment
	m.focus = 0
	m.draft().insert("Keep investigation draft", true)
	m.updateTree("enter")
	if m.explore.preference.Resource != "deployments.apps" || m.explore.preference.Namespace != "shop" || m.focus != 1 || m.draft().text != "Keep investigation draft" {
		t.Fatal("sidebar resource selection lost namespace, focus, or draft")
	}
	m.syncExploreTree()
	view := m.treeBody(36, 30)
	if !strings.Contains(ansi.Strip(view), "Deployments") || !strings.Contains(view, "\x1b[") {
		t.Fatal("current resource lacks persistent presentation")
	}
}

func TestExploreSidebarExpansionAndAPIIdentity(t *testing.T) {
	m := exploreModel(t)
	m.explore.resources = append(m.explore.resources, kubernetes.Resource{Group: "example.io", Version: "v1", Name: "pods", Kind: "Pod", Namespaced: true})
	m.syncExploreTree()
	key := categoryTreeKey(m.connection, "Other APIs")
	m.expanded[key] = true
	for _, row := range m.treeRows() {
		if row.resource == "pods.example.io" && row.parent != key {
			t.Fatal("extension Pod inherited native Workloads category")
		}
	}
	for i, row := range m.treeRows() {
		if row.key == categoryTreeKey(m.connection, "Workloads") {
			m.treeCursor = i
			break
		}
	}
	m.updateTree("enter")
	for _, row := range m.treeRows() {
		if row.resource == "pods" {
			t.Fatal("collapsed category still contains resources")
		}
	}
	if m.explore.preference.Resource != "pods" {
		t.Fatal("folding changed resource selection")
	}
}

func TestPodSummaryShowsFailureEvidenceBeforeRawMetadata(t *testing.T) {
	m := exploreModel(t)
	object := exploreObject("checkout-6d8f9-x2k7m", "pod-uid")
	state := object.Object["status"].(map[string]any)["containerStatuses"].([]any)[0].(map[string]any)
	state["ready"] = false
	state["restartCount"] = int64(12)
	state["state"] = map[string]any{"waiting": map[string]any{"reason": "CrashLoopBackOff"}}
	state["lastState"] = map[string]any{"terminated": map[string]any{"reason": "OOMKilled", "exitCode": int64(137)}}
	m.explore.detail = &object
	m.explore.detailValid = true
	m.explore.container = "app"
	for _, width := range []int{62, 80, 110} {
		view := ansi.Strip(m.exploreDetailBody(width, 23))
		for _, want := range []string{"CrashLoopBackOff", "Containers", "OOMKilled", "137", "Esc back"} {
			if !strings.Contains(view, want) {
				t.Fatalf("width %d missing first-screen evidence %q:\n%s", width, want, view)
			}
		}
		if strings.Contains(view, `{"`) {
			t.Fatal("Summary contains a serialized JSON object")
		}
	}
}

func TestPodListPreservesStatusAcrossWidths(t *testing.T) {
	m := exploreModel(t)
	state := m.explore.collection.Objects[0].Object["status"].(map[string]any)["containerStatuses"].([]any)[0].(map[string]any)
	state["ready"] = false
	state["state"] = map[string]any{"waiting": map[string]any{"reason": "CrashLoopBackOff"}}
	for _, width := range []int{34, 60, 90, 140} {
		view := ansi.Strip(m.exploreTableBody(width, 15))
		if !strings.Contains(view, "CrashLoopBackOff") || !strings.Contains(view, "Filter resources") {
			t.Fatalf("essential list context missing at width %d:\n%s", width, view)
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatalf("list overflow at %d: %q", width, line)
			}
		}
	}
}

func TestSummaryPreservesMultilineAndEmptyConfigValues(t *testing.T) {
	section := kubernetes.DetailSection{Title: "Data", Columns: []string{"Key", "Value"}, Rows: [][]string{{"settings", "first\nsecond"}, {"optional", ""}}}
	for _, width := range []int{28, 100} {
		view := ansi.Strip(strings.Join(renderDetailTable(section, width, ""), "\n"))
		first, second := strings.Index(view, "first"), strings.Index(view, "second")
		if first < 0 || second <= first || !strings.Contains(view[first:second], "\n") || !strings.Contains(view, "optional") || !strings.Contains(view, "(empty)") {
			t.Fatalf("width %d lost configuration data structure:\n%s", width, view)
		}
	}
}

func TestSummaryColorsOperationalValuesOnly(t *testing.T) {
	if summaryValue("Type", "Ready") != "Ready" || summaryValue("Value", "Error") != "Error" || summaryValue("Status", "False") != "False" {
		t.Fatal("non-operational text gained a misleading status color")
	}
	if summaryValue("State", "CrashLoopBackOff") == "CrashLoopBackOff" {
		t.Fatal("container failure lost its status color")
	}
}

func TestExploreResourceHeadingSurvivesLongConnectionStatus(t *testing.T) {
	m := exploreModel(t)
	m.explore.status = "Watch denied; refreshing every 5s"
	view := ansi.Strip(m.exploreBody(34, 20))
	if !strings.Contains(view, "Pods") {
		t.Fatalf("refresh status hid the resource identity:\n%s", view)
	}
}
