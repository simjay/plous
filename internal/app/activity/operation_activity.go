package activity

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/simjay/plous/internal/ui"
	"github.com/simjay/plous/internal/workspace"
)

type Filter int

const (
	All Filter = iota
	PlousActions
	WorkspaceChanges
	HelmHistory
)

func (f Filter) String() string {
	return []string{"All activity", "Plous actions", "Workspace changes", "Observed Helm history"}[int(f)%4]
}

type ActivityEntry struct {
	Change       *workspace.Change
	Operation    *workspace.Operation
	Observations []*workspace.Operation
	At           time.Time
	SourceTime   bool
}

func sourceTime(value string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999 -0700 MST", "2006-01-02 15:04:05.999999999 -0700"} {
		if at, err := time.Parse(layout, value); err == nil && !at.IsZero() {
			return at, true
		}
	}
	return time.Time{}, false
}

func operationEntry(op *workspace.Operation) ActivityEntry {
	e := ActivityEntry{Operation: op, At: op.ObservedAt}
	if op.Kind == "revision" {
		e.Observations = []*workspace.Operation{op}
	}
	return e
}

func (e *ActivityEntry) prepareObservations() {
	if len(e.Observations) == 0 {
		return
	}
	sort.Slice(e.Observations, func(i, j int) bool {
		a, b := e.Observations[i], e.Observations[j]
		if a.ObservedAt.Equal(b.ObservedAt) {
			return a.ID < b.ID
		}
		return a.ObservedAt.After(b.ObservedAt)
	})
	e.Operation = e.Observations[0]
	e.At = e.Operation.ObservedAt
	for _, op := range e.Observations {
		if at, ok := sourceTime(op.SourceTime); ok {
			e.At, e.SourceTime = at, true
			break
		}
	}
}

func (e ActivityEntry) Category() Filter {
	if e.Change != nil {
		return WorkspaceChanges
	}
	if e.Operation.Kind == "revision" {
		return HelmHistory
	}
	return PlousActions
}

func (e ActivityEntry) Label() string {
	if e.Change != nil {
		return ChangeTitle(*e.Change)
	}
	op := e.Operation
	if op.Kind == "revision" {
		return fmt.Sprintf("%s · r%d", ui.OneLine(op.Release), op.Revision)
	}
	return ui.OneLine(strings.ToUpper(op.Kind[:min(1, len(op.Kind))]) + op.Kind[min(1, len(op.Kind)):] + " " + op.Release)
}

func (e ActivityEntry) Outcome() string {
	if e.Change != nil {
		return "saved"
	}
	if e.Operation.Status == "submitted" || e.Operation.Status == "" {
		return "unknown"
	}
	return ui.OneLine(e.Operation.Status)
}

func (e ActivityEntry) Metadata() string {
	if e.Change != nil {
		return fmt.Sprintf("Workspace · r%d", e.Change.Revision)
	}
	category := "Plous action"
	if e.Category() == HelmHistory {
		category = "Observed Helm"
	}
	parts := []string{category}
	if e.Category() == HelmHistory && !e.SourceTime {
		parts = append(parts, "observation time")
	}
	parts = append(parts, e.Operation.Context, e.Operation.Namespace)
	if len(e.Observations) > 1 {
		parts = append(parts, fmt.Sprintf("%d observations", len(e.Observations)))
	}
	return ui.OneLine(strings.Join(parts, " · "))
}

func (e ActivityEntry) SearchText() string {
	text := e.Label() + " " + e.Metadata() + " " + e.Outcome() + " " + e.At.Local().Format("2006-01-02 Jan 02 15:04")
	if e.Change != nil {
		text += " " + e.Change.After.Name
		for _, c := range ChangedConnections(*e.Change) {
			text += " " + c.Name
		}
	}
	for _, op := range e.Observations {
		text += " " + op.Status
	}
	return text
}

func (e ActivityEntry) Preview() string {
	if e.Change != nil {
		return ChangeDetail(*e.Change)
	}
	op := e.Operation
	lines := []string{e.Label(), "", "Type: " + e.Category().String(), "Outcome: " + e.Outcome(), "Context: " + op.Context, "Namespace: " + op.Namespace}
	if e.Category() == HelmHistory {
		if e.SourceTime {
			lines = append(lines, "Source time: "+e.At.Local().Format(time.RFC3339))
		} else {
			lines = append(lines, "Source time unavailable; ordered by observation time.")
		}
		lines = append(lines, "Last observed: "+op.ObservedAt.Local().Format(time.RFC3339), "Chart: "+op.Chart+" "+op.Version,
			"", "Retained Helm history. Actor unknown; this is not a complete audit.", "Recorded status does not establish live workload readiness.")
	} else {
		lines = append(lines, "Recorded: "+op.ObservedAt.Local().Format(time.RFC3339), "", "Initiated by the user in Plous.", "A completed Helm operation does not establish workload readiness.")
		if e.Outcome() == "unknown" {
			lines = append(lines, "Outcome unknown: no final result was recorded. Inspect the release before retrying.")
		}
	}
	return ui.Plain(strings.Join(lines, "\n"))
}

func (e ActivityEntry) Detail(observations bool) string {
	text := e.Preview()
	if e.Operation == nil {
		return text
	}
	text += "\n\n" + ui.Plain("Saved connection ID: "+e.Operation.ConnectionID)
	if len(e.Observations) > 0 {
		noun := "observations"
		if len(e.Observations) == 1 {
			noun = "observation"
		}
		summary := fmt.Sprintf("%d saved %s", len(e.Observations), noun)
		if observations {
			header := e.Label() + "\n" + summary
			for _, op := range e.Observations {
				header += "\n\n" + ui.Plain(op.Status+" · "+op.ObservedAt.Local().Format(time.RFC3339)+"\nSource timestamp: "+op.SourceTime+"\nRecord: "+op.ID)
			}
			return header + "\n\n" + text
		}
		text += "\n\n" + summary
	}
	return text
}
