package monitor

import (
	"cmp"
	"sort"
	"strconv"
	"strings"
	"time"

	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Collections and metrics are replaced on receipt. Age and metric freshness are
// time dependent, so a prepared inventory lives for at most one second.
type ExploreInventoryKey struct {
	First                         *unstructured.Unstructured
	Count                         int
	Version, Resource, ApiVersion string
	Namespaced                    bool
	Preference                    ExplorePreference
	Usage                         *ExploreUsage
}

type ExploreInventory struct {
	Key             ExploreInventoryKey
	At              time.Time
	Columns         []string
	Rows            []kubernetes.Row
	Width           int
	Indices, Widths []int
	Compact         bool
	Summary         string
}

func (e *ExploreState) TableColumns(width int) ([]int, []int, bool) {
	c := e.Inventory
	if c.Width != width || c.Indices == nil {
		var indices []int
		for i, column := range c.Columns {
			if column != "Namespace" || e.Preference.Namespace == "" {
				indices = append(indices, i)
			}
		}
		c.Indices, c.Widths, c.Compact = ResourceTableColumns(c.Columns, c.Rows, indices, width, e.Resource.Key() == "pods")
		c.Width = width
	}
	return c.Indices, c.Widths, c.Compact
}

func (e *ExploreState) PrepareInventory(now time.Time) *ExploreInventory {
	var first *unstructured.Unstructured
	if len(e.Collection.Objects) > 0 {
		first = &e.Collection.Objects[0]
	}
	key := ExploreInventoryKey{First: first, Count: len(e.Collection.Objects), Version: e.Collection.Version, Resource: e.Resource.Key(), ApiVersion: e.Resource.Version, Namespaced: e.Resource.Namespaced, Preference: e.Preference, Usage: e.Usage}
	if e.Inventory != nil && e.Inventory.Key == key && now.Sub(e.Inventory.At) < time.Second {
		return e.Inventory
	}
	collection := e.Collection
	if e.Usage != nil {
		collection.Usage = e.Usage.Samples
	}
	columns, all := kubernetes.Table(e.Resource, collection, now)
	rows := make([]kubernetes.Row, 0, len(all))
	filter := strings.ToLower(e.Preference.Filter)
	for _, row := range all {
		if filter == "" || strings.Contains(strings.ToLower(strings.Join(row.Cells, " ")), filter) {
			rows = append(rows, row)
		}
	}
	column := max(0, min(e.Preference.Sort, len(columns)-1))
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i].Cells[column], rows[j].Cells[column]
		order := strings.Compare(a, b)
		if columns[column] == "Age" {
			order = rows[j].Object.GetCreationTimestamp().Time.Compare(rows[i].Object.GetCreationTimestamp().Time)
		} else if columns[column] != "Name" && columns[column] != "Namespace" {
			av, aErr := strconv.ParseFloat(a, 64)
			bv, bErr := strconv.ParseFloat(b, 64)
			aKnown, bKnown := aErr == nil, bErr == nil
			if value, known := rows[i].Numbers[column]; known {
				av, aKnown = value, true
			}
			if value, known := rows[j].Numbers[column]; known {
				bv, bKnown = value, true
			}

			switch {
			case aKnown && bKnown:
				order = cmp.Compare(av, bv)
			case aKnown:
				order = -1
			case bKnown:
				order = 1
			}
		}
		if order == 0 {
			order = strings.Compare(rows[i].Object.GetNamespace()+"/"+rows[i].Object.GetName(), rows[j].Object.GetNamespace()+"/"+rows[j].Object.GetName())
		}
		if e.Preference.Descending {
			return order > 0
		}
		return order < 0
	})
	e.Inventory = &ExploreInventory{Key: key, At: now, Columns: columns, Rows: rows}
	return e.Inventory
}
