package app

import (
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/app/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	textview "github.com/simjay/plous/internal/ui/text"
)

func (m *model) startResourceAction(item monitor.ResourceAction) {
	a := m.explore.Action
	if reason := a.Reasons[item.Id]; reason != "" {
		a.Err = reason
		return
	}
	a.Operation, a.Err, a.Note = item.Id, "", ""
	switch item.Id {
	case "copy":
		a.Pending = "copy"
	case "export":
		a.Stage = "export"
		a.ExportValue, a.ExportText = false, ""
		a.Input = textview.NewField(a.Object.GetName() + ".yaml")
	case "namespace":
		name := a.Object.GetName()
		m.stopResourceAction()
		m.chooseExploreNamespace(name)
	case "value":
		a.Values = monitor.ResourceValues(a.Resource, *a.Object)
		a.Stage, a.Cursor = "values", 0
	case "logs", "exec":
		a.Pending = "targets"
	default:
		a.Pending = "prepare"
	}
}

func (m *model) chooseActionPod() {
	a := m.explore.Action
	a.TargetChecked = false
	a.Containers = kubernetes.Containers(*a.Pod)
	if a.Operation == "exec" {
		a.Containers = kubernetes.RunningContainers(*a.Pod)
	}
	a.Cursor, a.Stage = 0, "containers"
	if len(a.Containers) == 1 {
		a.Container = a.Containers[0]
		m.chooseActionContainer()
	} else if a.Operation == "logs" {
		a.Containers = append([]string{monitor.AllApplicationContainers}, a.Containers...)
	}
}

func (m *model) chooseActionContainer() {
	a := m.explore.Action
	if !a.TargetChecked {
		a.Pending = "targetaccess"
		return
	}
	if a.Operation == "exec" {
		a.Stage = "execcommand"
		a.Input = textview.NewField("/bin/sh")
		return
	}
	resource, ok := monitor.ExactResource(a.Resources, "pods")
	if !ok {
		a.Err = "Pod API unavailable"
		return
	}
	m.openPodLogs(resource, a.Pod, a.Container)
}

func (m *model) backResourceAction() {
	a := m.explore.Action
	if a.Direct {
		m.stopResourceAction()
	} else {
		a.Stage, a.Err = "menu", ""
	}
}

func (m *model) updateResourceActionKey(msg tea.KeyPressMsg) tea.Cmd {
	a := m.explore.Action
	key := msg.String()
	if a.Stage == "valueview" && a.ValueDocument != nil {
		return m.valueDocumentKey(msg)
	}
	if a.Stage == "execcommand" && !a.Busy {
		return m.updateExecLaunch(msg)
	}
	if (a.Stage == "edit" || a.Stage == "review") && !a.Busy {
		return m.updateDocumentKey(msg)
	}
	if key == "esc" {
		if a.Busy {
			if a.Cancel != nil {
				a.Cancel()
			}
			a.Serial++
			a.Busy, a.Pending, a.Cancel = false, "", nil
			a.Err = "Request cancelled locally. A submitted change may already have reached the server; refresh before retrying."
			if a.Document != nil {
				if a.RunningJob == "apply" {
					a.FailDocument("Save result unknown", "Cancelled while saving. Check the live resource before retrying. Your draft is kept.")
				} else {
					a.FailDocument("Validation cancelled", "Nothing was saved. Your draft is kept.")
				}
			}
			return nil
		}
		switch a.Stage {
		case "confirm":
			if a.Operation == "scale" {
				a.Stage = "scale"
			} else {
				m.backResourceAction()
			}
		case "menu", "done":
			m.stopResourceAction()
		case "containers":
			if len(a.Pods) > 1 || a.AllPodsChoice() {
				a.Stage = "pods"
				a.Cursor = 0
			} else {
				m.backResourceAction()
			}
		case "export":
			if a.ExportValue {
				a.Stage, a.Cursor = "valueview", 0
			} else {
				m.backResourceAction()
			}
		default:
			m.backResourceAction()
		}
		return nil
	}
	if a.Busy {
		return nil
	}
	if a.Stage == "menu" {
		for _, item := range a.Items {
			if key == item.Alias {
				m.startResourceAction(item)
				return nil
			}
		}
	}
	switch a.Stage {
	case "scale", "export":
		if key != "enter" {
			a.Input.Key(msg, false)
			return nil
		}
		switch a.Stage {
		case "scale":
			count, err := strconv.ParseInt(strings.TrimSpace(a.Input.Text), 10, 32)
			if err != nil || count < 0 {
				a.Err = "Enter a non-negative whole replica count."
				return nil
			}
			a.Replicas, a.Stage, a.Cursor, a.Err = count, "confirm", 0, ""
		case "export":
			if strings.TrimSpace(a.Input.Text) != "" {
				a.Pending = "export"
			}
		}
	case "menu", "pods", "containers", "values":
		count := len(a.Items)
		if a.Stage == "pods" {
			count = len(a.Pods)
			if a.AllPodsChoice() {
				count++
			}
		}
		if a.Stage == "containers" {
			count = len(a.Containers)
		}
		if a.Stage == "values" {
			count = len(a.Values)
		}
		switch key {
		case "up", "k":
			a.Cursor = max(0, a.Cursor-1)
		case "down", "j":
			a.Cursor = min(max(0, count-1), a.Cursor+1)
		case "enter":
			if count == 0 {
				return nil
			}
			a.Cursor = min(a.Cursor, count-1)
			switch a.Stage {
			case "menu":
				m.startResourceAction(a.Items[a.Cursor])
			case "pods":
				index := a.Cursor
				if a.AllPodsChoice() {
					if index == 0 {
						m.openWorkloadLogs()
						return nil
					}
					index--
				}
				a.Pod = a.Pods[index].DeepCopy()
				m.chooseActionPod()
			case "containers":
				a.Container = a.Containers[a.Cursor]
				if a.Operation == "logs" && a.Container == monitor.AllApplicationContainers {
					a.Container = ""
				}
				m.chooseActionContainer()
			case "values":
				a.OpenValueDocument(a.Values[a.Cursor])
			}
		}
	case "confirm":
		switch key {
		case "left", "right":
			a.Cursor = 1 - a.Cursor
		case "up", "k":
			a.Scroll = max(0, a.Scroll-1)
		case "down", "j":
			a.Scroll++
		case "pgup":
			a.Scroll = max(0, a.Scroll-10)
		case "pgdown":
			a.Scroll += 10
		case "enter":
			if a.Cursor == 0 {
				m.backResourceAction()
			} else {
				a.Pending = a.Operation

			}
		}
	case "done":
		switch key {
		case "enter":
			m.stopResourceAction()
		case "up", "k":
			a.Scroll = max(0, a.Scroll-1)
		case "down", "j":
			a.Scroll++
		case "pgup":
			a.Scroll = max(0, a.Scroll-10)
		case "pgdown":
			a.Scroll += 10
		case "home", "g":
			a.Scroll = 0
			if key == "g" && a.Progress != nil && !a.Progress.Checking && a.Progress.Status.State != "complete" {
				a.Progress.Status = kubernetes.OperationProgress{State: "waiting", Summary: "Checking current status"}
				a.Progress.Deadline, a.Progress.Next = time.Now().Add(monitor.OperationObservationLimit), time.Now()
			}
		case "end", "G":
			a.Scroll = 1 << 20
		}

	}
	return nil
}
