package monitor

import (
	"sort"
	"strconv"
	"strings"

	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
	"github.com/simjay/plous/internal/ui"
)

// A list result replaces the release slice, including on an explicit refresh.
type HelmInventory struct {
	First        *helmtool.Release
	Count        int
	Preference   HelmPreference
	Rows         []helmtool.Release
	Groups       []HelmChartGroup
	HeadingWidth int
	Headings     [][]string
}

type HelmResourceCache struct {
	Snapshot  *helmtool.Snapshot
	Resources []HelmInspectorResource
	Err       error
}

// Retained snapshots are immutable within the inspector. Refresh replaces the
// snapshot even when Helm still reports the same revision.
func (h *HelmState) InspectorResources() ([]HelmInspectorResource, error) {
	if h.Detail == nil {
		h.Resources = nil
		return nil, nil
	}
	if h.Resources == nil || h.Resources.Snapshot != h.Detail {
		resources, err := HelmResources(h.Detail)
		h.Resources = &HelmResourceCache{Snapshot: h.Detail, Resources: resources, Err: err}
	}
	return h.Resources.Resources, h.Resources.Err
}

func (h *HelmState) ReleaseRows() []helmtool.Release {
	key := h.Preference
	key.Selected = ""
	var first *helmtool.Release
	if len(h.Releases) > 0 {
		first = &h.Releases[0]
	}
	if c := h.Inventory; c != nil && c.First == first && c.Count == len(h.Releases) && c.Preference == key {
		return c.Rows
	}
	var out []helmtool.Release
	for _, r := range h.Releases {
		if ui.SearchContains(strings.Join([]string{r.Key(), r.ChartKey(), r.Status, strconv.Itoa(r.Revision), r.AppVersion, r.Updated}, " "), h.Preference.Filter) {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		left, right := out[i], out[j]
		if h.Preference.Descending {
			left, right = right, left
		}
		if h.Preference.Sort == "Revision" {
			return left.Revision < right.Revision
		}
		value := func(r helmtool.Release) string {
			switch h.Preference.Sort {
			case "Namespace":
				return r.Namespace
			case "Status":
				return r.Status
			case "Chart":
				return r.Chart
			case "Version":
				return r.Version
			case "App version":
				return r.AppVersion
			case "Updated":
				return r.Updated
			default:
				return r.Name
			}
		}
		if value(left) == value(right) {
			return left.Key() < right.Key()
		}
		return value(left) < value(right)
	})
	if h.Preference.Section == "Charts" {
		sort.SliceStable(out, func(i, j int) bool {
			if h.Preference.Sort == "Chart" && h.Preference.Descending {
				return HelmChartName(out[i]) > HelmChartName(out[j])
			}
			return HelmChartName(out[i]) < HelmChartName(out[j])
		})
	}
	h.Inventory = &HelmInventory{First: first, Count: len(h.Releases), Preference: key, Rows: out}
	return out
}

func (h *HelmState) SortColumn(columns []string) int {
	for i, column := range columns {
		if column == h.Preference.Sort {
			return i
		}
	}
	return 0
}

func (h *HelmState) LessCells(columns, left, right []string) bool {
	if h.Preference.Descending {
		left, right = right, left
	}
	column := h.SortColumn(columns)
	if columns[column] == "Releases" || columns[column] == "Namespaces" {
		a, _ := strconv.Atoi(left[column])
		b, _ := strconv.Atoi(right[column])
		if a != b {
			return a < b
		}
	}
	if left[column] == right[column] {
		if left[0] == right[0] {
			return strings.Join(left, "\x00") < strings.Join(right, "\x00")
		}
		return left[0] < right[0]
	}
	return left[column] < right[column]
}
