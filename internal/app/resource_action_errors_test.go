package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestActionErrorsStayVisibleWithControlsAtEveryTerminalSize(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(0)
	m.explore.detailValid = true
	m.openResourceActions("")
	a := m.explore.action
	a.pending = ""
	a.busy = false
	a.operation = "delete"
	a.input = newEditor(strings.Repeat("metadata:\n  name: example\n", 40))
	a.proposed = a.object.DeepCopy()
	a.scale = &unstructured.Unstructured{Object: map[string]any{"spec": map[string]any{"replicas": int64(1)}}}
	a.err = "The resource changed since it was opened. Return to its details and review the current resource before trying again."
	for _, size := range [][2]int{{80, 24}, {120, 32}, {160, 40}, {200, 50}} {
		m.width, m.height = size[0], size[1]
		for _, stage := range []string{"confirm", "scale", "export"} {
			a.stage = stage
			view := ansi.Strip(m.render())
			for _, text := range []string{"resource changed since it was opened", "trying again.", "UID one", "Esc"} {
				if !strings.Contains(view, text) {
					t.Fatalf("%v %s clipped %q: %s", size, stage, text, view)
				}
			}
			if stage == "confirm" && !strings.Contains(view, "Confirm delete") {
				t.Fatalf("%v confirmation controls disappeared", size)
			}
			if stage == "review" && !strings.Contains(view, "Save to Kubernetes") {
				t.Fatalf("%v review controls disappeared", size)
			}
			lines := strings.Split(view, "\n")
			if len(lines) > m.height {
				t.Fatalf("%v %s error caused height overflow", size, stage)
			}
			for _, line := range lines {
				if ansi.StringWidth(line) > m.width {
					t.Fatalf("%v %s error caused width overflow", size, stage)
				}
			}
		}
	}
}
