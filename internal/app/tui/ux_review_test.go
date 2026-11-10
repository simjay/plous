package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	textview "github.com/simjay/plous/internal/ui/text"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Review fixtures exercise the same screen renderer as the executable. Exported
// frames are temporary review artifacts, not automatically accepted baselines.
func TestUXScreenMatrix(t *testing.T) {
	fixtures := map[string]func(*testing.T) *model{
		"first-launch": func(t *testing.T) *model { m, _ := testModel(t); return m },
		"workspace-overview": func(t *testing.T) *model {
			m, _ := testModel(t)
			createWorkspace(t, m, "Payments production")
			m.closeSettings()
			m.mode = 0
			return m
		},
		"welcome":       func(t *testing.T) *model { m, _ := testModel(t); m.mode = 1; return m },
		"new-workspace": func(t *testing.T) *model { m, _ := testModel(t); m.beginWorkspace(); return m },
		"settings":      func(t *testing.T) *model { m, _ := testModel(t); m.openSettings(); return m },
		"context-setup": func(t *testing.T) *model {
			m, _ := testModel(t)
			installDefaultKubeconfig(t, contextFile(t, "development", "production-長い-context"))
			createWorkspace(t, m, "Payments")
			openTool(t, m, "kubernetes")
			return m
		},
		"help-over-form": func(t *testing.T) *model {
			m, _ := testModel(t)
			m.beginWorkspace()
			m.form.Fields[0].Value = textview.NewField("Unfinished workspace")
			m.execute("help")
			return m
		},
		"chat-transcript": chatReadingModel,
		"model-picker": func(t *testing.T) *model {
			m, _ := testModel(t)
			addAIConnection(t, m, "Fixture provider", "openai")
			startChat(t, m)
			m.openModelPicker()
			return m
		},
		"command-input": func(t *testing.T) *model {
			m := exploreModel(t)
			typeText(m, ":names")
			return m
		},
		"activity-comparison": func(t *testing.T) *model { return readOnlyPersistenceModel(t, "Activity") },
		"chat-reader": func(t *testing.T) *model {
			m := sourceChatModel(t, 1)
			m.chat().Turns[0].Response.Text = "## Deployment status\n\n| Resource | State |\n| --- | --- |\n| payments | Ready |\n\n" + strings.Repeat("Observed namespace: production.\n", 20)
			m.openChatReply()
			return m
		},
		"chat-sources": func(t *testing.T) *model {
			m := sourceChatModel(t, 2)
			runSourceCommand(t, m, press(t, m, "enter"))
			return m
		},
		"kubernetes-list": exploreModel,
		"kubernetes-yaml": yamlViewModel,
		"kubernetes-logs": func(t *testing.T) *model {
			m := exploreModel(t)
			m.openExploreSelection(4)
			m.explore.DetailValid = true
			m.explore.Container, m.explore.LogKey = "app", "loaded"
			m.podLogViewer().Append(strings.Repeat("earlier log line\n", 30) + "latest log evidence")
			return m
		},
		"resource-editor": func(t *testing.T) *model { m, _ := documentModel(t); return m },
		"resource-confirmation": func(t *testing.T) *model {
			m := exploreModel(t)
			m.openExploreSelection(0)
			m.explore.DetailValid = true
			m.openResourceActions("")
			a := m.explore.Action
			a.Pending, a.Stage, a.Operation = "", "confirm", "delete"
			return m
		},
		"exec-review": func(t *testing.T) *model {
			m := exploreModel(t)
			width, height := m.terminalDimensions()
			tm := reviewTerminal(t, width, height)
			tm.Context, tm.Namespace, tm.Pod, tm.Container, tm.Command = "fixture", "shop", "checkout", "app", "sh"
			writeTerminalLines(t, tm, 0, 20)
			tm.BeginReview()
			m.explore.Terminal = tm
			return m
		},
		"resource-values": func(t *testing.T) *model {
			m, _ := valueModel(t)
			return m
		},
		"activity": func(t *testing.T) *model {
			m := exploreModel(t)
			m.mode = 2
			changes, err := m.store.History(m.active)
			if err != nil || len(changes) == 0 {
				t.Fatal("missing activity", err)
			}
			m.setActivityDetail(changes[0])
			return m
		},
	}
	for _, state := range []string{"ready", "waiting", "denied", "stale", "partial"} {
		fixtures["usage-"+state] = func(t *testing.T) *model {
			m, _ := chartModel(t)
			m.explore.Detail.SetName("checkout-service-with-a-long-name")
			m.syncExploreUsage()
			run := m.explore.Usage
			run.Loaded = true
			run.Points = nil
			now := time.Now().Add(-time.Second)
			for i := 0; i < 60; i++ {
				if i > 22 && i < 29 {
					continue
				}
				cpu := .1 + float64(i%7)/30
				if i == 40 {
					cpu = .9
				}
				run.Points = append(run.Points, kubernetes.Usage{CPU: cpu, Memory: float64(64+i%5) * 1048576, Timestamp: now.Add(time.Duration(i-59) * 15 * time.Second)})
			}
			run.Latest = run.Points[len(run.Points)-1]
			switch state {
			case "waiting":
				run.Loaded = false
				run.Points = nil
				run.Latest = kubernetes.Usage{}
			case "denied":
				run.Err = "Access denied (RBAC). Metrics are unavailable."
				run.Points = nil
				run.Latest = kubernetes.Usage{}
			case "stale":
				run.Latest.Timestamp = now.Add(-2 * time.Minute)
			case "partial":
				run.Err = "Waiting for complete, fresh Pod metrics."
				run.Note = "Metrics from 2/3 active Pods"
			}
			m.execute("charts")
			return m
		}
	}
	for _, phase := range []string{"loading", "empty", "error"} {
		fixtures["kubernetes-"+phase] = func(t *testing.T) *model {
			m := exploreModel(t)
			m.explore.Collection.Objects = nil
			if phase == "loading" {
				m.explore.Listed = false
			}
			if phase == "error" {
				m.explore.Err = "Cluster access denied. Check the selected context and credentials."
			}
			return m
		}
	}
	fixtures["resource-confirmation-error"] = func(t *testing.T) *model {
		m := fixtures["resource-confirmation"](t)
		m.explore.Action.Err = "Resource changed. Review it again before saving."
		return m
	}
	fixtures["kubernetes-yaml-search"] = func(t *testing.T) *model {
		m := yamlViewModel(t)
		d := m.syncYAMLDocument().Document
		d.Key("/", "")
		d.Paste("apiVersion")
		return m
	}
	for _, section := range []string{"Releases", "Charts", "Timeline", "Inspector", "Error"} {
		fixtures["helm-"+strings.ToLower(section)] = func(t *testing.T) *model {
			m, h := helmModel(t)
			h.Releases = []helmtool.Release{{Name: "payments", Namespace: "production", Chart: "platform", Version: "1.2.3", Revision: 4, Status: "deployed"}}
			h.Preference.Section = section
			switch section {
			case "Timeline":
				h.Preference.Section = "Releases"
				h.OpenTimeline(inspectorFixture())
			case "Inspector":
				h.Preference.Section = "Releases"
				h.ApplyInspectorSnapshot(inspectorFixture())
			case "Error":
				h.Preference.Section = "Releases"
				h.Err = strings.Repeat("Cannot read releases: permission denied. ", 8)
			}
			return m
		}
	}
	for scene, fixture := range fixtures {
		t.Run(scene, func(t *testing.T) {
			m := fixture(t)
			*m.draft() = textview.NewField("Draft preserved through resizing\n最後の行")
			active, connection, draft := m.active, m.connection, m.draft().Text
			for _, size := range [][2]int{{80, 16}, {80, 24}, {120, 30}, {180, 42}, {240, 60}} {
				m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				for _, hidden := range []bool{false, true} {
					m.leftHidden, m.rightHidden = hidden, hidden
					m.ensureVisibleFocus()
					frame := m.View().Content
					label := fmt.Sprintf("%s-%dx%d-mainonly-%t", scene, size[0], size[1], hidden)
					lines := strings.Split(frame, "\n")
					if len(lines) != size[1] {
						t.Fatalf("%s: %d rows, expected %d", label, len(lines), size[1])
					}
					for row, line := range lines {
						if width := ansi.StringWidth(line); width != size[0] {
							t.Fatalf("%s row %d: width %d, expected %d", label, row, width, size[0])
						}
					}
					if m.active != active || m.connection != connection || m.draft().Text != draft {
						t.Fatalf("%s lost scope or draft", label)
					}
					content := map[string]string{
						"resource-editor": "apiVersion", "kubernetes-yaml": "apiVersion",
						"kubernetes-loading":    "Loading Pods",
						"kubernetes-logs":       "latest log evidence",
						"resource-confirmation": "Confirm delete", "exec-review": "line-0019",
						"resource-confirmation-error": "Confirm delete", "kubernetes-yaml-search": "apiVersion: v1",
						"kubernetes-empty": "No Pods", "kubernetes-error": "Cluster access denied",
					}
					if want := content[scene]; want != "" && !strings.Contains(ansi.Strip(frame), want) {
						t.Errorf("%s hides primary content %q", label, want)
					}
					if strings.HasPrefix(scene, "usage-") {
						for _, want := range []string{"[I] Back", "Live usage", "CPU ", "Memory "} {
							if !strings.Contains(ansi.Strip(frame), want) {
								t.Errorf("%s hides chart content %q", label, want)
							}
						}
					}
					if scene == "chat-transcript" && !hidden && !strings.Contains(ansi.Strip(frame), "Latest answer") {
						t.Errorf("%s hides the latest answer behind controls", label)
					}
					if root := os.Getenv("PLOUS_REVIEW_DIR"); root != "" {
						if err := os.MkdirAll(root, 0700); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(filepath.Join(root, "ux-"+label+".ansi"), []byte(frame), 0600); err != nil {
							t.Fatal(err)
						}
					}
				}
			}
		})
	}
}

func usabilityObjects(t *testing.T) []unstructured.Unstructured {
	t.Helper()
	data, err := os.ReadFile("../../../tests/fixtures/usability/resources.json")
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
