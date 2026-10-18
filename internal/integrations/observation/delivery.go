package observation

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

// Selection changes delivery only. Nil Fields keeps all saved row facts; an
// empty selection returns identities. Aggregate facts always remain attached.
type Selection struct {
	Offset int      `json:"offset,omitempty"`
	Limit  int      `json:"limit,omitempty"`
	Fields []string `json:"fields"`
}

type Page struct {
	Offset     int      `json:"offset"`
	Returned   int      `json:"returned"`
	NextOffset *int     `json:"next_offset"`
	Fields     []string `json:"fields"`
}

// Delivery distinguishes the saved collection's coverage from the rows in this
// response. Ref always identifies the complete saved observation.
type Delivery struct {
	Ref         string  `json:"ref"`
	Observation *Result `json:"observation,omitempty"`
	Page        *Page   `json:"page,omitempty"`
}

func FieldsSchema() map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "uniqueItems": true, "description": "Saved row fact names to return. Use [] for identities only; omit for all facts. Identity, collection coverage and aggregate facts are always retained. Unknown field names receive feedback."}
}

// Deliver selects the largest ordered prefix that fits the complete delivery
// envelope. Limit is optional and never controls acquisition or collection totals.
func (r Result) Deliver(ref string, q Selection, maxBytes int) (Delivery, error) {
	if q.Offset < 0 || q.Offset > len(r.Rows) || q.Limit < 0 || maxBytes < 1 {
		return Delivery{}, errors.New("invalid observation page or delivery allowance")
	}
	available := map[string]bool{}
	for _, row := range r.Rows {
		for field := range row.Facts {
			available[field] = true
		}
	}
	seen := map[string]bool{}
	for _, field := range q.Fields {
		if field == "" || seen[field] {
			return Delivery{}, errors.New("fields must contain distinct nonempty names")
		}
		seen[field] = true
		if len(r.Rows) > 0 && !available[field] {
			names := make([]string, 0, len(available))
			for name := range available {
				names = append(names, name)
			}
			slices.Sort(names)
			return Delivery{}, fmt.Errorf("field %q is not recorded; available row fields: %v", field, names)
		}
	}
	end := len(r.Rows)
	if q.Limit > 0 && q.Limit < end-q.Offset {
		end = q.Offset + q.Limit
	}
	rows := r.Rows[q.Offset:end]
	if q.Fields != nil {
		rows = make([]Row, end-q.Offset)
		for i, row := range r.Rows[q.Offset:end] {
			facts := make(map[string]Value, len(q.Fields))
			for _, field := range q.Fields {
				v, ok := row.Facts[field]
				if !ok {
					v = Unknown()
				}
				facts[field] = v
			}
			rows[i] = Row{Identity: row.Identity, Facts: facts}
		}
	}
	length := len(r.Rows)
	page := func(n int) Delivery {
		part := r
		part.Rows = rows[:n]
		info := &Page{Offset: q.Offset, Returned: n, Fields: q.Fields}
		if next := q.Offset + n; next < length {
			info.NextOffset = &next
		}
		return Delivery{Ref: ref, Observation: &part, Page: info}
	}
	fits := func(n int) bool { b, err := json.Marshal(page(n)); return err == nil && len(b) <= maxBytes }
	if fits(len(rows)) {
		return page(len(rows)), nil
	}
	low, high := 0, len(rows)
	for low < high {
		mid := low + (high-low+1)/2
		if fits(mid) {
			low = mid
		} else {
			high = mid - 1
		}
	}
	if low == 0 {
		return Delivery{}, errors.New("observation fields exceed delivery allowance; select fewer row facts or a smaller text excerpt")
	}
	return page(low), nil
}
