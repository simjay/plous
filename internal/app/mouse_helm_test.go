package app

import (
	"strings"
	"testing"

	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
	diffview "github.com/simjay/plous/internal/ui/diff"
	textview "github.com/simjay/plous/internal/ui/text"
)

func TestMouseHelmCardIdentityAndSort(t *testing.T) {
	m, h := helmModel(t)
	m.width, m.height = 200, 50
	h.Releases = []helmtool.Release{{Name: "one", Namespace: "apps", Revision: 3, Chart: "demo", Version: "1.0.0"}, {Name: "two", Namespace: "other", Revision: 1, Chart: "demo", Version: "1.0.0"}}
	m.chooseHelmSection("Charts")
	clickMouse(t, m, "main", "helmrow/other/two")
	if h.Preference.Selected != "other/two" || h.Pending != "" {
		t.Fatal("card selection opened or lost identity")
	}
	clickMouse(t, m, "main", "helmrow/other/two")
	if h.Pending != "detail" {
		t.Fatal("card did not open selected release")
	}
	h.Pending = ""
	m.chooseHelmSection("Releases")
	clickMouse(t, m, "main", "sort/Name")
	if !h.Preference.Descending || h.Menu != nil {
		t.Fatal("sort heading did not apply sort")
	}
}

func TestMouseDiffDisabledAndStaleSaveStayGuarded(t *testing.T) {
	m, a := documentModel(t)
	m.width, m.height = 200, 50
	before := a.Document.Text()
	a.Document.Paste("# changed\n")
	a.Review = diffview.New(before, a.Document.Text(), a.Document.Target, a.Document.Revision, true)
	a.Review.ConfirmLabel, a.Review.ConfirmKey = "Save", ":w"
	a.Stage = "review"
	a.Review.CanConfirm = false
	m.activateMouse(textview.MouseSpan{Scope: "main", Target: "disabled"}, false)
	if a.Pending != "" || a.SaveRequested {
		t.Fatal("disabled save dispatched")
	}
	a.Review.CanConfirm = true
	a.Document.Paste("# changed after review\n")
	clickMouse(t, m, "main", "diffaction/1")
	if a.Pending != "" || a.Err == "" {
		t.Fatal("stale comparison saved")
	}
}

func TestMouseHelmDraftFormsPreserveValues(t *testing.T) {
	m, h := helmModel(t)
	m.width, m.height = 200, 50
	m.beginHelmDraft(h)
	f := h.Draft.Form
	var key string
	for _, field := range f.Fields {
		if len(field.Options) == 0 {
			key = field.Key
			break
		}
	}
	clickMouse(t, m, "main", "field/"+key)
	if h.Draft.Form != f || h.Pending != "" {
		t.Fatal("field click submitted workflow")
	}
	m.View()
	for _, span := range m.mouse.spans {
		if strings.HasPrefix(span.Target, "formaction/") {
			return
		}
	}
	t.Fatal("workflow actions lack mouse targets")
}
