package monitor

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func overviewChangeKey(row OverviewRow) string {
	t := row.Target
	return strings.Join([]string{row.Source, t.Resource.Key(), t.Ref.Key(), t.Namespace, t.Name, t.UID, row.Action, row.Revision, row.Status, row.At.UTC().Format(time.RFC3339Nano)}, "/")
}

// Retain reported transitions, including through denied reads and object deletion.
func (s *OverviewState) retainChanges(observed time.Time) {
	current := map[string]bool{}
	seen := map[string]bool{}
	for _, row := range s.Changes {
		seen[row.ID] = true
	}
	for _, row := range s.Rows[1] {
		if row.At.IsZero() {
			continue
		}
		row.ID = overviewChangeKey(row)
		current[row.ID] = true
		if !seen[row.ID] {
			row.ObservedAt = observed
			s.Changes = append(s.Changes, row)
			seen[row.ID] = true
		}
	}
	sort.SliceStable(s.Changes, func(i, j int) bool {
		if s.Changes[i].At.Equal(s.Changes[j].At) {
			return s.Changes[i].ID < s.Changes[j].ID
		}
		return s.Changes[i].At.After(s.Changes[j].At)
	})
	if len(s.Changes) > 200 {
		s.Changes = s.Changes[:200]
	}
	s.Rows[1] = make([]OverviewRow, len(s.Changes))
	for i, row := range s.Changes {
		row.Historical = !current[row.ID]
		s.Rows[1][i] = row
	}
}

func overviewChangeEvidence(row OverviewRow) string {
	return fmt.Sprintf("Recorded Overview change\n\n%s\nSource: %s\nNamespace: %s\nAction: %s\nRevision: %s\nOutcome: %s\nSource time: %s\nFirst observed: %s\n\n%s\n\nThis entry retains a previously reported change. It does not describe the current resource state.", row.Title, row.Source, row.Target.Namespace, row.Action, row.Revision, row.Status, overviewEvidenceTime(row.At), overviewEvidenceTime(row.ObservedAt), row.Detail)
}
