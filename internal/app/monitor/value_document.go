package monitor

import (
	"github.com/simjay/plous/internal/ui"
	docedit "github.com/simjay/plous/internal/ui/editor"
	"strings"
)

func (a *ResourceActionState) OpenValueDocument(value ResourceValue) {
	a.Container, a.Stage, a.Cursor = value.Label, "valueview", 0
	d := docedit.New(value.Text)
	d.ReadOnly, d.Controls = true, true
	d.Name = ui.OneLine(a.Object.GetName()) + " · " + ui.OneLine(value.Label)
	d.Target = a.Context + " · " + a.Object.GetNamespace() + " · " + a.Resource.Kind
	if a.Resource.Key() == "secrets" {
		d.Target += " · Value revealed"
	}
	if strings.HasSuffix(value.Label, ".yaml") || strings.HasSuffix(value.Label, ".yml") {
		d.Syntax = "yaml"
	}
	a.ValueDocument = d
}

func (a *ResourceActionState) ClearValueDocument() {
	if a.ValueDocument != nil {
		a.ValueDocument.RefreshReadOnly("")
		a.ValueDocument = nil
	}
	a.ExportText, a.ExportValue = "", false
}
