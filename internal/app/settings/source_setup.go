package settings

import (
	"errors"
	"github.com/charmbracelet/x/ansi"

	"github.com/simjay/plous/internal/source"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
	"os"
	"strings"
	"unicode"
)

type SourceSetup struct {
	Base, Err     string
	SavedRevision string
	Drafts        map[string]textview.Field
}

func (f *Form) LocalDirectory() bool {
	return f.Source != nil && f.Fields[1].Value.Text == "Local directory"
}

func (f *Form) ChangeSourceType() {
	for _, field := range f.Fields[2:] {
		key := field.Key
		if key == "path" {
			key = field.Label
		}
		f.Source.Drafts[key] = field.Value
	}
	f.Source.Err = ""
	f.Fields = f.Fields[:2]
	label := "Local Directory"
	if f.Fields[1].Value.Text == "Git repository" {
		label = "Git Repository URL"
	}
	f.Fields = append(f.Fields, Field{Key: "path", Label: label, Required: true, Value: f.Source.Drafts[label]})
	if !f.LocalDirectory() {
		f.Fields = append(f.Fields,
			Field{Key: "branch", Label: "Branch (Optional)", Value: f.Source.Drafts["branch"]},
			Field{Key: "browse_path", Label: "Path (Optional)", Value: f.Source.Drafts["browse_path"]})
	}
}

func (f *Form) DirectoryHint(width int) string {
	text := "Any folder works; Git is not required.\nUse /absolute/path, ~/folder, or a relative path.\nRelative to: " + f.Source.Base
	if value := f.Fields[2].Value.Text; strings.TrimSpace(value) != "" {
		if resolved, err := source.ResolveDirectory(value, f.Source.Base); err == nil {
			text += "\nResolved: " + resolved
		}
	}
	if f.Source.Err != "" {
		text += "\n" + f.Source.Err
	}
	return ui.Muted.Render(ansi.Wrap(ui.Plain(text), width, ""))
}

func NewSourceSetup() *SourceSetup {
	base, _ := os.Getwd()
	return &SourceSetup{Base: base, Drafts: map[string]textview.Field{}}
}

// Reject invalid names before saving or scheduling source acquisition.
func ValidateRepositoryBranch(branch string) error {
	invalid := branch == "HEAD" || strings.HasPrefix(branch, "-") || strings.HasPrefix(branch, "refs/") || strings.HasSuffix(branch, ".") || strings.Contains(branch, "..") || strings.Contains(branch, "@{") || strings.ContainsAny(branch, " ~^:?*[\\") || strings.IndexFunc(branch, unicode.IsControl) >= 0
	for _, part := range strings.Split(branch, "/") {
		invalid = invalid || part == "" || strings.HasPrefix(part, ".") || strings.HasSuffix(part, ".lock")
	}
	if invalid {
		return errors.New("Enter a branch name, such as main or release/production, or leave it blank for the default branch.")
	}
	return nil
}
