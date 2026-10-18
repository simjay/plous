package chat

import (
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ai/agent"
	"github.com/simjay/plous/internal/ai/history"
	"github.com/simjay/plous/internal/ui"
	docedit "github.com/simjay/plous/internal/ui/editor"
	markdownview "github.com/simjay/plous/internal/ui/markdown"
	textview "github.com/simjay/plous/internal/ui/text"

	"strings"
	"time"
)

type ChatEvidence struct {
	Conversation history.Conversation
	Turn         history.Turn
	Records      []history.ReasoningEntry
	Loaded       bool
	Err          error
}

type ChatEvidenceResult struct {
	Evidence *ChatEvidence
	Records  []history.ReasoningEntry
	Err      error
}

type ChatSources struct {
	Chat      *ChatState
	Evidence  *ChatEvidence
	Index     int
	List      bool
	Loading   bool
	Document  *docedit.Model
	Reply     *markdownview.Model
	ShowReply bool
	Follow    bool
	Tail      string
	Err       string
	Scroll    int
	Last      int
	Page      int
}

type ChatSourceDocumentResult struct {
	Sources *ChatSources
	Text    string
	Err     error
}

func (c *ChatState) SelectedReply() int {
	if c.ReplySelected {
		return max(0, min(c.Reply, len(c.Turns)-1))
	}
	return len(c.Turns) - 1
}

func (c *ChatState) SelectReplyForLine(line int) {
	c.Reply, c.ReplySelected = 0, true
	for i, header := range c.ReplyLines {
		start := header - strings.Count(ui.Plain(c.Turns[i].Request.Prompt), "\n") - 3
		if start <= line {
			c.Reply = i
		}
	}
}

func SourceSummary(record history.ReasoningEntry) (title, scope string, observed time.Time) {
	if r := record.Projection; r != nil {
		title = record.Tool
		switch record.Tool {
		case "kubernetes_resources", "kubernetes_resources_read", "kubernetes_logs", "kubernetes_logs_read", "kubernetes_usage", "kubernetes_usage_read":
			title = "Kubernetes"
		case "helm_releases", "helm_releases_read", "helm_configuration", "helm_configuration_read", "helm_compare", "helm_diff_read":
			title = "Helm"
		case "source_files", "source_files_read":
			title = "Source files"
		case "source_analysis", "source_analysis_read":
			title = "Source analysis"
		case "argo_resources", "argo_resources_read":
			title = "Argo"
		case "flux_resources", "flux_resources_read":
			title = "Flux"
		case "history", "history_read":
			title = "Saved history"
		}
		if r.Connection != "" {
			title += " · " + r.Connection
		}
		var request struct{ Resource, Name, Path string }
		_ = json.Unmarshal(record.Input, &request)
		scope = strings.TrimSpace(r.Operation + " " + request.Resource + " " + request.Name + " " + request.Path)
		if r.Namespace == "*" {
			scope += " · all namespaces"
		} else if r.Namespace != "" {
			scope += " · namespace " + r.Namespace
		}
		if !r.Coverage.Complete {
			scope += " · partial coverage"
		}
		return ui.OneLine(title), ui.OneLine(scope), r.ObservedAt
	}
	return "Saved history", "Saved conversation", record.Time
}

func PrettySource(value string) string {
	var data any
	if json.Unmarshal([]byte(value), &data) == nil {
		pretty, _ := json.MarshalIndent(data, "", "  ")
		return string(pretty)
	}
	return value
}

func (s *ChatSources) DetailsView(body string, width, height int) string {
	s.Page = height
	s.Last = max(0, strings.Count(ansi.Wrap(body, max(1, width), ""), "\n")+1-height)
	s.Scroll = min(s.Scroll, s.Last)
	return ui.StyledTextView(body, width, height, s.Scroll)
}

func (s *ChatSources) Body(width, height int) string {
	e := s.Evidence
	if s.ShowReply {
		return s.Reply.View(width, height, ui.WorkspaceTheme())
	}
	heading := ui.Muted.Render(ui.Clipped("Reply to: "+ui.OneLine(e.Turn.Request.Prompt), width))
	if s.Document != nil {
		return s.Document.View(width, height, ui.WorkspaceTheme())
	}
	if !e.Loaded || s.Loading {
		return heading + "\n\n" + ui.Muted.Render("Loading saved sources…")
	}
	if e.Err != nil {
		return s.DetailsView(heading+"\n\nSources unavailable: "+ui.Plain(e.Err.Error()), width, height)
	}
	if s.List {
		items := make([]string, len(e.Records))
		for i, record := range e.Records {
			title, scope, _ := SourceSummary(record)
			items[i] = fmt.Sprintf("%d  %s · %s", i+1, title, scope)
			if record.Status != "completed" {
				items[i] += " · " + record.Status
			}
		}
		return heading + "\n\n" + ui.ListView(items, s.Index, width, max(0, height-2))
	}
	lines := []string{heading, ""}
	if len(e.Records) > 0 {
		record := e.Records[s.Index]
		title, scope, observed := SourceSummary(record)
		lines = append(lines, ChatAssistantStyle.Render(ui.Clipped(title, width)), ui.Muted.Render(ui.Clipped(scope, width)), "Checked "+observed.Local().Format("2006-01-02 15:04:05 MST"))
		if record.Status != "completed" {
			lines = append(lines, ui.RemovedStyle.Render(ui.Clipped("Read "+record.Status+": "+ui.OneLine(record.Content), width)))
		}
		lines = append(lines, "", textview.ActionChip(textview.Action{Key: "Enter", Label: "View captured result", Primary: true}, width, ui.WorkspaceTheme()))
	} else {
		lines = append(lines, "No tool sources recorded for this reply.")
	}
	identity := ChatModelName(e.Turn.Request.Agent)
	if e.Turn.Request.Agent.Name != "" {
		identity += " · " + e.Turn.Request.Agent.Name
	}
	if e.Turn.Request.Backend.Effective() != agent.Plous {
		identity = e.Turn.Request.Backend.Label() + " · installed CLI"
	}
	usage := "Token usage not reported"
	if e.Turn.Response.Usage.Total > 0 {
		usage = fmt.Sprintf("%d tokens", e.Turn.Response.Usage.Total)
	}
	lines = append(lines, "", ui.Muted.Render(ui.Clipped(identity, width)), ui.Muted.Render(usage+" · "+e.Turn.Response.Status))
	if s.Err != "" {
		lines = append(lines, ui.RemovedStyle.Render(s.Err))
	}
	return s.DetailsView(strings.Join(lines, "\n"), width, height)
}

func (s *ChatSources) ApplyDocument(result ChatSourceDocumentResult) {
	if s != result.Sources {
		return
	}
	s.Loading = false
	if result.Err != nil {
		s.Err = result.Err.Error()
		return
	}
	s.Document = docedit.New(ui.Plain(result.Text))
	s.Document.ReadOnly, s.Document.LineNumbers = true, false
	s.Document.Name = "Captured source"
	s.Document.Target, _, _ = SourceSummary(s.Evidence.Records[s.Index])
	s.Document.Actions = []textview.ActionGroup{{Label: "Read", Actions: []textview.Action{{Key: "Esc", Label: "Back"}, {Key: "/", Label: "Find"}, {Key: "v", Label: "Select"}, {Key: "y", Label: "Copy"}}}}
}
