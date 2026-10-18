package monitor

import (
	"strings"

	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	docedit "github.com/simjay/plous/internal/ui/editor"
	textview "github.com/simjay/plous/internal/ui/text"
)

func (a *ResourceActionState) InitDocument() {
	a.Document = docedit.New(kubernetes.YAML(*a.Object, true))
	a.Document.Syntax = "yaml"
	a.Document.Name = a.Resource.Kind + " / " + a.Object.GetName()
	a.Document.Target = a.Context + " · " + a.Object.GetNamespace() + "\nUID " + string(a.Object.GetUID())
}

func (a *ResourceActionState) ShowDiagnostic(message string) {
	a.Diagnostic = &textview.Viewer{Wrap: true}
	rows := []textview.Line{}
	for _, line := range strings.Split(message, "\n") {
		rows = append(rows, textview.Line{Text: line})
	}
	a.Diagnostic.SetLines(rows)
}

func (a *ResourceActionState) FailDocument(title, message string) {
	a.Err, a.DocumentErrorTitle = message, title
	a.Pending, a.RunningJob, a.ConfirmedID = "", "", ""
	a.SaveRequested, a.CloseAfterApply = false, false
	a.Diagnostic = nil
	if a.Review != nil {
		a.Review.Invalidate()
	}
	if a.Document != nil {
		a.Stage = "edit"
		a.Document.Controls = false
		a.Document.Diagnostic = title + " · ! Error details\n" + message
	}
}
