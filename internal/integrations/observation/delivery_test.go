package observation

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDeliveryFitsCompleteEnvelopeAndPreservesCollection(t *testing.T) {
	for _, schema := range []string{"kubernetes/pods", "helm.release.v1"} {
		t.Run(schema, func(t *testing.T) {
			r := New(schema, "list", time.Now().UTC())
			for i := range 109 {
				r.Rows = append(r.Rows, Row{Identity: Identity{Name: fmt.Sprintf("subject-%d", i)}, Facts: map[string]Value{"status": Text(strings.Repeat("x", 200))}})
			}
			r.Coverage.Observed = len(r.Rows)
			r.Complete()
			r.Facts["total"] = Number(109)
			ref := strings.Repeat("reference", 30)
			full, err := r.Deliver(ref, Selection{}, 65536)
			if err != nil || full.Page.Returned != 109 || full.Page.NextOffset != nil {
				t.Fatal(full.Page, err)
			}
			encoded, _ := json.Marshal(full)
			for _, allowance := range []int{len(encoded), len(encoded) - 1, 3000} {
				var received []Row
				offset := 0
				for {
					page, err := r.Deliver(ref, Selection{Offset: offset}, allowance)
					if err != nil {
						t.Fatal(err)
					}
					wire, _ := json.Marshal(page)
					if len(wire) > allowance || page.Page.Returned != len(page.Observation.Rows) || page.Page.Offset != offset || !page.Observation.Coverage.Complete || *page.Observation.Coverage.Total != 109 {
						t.Fatal("incorrect page or collection coverage", string(wire))
					}
					received = append(received, page.Observation.Rows...)
					if page.Page.NextOffset == nil {
						break
					}
					if *page.Page.NextOffset <= offset {
						t.Fatal("page made no progress")
					}
					offset = *page.Page.NextOffset
				}
				if !reflect.DeepEqual(received, r.Rows) {
					t.Fatal("paging changed saved identities or values")
				}
			}
		})
	}
}

func TestDeliveryFieldSelectionDoesNotChangeSavedFacts(t *testing.T) {
	r := New("fixture", "get", time.Now().UTC())
	r.Rows = []Row{{Identity: Identity{Name: "one"}, Facts: map[string]Value{"ready": Boolean(false), "large": Text(strings.Repeat("x", 12000))}}, {Identity: Identity{Name: "two"}, Facts: map[string]Value{"large": Text("other")}}}
	r.Coverage.Observed = 2
	r.Partial("API read stopped")
	before, _ := json.Marshal(r)
	for _, fields := range [][]string{{}, {"ready"}} {
		page, err := r.Deliver("ref", Selection{Fields: fields}, 1500)
		if err != nil || page.Page.Returned != 2 || page.Page.NextOffset != nil || page.Observation.Coverage.Complete || page.Observation.Coverage.Total != nil {
			t.Fatal(page, err)
		}
		if len(fields) > 0 && !page.Observation.Rows[1].Facts["ready"].Unknown {
			t.Fatal("missing retained field was inferred")
		}
		for _, row := range page.Observation.Rows {
			if _, ok := row.Facts["large"]; ok {
				t.Fatal("unrequested field delivered")
			}
		}
	}
	for _, fields := range [][]string{{"missing"}, {"ready", "ready"}, {""}} {
		if _, err := r.Deliver("ref", Selection{Fields: fields}, 65536); err == nil {
			t.Fatal("invalid field selection accepted", fields)
		}
	}
	after, _ := json.Marshal(r)
	if string(before) != string(after) {
		t.Fatal("delivery changed canonical evidence")
	}
}
