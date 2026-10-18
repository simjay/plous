package code

import (
	"fmt"
	"github.com/simjay/plous/internal/ui"
	docedit "github.com/simjay/plous/internal/ui/editor"
	"path"
	"slices"
	"strings"
)

type CodeFileRow struct {
	Path, Label string
	Tags        string
	Directory   bool
	Open        bool
}

// Search reveals matching ancestors without changing deliberately opened folders.
func (s *CodeState) FileRows() []CodeFileRow {
	if s.Inventory == nil {
		return nil
	}
	if s.FileExpanded == nil {
		s.FileExpanded = map[string]bool{}
		s.RevealFile(s.Preference.File)
	}
	children := map[string]map[string]bool{}
	root := s.FileRoot
	for _, name := range s.Files() {
		if root != "" && !strings.HasPrefix(name, root+"/") {
			continue
		}
		relative := strings.TrimPrefix(name, root+"/")
		parts := strings.Split(relative, "/")
		parent := root
		for n, part := range parts {
			if children[parent] == nil {
				children[parent] = map[string]bool{}
			}
			child := part
			if parent != "" {
				child = parent + "/" + part
			}
			children[parent][child] = n < len(parts)-1
			parent = child
		}
	}
	rootPath := root
	if rootPath == "" {
		rootPath = "."
	}
	rootLabel := "▾ /" + root
	rows := []CodeFileRow{{Path: rootPath, Label: rootLabel, Tags: s.Inventory.DirectoryTags[rootPath], Directory: true, Open: true}}
	var walk func(string, string)
	walk = func(parent, prefix string) {
		names := make([]string, 0, len(children[parent]))
		for name := range children[parent] {
			names = append(names, name)
		}
		slices.SortFunc(names, func(a, b string) int {
			if children[parent][a] != children[parent][b] {
				if children[parent][a] {
					return -1
				}
				return 1
			}
			return strings.Compare(a, b)
		})
		for n, name := range names {
			branch, next := "├─ ", "│  "
			if n == len(names)-1 {
				branch, next = "└─ ", "   "
			}
			directory := children[parent][name]
			open := directory && (s.FileExpanded[name] || s.Filter != "")
			label := path.Base(name)
			if directory {
				marker := "▸ "
				if open {
					marker = "▾ "
				}
				label = marker + label + "/"
			}
			rows = append(rows, CodeFileRow{Path: name, Label: prefix + branch + label, Tags: s.Inventory.DirectoryTags[name], Directory: directory, Open: open})
			if open {
				walk(name, prefix+next)
			}
		}
	}
	if s.Filter != "" {
		rows = nil
	}
	walk(root, "")
	return rows
}

func (s *CodeState) RevealFile(name string) {
	if s.FileExpanded == nil {
		s.FileExpanded = map[string]bool{}
	}
	for parent := path.Dir(name); parent != "." && parent != "/"; parent = path.Dir(parent) {
		s.FileExpanded[parent] = true
	}
}

func (s *CodeState) SelectedFileRow() (CodeFileRow, bool) {
	rows := s.FileRows()
	if s.Cursor < 0 || s.Cursor >= len(rows) {
		return CodeFileRow{}, false
	}
	return rows[s.Cursor], true
}

func (s *CodeState) FileTreeKey(key string) {
	row, ok := s.SelectedFileRow()
	if !ok {
		return
	}
	if row.Path == "." || row.Path == s.FileRoot {
		if key == "left" && s.FileRoot != "" {
			s.FileRoot = path.Dir(s.FileRoot)
			if s.FileRoot == "." {
				s.FileRoot = ""
			}
			s.Cursor = 0
		}
		if key == "right" {
			s.Cursor = min(1, len(s.FileRows())-1)
		}
		s.RememberSelection()
		return
	}
	switch key {
	case "enter":
		if row.Directory && s.Filter == "" {
			s.FileExpanded[row.Path] = !row.Open
		} else if row.Directory {
			s.Cursor = min(s.Cursor+1, len(s.FileRows())-1)
		}
	case "right":
		if row.Directory {
			if row.Open {
				s.Cursor = min(s.Cursor+1, len(s.FileRows())-1)
			} else {
				s.FileExpanded[row.Path] = true
			}
		}
	case "left":
		if row.Directory && row.Open && s.Filter == "" {
			s.FileExpanded[row.Path] = false
			break
		}
		parent := path.Dir(row.Path)
		if parent == s.FileRoot && s.FileRoot != "" {
			s.FileRoot = ""
		}
		for n, candidate := range s.FileRows() {
			if candidate.Path == parent {
				s.Cursor = n
				break
			}
		}
	}
	s.RememberSelection()
}

func (s *CodeState) FilesView(width, height int) string {
	treeWidth := width
	if width >= 66 {
		treeWidth = min(56, max(30, width/2))
	}
	rows := []string{}
	for n, file := range s.FileRows() {
		rows = append(rows, ui.MouseTarget(fileTreeLine(file, n == s.Cursor, treeWidth), fmt.Sprintf("code-row/%d", n)))
	}
	tree := ui.ScrollSelection(rows, s.Cursor, max(1, height-2))
	if len(rows) == 0 {
		tree = "No matching readable files."
	}
	if width < 66 {
		return tree
	}
	previewWidth := width - treeWidth - 3
	title, body := "SOURCE PREVIEW", "Choose a file to preview its captured contents. Enter opens Plous Editor."
	if file, ok := s.SelectedFileRow(); ok {
		title = file.Path
		if !file.Directory {
			lines := strings.SplitN(CodeFilePreview(s, file.Path), "\n", max(2, height-2))
			if len(lines) > height-3 {
				lines = lines[:max(1, height-3)]
			}
			for n, line := range lines {
				lines[n] = docedit.YAMLLine(ui.Clipped(fmt.Sprintf("%3d │ %s", n+1, line), previewWidth), ui.WorkspaceTheme())
			}
			body = strings.Join(lines, "\n") + "\n" + ui.Muted.Render(ui.Clipped("Captured preview · Enter opens the complete file", previewWidth))
		} else {
			body = "Folder: " + file.Path + "\n\nEnter or Right expands this folder.\nSelect a technology view to render a configuration."
		}
	}
	return CodeColumns(CodePane("FILES", tree, treeWidth, height, true), CodePane(title, body, previewWidth, height, false), treeWidth, previewWidth, height)
}
