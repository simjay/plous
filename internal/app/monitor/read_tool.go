package monitor

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/simjay/plous/internal/integrations/observation"
)

type ReadTool struct {
	Root string
}

func (ReadTool) Description() string {
	return "Read Plous monitor status or saved alerts for the captured Kubernetes connection using status, list, or get. Pass monitor_name to narrow to one monitor."
}

var readOperations = []observation.OperationVariant{
	{Operation: "status", Arguments: []string{"monitor_name"}},
	{Operation: "list", Arguments: []string{"monitor_name"}},
	{Operation: "get", Arguments: []string{"alert_id", "monitor_name"}, Schema: map[string]any{"required": []string{"alert_id"}}},
}

func (ReadTool) Schema() map[string]any {
	return observation.OperationSchema(map[string]any{
		"monitor_name": map[string]any{"type": "string", "minLength": 1, "maxLength": 63, "pattern": "^[a-z0-9]([a-z0-9-]*[a-z0-9])?$"},
		"alert_id":     map[string]any{"type": "string", "minLength": 1, "maxLength": 1024},
	}, readOperations)
}

func (reader ReadTool) Read(fingerprint, connectionID string, fields map[string]json.RawMessage) (observation.Result, error) {
	var op, id string
	if json.Unmarshal(fields["operation"], &op) != nil {
		return observation.Result{}, errors.New("expected status, list, or get")
	}
	index := slices.IndexFunc(readOperations, func(v observation.OperationVariant) bool { return v.Operation == op })
	if index < 0 {
		return observation.Result{}, errors.New("expected status, list, or get")
	}
	for name := range fields {
		if name != "operation" && !slices.Contains(readOperations[index].Arguments, name) {
			return observation.Result{}, errors.New("unsupported argument for monitor " + op)
		}
	}
	if raw, found := fields["alert_id"]; found {
		if json.Unmarshal(raw, &id) != nil || len(id) == 0 || len(id) > 1024 {
			return observation.Result{}, errors.New("alert_id must be a valid ID")
		}
	}
	if op == "get" && id == "" {
		return observation.Result{}, errors.New("get requires alert_id")
	}
	var states []State
	var incomplete bool
	if raw, found := fields["monitor_name"]; found {
		var name string
		if json.Unmarshal(raw, &name) != nil || validateName(name) != nil {
			return observation.Result{}, errors.New("monitor_name must be a valid monitor name")
		}
		state, err := status(reader.Root, name)
		if err == nil {
			states = []State{state}
		} else if !errors.Is(err, os.ErrNotExist) {
			return observation.Result{}, errors.New("could not read monitor records")
		}
	} else {
		var err error
		states, err = List(reader.Root)
		incomplete = err != nil
	}
	states = slices.DeleteFunc(states, func(state State) bool { return state.Fingerprint != fingerprint })
	r := observation.New("monitor_read.v1", op, time.Now().UTC())
	r.Connection = connectionID
	if incomplete {
		r.Partial("Some saved monitor records could not be read.")
	}
	if len(states) == 0 {
		r.Facts["status"] = observation.Text("no saved monitor in the captured scope")
		if !incomplete {
			r.Complete()
		}
		return r, nil
	}
	if len(states) == 1 {
		r.Namespace = states[0].Namespace
		r.Facts = monitorFacts(states[0], r.ObservedAt)
	}
	for _, state := range states {
		for _, key := range slices.Sorted(maps.Keys(state.Coverage)) {
			r.Partial(state.Name + " / " + key + ": " + state.Coverage[key])
		}
	}
	if op == "status" {
		for _, state := range states {
			r.Rows = append(r.Rows, observation.Row{
				Identity: observation.Identity{API: "plous.monitor", Namespace: state.Namespace, Kind: "Monitor", Name: state.Name},
				Facts:    monitorFacts(state, r.ObservedAt),
			})
		}
	} else {
		type savedAlert struct {
			state *State
			item  Alert
		}
		var alerts []savedAlert
		for i := range states {
			state := &states[i]
			for _, item := range state.Alerts {
				if op != "get" || item.ID == id {
					alerts = append(alerts, savedAlert{state, item})
				}
			}
		}
		slices.SortFunc(alerts, func(a, b savedAlert) int {
			if order := b.item.FirstSeen.Compare(a.item.FirstSeen); order != 0 {
				return order
			}
			return strings.Compare(a.item.ID, b.item.ID)
		})
		if len(alerts) > 100 {
			r.Partial("Only the 100 most recent matching alerts are included.")
			alerts = alerts[:100]
		}
		for _, item := range alerts {
			r.Rows = append(r.Rows, alertRow(*item.state, item.item))
		}
		if op == "get" && len(r.Rows) == 0 {
			r.Facts["status"] = observation.Text("alert was not found in the captured scope")
		}
	}
	r.Coverage.Observed = len(r.Rows)
	if r.Outcome == "completed" {
		r.Complete()
	}
	return r, nil
}

func monitorFacts(state State, now time.Time) map[string]observation.Value {
	facts := map[string]observation.Value{
		"monitor_name": observation.Text(state.Name),
		"status":       observation.Text(StatusLabel(state)),
		"running":      observation.Boolean(state.Running),
		"overdue":      observation.Boolean(state.Overdue(now)),
		"open_alerts":  observation.Number(int64(state.OpenAlerts())),
	}
	if !state.LastCheck.IsZero() {
		facts["last_check"] = observation.Text(state.LastCheck.Format(time.RFC3339))
	}
	if !state.LastOK.IsZero() {
		facts["last_complete_check"] = observation.Text(state.LastOK.Format(time.RFC3339))
	}
	if state.Running && !state.NextCheck.IsZero() {
		facts["next_check"] = observation.Text(state.NextCheck.Format(time.RFC3339))
	}
	return facts
}

func alertRow(state State, item Alert) observation.Row {
	row := observation.Row{
		Identity: observation.Identity{API: "plous.monitor", Namespace: state.Namespace, Kind: item.Kind, Name: item.Name, UID: item.ID},
		Facts: map[string]observation.Value{
			"monitor_name": observation.Text(state.Name), "alert_id": observation.Text(item.ID), "workload_uid": observation.Text(item.UID),
			"state": observation.Text(item.State), "signal": observation.Text(item.Signal),
			"container": observation.Text(item.Container), "detail": observation.Text(item.Detail),
			"first_seen":       observation.Text(item.FirstSeen.Format(time.RFC3339)),
			"last_seen":        observation.Text(item.LastSeen.Format(time.RFC3339)),
			"present_at_start": observation.Boolean(item.PresentAtStart),
		},
	}
	if !item.WorsenedAt.IsZero() {
		row.Facts["last_worsened"] = observation.Text(item.WorsenedAt.Format(time.RFC3339))
	}
	if !item.ClosedAt.IsZero() {
		row.Facts["closed_at"] = observation.Text(item.ClosedAt.Format(time.RFC3339))
	}
	return row
}
