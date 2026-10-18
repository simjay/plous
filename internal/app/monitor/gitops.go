package monitor

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	gitops "github.com/simjay/plous/internal/integrations/gitops/common"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
)

type GitOpsAction struct {
	Kind        string
	Ref         gitops.Reference
	Destination gitops.Destination
	Label       string
	Disabled    string
}

type gitOpsTrail struct {
	Item   gitops.Item
	Cursor int
	Scroll int
	Config bool
}

type GitOpsState struct {
	Tool                  gitops.Tool
	Section               gitops.Section
	Namespace             string
	Snapshot              gitops.Snapshot
	Detail                *gitops.Item
	Cursor, DetailCursor  int
	Scroll                int
	Query                 string
	Input                 *textview.Field
	Find                  *textview.Viewer
	Loading, Loaded       bool
	Err                   string
	Pending               *GitOpsAction
	ConfigurationExpanded bool
	DestinationErrors     map[string]string
	trail                 []gitOpsTrail
	listSelection         string
	actionLines           []int
	detailHeight          int
	revealAction          bool
}

func NewGitOpsState(tool gitops.Tool, namespace string) *GitOpsState {
	if namespace == "*" {
		namespace = ""
	}
	return &GitOpsState{Tool: tool, Section: gitops.NormalizeSection(tool, ""), Namespace: namespace, DestinationErrors: map[string]string{}}
}

func (s *GitOpsState) Title() string {
	if s.Tool == gitops.ArgoCD {
		return "Argo"
	}
	return "Flux"
}

func (s *GitOpsState) Rows() []gitops.Item {
	query := s.Query
	if s.Input != nil {
		query = s.Input.Text
	}
	var rows []gitops.Item
	for _, item := range s.Snapshot.Items {
		text := strings.Join([]string{item.Ref.Name, item.Ref.Namespace, item.Ref.Kind, item.Sync, item.Health, item.Status, item.Reason, gitOpsDestination(item.Destination)}, " ")
		if ui.SearchContains(text, query) {
			rows = append(rows, item)
		}
	}
	gitops.SortItems(rows)
	return rows
}

func (s *GitOpsState) Selected() *gitops.Item {
	rows := s.Rows()
	if len(rows) == 0 {
		return nil
	}
	s.Cursor = max(0, min(s.Cursor, len(rows)-1))
	return &rows[s.Cursor]
}

func (s *GitOpsState) Apply(snapshot gitops.Snapshot) {
	key := ""
	if selected := s.Selected(); selected != nil {
		key = selected.Ref.Key()
	}
	s.Loading, s.Loaded, s.Err = false, true, ""
	failed := snapshot.State == gitops.ReadDenied || snapshot.State == gitops.APIUnavailable || snapshot.State == gitops.ConnectionUnavailable
	if failed && len(s.Snapshot.Items) > 0 && len(snapshot.Items) == 0 {
		s.Err = snapshot.Message
		if s.Err == "" {
			s.Err = "Refresh failed. Previous observations are shown."
		}
		return
	}
	s.Snapshot = snapshot
	s.restoreSelection(key)
	if s.Detail != nil && len(s.trail) == 0 {
		for _, item := range snapshot.Items {
			if item.Ref.Key() == s.Detail.Ref.Key() {
				s.Detail = &item
				break
			}
		}
	}
}

func (s *GitOpsState) ApplyDetail(item gitops.Item) {
	if s.Detail != nil && item.Ref.Key() == s.Detail.Ref.Key() {
		s.Detail, s.Loading, s.Err = &item, false, ""
		return
	}
	if s.Detail != nil && item.Ref.Key() != s.Detail.Ref.Key() {
		s.trail = append(s.trail, gitOpsTrail{Item: *s.Detail, Cursor: s.DetailCursor, Scroll: s.Scroll, Config: s.ConfigurationExpanded})
	}
	s.Detail, s.Loading, s.Err = &item, false, ""
	s.DetailCursor, s.Scroll = 0, 0
	s.ConfigurationExpanded, s.revealAction, s.Find = false, false, nil
}

func (s *GitOpsState) SetNamespace(namespace string) {
	if namespace == "*" {
		namespace = ""
	}
	if namespace == s.Namespace {
		return
	}
	s.Namespace, s.Cursor = namespace, 0
	s.Snapshot, s.Detail = gitops.Snapshot{}, nil
	s.trail, s.Find, s.Input = nil, nil, nil
	s.Loaded, s.Loading, s.Err = false, false, ""
	s.Pending = &GitOpsAction{Kind: "refresh"}
}

func (s *GitOpsState) restoreSelection(key string) {
	s.Cursor = max(0, min(s.Cursor, len(s.Rows())-1))
	for i, item := range s.Rows() {
		if item.Ref.Key() == key {
			s.Cursor = i
			return
		}
	}
}

func (s *GitOpsState) Select(index int) {
	if s.Detail == nil {
		s.Cursor = max(0, min(index, len(s.Rows())-1))
	} else {
		s.DetailCursor = max(0, min(index, len(s.Actions())-1))
		s.revealAction = true
	}
}

func (s *GitOpsState) Actions() []GitOpsAction {
	if s.Detail == nil {
		return nil
	}
	item := s.Detail
	var actions []GitOpsAction
	for _, related := range item.Related {
		actions = append(actions, GitOpsAction{Kind: "detail", Ref: related.Ref, Label: related.Ref.Name + " · " + related.Ref.Namespace})
	}
	for _, ref := range item.Dependencies {
		actions = append(actions, GitOpsAction{Kind: "detail", Ref: ref, Label: "Dependency: " + ref.Label() + " · " + ref.Namespace})
	}
	for _, source := range item.Sources {
		if source.Ref.Name != "" {
			actions = append(actions, GitOpsAction{Kind: "detail", Ref: source.Ref, Label: "Source: " + source.Ref.Label() + " · " + source.Ref.Namespace})
		}
	}
	for _, resource := range item.Resources {
		disabled := s.DestinationErrors[item.Ref.Key()]
		if disabled == "" && item.Destination.Remote {
			disabled = "Remote destination access is unavailable. Plous does not read the controller's kubeconfig credentials."
		}
		actions = append(actions, GitOpsAction{Kind: "resource", Ref: resource.Ref, Destination: item.Destination, Label: resource.Ref.Label() + " · " + resource.Ref.Namespace, Disabled: disabled})
	}
	return append(actions, GitOpsAction{Kind: "configuration", Label: "Configuration & revisions"})
}

func (s *GitOpsState) Open() *GitOpsAction {
	if s.Detail == nil {
		if selected := s.Selected(); selected != nil {
			s.listSelection = selected.Ref.Key()
			s.Detail = selected
			s.DetailCursor, s.Scroll = 0, 0
			s.ConfigurationExpanded, s.revealAction = false, false
		}
		return nil
	}
	actions := s.Actions()
	if len(actions) == 0 {
		return nil
	}
	action := actions[max(0, min(s.DetailCursor, len(actions)-1))]
	if action.Disabled != "" {
		return nil
	}
	if action.Kind == "configuration" {
		s.ConfigurationExpanded = !s.ConfigurationExpanded
		return nil
	}
	s.Pending = &action
	return &action
}

func (s *GitOpsState) TakeAction() *GitOpsAction {
	action := s.Pending
	s.Pending = nil
	return action
}

func (s *GitOpsState) Back() bool {
	if s.Input != nil {
		s.Input = nil
		s.restoreSelection(s.listSelection)
		return true
	}
	if s.Find != nil {
		s.Scroll = s.Find.Top
		s.Find = nil
		return true
	}
	if s.Detail != nil {
		if n := len(s.trail); n > 0 {
			previous := s.trail[n-1]
			s.trail = s.trail[:n-1]
			s.Detail = &previous.Item
			s.DetailCursor, s.Scroll, s.ConfigurationExpanded = previous.Cursor, previous.Scroll, previous.Config
		} else {
			s.Detail = nil
			s.restoreSelection(s.listSelection)
		}
		s.Err, s.Pending = "", nil
		return true
	}
	if s.Query != "" {
		s.Query = ""
		return true
	}
	return false
}

func (s *GitOpsState) FilterInput() bool { return s.Input != nil || s.Find != nil && s.Find.Searching }

func (s *GitOpsState) HasLocalEscape() bool {
	return s.Input != nil || s.Find != nil || s.Detail != nil || s.Query != ""
}

func (s *GitOpsState) SelectedKey() string {
	if item := s.Selected(); item != nil {
		return item.Ref.Key()
	}
	return ""
}

func (s *GitOpsState) SelectKey(key string) { s.restoreSelection(key) }

func (s *GitOpsState) RowsCount() int {
	if s.Detail != nil {
		return len(s.Actions())
	}
	return len(s.Rows())
}

func (s *GitOpsState) Paste(text string) {
	if s.Input != nil {
		s.Input.Insert(text, false)
		s.Cursor = 0
	} else if s.Find != nil && s.Find.Searching {
		s.Find.Paste(text)
	}
}

func (s *GitOpsState) Key(msg tea.KeyPressMsg) bool {
	key := msg.String()
	if s.Input != nil {
		switch key {
		case "esc":
			s.Back()
		case "enter":
			s.Query, s.Input = strings.TrimSpace(s.Input.Text), nil
			s.Open()
		default:
			s.Input.Key(msg, false)
			s.Cursor = 0
		}
		return true
	}
	if s.Find != nil {
		if key == "esc" {
			s.Back()
			return true
		}
		if s.Find.Searching || key == "n" || key == "N" || key == "/" || key == "up" || key == "down" || key == "pgup" || key == "pgdown" || key == "home" || key == "end" {
			handled, _ := s.Find.Key(key, msg.Text)
			return handled
		}
	}
	switch key {
	case "esc":
		return s.Back()
	case "/":
		if s.Detail != nil {
			s.Find = &textview.Viewer{Wrap: true, Searching: true, Top: s.Scroll}
		} else {
			if selected := s.Selected(); selected != nil {
				s.listSelection = selected.Ref.Key()
			}
			field := textview.NewField(s.Query)
			s.Input = &field
		}
	case "backspace":
		if s.Detail != nil || s.Query == "" {
			return false
		}
		s.Query = ""
	case "g":
		if s.Detail != nil {
			s.Pending = &GitOpsAction{Kind: "detail", Ref: s.Detail.Ref}
		} else {
			s.Pending = &GitOpsAction{Kind: "refresh"}
		}
	case "n":
		if s.Detail != nil {
			return false
		}
		s.Pending = &GitOpsAction{Kind: "namespace"}
	case "enter":
		s.Open()
	case "up", "k", "down", "j", "home", "end", "pgup", "pgdown":
		step := 1
		if key == "up" || key == "k" || key == "pgup" {
			step = -1
		}
		if s.Detail != nil {
			if key == "pgup" || key == "pgdown" {
				s.Scroll = max(0, s.Scroll+step*max(1, s.detailHeight-2))
				s.revealAction = false
			} else {
				s.Select(s.DetailCursor + step)
				if key == "home" {
					s.DetailCursor, s.Scroll, s.revealAction = 0, 0, false
				} else if key == "end" {
					s.Select(len(s.Actions()) - 1)
				}
			}
		} else {
			if key == "pgup" || key == "pgdown" {
				step *= 10
			}
			s.Select(s.Cursor + step)
			if key == "home" {
				s.Select(0)
			} else if key == "end" {
				s.Select(len(s.Rows()) - 1)
			}
		}
	default:
		return false
	}
	return true
}

func (s *GitOpsState) Hints() []string {
	if s.Input != nil {
		return []string{"Enter Open", "Esc Cancel"}
	}
	if s.Find != nil {
		if s.Find.Searching {
			return []string{"Enter Finish", "Esc Clear"}
		}
		return []string{"n/N Matches", "Esc Clear"}
	}
	if s.Detail != nil {
		hints := []string{"↑↓ Select"}
		actions := s.Actions()
		if len(actions) > 0 {
			action := actions[max(0, min(s.DetailCursor, len(actions)-1))]
			if action.Disabled == "" {
				label := "Enter Open"
				if action.Kind == "configuration" {
					label = "Enter Expand"
					if s.ConfigurationExpanded {
						label = "Enter Collapse"
					}
				}
				hints = append(hints, label)
			}
		}
		return append(hints, "PgUp/PgDn Scroll", "/ Find", "g Refresh", "Esc Back")
	}
	return []string{"↑↓ Select", "Enter Details", "/ Filter", "n Namespace", "g Refresh"}
}
