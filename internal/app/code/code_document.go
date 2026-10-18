package code

import (
	"fmt"

	"github.com/simjay/plous/internal/source"
	diffview "github.com/simjay/plous/internal/ui/diff"
	docedit "github.com/simjay/plous/internal/ui/editor"
)

type CodeDocumentFrame struct {
	Document   *docedit.Model
	Source     *source.Inventory
	File       *source.Document
	Kind, Path string
}

type CodeViewDocument struct {
	Frame   CodeDocumentFrame
	Review  *diffview.Model
	Parents []CodeDocumentFrame
	Action  int
}

// View navigation suspends document models, retaining their cursors and undo history.
func (s *CodeState) SyncDocumentView(view string) {
	if s.DocumentView == "" {
		s.DocumentView = view
	}
	if s.DocumentView == view {
		return
	}
	if s.ViewDocuments == nil {
		s.ViewDocuments = map[string]CodeViewDocument{}
	}
	if s.Document != nil || s.Review != nil {
		s.ViewDocuments[s.DocumentView] = CodeViewDocument{
			Frame:  CodeDocumentFrame{s.Document, s.DocumentSource, s.File, s.DocumentKind, s.DocumentPath},
			Review: s.Review, Parents: s.Documents, Action: s.DocumentAction,
		}
	}
	next := s.ViewDocuments[view]
	delete(s.ViewDocuments, view)
	s.Document, s.DocumentSource, s.File = next.Frame.Document, next.Frame.Source, next.Frame.File
	s.DocumentKind, s.DocumentPath = next.Frame.Kind, next.Frame.Path
	s.Review, s.Documents, s.DocumentAction = next.Review, next.Parents, next.Action
	s.Menu = ""
	s.DocumentView = view
}

func (s *CodeState) OtherEditorView() string {
	for view, document := range s.ViewDocuments {
		if document.Frame.Document != nil && !document.Frame.Document.ReadOnly {
			return view
		}
		for _, parent := range document.Parents {
			if parent.Document != nil && !parent.Document.ReadOnly {
				return view
			}
		}
	}
	return ""
}

func (s *CodeState) PushDocument() {
	s.Documents = append(s.Documents, CodeDocumentFrame{s.Document, s.DocumentSource, s.File, s.DocumentKind, s.DocumentPath})
}

func (s *CodeState) CloseDocument() {
	s.Menu, s.Discard, s.Review = "", false, nil
	s.Document, s.File = nil, nil
	s.DocumentSource = nil
	s.DocumentKind, s.DocumentPath = "", ""
	if len(s.Documents) > 0 {
		last := s.Documents[len(s.Documents)-1]
		s.Documents = s.Documents[:len(s.Documents)-1]
		s.Document, s.File, s.DocumentKind, s.DocumentPath = last.Document, last.File, last.Kind, last.Path
		s.DocumentSource = last.Source
	}
}

func CodeIdentity(s *CodeState) string {
	if s.Inventory == nil {
		if s.Config.Repository != "" {
			return "Git repository"
		}
		return "Local directory"
	}
	snapshot := s.Inventory.Source
	revision := "Working tree"
	if snapshot.Revision != "" {
		revision = "Git " + snapshot.Revision[:min(12, len(snapshot.Revision))]
	}
	count := CodeCountLabel(snapshot.FileCount(), "file", "files")
	if s.View == "manifests" {
		count = CodeCountLabel(len(s.Inventory.Entries)+len(s.Inventory.Terraform), "source", "sources")
	} else if s.View == "tfcode" {
		count = CodeCountLabel(len(s.Inventory.Terraform), "directory", "directories")
	}
	return revision + " · " + count
}

func (s *CodeState) InputHelp() string {
	switch s.InputKind {
	case "Export Rendered YAML":
		if s.ExportResult != nil {
			return fmt.Sprintf("Export all %d final resources from %s to a new file. Filters, comparison direction, and captured stages do not change this export. Sensitive values are masked. Existing files are never overwritten. Enter writes the file; Esc cancels.", len(s.ExportResult.Resources), s.ExportResult.Path)
		}
		return "Render a source first."

	case "Filter":
		return "Filter the current list. Clear it with Esc or Backspace."
	case "Helm Values Files (In Order)":
		return "Comma-separated paths from the source root. Later files override earlier ones."
	case "Helm Overrides":
		return "Helm --set assignments, such as replicas=3,image.tag=1.2."
	case "Helm Release Name":
		return "Blank uses preview. These inputs apply to the selected chart."
	case "Helm Namespace":
		return "Blank uses default. Rendering does not connect to a cluster."
	case "Helm Kubernetes Version":
		return "Optional version for template capabilities, for example 1.31."
	case "Helm API Versions":
		return "Optional comma-separated API versions for template capabilities."
	case "Git Revision", "Compare Git Revision":
		return "Branch, tag, or commit. Blank resolves HEAD in a separate snapshot."
	case "Create Editable Local Copy":
		return "Choose a new local directory. Existing directories are never overwritten."
	}
	return ""
}
