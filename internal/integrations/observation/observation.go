// Package observation defines the data contract between integrations and history.
// It contains no clients, storage, model calls or resource-specific calculations.
package observation

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const Version = 1

type Value struct {
	Text    *string `json:"text,omitempty"`
	Number  *int64  `json:"number,omitempty"`
	Boolean *bool   `json:"boolean,omitempty"`
	Unknown bool    `json:"unknown,omitempty"`
}

func Text(s string) Value  { return Value{Text: &s} }
func Number(n int64) Value { return Value{Number: &n} }
func Boolean(b bool) Value { return Value{Boolean: &b} }
func Unknown() Value       { return Value{Unknown: true} }
func (v Value) String() string {
	switch {
	case v.Unknown:
		return "unknown"
	case v.Text != nil:
		return *v.Text
	case v.Number != nil:
		return fmt.Sprint(*v.Number)
	case v.Boolean != nil:
		return fmt.Sprint(*v.Boolean)
	}
	return "unknown"
}

type Identity struct {
	API       string `json:"api,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
	UID       string `json:"uid,omitempty"`
	Revision  string `json:"revision,omitempty"`
}

func (i Identity) Key() string {
	if i.UID != "" {
		return i.API + "/" + i.UID
	}
	return i.API + "/" + i.Kind + "/" + i.Namespace + "/" + i.Name
}

type Row struct {
	Identity Identity         `json:"identity"`
	Facts    map[string]Value `json:"facts"`
}
type Coverage struct {
	Complete    bool     `json:"complete"`
	Observed    int      `json:"observed"`
	Total       *int     `json:"total,omitempty"`
	Snapshot    string   `json:"snapshot,omitempty"`
	Limitations []string `json:"limitations,omitempty"`
}
type Result struct {
	Version    int    `json:"version"`
	Schema     string `json:"schema"`
	Connection string `json:"connection_id,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	Operation  string `json:"operation"`
	// Selection identifies the population chosen by the integration, excluding
	// pagination and observation time. History compares it without domain rules.
	Selection  map[string]string `json:"selection,omitempty"`
	ObservedAt time.Time         `json:"observed_at"`
	Outcome    string            `json:"outcome"`
	Coverage   Coverage          `json:"coverage"`
	Facts      map[string]Value  `json:"facts"`
	Rows       []Row             `json:"rows,omitempty"`
}

func New(schema, operation string, now time.Time) Result {
	return Result{Version: Version, Schema: schema, Operation: operation, ObservedAt: now, Outcome: "completed", Facts: map[string]Value{}}
}
func (r *Result) Partial(reason string) {
	r.Coverage.Complete = false
	r.Coverage.Total = nil
	r.Outcome = "partial"
	r.Coverage.Limitations = append(r.Coverage.Limitations, reason)
}
func (r *Result) Complete() {
	r.Coverage.Complete = true
	n := r.Coverage.Observed
	r.Coverage.Total = &n
}
func (r Result) Validate() error {
	if r.Version != Version || r.Schema == "" || r.Operation == "" || r.ObservedAt.IsZero() {
		return errors.New("invalid projected observation identity")
	}
	switch r.Outcome {
	case "completed", "partial", "error", "denied", "unsupported":
	default:
		return errors.New("invalid observation outcome")
	}
	if r.Coverage.Observed < 0 || r.Coverage.Complete && (r.Coverage.Total == nil || *r.Coverage.Total != r.Coverage.Observed) || !r.Coverage.Complete && r.Coverage.Total != nil {
		return errors.New("inconsistent observation coverage")
	}
	check := func(facts map[string]Value) error {
		for k, v := range facts {
			n := 0
			if v.Unknown {
				n++
			}
			if v.Text != nil {
				n++
				if !utf8.ValidString(*v.Text) {
					return errors.New("invalid observation text")
				}
			}
			if v.Number != nil {
				n++
			}
			if v.Boolean != nil {
				n++
			}
			if k == "" || n != 1 {
				return errors.New("invalid typed observation field")
			}
		}
		return nil
	}
	if err := check(r.Facts); err != nil {
		return err
	}
	for _, row := range r.Rows {
		if err := check(row.Facts); err != nil {
			return err
		}
	}
	return nil
}
func (r Result) SearchText() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s %s %s\n", r.Schema, r.Operation, r.Namespace, r.Outcome)
	write := func(facts map[string]Value) {
		keys := make([]string, 0, len(facts))
		for k := range facts {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "%s: %s\n", k, facts[k].String())
		}
	}
	write(r.Facts)
	for _, row := range r.Rows {
		fmt.Fprintf(&b, "%s %s/%s %s\n", row.Identity.Kind, row.Identity.Namespace, row.Identity.Name, row.Identity.UID)
		write(row.Facts)
	}
	for _, s := range r.Coverage.Limitations {
		fmt.Fprintln(&b, s)
	}
	return b.String()
}
