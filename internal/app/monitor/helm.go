package monitor

import (
	"context"

	appsettings "github.com/simjay/plous/internal/app/settings"
	helmcommon "github.com/simjay/plous/internal/integrations/helm/common"
	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
	diffview "github.com/simjay/plous/internal/ui/diff"
	docedit "github.com/simjay/plous/internal/ui/editor"
	textview "github.com/simjay/plous/internal/ui/text"
	"github.com/simjay/plous/internal/workspace"
)

var HelmSections = []string{"Releases", "Charts"}

var HelmDetailSections = map[int]string{0: "Summary", 1: "Chart", 2: "Values", 3: "Manifests", 4: "Hooks", 5: "Resources", 7: "Notes"}

type HelmPreference struct {
	Section    string `json:"section"`
	Namespace  string `json:"namespace"`
	Filter     string `json:"filter,omitempty"`
	Selected   string `json:"selected,omitempty"`
	Sort       string `json:"sort,omitempty"`
	Descending bool   `json:"descending,omitempty"`
}

type HelmDraft struct {
	Form     *appsettings.Form
	Values   string
	Artifact *helmcommon.Artifact
	Baseline *helmtool.Snapshot
}

type HelmWorkflow struct {
	Active                       bool
	Preference                   HelmPreference
	Detail                       *helmtool.Snapshot
	Timeline                     *HelmTimeline
	Artifact                     *helmcommon.Artifact
	Cursor, Tab, Subcursor       int
	ValuesVariant, ChartDocument int
	InspectorPositions           map[string]int
}

type HelmState struct {
	Temporary                  bool
	Key, Workspace, Connection string
	Config                     helmtool.Config
	Client                     *helmtool.Client
	Preference                 HelmPreference
	Releases                   []helmtool.Release
	Inventory                  *HelmInventory
	Resources                  *HelmResourceCache
	Loaded                     bool
	Cursor, Tab, Subcursor     int
	Positions                  map[string]int
	InspectorPositions         map[string]int
	ValuesVariant              int
	ChartDocument              int
	Find                       *HelmFind
	Detail                     *helmtool.Snapshot
	Timeline                   *HelmTimeline
	Artifact                   *helmcommon.Artifact
	Input                      *textview.Field
	Namespaces                 *NamespacePicker
	PickerCursor               int
	InputKind                  string
	Menu                       []string
	MenuKind                   string
	Draft                      *HelmDraft
	Plan                       *helmtool.Plan
	Workflow                   *HelmWorkflow
	Document                   *docedit.Model
	Comparison                 *diffview.Model
	Discard                    bool
	Confirm                    bool
	ConfirmScroll              int
	Choice                     int
	Err, Notice                string
	Pending                    string
	Running                    string
	JobRevision                int
	RequestedRelease           *helmtool.Release
	LiveReveal                 bool
	Run                        uint64
	Busy                       bool
	Cancel                     context.CancelFunc
}

type HelmResult struct {
	State        *HelmState
	Run          uint64
	Kind         string
	Client       *helmtool.Client
	Releases     []helmtool.Release
	Snapshot     *helmtool.Snapshot
	Artifact     *helmcommon.Artifact
	Plan         *helmtool.Plan
	Live         map[string]any
	LiveReveal   bool
	Outcome      string
	HistoryError string
	TimelineLoad *HelmTimelineLoad
	Changes      *helmtool.RevisionChanges
	Snapshots    []*helmtool.Snapshot
	Err          error
}

func HelmConfig(c *workspace.Connection) helmtool.Config {
	return helmtool.Config{Target: c.Target, TargetCaptured: c.TargetCaptured, ConnectionID: c.ID, Kubeconfig: c.Settings["kubeconfig"], Context: c.Settings["context"], Namespace: c.Settings["namespace"], Driver: c.Settings["driver"], RepositoryConfig: c.Settings["repository_config"], RepositoryCache: c.Settings["repository_cache"], RegistryConfig: c.Settings["registry_config"]}
}

func ContainsString(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func (h *HelmState) PositionKey() string {
	return h.Preference.Section
}

func (h *HelmState) SavePosition() {
	if h.Positions == nil {
		h.Positions = map[string]int{}
	}
	h.Positions[h.PositionKey()] = h.Cursor
}

func (h *HelmState) RowsCount() int {
	switch {
	case h.Menu != nil:
		return len(h.Menu)
	case h.Detail != nil && h.Tab == 5:
		rs, _ := h.InspectorResources()
		return len(rs)
	default:
		return len(h.ReleaseRows())
	}
}
