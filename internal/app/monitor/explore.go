package monitor

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	logview "github.com/simjay/plous/internal/ui/logs"
	textview "github.com/simjay/plous/internal/ui/text"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type ExplorePreference struct {
	Resource   string `json:"resource"`
	Namespace  string `json:"namespace"`
	Filter     string `json:"filter,omitempty"`
	Selector   string `json:"selector,omitempty"`
	Sort       int    `json:"sort,omitempty"`
	Descending bool   `json:"descending,omitempty"`
}

type DetailSectionReturn struct {
	Tab, Scroll, RelatedCursor int
}

type ExploreLocation struct {
	YamlOrigin                 *DetailSectionReturn
	Preference                 ExplorePreference
	Resource                   kubecommon.Resource
	Selected                   string
	Object                     *unstructured.Unstructured
	Tab, Scroll, RelatedCursor int
	Container                  string
}

type ExploreState struct {
	Temporary                          bool
	Preloaded                          bool
	Key                                string
	Generation                         uint64
	Client                             *kubernetes.Client
	Resources                          []kubecommon.Resource
	Resource                           kubecommon.Resource
	Preference                         ExplorePreference
	Collection                         kubernetes.Collection
	Inventory                          *ExploreInventory
	Listed                             bool
	Selected                           string
	Status, Err, Warning               string
	Connecting                         bool
	PreserveTree                       bool
	DefaultNamespace                   bool
	Stream                             *ExploreStream
	Usage                              *ExploreUsage
	Detail                             *unstructured.Unstructured
	DetailIdentity                     string
	DetailLoaded                       bool
	DetailSections                     [4]DetailSection
	DetailValid                        bool
	Supplement                         []kubernetes.DetailSection
	SupplementNote                     string
	SupplementAt                       time.Time
	DetailRun                          *ExploreDetailRun
	DetailText, DetailError, DetailKey string
	References                         []kubernetes.Reference
	RelatedCursor                      int
	YamlDocument                       *YamlDocument
	YamlOrigin                         *DetailSectionReturn
	Tab, Scroll                        int
	Trail                              []ExploreLocation
	LogOrigin                          *ExploreLocation
	Log                                *ExploreLogRun
	LogError, LogKey, Container        string
	LogViewer                          *logview.Model
	Previous, Reveal                   bool
	Picker                             string
	PickerCursor                       int
	Input                              *textview.Field
	InputKind                          string
	Requested                          string
	PendingCommand                     string
	Namespaces                         *NamespacePicker
	Action                             *ResourceActionState
	Terminal                           *ResourceTerminal
	LogSince                           int64
	LogTimestamps                      bool
	MergedLogs                         bool
	WorkloadLogStatus                  kubernetes.WorkloadLogStatus
	LogConnected                       bool
}

type ExploreConnected struct {
	Generation uint64
	Client     *kubernetes.Client
	Resources  []kubecommon.Resource
	Warning    string
	Err        error
}

type ExploreStream struct {
	Key    string
	Cancel context.CancelFunc
	Ctx    context.Context
	Events chan kubernetes.Snapshot
}

type ExploreUpdate struct {
	Stream   *ExploreStream
	Snapshot kubernetes.Snapshot
	Done     bool
}

type DetailSection struct {
	Text, Note string
	References []kubernetes.Reference
	Supplement []kubernetes.DetailSection
	At         time.Time
	Loaded     bool
}

type ExploreDetailRun struct{ Cancel context.CancelFunc }

type ExploreDetailResult struct {
	Run          *ExploreDetailRun
	Object       *unstructured.Unstructured
	Text         string
	References   []kubernetes.Reference
	Supplement   []kubernetes.DetailSection
	Note         string
	SupplementAt time.Time
	Err          error
}

type ExploreLogRun struct {
	Cancel context.CancelFunc
	Ctx    context.Context
	Events chan ExploreLogEvent
}

type ExploreLogEvent struct {
	WorkloadStatus *kubernetes.WorkloadLogStatus
	Run            *ExploreLogRun
	Text           string
	Err            error
	Done           bool
	Connected      bool
}

func WaitExplore(stream *ExploreStream) tea.Cmd {
	return func() tea.Msg {
		select {
		case snapshot, ok := <-stream.Events:
			return ExploreUpdate{Stream: stream, Snapshot: snapshot, Done: !ok}
		case <-stream.Ctx.Done():
			return ExploreUpdate{Stream: stream, Done: true}
		}
	}
}

func WaitExploreLog(run *ExploreLogRun) tea.Cmd {
	return func() tea.Msg {
		var text strings.Builder
		batch := ExploreLogEvent{Run: run}
		var deadline <-chan time.Time
		// Coalesce off the UI loop. Bound both busy streams and sparse output
		// so a backlog cannot require one application redraw per log line.
		for count := 0; count < 1024 && text.Len() < 64*1024; count++ {
			if run.Ctx.Err() != nil {
				return ExploreLogEvent{Run: run, Done: true}
			}
			select {
			case event, ok := <-run.Events:
				text.WriteString(event.Text)
				batch.Connected = batch.Connected || event.Connected
				if event.WorkloadStatus != nil {
					batch.WorkloadStatus = event.WorkloadStatus
				}
				if event.Err != nil {
					batch.Err = event.Err
				}
				if !ok || event.Done {
					batch.Text, batch.Done = text.String(), true
					return batch
				}
				if deadline == nil {
					timer := time.NewTimer(16 * time.Millisecond)
					defer timer.Stop()
					deadline = timer.C
				}
			case <-deadline:
				batch.Text = text.String()
				return batch
			case <-run.Ctx.Done():
				return ExploreLogEvent{Run: run, Done: true}
			}
		}
		batch.Text = text.String()
		return batch
	}
}
