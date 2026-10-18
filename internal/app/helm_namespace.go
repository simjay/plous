package app

import (
	"context"
	"sort"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/app/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
)

func (m *model) openHelmNamespacePicker(h *monitor.HelmState) {
	h.StopNamespacePicker()
	m.helmInput(h, "namespace", "")
	h.Namespaces = &monitor.NamespacePicker{}
	h.PickerCursor = 0
	if h.NamespaceScope() != "" {
		h.PickerCursor = 1
	}
	m.status = ""
}

func (m *model) syncHelmNamespaces(h *monitor.HelmState) tea.Cmd {
	p := h.Namespaces
	if p == nil || p.Loaded || p.Run != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	run := &monitor.NamespaceRequest{Cancel: cancel}
	p.Run = run
	config := h.Config
	return func() tea.Msg {
		defer cancel()
		r := monitor.HelmNamespaceResult{State: h, Run: run}
		client, err := kubernetes.NewTarget(config.Target, "")
		if err != nil {
			r.Err = err
			return r
		}
		collection, err := client.List(ctx, kubecommon.Resource{Name: "namespaces", Kind: "Namespace", Version: "v1"}, "", "")
		r.Err = err
		for _, object := range collection.Objects {
			r.Names = append(r.Names, object.GetName())
		}
		return r
	}
}

func (m *model) updateHelmNamespaceResult(r monitor.HelmNamespaceResult) {
	if r.State == nil || r.State.Namespaces == nil || r.State.Namespaces.Run != r.Run {
		return
	}
	h, p := r.State, r.State.Namespaces
	p.Run, p.Loaded, p.Err = nil, true, kubernetes.ErrorText(r.Err)
	p.Names = r.Names
	sort.Strings(p.Names)
	h.PickerCursor = min(h.PickerCursor, len(monitor.NamespaceChoicesFor(p, h.Input, h.NamespaceScope()))-1)
}

func (m *model) chooseHelmNamespace(h *monitor.HelmState, value string) {
	h.StopNamespacePicker()
	if value == "" {
		value = "*"
	}
	h.Preference.Namespace = value
	h.Preference.Selected = ""
	h.Err, h.Notice = "", ""
	h.Loaded = false
	h.Releases, h.Positions = nil, nil
	h.Detail = nil
	h.StopTimelineLoad()
	h.Timeline = nil
	h.Find = nil
	h.Cursor, h.Subcursor = 0, 0
	h.Input, h.Menu = nil, nil
	m.status = ""
	m.rememberHelm(h)
}

func (m *model) updateHelmNamespaceInput(h *monitor.HelmState, msg tea.KeyPressMsg) {
	value, apply, close, status := monitor.UpdateNamespacePicker(h.Namespaces, &h.Input, &h.PickerCursor, h.NamespaceScope(), msg)
	m.status = status
	if apply {
		m.chooseHelmNamespace(h, value)
	} else if close {
		h.StopNamespacePicker()
		h.Input = nil
	}
}
