package monitor

import (
	"cmp"
	"slices"
	"time"

	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
)

type Alert struct {
	ID, Kind, Name, UID, Signal, Container, State, Detail string
	FirstSeen, LastSeen, WorsenedAt, ClosedAt             time.Time
	PresentAtStart, ZeroAvailable                         bool
}

type State struct {
	Name, Context, Namespace, Fingerprint string
	Targets                               []kubernetes.CheckTarget
	Interval                              time.Duration
	Running                               bool
	StartedAt, LastCheck, LastOK          time.Time
	NextCheck                             time.Time `json:",omitzero"`
	Coverage                              map[string]string
	Alerts                                []Alert `json:"Incidents"` // Preserve the key in saved monitor records.
}

func (state State) Overdue(now time.Time) bool {
	return state.Running && !state.NextCheck.IsZero() && !now.Before(state.NextCheck.Add(checkTimeout))
}

func (state State) OpenAlerts() int {
	open := 0
	for _, item := range state.Alerts {
		if item.State == "open" {
			open++
		}
	}
	return open
}

type alertKey struct {
	uid, signal, container string
}

type alertRules struct {
	pending map[alertKey]streak
}

type streak struct {
	first   time.Time
	atStart bool
	count   int
	bad     bool
}

func (r *alertRules) apply(state *State, check kubernetes.CheckResult, now time.Time) []Alert {
	baseline := state.LastCheck.IsZero()
	state.LastCheck, state.Coverage = now, check.Coverage
	if len(check.Coverage) == 0 {
		state.LastOK = now
	}
	open := map[alertKey]int{}
	for i, item := range state.Alerts {
		if item.State == "open" {
			open[alertKey{item.UID, item.Signal, item.Container}] = i
		}
	}
	next := map[alertKey]streak{}
	present := map[string]kubernetes.ResourceCheck{}
	var changed []Alert
	for _, object := range check.Objects {
		present[object.UID] = object
		if object.Inactive != "" {
			for key, i := range open {
				if key.uid == object.UID {
					changed = append(changed, closeAlert(&state.Alerts[i], object.Inactive, now))
				}
			}
			continue
		}
		for _, signal := range object.Signals {
			key := alertKey{object.UID, signal.Rule.Name, signal.Container}
			s := r.pending[key]
			if s.count == 0 || s.bad != signal.Bad {
				s = streak{first: now, atStart: baseline, bad: signal.Bad}
			}
			s.count = min(s.count+1, signal.Rule.Checks)
			next[key] = s
			if i, exists := open[key]; exists {
				item := &state.Alerts[i]
				item.LastSeen = now
				switch {
				case !signal.Bad && s.count == signal.Rule.Checks:
					changed = append(changed, closeAlert(item, "recovered", now))
				case signal.Bad:
					worsened := signal.ZeroAvailable && !item.ZeroAvailable
					item.Detail, item.ZeroAvailable = signal.Detail, signal.ZeroAvailable
					if worsened {
						item.WorsenedAt = now
						changed = append(changed, *item)
					}
				}
				continue
			}
			if signal.Bad && s.count == signal.Rule.Checks {
				item := Alert{
					ID:   object.UID + "/" + signal.Rule.Name + "/" + signal.Container + "/" + now.UTC().Format(time.RFC3339Nano),
					Kind: object.Kind, Name: object.Name, UID: object.UID, Signal: signal.Rule.Name, Container: signal.Container,
					State: "open", Detail: signal.Detail, FirstSeen: s.first, LastSeen: now,
					PresentAtStart: s.atStart, ZeroAvailable: signal.ZeroAvailable,
				}
				state.Alerts = append(state.Alerts, item)
				changed = append(changed, item)
			}
		}
	}
	for key, i := range open {
		item := &state.Alerts[i]
		if object, found := present[key.uid]; found {
			_, observed := next[key]
			if !observed && slices.ContainsFunc(object.CompleteRules, func(rule kubernetes.AlertRule) bool { return rule.Name == key.signal }) {
				changed = append(changed, closeAlert(item, "removed", now))
			}
			continue
		}
		if check.Coverage[item.Kind] == "" && check.Coverage["target"] == "" {
			changed = append(changed, closeAlert(item, "removed", now))
		}
	}
	r.pending = next
	closed := 0
	for i := len(state.Alerts) - 1; i >= 0; i-- {
		if state.Alerts[i].State != "open" {
			closed++
			if closed > 200 {
				state.Alerts = slices.Delete(state.Alerts, i, i+1)
			}
		}
	}
	slices.SortFunc(changed, func(a, b Alert) int { return cmp.Compare(a.ID, b.ID) })
	return changed
}

func closeAlert(item *Alert, outcome string, now time.Time) Alert {
	item.State, item.ClosedAt, item.LastSeen = outcome, now, now
	return *item
}
