package app

import (
	"fmt"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
	textview "github.com/simjay/plous/internal/ui/text"
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
		"source-setup": func(t *testing.T) *model {
			m, _ := codeModel(t)
			m.beginToolSetup(false)
			m.beginConnection("code", nil)
			return m
		},
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
	for _, scene := range []string{"files", "sources", "render", "trace", "validate", "editor", "review", "loading", "failure"} {
		fixtures["code-"+scene] = func(t *testing.T) *model {
			m, _ := codeModel(t)
			if scene == "files" {
				return m
			}
			m.selectScope(m.active, "manifests", m.connection)
			s := m.coder()
			s.Preference.Selected = "kustomize:prod"
			if scene == "loading" {
				s.Inventory, s.Busy, s.Running = nil, true, "load"
				return m
			}
			if scene == "failure" {
				s.Busy, s.Pending, s.Running, s.RunningEntry = false, "", "", ""
				s.MarkPreviewAttempt(s.Preference.Selected)
				s.Failures[s.Preference.Selected] = strings.Repeat("Cannot load a referenced file. ", 12)
				return m
			}
			s.Pending = "render"
			finishCode(t, m)
			s.ChooseSection(map[string]int{"sources": 0, "render": 1, "trace": 2, "validate": 3}[scene])
			if scene == "editor" || scene == "review" {
				s.Preference.File = "README.md"
				m.openCodeFile(true)
				s.Document.Paste("unsaved changes\n")
				if scene == "review" {
					m.codeDocumentCommand("v")
				}
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
						"code-editor": "unsaved changes", "code-review": "+ unsaved changes",
						"resource-editor": "apiVersion", "kubernetes-yaml": "apiVersion",
						"code-validate": "▸ warning", "kubernetes-loading": "Loading Pods",
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
