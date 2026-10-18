package settings

import (
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/source"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type DirectoryPicker struct {
	Path, Pending, Err, Note string
	Input                    textview.Field
	Editing, Loading         bool
	Entries                  []string
	Cursor                   int
	Run                      uint64
}

type DirectoryResult struct {
	Picker     *DirectoryPicker
	Run        uint64
	Path, Note string
	Names      []string
	Err        error
}

func (p *DirectoryPicker) Identity() string {
	return fmt.Sprintf("%p/%s/%d/%t/%t", p, p.Path, p.Run, p.Loading, p.Editing)
}

func (f *Form) OpenDirectory() {
	path, err := source.ResolveDirectory(f.Fields[2].Value.Text, f.Source.Base)
	if err != nil || source.CheckDirectory(path) != nil {
		path = f.Source.Base
	}
	f.Index = len(f.Fields)
	f.Directory = &DirectoryPicker{Pending: path, Input: textview.NewField(path)}
}

func (f *Form) SyncDirectory() tea.Cmd {
	if f == nil || f.Directory == nil {
		return nil
	}
	p := f.Directory
	if p.Pending == "" || p.Loading {
		return nil
	}
	path := p.Pending
	p.Pending, p.Loading, p.Err = "", true, ""
	p.Run++
	run := p.Run
	return func() tea.Msg {
		r := DirectoryResult{Picker: p, Run: run, Path: path}
		file, err := os.Open(path)
		if err != nil {
			r.Err = err
			return r
		}
		defer file.Close()
		visited := 0
		for {
			entries, err := file.ReadDir(256)
			for _, entry := range entries {
				visited++
				isDir := entry.IsDir()
				if entry.Type()&os.ModeSymlink != 0 {
					if info, err := os.Stat(filepath.Join(path, entry.Name())); err == nil {
						isDir = info.IsDir()
					}
				}
				if isDir {
					r.Names = append(r.Names, entry.Name())
				}
			}
			if err != nil && err != io.EOF {
				r.Err = err
				return r
			}
			if err == io.EOF {
				break
			}
			if visited >= 10000 {
				r.Note = "Large directory: showing the first 10,000 entries. Use / to enter a path."
				break
			}
		}
		sort.Strings(r.Names)
		return r
	}
}

func (f *Form) ApplyDirectoryResult(r DirectoryResult) {
	if f == nil || f.Directory != r.Picker || r.Run != r.Picker.Run {
		return
	}
	p := r.Picker
	p.Loading = false
	if r.Err != nil {
		p.Err = ui.Plain(r.Err.Error())
		return
	}
	p.Path, p.Entries, p.Note, p.Err = r.Path, r.Names, r.Note, ""
	p.Input, p.Editing, p.Cursor = textview.NewField(r.Path), false, 0
}

func (f *Form) UpdateDirectory(msg tea.KeyPressMsg) {
	key := msg.String()
	p := f.Directory
	if p.Editing {
		switch key {
		case "esc":
			p.Editing = false
			p.Input = textview.NewField(p.Path)
		case "enter":
			base := p.Path
			if base == "" {
				base = f.Source.Base
			}
			path, err := source.ResolveDirectory(p.Input.Text, base)
			if err != nil {
				p.Err = err.Error()
			} else if !p.Loading {
				p.Pending = path
			}
		default:
			p.Input.Key(msg, false)
		}
		return
	}
	if key == "esc" {
		f.Directory = nil
		return
	}
	if p.Loading {
		return
	}
	switch key {
	case "/":
		p.Editing = true
	case "left", "backspace":
		if p.Path != "" {
			p.Pending = filepath.Dir(p.Path)
		}
	case "enter", "right":
		switch p.Cursor {
		case 0:
			if key == "enter" && p.Path != "" {
				f.Fields[2].Value = textview.NewField(p.Path)
				f.Source.Err, f.Directory, f.Index = "", nil, len(f.Fields)+1
			}
		case 1:
			if p.Path != "" {
				p.Pending = filepath.Dir(p.Path)
			}
		default:
			if i := p.Cursor - 2; i >= 0 && i < len(p.Entries) {
				p.Pending = filepath.Join(p.Path, p.Entries[i])
			}
		}
	default:
		p.Cursor = ui.MoveCursor(p.Cursor, len(p.Entries)+2, key)
	}
}

func (f *Form) DirectoryBody(width, height int) string {
	p := f.Directory
	header := ui.SectionHeadingStyle.Render("Choose Local Directory") + "\n"
	compact := height < 10
	if p.Editing {
		header += ui.SearchField(p.Input, width, true, "Directory path", "/path, ~/folder, or relative path", "Enter open · Esc cancel typing")
	} else {
		path := p.Path
		if path == "" {
			path = p.Input.Text
		}
		path = ui.Plain(path)
		if compact {
			path = ui.Clipped(path, width)
		} else {
			lines := strings.Split(ansi.Wrap(path, width, ""), "\n")
			path = strings.Join(lines[:min(2, len(lines))], "\n")
		}
		header += ui.MouseTarget(ui.Muted.Render(path), "directory/path")
	}
	if p.Loading {
		header += "\n" + ui.Muted.Render("Reading folders…")
	}
	if p.Err != "" {
		header += "\n" + ui.RemovedStyle.Render(ui.Clipped(p.Err, width))
	}
	if p.Note != "" {
		header += "\n" + ui.Muted.Render(ansi.Wrap(p.Note, width, ""))
	}
	if !compact {
		header += "\n" + ui.Muted.Render("Enter open/select · ← parent · / type path · Esc back")
	}
	if compact && p.Editing {
		return header
	}
	labels := []string{"[ Use This Directory ]", "../  Parent Directory"}
	for _, name := range p.Entries {
		labels = append(labels, ui.Plain(name)+"/")
	}
	var rows []string
	for i, label := range labels {
		rows = append(rows, ui.MouseTarget(ui.SettingsChoiceLine(label, p.Cursor == i, width), "directory/"+strconv.Itoa(i)))
	}
	return header + "\n" + ui.ScrollSelection(rows, p.Cursor, max(1, height-lipgloss.Height(header)-1))
}
