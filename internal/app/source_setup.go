package app

import (
	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/source"
	textview "github.com/simjay/plous/internal/ui/text"
	"strings"
)

func (m *model) updateSourceSetup(msg tea.KeyPressMsg) bool {
	f := m.form
	if f.Directory != nil {
		m.updateDirectoryPicker(msg)
		return true
	}
	if !f.LocalDirectory() || msg.String() != "enter" {
		return false
	}
	n := len(f.Fields)
	switch f.Index {
	case 2:
		if strings.TrimSpace(f.Fields[2].Value.Text) == "" {
			m.openDirectoryPicker()
		} else if m.validateLocalDirectory() {
			f.Index = n + 1
		}
	case n:
		m.openDirectoryPicker()
	case n + 1:
		m.submitForm()
	case n + 2:
		m.cancelForm()
	default:
		return false
	}
	return true
}

func (m *model) validateLocalDirectory() bool {
	f := m.form
	resolved, err := source.ResolveDirectory(f.Fields[2].Value.Text, f.Source.Base)
	if err == nil {
		err = source.CheckDirectory(resolved)
	}
	if err != nil {
		f.Source.Err, m.status, f.Index = err.Error(), err.Error(), 2
		return false
	}
	f.Fields[2].Value = textview.NewField(resolved)
	f.Source.Err = ""
	return true
}
