package tui

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/simjay/plous/internal/app/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/ui"
	diffview "github.com/simjay/plous/internal/ui/diff"
	textview "github.com/simjay/plous/internal/ui/text"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func actionCommand(command string) string {
	if canonicalCommand(command) == "actions" {
		return "menu"
	}
	for _, item := range monitor.ResourceActionDefinitions {
		if command == item.Alias || command == item.Command {
			return item.Id
		}
	}
	return ""
}

func (m *model) openResourceActions(operation string) {
	e := m.explorer()
	if e.Action != nil {
		m.status = "Finish or cancel the open action first."
		return
	}
	if e.Detail == nil {
		m.status = "Open a resource's details first to use its actions."
		return
	}
	if !e.DetailValid || e.Client == nil {
		m.status = "Wait for the current object to load, or refresh its details."
		return
	}
	a := &monitor.ResourceActionState{Resource: e.Resource, Object: e.Detail.DeepCopy(), Client: e.Client, Resources: slices.Clone(e.Resources), Stage: "menu", Pending: "permissions", Reasons: map[string]string{}}
	if c := m.currentConnection(); c != nil {
		a.Context = c.Settings["context"]
	}
	a.Items = monitor.ActionsFor(a.Resource, a.Object)
	if operation != "" && operation != "menu" {
		found := false
		for i, item := range a.Items {
			if item.Id == operation {
				a.Cursor = i
				found = true
			}
		}
		if !found {
			m.status = "That action does not apply to this resource."
			return
		}
		a.Requested = true
		a.Direct = true
	}
	e.Action = a
	m.shortcutPrefix, m.status = "", ""
}

func (m *model) stopResourceAction() { m.explore.StopAction() }

func (m *model) resourceActionVisible() bool {
	return m.exploreVisible() && m.explore.Action != nil && m.focus == 1 && !m.helpVisible() && m.settingsOverlay == nil && m.command == nil
}

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

func (m *model) syncResourceAction() tea.Cmd {
	if m.explore == nil || m.explore.Action == nil {
		return nil
	}
	a := m.explore.Action
	if a.Stage == "done" && a.Progress != nil {
		return m.syncResourceProgress(a)
	}
	if a.Busy || a.Pending == "" {
		return nil
	}
	job := a.Pending
	if job == "validate" && a.Document == nil {
		a.Pending = ""
		a.Err = "Open the document before validating."
		return nil
	}
	if job == "apply" && (a.Document == nil || a.Proposed == nil || !a.SaveRequested || a.SaveRevision != a.Document.Revision || a.ValidatedRevision != a.Document.Revision || a.ConfirmedID != "" && (a.Review == nil || a.ConfirmedID != a.Review.ID || a.Review.Stale)) {
		a.FailDocument("Save stopped", "The draft changed before saving. Check it and use :w again.")
		return nil
	}
	a.Pending, a.Busy, a.Err = "", true, ""
	a.RunningJob = job
	a.Serial++
	serial := a.Serial
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	a.Cancel = cancel
	r, object, proposed, scale, replicas, input := a.Resource, a.Object.DeepCopy(), a.Proposed.DeepCopy(), a.Scale.DeepCopy(), a.Replicas, a.Input.Text
	revision := uint64(0)
	if job == "validate" {
		input, revision = a.Document.Text(), a.Document.Revision
	}
	client, resources := a.Client, slices.Clone(a.Resources)
	pod := a.Pod.DeepCopy()
	items, contextName, operation := slices.Clone(a.Items), a.Context, a.Operation
	exportText, exportValue := a.ExportText, a.ExportValue
	if job == "copy" {
		cancel()
		a.Busy = false
		a.Note = "Identity copy requested."
		return m.copyWorkspaceText(contextName + " / " + r.Key() + " / " + object.GetNamespace() + " / " + object.GetName() + " / " + string(object.GetUID()))
	}
	return func() tea.Msg {
		defer cancel()
		result := monitor.ResourceActionResult{Action: a, Serial: serial, Job: job, Revision: revision}
		switch job {
		case "permissions":
			result.Reasons = map[string]string{}
			for _, item := range items {
				if item.Verb == "" {
					continue
				}
				target, ns, name := r, object.GetNamespace(), object.GetName()
				if item.Id == "logs" || item.Id == "exec" {
					var ok bool
					target, ok = monitor.ExactResource(resources, "pods")
					if !ok {
						result.Reasons[item.Id] = "Pod API unavailable"
						continue
					}
					if r.Key() != "pods" {
						// RBAC can restrict access to named Pods. Check the chosen
						// descendant instead of treating a broad denial as final.
						if !target.Supports(item.Verb, item.Subresource) {
							result.Reasons[item.Id] = "API does not advertise this operation"
						}
						continue
					}
				}
				if !target.Supports(item.Verb, item.Subresource) {
					result.Reasons[item.Id] = "API does not advertise this operation"
					continue
				}
				access, err := client.CheckAccess(ctx, target, ns, name, item.Verb, item.Subresource)
				if err != nil || !access.Known {
					result.PermissionUnknown = true
				}
				if err == nil && access.Known && !access.Allowed {
					result.Reasons[item.Id] = "Permission denied: " + access.Reason
				}
			}
		case "prepare":
			result.Object, result.Err = client.Get(ctx, r, object.GetNamespace(), object.GetName())
			if result.Err == nil && (result.Object.GetUID() != object.GetUID() || object.GetUID() == "") {
				result.Err = kubernetes.TargetChangedError()
			}
			if result.Err != nil {
				break
			}
			if operation == "delete" && slices.Contains([]string{"persistentvolumeclaims", "persistentvolumes", "volumesnapshots.snapshot.storage.k8s.io", "volumesnapshotcontents.snapshot.storage.k8s.io"}, r.Key()) {
				result.Note = client.StorageDeleteNotice(ctx, r, result.Object, resources)
			}
			if operation == "scale" {
				result.Scale, result.Err = client.GetScale(ctx, r, result.Object)
				if hpa, ok := monitor.ExactResource(resources, "horizontalpodautoscalers.autoscaling"); ok {
					collection, err := client.List(ctx, hpa, object.GetNamespace(), "")
					if err != nil {
						result.Note = "Autoscaler lookup unavailable. An HPA may override manual replicas."
					} else {
						for _, autoscaler := range collection.Objects {
							target, _, _ := unstructured.NestedMap(autoscaler.Object, "spec", "scaleTargetRef")
							if kubernetes.Value(target, "name") == object.GetName() && kubernetes.Value(target, "kind") == r.Kind && monitor.ApiGroup(kubernetes.Value(target, "apiVersion")) == r.Group {
								result.Note += "HPA " + autoscaler.GetName() + " controls this target and may override replicas.\n"
							}
						}
						if collection.Truncated {
							result.Note += "Autoscaler lookup is incomplete. Other HPAs may override manual replicas.\n"
						}
					}
				}
			}
		case "targets":
			result.Pods, result.Err = client.PodTargets(ctx, r, object, resources)
		case "targetaccess":
			target, ok := monitor.ExactResource(resources, "pods")
			if !ok {
				result.Err = kubernetes.TargetChangedError()
				break
			}
			verb, subresource := "get", "log"
			if operation == "exec" {
				verb, subresource = "create", "exec"
			}
			access, err := client.CheckAccess(ctx, target, pod.GetNamespace(), pod.GetName(), verb, subresource)
			if err == nil && access.Known && !access.Allowed {
				result.Err = apierrors.NewForbidden(target.GVR().GroupResource(), pod.GetName(), nil)
			}
		case "validate":
			result.Object, result.Err = kubernetes.ParseManifest(r, object, input)
			if result.Err == nil {
				result.Object, result.Err = client.ValidateEdit(ctx, r, object, result.Object)
			}
		case "apply":
			result.Object, result.Err = client.ApplyEdit(ctx, r, object, proposed)
		case "delete":
			result.Err = client.Delete(ctx, r, object)
		case "scale":
			result.Object, result.Err = client.Scale(ctx, r, object, scale, replicas)
		case "restart":
			result.Object, result.Err = client.Restart(ctx, r, object)
		case "export":
			path, err := filepath.Abs(strings.TrimSpace(input))
			if err == nil {
				var file *os.File
				file, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
				if err == nil {
					contents := kubernetes.YAML(*object, false)
					if exportValue {
						contents = exportText
					}
					_, err = file.WriteString(contents)
					closeErr := file.Close()
					if err == nil {
						err = closeErr
					}
				}
			}
			result.Err = err
			if err == nil {
				result.Note = "Exported YAML to " + path
				if exportValue {
					result.Note = "Exported selected value to " + path
				}
				if r.Key() == "secrets" {
					result.Note += " (values redacted)"
				}
			}
		}
		return result
	}
}

func (m *model) updateResourceActionResult(msg monitor.ResourceActionResult) {
	if m.explore == nil || m.explore.Action != msg.Action || msg.Serial != msg.Action.Serial {
		return
	}
	a := msg.Action
	a.Busy, a.Cancel, a.RunningJob = false, nil, ""
	if msg.Err != nil {
		a.Err = kubernetes.OperationErrorText(msg.Err)
		if a.Document != nil {
			title := "Validation failed"
			if a.SaveRequested || msg.Job == "apply" {
				title = "Save failed"
			}
			if msg.Job == "apply" && !(apierrors.IsConflict(msg.Err) || apierrors.IsInvalid(msg.Err) || apierrors.IsBadRequest(msg.Err) || apierrors.IsForbidden(msg.Err) || apierrors.IsUnauthorized(msg.Err) || apierrors.IsNotFound(msg.Err) || apierrors.IsMethodNotSupported(msg.Err)) {
				title = "Save result unknown"
				a.Err = "Check the live resource before retrying. " + a.Err
			}
			a.FailDocument(title, a.Err)
		}
		if msg.Job == "export" {
			a.Err = "Could not create the export. Choose a writable path to a file that does not exist."
		}
		return
	}
	switch msg.Job {
	case "permissions":
		a.Reasons = msg.Reasons
		a.PermissionUnknown = msg.PermissionUnknown
		if a.Requested {
			a.Requested = false
			m.startResourceAction(a.Items[a.Cursor])
		}
	case "prepare":
		a.Object, a.Scale, a.Note = msg.Object, msg.Scale, msg.Note
		switch a.Operation {
		case "edit":
			a.Stage = "edit"
			a.InitDocument()
		case "scale":
			a.Stage = "scale"
			a.Input = textview.NewField(kubernetes.Value(a.Scale.Object, "spec", "replicas"))
		default:
			a.Stage = "confirm"
			a.Cursor = 0
		}
	case "targets":
		a.Pods = msg.Pods
		a.Cursor = 0
		a.Stage = "pods"
		if a.Operation == "exec" {
			a.Pods = slices.DeleteFunc(a.Pods, func(o unstructured.Unstructured) bool { return len(kubernetes.RunningContainers(o)) == 0 })
		}
		if len(a.Pods) == 1 && !a.AllPodsChoice() {
			a.Pod = a.Pods[0].DeepCopy()
			m.chooseActionPod()
		}
	case "validate":
		if a.Document == nil || msg.Revision != a.Document.Revision {
			a.FailDocument("Validation stopped", "The draft changed during validation. Check it and try again.")
			return
		}
		a.Proposed = msg.Object
		a.ValidatedRevision = msg.Revision
		a.ConfirmedID = ""
		if a.SaveRequested {
			a.Pending = "apply"
			return
		}
		a.Review = diffview.New(kubernetes.YAML(*a.Object, true), kubernetes.YAML(*msg.Object, true), a.Document.Target, msg.Revision, true)
		a.Review.Name = a.Document.Name
		a.Review.BeforeLabel, a.Review.AfterLabel = "Current in Kubernetes", "Changes to save"
		a.Review.BackLabel, a.Review.ConfirmLabel = "Back to editor", "Save"
		a.Review.ConfirmKey = ":w"
		a.Stage = "review"
		a.Cursor, a.Scroll = 0, 0
	case "targetaccess":
		a.TargetChecked = true
		m.chooseActionContainer()
	case "apply", "scale", "restart", "delete":
		if msg.Job == "apply" && a.Document != nil && msg.Object != nil {
			a.Document.MarkSaved(kubernetes.YAML(*msg.Object, true))
			a.Document.SavedStatus = "Saved to Kubernetes"
			a.Document.Notice = ""
			a.Document.Diagnostic = ""
			a.SaveRequested = false
			a.ConfirmedID, a.DocumentErrorTitle = "", ""
			a.Object = msg.Object.DeepCopy()
			m.explore.Detail = msg.Object.DeepCopy()
			m.refreshExplore()
			if a.CloseAfterApply {
				m.status = "Saved to Kubernetes: " + a.Document.Name
				m.stopResourceAction()
			} else {
				a.Stage, a.Review, a.Proposed = "edit", nil, nil
				a.CloseAfterApply = false
			}
			return
		}
		a.Stage = "done"
		a.Note = map[string]string{"apply": "Saved to Kubernetes. Check the resource's reported state for readiness.", "scale": "Scale request accepted.", "restart": "Pod template updated.", "delete": "Deletion requested."}[msg.Job]
		if msg.Job != "apply" {
			a.Progress = &monitor.ResourceProgress{
				Target:   kubernetes.NewOperationTarget(a.Resource, msg.Job, a.Object, msg.Object, a.Replicas),
				Status:   kubernetes.OperationProgress{State: "waiting", Summary: "Checking current status"},
				Deadline: time.Now().Add(monitor.OperationObservationLimit),
			}
			a.Scroll = 0
		}
		m.refreshExplore()
	case "export":
		a.Stage = "done"
		a.Note = msg.Note
	}
}

func (m *model) syncResourceProgress(a *monitor.ResourceActionState) tea.Cmd {
	p := a.Progress
	if p == nil || p.Checking || !p.Status.Watching() {
		return nil
	}
	if !time.Now().Before(p.Deadline) {
		p.Status = kubernetes.OperationProgress{State: "attention", Summary: "Still waiting after 5 minutes", Details: p.Status.Details + "\nAutomatic checks stopped. This does not mean Kubernetes stopped the operation."}
		return nil
	}
	p.Checking = true
	p.Serial++
	serial, target, client, delay := p.Serial, p.Target, a.Client, max(time.Duration(0), time.Until(p.Next))
	ctx, cancel := context.WithCancel(context.Background())
	a.Cancel = cancel
	return func() tea.Msg {
		defer cancel()
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
		}
		request, stop := context.WithTimeout(ctx, 10*time.Second)
		defer stop()
		object, err := client.Get(request, target.Resource, target.Object.GetNamespace(), target.Object.GetName())
		return monitor.ResourceProgressResult{Action: a, Progress: p, Serial: serial, Status: target.Progress(object, err)}
	}
}

func (m *model) updateResourceProgress(msg monitor.ResourceProgressResult) {
	if m.explore == nil || m.explore.Action != msg.Action || msg.Action.Progress != msg.Progress || msg.Serial != msg.Progress.Serial {
		return
	}
	p := msg.Progress
	p.Checking, p.Status, p.Next = false, msg.Status, time.Now().Add(2*time.Second)
	msg.Action.Cancel = nil
	if !p.Status.Watching() {
		m.refreshExplore()
	}
}

func (m *model) updateExecLaunch(msg tea.KeyPressMsg) tea.Cmd {
	a := m.explore.Action
	key := msg.String()
	if key == "esc" {
		if a.ExecEditing {
			a.ExecEditing = false
			return nil
		}
		if a.PreviousTerminal != nil {
			m.explore.Terminal = a.PreviousTerminal
			a.PreviousTerminal = nil
			m.stopResourceAction()
		} else {
			a.Stage, a.Cursor = "containers", 0
		}
		return nil
	}
	if a.ExecEditing {
		if key == "enter" {
			a.ExecEditing = false
			a.ExecChoice = 3
		} else {
			a.Input.Key(msg, false)
		}
		return nil
	}
	switch key {
	case "up", "k":
		a.ExecChoice = max(0, a.ExecChoice-1)
	case "down", "j":
		a.ExecChoice = min(4, a.ExecChoice+1)
	case "enter":
		switch a.ExecChoice {
		case 0:
			a.Input = textview.NewField("/bin/sh")
			a.ExecChoice = 3
		case 1:
			a.Input = textview.NewField("/bin/bash")
			a.ExecChoice = 3
		case 2:
			a.ExecEditing = true
		case 3:
			return m.startResourceTerminal(a)
		case 4:
			return m.updateExecLaunch(tea.KeyPressMsg{Code: tea.KeyEscape})
		}
	}
	return nil
}

func (m *model) execLaunchBody(width, height int) string {
	a := m.explore.Action
	title := ui.TitleStyle.Render(ui.Clipped("Exec · "+a.Pod.GetName()+" / "+a.Container, width))
	choices := []string{"sh (/bin/sh)", "bash (/bin/bash)", "Custom command", "Start", "Cancel"}
	buttons := []textview.Action{}
	for i, label := range choices {
		button := textview.Action{Mouse: "execaction/" + strconv.Itoa(i), Label: label, Focused: !a.ExecEditing && i == a.ExecChoice, Primary: i == 3}
		if button.Focused {
			button.Key = "Enter"
		}
		buttons = append(buttons, button)
	}
	fieldBorder := ui.BorderColor
	if a.ExecEditing {
		fieldBorder = ui.FocusedBorder
	}
	field := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(fieldBorder).Width(width).MaxWidth(width).Render(ui.Muted.Render("Command") + "\n" + ui.EditorLine(a.Input, width-2, a.ExecEditing, "execfield"))
	choicesView := textview.ActionBar([]textview.ActionGroup{{Label: "Shell", Actions: buttons[:3]}}, width, 3, ui.WorkspaceTheme())
	startView := textview.ActionBar([]textview.ActionGroup{{Label: "Session", Actions: buttons[3:]}}, width, 2, ui.WorkspaceTheme())
	scope := ui.Muted.Render(ui.Clipped(a.Context+" · "+a.Pod.GetNamespace(), width))
	rows := []string{title, scope, choicesView, ui.MouseTarget(field, "execfield"), startView}
	if a.Err != "" {
		rows = append(rows, ui.ChangedStyle.Render(ui.Clipped(a.Err, width)))
	}
	if lipgloss.Height(strings.Join(rows, "\n")) < height {
		rows = append(rows, ui.Muted.Render(ui.Clipped("Shell availability depends on the image.", width)))
	}
	if lipgloss.Height(strings.Join(rows, "\n")) < height {
		rows = append(rows, ui.Muted.Render(ui.Clipped("Ctrl+] review · Ctrl+C exits Plous", width)))
	}
	return textview.Fit(strings.Join(rows, "\n"), width, height)
}

func (m *model) renderExecLaunchOverlay(base string) string {
	width, height := min(110, m.width-6), min(18, m.height-2)
	innerWidth, bodyHeight := width-4, height-3
	body := m.execLaunchBody(innerWidth, bodyHeight)
	footer := "↑↓ choose · Enter select/start · Esc back"
	if m.explore.Action.ExecEditing {
		footer = "Enter finish command · Esc leave field"
	}
	content := lipgloss.NewStyle().Height(bodyHeight).MaxHeight(bodyHeight).Render(body) + "\n" + ui.Muted.Render(ui.Clipped(footer, innerWidth))
	window := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ui.Accent).Background(ui.Surface).Foreground(ui.Foreground).Padding(0, 1).Width(width).Height(height).MaxHeight(height).Render(content)
	return ui.CenteredOverlay(base, window)
}
