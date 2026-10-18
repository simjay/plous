package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func usabilityObjects(t *testing.T) []unstructured.Unstructured {
	t.Helper()
	data, err := os.ReadFile("../../tests/fixtures/usability/resources.json")
	if err != nil {
		t.Fatal(err)
	}
	var objects []unstructured.Unstructured
	if err := json.Unmarshal(data, &objects); err != nil {
		t.Fatal(err)
	}
	return objects
}

func usabilityModel(t *testing.T, object unstructured.Unstructured) *model {
	t.Helper()
	m := exploreModel(t)
	resource := kubecommon.Resource{Name: map[string]string{"Pod": "pods", "Deployment": "deployments", "StatefulSet": "statefulsets", "Service": "services", "Ingress": "ingresses"}[object.GetKind()], Kind: object.GetKind(), Version: "v1", Namespaced: true, Verbs: []string{"get", "list", "watch", "update", "patch", "delete"}}
	if group, _, ok := strings.Cut(object.GetAPIVersion(), "/"); ok {
		resource.Group = group
	}
	if resource.Kind == "Deployment" || resource.Kind == "StatefulSet" {
		resource.Subresources = map[string][]string{"scale": {"get", "update", "patch"}}
	}
	replaced := false
	for i, existing := range m.explore.Resources {
		if existing.Key() == resource.Key() {
			m.explore.Resources[i], replaced = resource, true
		}
	}
	if !replaced {
		m.explore.Resources = append(m.explore.Resources, resource)
	}
	m.explore.Resource = resource
	m.explore.Preference.Resource, m.explore.Preference.Namespace = resource.Key(), object.GetNamespace()
	m.explore.Collection = kubernetes.Collection{Objects: []unstructured.Unstructured{object}}
	m.explore.Selected = kubernetes.Identity(object)
	m.openExploreSelection(0)
	m.explore.DetailValid = true
	m.status = ""
	m.syncExploreTree()
	return m
}

func TestUsabilityResourceViewsFitAndRetainEssentialContent(t *testing.T) {
	for _, object := range usabilityObjects(t) {
		for _, size := range [][2]int{{80, 24}, {120, 32}, {160, 40}, {240, 50}, {320, 60}} {
			t.Run(fmt.Sprintf("%s/%d", object.GetKind(), size[0]), func(t *testing.T) {
				m := usabilityModel(t, object)
				m.width, m.height = size[0], size[1]
				view := m.render()
				if len(strings.Split(view, "\n")) != m.height {
					t.Fatal("wrong screen height")
				}
				for _, line := range strings.Split(view, "\n") {
					if ansi.StringWidth(line) != m.width {
						t.Fatalf("row width %d, expected %d", ansi.StringWidth(line), m.width)
					}
				}
				if size[0] >= 120 {
					essential := map[string]string{"Pod": "CrashLoopBackOff", "Deployment": "ProgressDeadlineExceeded", "StatefulSet": "Ready replicas", "Service": "10.96.0.42", "Ingress": "checkout.example.com"}[object.GetKind()]
					compact := strings.Join(strings.Fields(ansi.Strip(view)), "")
					if !strings.Contains(compact, strings.ReplaceAll(essential, " ", "")) {
						t.Fatalf("first screen hides %q: %s", essential, ansi.Strip(view))
					}
				}
				m.updateExplore("end")
				if !strings.Contains(ansi.Strip(m.render()), "Finalizers") {
					t.Fatalf("End cannot reach metadata at %dx%d (scroll %d, limit %d)", m.width, m.height, m.explore.Scroll, m.exploreDetailScrollLimit())
				}
				m.updateExplore("home")
				if dir := os.Getenv("PLOUS_REVIEW_DIR"); dir != "" {
					if err := os.MkdirAll(dir, 0700); err != nil {
						t.Fatal(err)
					}
					for _, scene := range []string{"summary", "logs", "error"} {
						if scene != "summary" && object.GetKind() != "Pod" {
							continue
						}
						if scene != "summary" {
							m.openExploreSelection(4)
							m.explore.DetailValid = true
							m.explore.Container, m.explore.LogKey = "checkout", "fixture-stream"
							m.podLogViewer().Reset()
							m.podLogViewer().Append("ERROR database connection refused\n" + strings.Repeat("INFO retrying connection to checkout-db:5432\n", 30))
							if scene == "error" {
								m.explore.LogError = "Permission denied by Kubernetes RBAC."
							}
						}
						if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s-%s-%d.ansi", object.GetKind(), scene, m.width)), []byte(m.render()), 0600); err != nil {
							t.Fatal(err)
						}
					}
				}
			})
		}
	}
}

func TestUsabilityDetailActionsAndYAMLReturnAcrossResourceTypes(t *testing.T) {
	for _, object := range usabilityObjects(t) {
		t.Run(object.GetKind(), func(t *testing.T) {
			m := usabilityModel(t, object)
			e := m.explore
			e.Preference.Filter, e.Preference.Selector, e.Preference.Sort = "checkout", "app=checkout", 1
			e.Scroll = 7
			preference, selection := e.Preference, e.Selected
			actions := map[string]bool{}
			for _, action := range monitor.ActionsFor(e.Resource, e.Detail) {
				actions[action.Id] = true
			}
			workload := object.GetKind() == "Deployment" || object.GetKind() == "StatefulSet"
			for action, want := range map[string]bool{"logs": workload || object.GetKind() == "Pod", "exec": workload || object.GetKind() == "Pod", "scale": workload, "restart": workload, "edit": true, "delete": true} {
				if actions[action] != want {
					t.Fatalf("%s applicability = %t, want %t", action, actions[action], want)
				}
			}
			typeText(m, "y")
			m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
			m.Update(tea.WindowSizeMsg{Width: 240, Height: 50})
			press(t, m, "esc")
			if e.Detail == nil || e.Detail.GetUID() != object.GetUID() || e.Tab != 0 || e.Scroll != 7 || e.Preference != preference || e.Selected != selection {
				t.Fatal("YAML and resize lost the resource or its browsing state")
			}
		})
	}
}

func TestUsabilityDetailErrorCanBeReadToTheEnd(t *testing.T) {
	m := usabilityModel(t, usabilityObjects(t)[0])
	m.width, m.height = 120, 32
	e := m.explore
	e.DetailValid = false
	e.DetailText = strings.Repeat("The requested object could not be read.\n", 40) + "Recovery instruction at end"
	if strings.Contains(ansi.Strip(m.render()), "Recovery instruction at end") {
		t.Fatal("fixture does not require scrolling")
	}
	m.updateExplore("end")
	if !strings.Contains(ansi.Strip(m.render()), "Recovery instruction at end") {
		t.Fatalf("long detail error cannot be read: scroll=%d limit=%d\n%s", e.Scroll, m.exploreDetailScrollLimit(), ansi.Strip(m.render()))
	}
	m.updateExplore("home")
	if strings.Contains(ansi.Strip(m.render()), "Recovery instruction at end") {
		t.Fatal("Home did not restore the start of the error")
	}
}

func TestUsabilityCompactLogSourceAndExpansionStayVisible(t *testing.T) {
	m := usabilityModel(t, usabilityObjects(t)[0])
	m.openExploreSelection(4)
	m.explore.DetailValid, m.explore.Previous = true, true
	m.explore.LogKey = "fixture-stream"
	m.podLogViewer().Append("latest evidence")
	for _, width := range []int{28, 48, 60} {
		text := ansi.Strip(m.exploreLogBody(width, 18))
		for _, want := range []string{"Logs · checkout", "Previous", "Latest 200 lines", "[ff] Fullscreen", "latest evidence"} {
			if !strings.Contains(text, want) {
				t.Fatalf("width %d hides %q: %s", width, want, text)
			}
		}
	}
}

func TestUsabilityExecFailureAndEditCommandStayVisible(t *testing.T) {
	m := usabilityModel(t, usabilityObjects(t)[0])
	tm := reviewTerminal(t, 28, 8)
	m.explore.Terminal = tm
	tm.Done, tm.Captured, tm.Err = true, false, "The executable is unavailable. Choose another command."
	tm.BeginReview()
	text := ansi.Strip(m.terminalBody(28, 18))
	if !strings.Contains(text, "Session failed") || !strings.Contains(text, "[e] Edit command") {
		t.Fatalf("failure recovery hidden in compact Exec: %s", text)
	}
}
