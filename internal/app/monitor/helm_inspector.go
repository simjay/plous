package monitor

import (
	"fmt"
	"sort"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	manifestutil "github.com/simjay/plous/internal/integrations/manifest"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
)

// Document indices also identify values and resource actions used by the editor.
var HelmInspectorTabs = []int{5, 2, 1}

var HelmResourceGroups = []string{"Workloads", "Networking", "Configuration", "Storage", "Scaling & availability", "Access", "Other resources", "Hooks"}

type HelmInspectorResource struct {
	manifestutil.Resource
	Group string
}

func HelmResources(s *helmtool.Snapshot) ([]HelmInspectorResource, error) {
	resources, err := manifestutil.Resources(s.Manifest)
	if err != nil {
		return nil, err
	}
	var out []HelmInspectorResource
	for _, r := range resources {
		group := ResourceGroup(r)
		if HelmtoolHook(r) != "" {
			group = "Hooks"
		}
		out = append(out, HelmInspectorResource{r, group})
	}
	for _, hook := range s.Hooks {
		rs, e := manifestutil.Resources(hook.Manifest)
		if e != nil {
			return nil, e
		}
		for _, r := range rs {
			out = append(out, HelmInspectorResource{r, "Hooks"})
		}
	}
	rank := func(group string) int {
		for i, g := range HelmResourceGroups {
			if g == group {
				return i
			}
		}
		return len(HelmResourceGroups)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Group != out[j].Group {
			return rank(out[i].Group) < rank(out[j].Group)
		}
		return out[i].Kind+"/"+out[i].Name < out[j].Kind+"/"+out[j].Name
	})
	return out, nil
}

func (h *HelmState) InspectorReleases() []helmtool.Release {
	if h.Detail == nil {
		return nil
	}
	current := h.Detail.Release()
	var out []helmtool.Release
	found := false
	for _, r := range h.Releases {
		// The selected historical release remains available even if its latest chart changed.
		if r.Key() == current.Key() || r.Chart == current.Chart && r.Error == "" {
			out = append(out, r)
			if r.Key() == current.Key() {
				found = true
			}
		}
	}
	if !found {
		out = append(out, current)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key() < out[j].Key() })
	return out
}

func (h *HelmState) InspectorRevisions() []helmtool.Revision {
	out := append([]helmtool.Revision(nil), h.Detail.History...)
	sort.Slice(out, func(i, j int) bool { return out[i].Revision > out[j].Revision })
	return out
}

func (h *HelmState) OpenInspectorPicker(kind string) {
	if h.Detail == nil || h.Busy {
		return
	}
	h.MenuKind, h.Choice = kind, 0
	input := textview.NewField("")
	h.Input, h.InputKind = &input, "picker"
	h.Menu = []string{}
	if kind == "release" {
		for i, r := range h.InspectorReleases() {
			h.Menu = append(h.Menu, fmt.Sprintf("%s · %s · Revision %d", r.Key(), HelmChartVersion(r.Version), r.Revision))
			if r.Key() == h.Detail.Release().Key() {
				h.Choice = i
				h.Menu[i] += "  ✓"
			}
		}
	} else {
		for i, r := range h.InspectorRevisions() {
			h.Menu = append(h.Menu, fmt.Sprintf("Revision %d · %s · %s", r.Revision, r.Status, r.Updated))
			if r.Revision == h.Detail.Version {
				h.Choice = i
				h.Menu[i] += "  ✓"
			}
		}
	}
}

func (h *HelmState) LatestRevision() int {
	latest := 0
	if h.Timeline != nil && h.Detail == nil {
		return h.Timeline.Release.Revision
	}
	if h.Detail != nil {
		latest = h.Detail.Version
		for _, r := range h.Detail.History {
			latest = max(latest, r.Revision)
		}
	}
	return latest
}

func HelmResourceInventory(h *HelmState, width, height int) string {
	rs, err := h.InspectorResources()
	if err != nil {
		return ui.TextView(err.Error(), width, height, 0)
	}
	if len(rs) == 0 {
		return ui.Muted.Render("No recorded resources or hooks.")
	}
	h.Subcursor = min(max(0, h.Subcursor), len(rs)-1)
	listWidth := width
	if width >= 90 {
		listWidth = width/2 - 2
	}
	var entries []int
	selectedLine := 0
	group := ""
	for i, r := range rs {
		if r.Group != group {
			if group != "" {
				entries = append(entries, len(rs))
			}
			group = r.Group
			entries = append(entries, -i-1)
		}
		if i == h.Subcursor {
			selectedLine = len(entries)
		}
		entries = append(entries, i)
	}
	listView := func(available int, keepHeading bool) string {
		start, end := ui.SelectionWindow(len(entries), selectedLine, available)
		if keepHeading && selectedLine < available {
			start, end = 0, min(len(entries), available)
		}
		var lines []string
		for _, entry := range entries[start:end] {
			if entry == len(rs) {
				lines = append(lines, "")
				continue
			}
			if entry < 0 {
				lines = append(lines, SummaryHeading(rs[-entry-1].Group, listWidth))
				continue
			}
			r := rs[entry]
			prefix := "  "
			if entry == h.Subcursor {
				prefix = "› "
			}
			label := ui.Clipped(prefix+ui.OneLine(r.Kind)+" / "+ui.OneLine(r.Name), listWidth)
			if entry == h.Subcursor {
				label = ui.Selected.Background(ui.Highlight).Width(listWidth).Render(label)
			}
			lines = append(lines, ui.MouseTarget(label, "helmresource/"+r.Kind+"/"+r.Namespace+"/"+r.Name))
		}
		return strings.Join(lines, "\n")
	}
	chosen := rs[h.Subcursor]
	summary := HelmResourceSummary(chosen, h.Detail.Namespace, max(1, width-listWidth-3))
	if width >= 90 {
		list := listView(height, false)
		return lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(listWidth+3).Render(list), ui.StyledTextView(summary, width-listWidth-3, height, 0))
	}
	if height < 9 {
		return listView(height, false)
	}
	listHeight := max(3, height/3)
	list := listView(listHeight, true)
	summary = HelmResourceSummary(chosen, h.Detail.Namespace, width)
	return list + "\n\n" + ui.StyledTextView(summary, width, max(1, height-lipgloss.Height(list)-2), 0)
}

func HelmResourceSummary(r HelmInspectorResource, namespace string, width int) string {
	ns := r.Namespace
	if r.ClusterScoped() {
		ns = "cluster-scoped"
	} else if ns == "" {
		ns = "omitted · release " + namespace
	}
	lines := []string{SummaryHeading(ui.OneLine(r.Kind)+" / "+ui.OneLine(r.Name), width), ui.Muted.Render("Recorded configuration"), ""}
	fields := []kubernetes.DetailField{{Label: "Namespace", Value: ns}}
	if r.Source != "" {
		fields = append(fields, kubernetes.DetailField{Label: "Template", Value: r.Source})
	}
	lines = append(lines, RenderSummaryFields(fields, width)...)
	if facts := ResourceFacts(r.Resource, width); len(facts) > 0 {
		lines = append(lines, "")
		lines = append(lines, facts...)
	}
	return strings.Join(lines, "\n")
}

func (h *HelmState) MoveInspectorTab(delta int) {
	index := 0
	for i, tab := range HelmInspectorTabs {
		if h.Tab == tab {
			index = i
			break
		}
	}
	h.SaveInspectorPosition()
	h.Tab = HelmInspectorTabs[(index+delta+len(HelmInspectorTabs))%len(HelmInspectorTabs)]
	h.Subcursor = h.InspectorPositions[h.InspectorPositionKey()]
	h.Find = nil
}

func (h *HelmState) InspectorPositionKey() string {
	variant := 0
	if h.Tab == 2 {
		variant = h.ValuesVariant
	}
	if h.Tab == 1 {
		variant = h.ChartDocument
	}
	return fmt.Sprintf("%d/%d", h.Tab, variant)
}

func (h *HelmState) SaveInspectorPosition() {
	if h.Find != nil && h.Tab != 5 {
		h.Subcursor = h.Find.Reader.Top
	}
	if h.InspectorPositions == nil {
		h.InspectorPositions = map[string]int{}
	}
	h.InspectorPositions[h.InspectorPositionKey()] = h.Subcursor
}

func (h *HelmState) ApplyInspectorSnapshot(s *helmtool.Snapshot) {
	same := h.Detail != nil && h.Detail.Name == s.Name && h.Detail.Namespace == s.Namespace && h.Detail.Version == s.Version
	if !same {
		h.Detail, h.Tab, h.Subcursor = s, 5, 0
		h.InspectorPositions, h.Find = nil, nil
		h.ValuesVariant, h.ChartDocument = 0, 0
		return
	}
	h.SaveInspectorPosition()
	shadow := *h
	shadow.Tab = 5
	index := h.InspectorPositions[shadow.InspectorPositionKey()]
	previous, _ := h.InspectorResources()
	key := ""
	if index < len(previous) {
		key = previous[index].Group + previous[index].Key()
	}
	h.Detail = s
	current, _ := h.InspectorResources()
	index = min(index, max(0, len(current)-1))
	for i, resource := range current {
		if resource.Group+resource.Key() == key {
			index = i
			break
		}
	}
	h.InspectorPositions[shadow.InspectorPositionKey()] = index
	h.Subcursor = h.InspectorPositions[h.InspectorPositionKey()]
	if h.Find != nil {
		h.RefreshFind()
	}
}

func (h *HelmState) MenuChoices() ([]string, []int) {
	var labels []string
	var indices []int
	for i, label := range h.Menu {
		if h.InputKind == "picker" && h.Input != nil && !ui.SearchContains(label, h.Input.Text) {
			continue
		}
		labels, indices = append(labels, label), append(indices, i)
	}
	return labels, indices
}

func (h *HelmState) ChooseInspectorDocument(kind string, index int) {
	h.SaveInspectorPosition()
	if kind == "values" {
		h.ValuesVariant = index
	} else {
		h.ChartDocument = index
	}
	h.Subcursor = h.InspectorPositions[h.InspectorPositionKey()]
	h.Find = nil
}

var HelmValueLabels = []string{"Computed values", "Supplied values", "Chart defaults"}

var HelmChartDocuments = []string{"Metadata and schema", "Rendered manifests", "Hooks", "Notes", "Release metadata", "Retained chart files"}

func (h *HelmState) OpenInspectorDocuments() {
	h.Choice, h.MenuKind = h.ValuesVariant, "values"
	h.Menu = append([]string(nil), HelmValueLabels...)
	if h.Tab == 1 {
		h.Choice, h.MenuKind = h.ChartDocument, "chart-document"
		h.Menu = append([]string(nil), HelmChartDocuments...)
	}
	h.Menu[h.Choice] += "  ✓"
}

func HelmtoolHook(r manifestutil.Resource) string {
	meta, _ := r.Object["metadata"].(map[string]any)
	annotations, _ := meta["annotations"].(map[string]any)
	hook, _ := annotations["helm.sh/hook"].(string)
	return hook
}
