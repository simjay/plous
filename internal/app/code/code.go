package code

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"fmt"

	manifest "github.com/simjay/plous/internal/integrations/manifest"
	"github.com/simjay/plous/internal/source"
	diffview "github.com/simjay/plous/internal/ui/diff"
	docedit "github.com/simjay/plous/internal/ui/editor"
	textview "github.com/simjay/plous/internal/ui/text"
	"strings"
)

// Legacy indexes now identify contextual Render details, not sidebar views.
var CodeSections = []string{"Configuration Map", "Render", "Field History", "Findings"}

var CodeViews = []struct {
	Id, Label string
}{
	{"files", "Files"},
	{"helm", "Helm"},
	{"kustomize", "Kustomize"},
	{"argo", "Argo"},
	{"flux", "Flux"},
}

func (s *CodeState) ViewID() string {
	if s.View == "manifests" {
		if IsBrowser(s.ReturnView) {
			return strings.TrimPrefix(s.ReturnView, "code-")
		}
		return "files"
	}
	if IsBrowser(s.View) {
		return strings.TrimPrefix(s.View, "code-")
	}
	return "files"
}
func (s *CodeState) ViewLabel() string {
	if s.View == "manifests" {
		return BrowserLabel(s.ReturnView) + " / Render"
	}
	return BrowserLabel(s.View)
}

type CodePreference struct {
	Browser        string `json:"browser,omitempty"`
	BrowserItem    string `json:"browser_item,omitempty"`
	KustomizeGroup string `json:"kustomize_group,omitempty"`
	Selected       string `json:"selected,omitempty"`
	File           string `json:"file,omitempty"`
	Revision       string `json:"revision,omitempty"`
	Fingerprint    string `json:"fingerprint,omitempty"`
	Section        int    `json:"section,omitempty"`
	Resource       string `json:"resource,omitempty"`
	Field          string `json:"field,omitempty"`
}

type CodeState struct {
	Key, Workspace, Connection string
	Config                     source.Config
	Preference                 CodePreference
	Inventory                  *source.Inventory
	Builds                     map[string]*source.Result
	Failures                   map[string]string
	FailureSource              string
	Inputs                     map[string]source.Inputs
	PreviewAttempts            map[string]string
	RunningEntry               string
	RunningView                string
	FileExpanded               map[string]bool
	FileRoot                   string
	Renderer                   source.Renderer
	Cursor                     int
	Filter, Menu               string
	Choice                     int
	Input                      *textview.Field
	InputKind                  string
	View                       string
	ListScope                  string
	Filters                    map[string]string
	Cursors                    map[string]int
	InputMenu                  string
	InputChoice                int
	Pending, Running, Other    string
	Run                        uint64
	Busy                       bool
	LoadAttempted              bool
	Rescan                     bool
	ScanFailed, ScanCancelled  bool
	Cancel                     context.CancelFunc
	Err, Notice                string
	Document                   *docedit.Model
	DocumentSource             *source.Inventory
	File                       *source.Document
	Review                     *diffview.Model
	Discard                    bool
	DiscardChoice              int
	DocumentKind, DocumentPath string
	DocumentAction             int
	Documents                  []CodeDocumentFrame
	DocumentView               string
	ViewDocuments              map[string]CodeViewDocument
	ReturnView                 string
	RenderRoot                 string
	RenderTargets              []source.Entry
	BrowserDetail              bool
	Group                      string
	NodeGraph                  string
	InspectorArea              int
	DetailOffset               int
	FieldCursor                int
	FieldResource              string
	TraceFields                []source.Field
	Comparison                 *source.Result
	ComparisonBase             *source.Result
	ComparisonScope            string
	ComparisonUnified          bool
	ComparisonSwapped          bool
	ComparisonChanged          bool
	ComparisonHidden           bool
	ComparisonOffset           int
	ComparisonResource         string
	ComparisonIdentity         string
	ComparisonCache            []source.ResourceChange
	ValidationView             string
	ValidationCategory         string
	ValidationSource           string
	ExportResult               *source.Result
	TraceReference             *CodeTraceReference
	TraceUnavailable           string
	Details                    []CodeDetailReturn
	SavedBaseline              *source.Result
	SavedComparison            bool
	Stage                      int
	Yank                       string
}

type CodeResult struct {
	Continue      tea.Cmd
	State         *CodeState
	Run           uint64
	Inventory     *source.Inventory
	Results       map[string]*source.Result
	Failures      map[string]string
	Before, After *source.Result
	Err           error
	Kind          string
	Inputs        map[string]source.Inputs
}

func (s *CodeState) RestoreCursor() {
	if s.Inventory == nil {
		s.Cursor = 0
		return
	}
	if s.KustomizeDirectory() {
		groups := s.KustomizeGroupItems()
		s.Cursor = max(0, min(s.Cursor, len(groups)-1))
		for n, group := range groups {
			if group.Path == s.Preference.KustomizeGroup {
				s.Cursor = n
				break
			}
		}
	} else if IsBrowser(s.View) {
		items := s.CatalogItems()
		s.Cursor = max(0, min(s.Cursor, len(items)-1))
		for n, item := range items {
			if item.ID == s.Preference.BrowserItem {
				s.Cursor = n
				break
			}
		}
	} else if s.View == "files" || s.View == "code" {
		files := s.FileRows()
		s.Cursor = max(0, min(s.Cursor, len(files)-1))
		for n, file := range files {
			if file.Path == s.Preference.File {
				s.Cursor = n
				break
			}
		}
	} else if s.View == "manifests" && s.Preference.Section == 0 {
		entries := s.Entries()
		s.Cursor = max(0, min(s.Cursor, len(entries)-1))
		for n, entry := range entries {
			if entry.ID == s.Preference.Selected {
				s.Cursor = n
				break
			}
		}
	} else if s.View == "manifests" && (s.Preference.Section == 1 || s.Preference.Section == 2) {
		resources := s.Resources()
		s.Cursor = max(0, min(s.Cursor, len(resources)-1))
		for n, resource := range resources {
			if resource.Key() == s.Preference.Resource {
				s.Cursor = n
				break
			}
		}
	}
}

func (s *CodeState) ChooseSection(section int) {
	if section != 2 {
		s.TraceReference, s.TraceUnavailable = nil, ""
	}
	if section != 1 {
		s.SelectFinalResource()
	}
	if s.Preference.Section != section {
		s.InspectorArea, s.DetailOffset = 0, 0
		if section == 1 || section == 2 {
			s.InspectorArea = 2
			if section == 2 {
				s.InspectorArea = 3
			}
		}
	}
	s.Preference.Section = section
	s.SyncListScope()
}

func (s *CodeState) BrowsingDuringDiscovery() bool {
	return s.Busy && s.Running == "load" && s.Inventory != nil && (s.View == "files" || s.View == "code")
}

func (s *CodeState) Stop() {
	if s.Busy && s.Running == "load" {
		s.ScanCancelled = true
	}
	if s.Busy && (s.Running == "preview" || s.Running == "render") {
		s.Failures[s.RunningEntry] = "Rendering cancelled. Use :r to retry."
	}
	if s.Cancel != nil {
		s.Cancel()
	}
	s.Cancel = nil
	s.Run++
	s.Busy = false
	s.Pending = ""
	s.Rescan = false
}

func (s *CodeState) Status(id string) string {
	identity := ""
	if s.Inventory != nil {
		identity = s.Inventory.Source.Identity
	}
	status := source.ResultState(identity, s.Inputs[id].Identity(), s.Builds[id], s.PreviewFailure(id))
	if status == "rendered" {
		return CodeCountLabel(len(s.Builds[id].Resources), "resource", "resources")
	}
	return status
}

func CodeCountLabel(count int, singular, plural string) string {
	if count == 1 {
		return "1 " + singular
	}
	return fmt.Sprintf("%d %s", count, plural)
}

func CodeComparison(a, b *source.Result) (*diffview.Model, error) {
	before, after := []string{}, []string{}
	for _, change := range source.Compare(a, b) {
		if change.Status != "Unchanged" {
			old, next := manifest.MaskedComparison(change.Before.Object, change.After.Object)
			if change.Before.Name != "" {
				before = append(before, "# "+change.Before.Key()+"\n"+old)
			}
			if change.After.Name != "" {
				after = append(after, "# "+change.After.Key()+"\n"+next)
			}
		}
	}
	left, right := strings.Join(before, "\n"), strings.Join(after, "\n")
	if len(left)+len(right) > 2<<20 || strings.Count(left, "\n")+strings.Count(right, "\n") > 20000 {
		return nil, fmt.Errorf("comparison exceeds viewer limits; inspect individual resources")
	}
	d := diffview.New(left, right, a.ID+" → "+b.ID, 0, false)
	d.Name = "Rendered resource comparison"
	d.BeforeLabel = a.Path + " " + a.Revision
	d.AfterLabel = b.Path + " " + b.Revision
	d.Notice = "Unique verified source identities, then exact resource identities. Ambiguous resources stay separate. Sensitive values masked."
	return d, nil
}
