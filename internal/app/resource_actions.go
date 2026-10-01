package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/diffview"
	docedit "github.com/simjay/plous/internal/editor"
	"github.com/simjay/plous/internal/kubernetes"
	"github.com/simjay/plous/internal/textview"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type resourceAction struct {
	id, label, alias, command, verb, subresource string
}

type resourceValue struct{ label, text string }

func resourceValues(r kubernetes.Resource, object unstructured.Unstructured) []resourceValue {
	var values []resourceValue
	if r.Key() == "controllerrevisions.apps" {
		payload, found, _ := unstructured.NestedFieldCopy(object.Object, "data")
		if found {
			if text, err := json.MarshalIndent(payload, "", "  "); err == nil {
				values = append(values, resourceValue{"Revision snapshot", string(text)})
			}
		}
		return values
	}
	if r.Key() == "certificatesigningrequests.certificates.k8s.io" {
		for _, field := range []struct{ section, name string }{{"spec", "request"}, {"status", "certificate"}} {
			encoded, _, _ := unstructured.NestedString(object.Object, field.section, field.name)
			if encoded == "" {
				continue
			}
			decoded, err := base64.StdEncoding.DecodeString(encoded)
			if err == nil && utf8.Valid(decoded) {
				values = append(values, resourceValue{field.name, string(decoded)})
			}
		}
		return values
	}
	for _, field := range []string{"data", "binaryData", "stringData"} {
		entries, _, _ := unstructured.NestedStringMap(object.Object, field)
		keys := make([]string, 0, len(entries))
		for key := range entries {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			value, label := entries[key], field+" / "+key
			if field == "binaryData" || field == "data" && r.Key() == "secrets" {
				decoded, err := base64.StdEncoding.DecodeString(value)
				if err == nil && utf8.Valid(decoded) {
					value = string(decoded)
				} else {
					label += " (base64)"
				}
			}
			values = append(values, resourceValue{label, value})
		}
	}
	return values
}

var resourceActionDefinitions = []resourceAction{
	{"logs", "Logs", "l", "logs", "get", "log"},
	{"exec", "Exec", "x", "exec", "create", "exec"},
	{"scale", "Scale", "S", "scale", "update", "scale"},
	{"restart", "Restart rollout", "R", "restart", "patch", ""},
	{"edit", "Edit YAML", "E", "edit", "update", ""},
	{"delete", "Delete", "D", "delete", "delete", ""},
	{"namespace", "Use namespace", "u", "usenamespace", "", ""},
	{"copy", "Copy identity", "Y", "copyidentity", "", ""},
	{"value", "Inspect/copy value", "V", "copyvalue", "", ""},
	{"export", "Export YAML", "w", "exportyaml", "", ""},
}

type resourceActionState struct {
	valueDocument                                 *docedit.Model
	document                                      *docedit.Model
	review                                        *diffview.Model
	diagnostic                                    *textview.Viewer
	validatedRevision                             uint64
	confirmedID                                   string
	saveRequested                                 bool
	saveRevision                                  uint64
	runningJob, documentErrorTitle                string
	yank                                          string
	documentAction                                int
	discard                                       bool
	closeAfterApply                               bool
	execChoice                                    int
	execEditing                                   bool
	previousTerminal                              *resourceTerminal
	progress                                      *resourceProgress
	resource                                      kubernetes.Resource
	object, proposed, scale                       *unstructured.Unstructured
	client                                        *kubernetes.Client
	resources                                     []kubernetes.Resource
	context, stage, operation, pending, err, note string
	items                                         []resourceAction
	reasons                                       map[string]string
	input                                         editor
	cursor, scroll                                int
	busy                                          bool
	cancel                                        context.CancelFunc
	serial                                        uint64
	pods                                          []unstructured.Unstructured
	pod                                           *unstructured.Unstructured
	containers                                    []string
	container                                     string
	replicas                                      int64
	requested                                     bool
	direct                                        bool
	targetChecked                                 bool
	permissionUnknown                             bool
	values                                        []resourceValue
	exportText                                    string
	exportValue                                   bool
}

type resourceActionResult struct {
	revision          uint64
	action            *resourceActionState
	serial            uint64
	job               string
	object, scale     *unstructured.Unstructured
	pods              []unstructured.Unstructured
	reasons           map[string]string
	note              string
	permissionUnknown bool
	err               error
}

func actionCommand(command string) string {
	if canonicalCommand(command) == "actions" {
		return "menu"
	}
	for _, item := range resourceActionDefinitions {
		if command == item.alias || command == item.command {
			return item.id
		}
	}
	return ""
}

func managedResource(r kubernetes.Resource) bool {
	return slices.Contains([]string{"events", "events.events.k8s.io", "leases.coordination.k8s.io", "leasecandidates.coordination.k8s.io", "csinodes.storage.k8s.io", "csistoragecapacities.storage.k8s.io", "volumeattachments.storage.k8s.io", "resourceslices.resource.k8s.io", "endpointslices.discovery.k8s.io", "controllerrevisions.apps", "volumesnapshotcontents.snapshot.storage.k8s.io"}, r.Key())
}

func (m *model) resourceActionHelp() string {
	text := "\n\nResource details only\na / :a / :actions  More actions\n"
	if m.explore == nil || m.explore.detail == nil {
		return text + "Open an object to see applicable actions.\nEditor Save writes YAML after validation. Other changes require confirmation."
	}
	for _, item := range actionsFor(m.explore.resource, m.explore.detail) {
		text += item.alias + " / :" + item.alias + " / :" + item.command + "  " + item.label + "\n"
	}
	return text + "Editor Save writes YAML after validation. Other changes require confirmation."
}

func actionsFor(r kubernetes.Resource, object *unstructured.Unstructured) []resourceAction {
	var result []resourceAction
	for _, item := range resourceActionDefinitions {
		include := true
		switch item.id {
		case "logs", "exec":
			include = kubernetes.HasPodTargets(r)
		case "scale":
			include = r.Supports("update", "scale")
			switch r.Key() {
			case "deployments.apps", "statefulsets.apps", "replicasets.apps", "replicationcontrollers":
				include = true
			}
		case "restart":
			include = slices.Contains([]string{"deployments.apps", "statefulsets.apps", "daemonsets.apps"}, r.Key())
		case "edit":
			include = r.Key() != "events" && r.Key() != "events.events.k8s.io"
			if kubernetes.MetadataEditOnly(r, object) {
				item.label = "Edit metadata"
			}
			if r.Key() == "secrets" {
				item.label += " (reveals values)"
			}
		case "namespace":
			include = r.Key() == "namespaces"
		case "value":
			include = slices.Contains([]string{"configmaps", "secrets", "certificatesigningrequests.certificates.k8s.io", "controllerrevisions.apps"}, r.Key())
			if r.Key() == "secrets" {
				item.label = "Reveal/copy selected value"
			}
			if r.Key() == "certificatesigningrequests.certificates.k8s.io" {
				item.label = "Inspect/copy/export request or certificate"
			}
			if r.Key() == "controllerrevisions.apps" {
				item.label = "Inspect/copy revision snapshot"
			}
		}
		if include {
			if item.id == "edit" || item.id == "delete" {
				if managedResource(r) {
					item.label += " (advanced)"
				}
			}
			result = append(result, item)
		}
	}
	return result
}

func (m *model) openResourceActions(operation string) {
	e := m.explorer()
	if e.action != nil {
		m.status = "Finish or cancel the open action first."
		return
	}
	if e.detail == nil {
		m.status = "Open a resource's details first to use its actions."
		return
	}
	if !e.detailValid || e.client == nil {
		m.status = "Wait for the current object to load, or refresh its details."
		return
	}
	a := &resourceActionState{resource: e.resource, object: e.detail.DeepCopy(), client: e.client, resources: slices.Clone(e.resources), stage: "menu", pending: "permissions", reasons: map[string]string{}}
	if c := m.currentConnection(); c != nil {
		a.context = c.Settings["context"]
	}
	a.items = actionsFor(a.resource, a.object)
	if operation != "" && operation != "menu" {
		found := false
		for i, item := range a.items {
			if item.id == operation {
				a.cursor = i
				found = true
			}
		}
		if !found {
			m.status = "That action does not apply to this resource."
			return
		}
		a.requested = true
		a.direct = true
	}
	e.action = a
	m.shortcutPrefix, m.status = "", ""
}

func (m *model) stopResourceAction() {
	if m.explore != nil && m.explore.action != nil {
		m.explore.action.clearValueDocument()
		if cancel := m.explore.action.cancel; cancel != nil {
			cancel()
		}
		m.explore.action = nil
	}
}

func (m *model) resourceActionVisible() bool {
	return m.exploreVisible() && m.explore.action != nil && m.focus == 1 && m.help == nil && m.settingsOverlay == nil && m.command == nil
}

func (m *model) syncResourceAction() tea.Cmd {
	if m.explore == nil || m.explore.action == nil {
		return nil
	}
	a := m.explore.action
	if a.stage == "done" && a.progress != nil {
		return m.syncResourceProgress(a)
	}
	if a.busy || a.pending == "" {
		return nil
	}
	job := a.pending
	if job == "validate" && a.document == nil {
		a.pending = ""
		a.err = "Open the document before validating."
		return nil
	}
	if job == "apply" && (a.document == nil || a.proposed == nil || !a.saveRequested || a.saveRevision != a.document.Revision || a.validatedRevision != a.document.Revision || a.confirmedID != "" && (a.review == nil || a.confirmedID != a.review.ID || a.review.Stale)) {
		a.failDocument("Save stopped", "The draft changed before saving. Check it and use :w again.")
		return nil
	}
	a.pending, a.busy, a.err = "", true, ""
	a.runningJob = job
	a.serial++
	serial := a.serial
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	a.cancel = cancel
	r, object, proposed, scale, replicas, input := a.resource, a.object.DeepCopy(), a.proposed.DeepCopy(), a.scale.DeepCopy(), a.replicas, a.input.text
	revision := uint64(0)
	if job == "validate" {
		input, revision = a.document.Text(), a.document.Revision
	}
	client, resources := a.client, slices.Clone(a.resources)
	pod := a.pod.DeepCopy()
	items, contextName, operation := slices.Clone(a.items), a.context, a.operation
	exportText, exportValue := a.exportText, a.exportValue
	if job == "copy" {
		cancel()
		a.busy = false
		a.note = "Identity sent to the terminal clipboard. Clipboard support depends on the terminal."
		return tea.SetClipboard(contextName + " / " + r.Key() + " / " + object.GetNamespace() + " / " + object.GetName() + " / " + string(object.GetUID()))
	}
	return func() tea.Msg {
		defer cancel()
		result := resourceActionResult{action: a, serial: serial, job: job, revision: revision}
		switch job {
		case "permissions":
			result.reasons = map[string]string{}
			for _, item := range items {
				if item.verb == "" {
					continue
				}
				target, ns, name := r, object.GetNamespace(), object.GetName()
				if item.id == "logs" || item.id == "exec" {
					var ok bool
					target, ok = exactResource(resources, "pods")
					if !ok {
						result.reasons[item.id] = "Pod API unavailable"
						continue
					}
					if r.Key() != "pods" {
						// RBAC can restrict access to named Pods. Check the chosen
						// descendant instead of treating a broad denial as final.
						if !target.Supports(item.verb, item.subresource) {
							result.reasons[item.id] = "API does not advertise this operation"
						}
						continue
					}
				}
				if !target.Supports(item.verb, item.subresource) {
					result.reasons[item.id] = "API does not advertise this operation"
					continue
				}
				access, err := client.CheckAccess(ctx, target, ns, name, item.verb, item.subresource)
				if err != nil || !access.Known {
					result.permissionUnknown = true
				}
				if err == nil && access.Known && !access.Allowed {
					result.reasons[item.id] = "Permission denied: " + access.Reason
				}
			}
		case "prepare":
			result.object, result.err = client.Get(ctx, r, object.GetNamespace(), object.GetName())
			if result.err == nil && (result.object.GetUID() != object.GetUID() || object.GetUID() == "") {
				result.err = kubernetes.TargetChangedError()
			}
			if result.err != nil {
				break
			}
			if operation == "delete" && slices.Contains([]string{"persistentvolumeclaims", "persistentvolumes", "volumesnapshots.snapshot.storage.k8s.io", "volumesnapshotcontents.snapshot.storage.k8s.io"}, r.Key()) {
				result.note = client.StorageDeleteNotice(ctx, r, result.object, resources)
			}
			if operation == "scale" {
				result.scale, result.err = client.GetScale(ctx, r, result.object)
				if hpa, ok := exactResource(resources, "horizontalpodautoscalers.autoscaling"); ok {
					collection, err := client.List(ctx, hpa, object.GetNamespace(), "")
					if err != nil {
						result.note = "Autoscaler lookup unavailable. An HPA may override manual replicas."
					} else {
						for _, autoscaler := range collection.Objects {
							target, _, _ := unstructured.NestedMap(autoscaler.Object, "spec", "scaleTargetRef")
							if kubernetes.Value(target, "name") == object.GetName() && kubernetes.Value(target, "kind") == r.Kind && apiGroup(kubernetes.Value(target, "apiVersion")) == r.Group {
								result.note += "HPA " + autoscaler.GetName() + " controls this target and may override replicas.\n"
							}
						}
						if collection.Truncated {
							result.note += "Autoscaler lookup is incomplete. Other HPAs may override manual replicas.\n"
						}
					}
				}
			}
		case "targets":
			result.pods, result.err = client.PodTargets(ctx, r, object, resources)
		case "targetaccess":
			target, ok := exactResource(resources, "pods")
			if !ok {
				result.err = kubernetes.TargetChangedError()
				break
			}
			verb, subresource := "get", "log"
			if operation == "exec" {
				verb, subresource = "create", "exec"
			}
			access, err := client.CheckAccess(ctx, target, pod.GetNamespace(), pod.GetName(), verb, subresource)
			if err == nil && access.Known && !access.Allowed {
				result.err = apierrors.NewForbidden(target.GVR().GroupResource(), pod.GetName(), nil)
			}
		case "validate":
			result.object, result.err = kubernetes.ParseManifest(r, object, input)
			if result.err == nil {
				result.object, result.err = client.ValidateEdit(ctx, r, object, result.object)
			}
		case "apply":
			result.object, result.err = client.ApplyEdit(ctx, r, object, proposed)
		case "delete":
			result.err = client.Delete(ctx, r, object)
		case "scale":
			result.object, result.err = client.Scale(ctx, r, object, scale, replicas)
		case "restart":
			result.object, result.err = client.Restart(ctx, r, object)
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
			result.err = err
			if err == nil {
				result.note = "Exported YAML to " + path
				if exportValue {
					result.note = "Exported selected value to " + path
				}
				if r.Key() == "secrets" {
					result.note += " (values redacted)"
				}
			}
		}
		return result
	}
}

func exactResource(resources []kubernetes.Resource, key string) (kubernetes.Resource, bool) {
	for _, r := range resources {
		if r.Key() == key {
			return r, true
		}
	}
	return kubernetes.Resource{}, false
}
func apiGroup(version string) string {
	group, _, found := strings.Cut(version, "/")
	if found {
		return group
	}
	return ""
}

func (m *model) updateResourceActionResult(msg resourceActionResult) {
	if m.explore == nil || m.explore.action != msg.action || msg.serial != msg.action.serial {
		return
	}
	a := msg.action
	a.busy, a.cancel, a.runningJob = false, nil, ""
	if msg.err != nil {
		a.err = kubernetes.OperationErrorText(msg.err)
		if a.document != nil {
			title := "Validation failed"
			if a.saveRequested || msg.job == "apply" {
				title = "Save failed"
			}
			if msg.job == "apply" && !(apierrors.IsConflict(msg.err) || apierrors.IsInvalid(msg.err) || apierrors.IsBadRequest(msg.err) || apierrors.IsForbidden(msg.err) || apierrors.IsUnauthorized(msg.err) || apierrors.IsNotFound(msg.err) || apierrors.IsMethodNotSupported(msg.err)) {
				title = "Save result unknown"
				a.err = "Check the live resource before retrying. " + a.err
			}
			a.failDocument(title, a.err)
		}
		if msg.job == "export" {
			a.err = "Could not create the export. Choose a writable path to a file that does not exist."
		}
		return
	}
	switch msg.job {
	case "permissions":
		a.reasons = msg.reasons
		a.permissionUnknown = msg.permissionUnknown
		if a.requested {
			a.requested = false
			m.startResourceAction(a.items[a.cursor])
		}
	case "prepare":
		a.object, a.scale, a.note = msg.object, msg.scale, msg.note
		switch a.operation {
		case "edit":
			a.stage = "edit"
			a.initDocument()
		case "scale":
			a.stage = "scale"
			a.input = newEditor(kubernetes.Value(a.scale.Object, "spec", "replicas"))
		default:
			a.stage = "confirm"
			a.cursor = 0
		}
	case "targets":
		a.pods = msg.pods
		a.cursor = 0
		a.stage = "pods"
		if a.operation == "exec" {
			a.pods = slices.DeleteFunc(a.pods, func(o unstructured.Unstructured) bool { return len(kubernetes.RunningContainers(o)) == 0 })
		}
		if len(a.pods) == 1 && !a.allPodsChoice() {
			a.pod = a.pods[0].DeepCopy()
			m.chooseActionPod()
		}
	case "validate":
		if a.document == nil || msg.revision != a.document.Revision {
			a.failDocument("Validation stopped", "The draft changed during validation. Check it and try again.")
			return
		}
		a.proposed = msg.object
		a.validatedRevision = msg.revision
		a.confirmedID = ""
		if a.saveRequested {
			a.pending = "apply"
			return
		}
		a.review = diffview.New(kubernetes.YAML(*a.object, true), kubernetes.YAML(*msg.object, true), a.document.Target, msg.revision, true)
		a.review.Name = a.document.Name
		a.review.BeforeLabel, a.review.AfterLabel = "Current in Kubernetes", "Changes to save"
		a.review.BackLabel, a.review.ConfirmLabel = "Back to editor", "Save"
		a.review.ConfirmKey = ":w"
		a.stage = "review"
		a.cursor, a.scroll = 0, 0
	case "targetaccess":
		a.targetChecked = true
		m.chooseActionContainer()
	case "apply", "scale", "restart", "delete":
		if msg.job == "apply" && a.document != nil && msg.object != nil {
			a.document.MarkSaved(kubernetes.YAML(*msg.object, true))
			a.document.SavedStatus = "Saved to Kubernetes"
			a.document.Notice = ""
			a.document.Diagnostic = ""
			a.saveRequested = false
			a.confirmedID, a.documentErrorTitle = "", ""
			a.object = msg.object.DeepCopy()
			m.explore.detail = msg.object.DeepCopy()
			m.refreshExplore()
			if a.closeAfterApply {
				m.status = "Saved to Kubernetes: " + a.document.Name
				m.stopResourceAction()
			} else {
				a.stage, a.review, a.proposed = "edit", nil, nil
				a.closeAfterApply = false
			}
			return
		}
		a.stage = "done"
		a.note = map[string]string{"apply": "Saved to Kubernetes. Check the resource's reported state for readiness.", "scale": "Scale request accepted.", "restart": "Pod template updated.", "delete": "Deletion requested."}[msg.job]
		if msg.job != "apply" {
			a.progress = &resourceProgress{
				target:   kubernetes.NewOperationTarget(a.resource, msg.job, a.object, msg.object, a.replicas),
				status:   kubernetes.OperationProgress{State: "waiting", Summary: "Checking current status"},
				deadline: time.Now().Add(operationObservationLimit),
			}
			a.scroll = 0
		}
		m.refreshExplore()
	case "export":
		a.stage = "done"
		a.note = msg.note
	}
}

func (m *model) startResourceAction(item resourceAction) {
	a := m.explore.action
	if reason := a.reasons[item.id]; reason != "" {
		a.err = reason
		return
	}
	a.operation, a.err, a.note = item.id, "", ""
	switch item.id {
	case "copy":
		a.pending = "copy"
	case "export":
		a.stage = "export"
		a.exportValue, a.exportText = false, ""
		a.input = newEditor(a.object.GetName() + ".yaml")
	case "namespace":
		name := a.object.GetName()
		m.stopResourceAction()
		m.chooseExploreNamespace(name)
	case "value":
		a.values = resourceValues(a.resource, *a.object)
		a.stage, a.cursor = "values", 0
	case "logs", "exec":
		a.pending = "targets"
	default:
		a.pending = "prepare"
	}
}

func (m *model) chooseActionPod() {
	a := m.explore.action
	a.targetChecked = false
	a.containers = kubernetes.Containers(*a.pod)
	if a.operation == "exec" {
		a.containers = kubernetes.RunningContainers(*a.pod)
	}
	a.cursor, a.stage = 0, "containers"
	if len(a.containers) == 1 {
		a.container = a.containers[0]
		m.chooseActionContainer()
	} else if a.operation == "logs" {
		a.containers = append([]string{allApplicationContainers}, a.containers...)
	}
}

func (m *model) chooseActionContainer() {
	a := m.explore.action
	if !a.targetChecked {
		a.pending = "targetaccess"
		return
	}
	if a.operation == "exec" {
		a.stage = "execcommand"
		a.input = newEditor("/bin/sh")
		return
	}
	resource, ok := exactResource(a.resources, "pods")
	if !ok {
		a.err = "Pod API unavailable"
		return
	}
	m.openPodLogs(resource, a.pod, a.container)
}

func (m *model) backResourceAction() {
	a := m.explore.action
	if a.direct {
		m.stopResourceAction()
	} else {
		a.stage, a.err = "menu", ""
	}
}

func (m *model) updateResourceActionKey(msg tea.KeyPressMsg) tea.Cmd {
	a := m.explore.action
	key := msg.String()
	if a.stage == "valueview" && a.valueDocument != nil {
		return m.valueDocumentKey(msg)
	}
	if a.stage == "execcommand" && !a.busy {
		return m.updateExecLaunch(msg)
	}
	if (a.stage == "edit" || a.stage == "review") && !a.busy {
		return m.updateDocumentKey(msg)
	}
	if key == "esc" {
		if a.busy {
			if a.cancel != nil {
				a.cancel()
			}
			a.serial++
			a.busy, a.pending, a.cancel = false, "", nil
			a.err = "Request cancelled locally. A submitted change may already have reached the server; refresh before retrying."
			if a.document != nil {
				if a.runningJob == "apply" {
					a.failDocument("Save result unknown", "Cancelled while saving. Check the live resource before retrying. Your draft is kept.")
				} else {
					a.failDocument("Validation cancelled", "Nothing was saved. Your draft is kept.")
				}
			}
			return nil
		}
		switch a.stage {
		case "confirm":
			if a.operation == "scale" {
				a.stage = "scale"
			} else {
				m.backResourceAction()
			}
		case "menu", "done":
			m.stopResourceAction()
		case "containers":
			if len(a.pods) > 1 || a.allPodsChoice() {
				a.stage = "pods"
				a.cursor = 0
			} else {
				m.backResourceAction()
			}
		case "export":
			if a.exportValue {
				a.stage, a.cursor = "valueview", 0
			} else {
				m.backResourceAction()
			}
		default:
			m.backResourceAction()
		}
		return nil
	}
	if a.busy {
		return nil
	}
	if a.stage == "menu" {
		for _, item := range a.items {
			if key == item.alias {
				m.startResourceAction(item)
				return nil
			}
		}
	}
	switch a.stage {
	case "scale", "export":
		if key != "enter" {
			a.input.key(msg, false)
			return nil
		}
		switch a.stage {
		case "scale":
			count, err := strconv.ParseInt(strings.TrimSpace(a.input.text), 10, 32)
			if err != nil || count < 0 {
				a.err = "Enter a non-negative whole replica count."
				return nil
			}
			a.replicas, a.stage, a.cursor, a.err = count, "confirm", 0, ""
		case "export":
			if strings.TrimSpace(a.input.text) != "" {
				a.pending = "export"
			}
		}
	case "menu", "pods", "containers", "values":
		count := len(a.items)
		if a.stage == "pods" {
			count = len(a.pods)
			if a.allPodsChoice() {
				count++
			}
		}
		if a.stage == "containers" {
			count = len(a.containers)
		}
		if a.stage == "values" {
			count = len(a.values)
		}
		switch key {
		case "up", "k":
			a.cursor = max(0, a.cursor-1)
		case "down", "j":
			a.cursor = min(max(0, count-1), a.cursor+1)
		case "enter":
			if count == 0 {
				return nil
			}
			a.cursor = min(a.cursor, count-1)
			switch a.stage {
			case "menu":
				m.startResourceAction(a.items[a.cursor])
			case "pods":
				index := a.cursor
				if a.allPodsChoice() {
					if index == 0 {
						m.openWorkloadLogs()
						return nil
					}
					index--
				}
				a.pod = a.pods[index].DeepCopy()
				m.chooseActionPod()
			case "containers":
				a.container = a.containers[a.cursor]
				if a.operation == "logs" && a.container == allApplicationContainers {
					a.container = ""
				}
				m.chooseActionContainer()
			case "values":
				a.openValueDocument(a.values[a.cursor])
			}
		}
	case "confirm":
		switch key {
		case "left", "right":
			a.cursor = 1 - a.cursor
		case "up", "k":
			a.scroll = max(0, a.scroll-1)
		case "down", "j":
			a.scroll++
		case "pgup":
			a.scroll = max(0, a.scroll-10)
		case "pgdown":
			a.scroll += 10
		case "enter":
			if a.cursor == 0 {
				m.backResourceAction()
			} else {
				a.pending = a.operation

			}
		}
	case "done":
		switch key {
		case "enter":
			m.stopResourceAction()
		case "up", "k":
			a.scroll = max(0, a.scroll-1)
		case "down", "j":
			a.scroll++
		case "pgup":
			a.scroll = max(0, a.scroll-10)
		case "pgdown":
			a.scroll += 10
		case "home", "g":
			a.scroll = 0
			if key == "g" && a.progress != nil && !a.progress.checking && a.progress.status.State != "complete" {
				a.progress.status = kubernetes.OperationProgress{State: "waiting", Summary: "Checking current status"}
				a.progress.deadline, a.progress.next = time.Now().Add(operationObservationLimit), time.Now()
			}
		case "end", "G":
			a.scroll = 1 << 20
		}

	}
	return nil
}

func valueControls(r kubernetes.Resource) []string {
	labels := []string{"Back", "Copy value"}
	if r.Key() == "certificatesigningrequests.certificates.k8s.io" {
		labels = append(labels, "Export PEM")
	}
	return labels
}

func actionWarning(a *resourceActionState) string {
	var notes []string
	if managedResource(a.resource) {
		notes = append(notes, "Advanced operation on a controller or system record. Reconciliation can overwrite changes or recreate the record.")
	}
	if a.operation == "delete" {
		notes = append(notes, "Normal graceful deletion. Finalizers remain in force.")
		switch a.resource.Key() {
		case "namespaces":
			notes = append(notes, "This deletes the namespace and its namespaced resources.")
		case "customresourcedefinitions.apiextensions.k8s.io":
			notes = append(notes, "This removes the API definition and its custom resources.")
		case "nodes":
			notes = append(notes, "This deletes the Node API object, not the machine. It does not drain workloads.")
		case "persistentvolumes", "persistentvolumeclaims":
			notes = append(notes, "Storage deletion can affect data. Review the bound volume's reclaim policy and consumers in Related before confirming.")
		case "volumesnapshots.snapshot.storage.k8s.io", "volumesnapshotcontents.snapshot.storage.k8s.io":
			notes = append(notes, "Backing snapshot retention depends on deletionPolicy. Review the snapshot content before confirming.")
		case "volumeattachments.storage.k8s.io":
			notes = append(notes, "This is a controller-managed attachment record. Deletion is not proof of safe volume detachment.")
		}
	}
	if a.operation == "restart" {
		notes = append(notes, kubernetes.RestartNotice(a.resource, *a.object))
		if a.resource.Key() == "deployments.apps" && kubernetes.Value(a.object.Object, "spec", "paused") == "true" {
			notes = append(notes, "This Deployment is paused. Rollout will not progress until resumed outside this operation.")
		}
	}
	if a.resource.Key() == "statefulsets.apps" && (a.operation == "scale" || a.operation == "delete") {
		notes = append(notes, "PVC retention: whenScaled="+kubernetes.Value(a.object.Object, "spec", "persistentVolumeClaimRetentionPolicy", "whenScaled")+", whenDeleted="+kubernetes.Value(a.object.Object, "spec", "persistentVolumeClaimRetentionPolicy", "whenDeleted")+". Absent policy defaults to Retain.")
	}
	for _, owner := range a.object.GetOwnerReferences() {
		if owner.Controller != nil && *owner.Controller {
			notes = append(notes, "Managed by "+owner.Kind+" / "+owner.Name+". The controller may reconcile direct changes or recreate this object.")
		}
	}
	if len(a.object.GetFinalizers()) > 0 {
		notes = append(notes, "Finalizers: "+strings.Join(a.object.GetFinalizers(), ", "))
	}
	return strings.Join(notes, "\n\n")
}

func (m *model) renderResourceActionOverlay(base string) string {
	a := m.explore.action
	if a.stage == "execcommand" {
		return m.renderExecLaunchOverlay(base)
	}
	width, height := min(110, m.width-6), min(36, m.height-6)
	switch a.stage {
	case "menu":
		height = min(height, 10+len(a.items))
	case "scale", "export":
		height = min(height, 16)
	case "confirm":
		lines := lipgloss.Height(ansi.Wrap(actionWarning(a)+"\n"+a.note, max(1, width-4), ""))
		height = min(height, max(13, lines+10))
	case "done":
		height = min(height, 12)
		if a.progress != nil {
			height = min(m.height-6, 16)
		}
	case "pods":
		count := len(a.pods)
		if a.allPodsChoice() {
			count++
		}
		height = min(height, 10+min(count, 10))
	case "containers":
		height = min(height, 10+min(len(a.containers), 10))
	case "values":
		height = min(height, 10+min(len(a.values), 10))
	}
	errorText := ansi.Wrap(a.err, max(1, width-4), "")
	if errorText != "" {
		height = min(36, m.height-6, height+lipgloss.Height(errorText)+1)
	}
	innerWidth, innerHeight := width-4, height-2
	scope := a.object.GetNamespace()
	if scope == "" {
		scope = "cluster"
	}
	heading := titleStyle.Render(a.resource.Kind+" / "+oneLine(a.object.GetName())) + "\n" + muted.Render(clipped(a.context+" · "+scope+" · UID "+string(a.object.GetUID()), innerWidth))
	footer := "Esc back"
	var body string
	available := max(1, innerHeight-6)
	var errorNotice string
	if errorText != "" {
		errorHeight := min(lipgloss.Height(errorText), max(1, available-4))
		errorNotice = changedStyle.Render(textView(errorText, innerWidth, errorHeight, 0))
		available = max(1, available-errorHeight-1)
	}
	switch a.stage {
	case "menu":
		var items []string
		for _, item := range a.items {
			label := item.alias + "  " + item.label
			if a.busy {
				label += " · checking…"
			} else if reason := a.reasons[item.id]; reason != "" {
				label += " · unavailable"
			}
			items = append(items, label)
		}
		body = listView(items, a.cursor, innerWidth, max(1, available-2))
		if a.cursor < len(a.items) {
			reason := a.reasons[a.items[a.cursor].id]
			if reason != "" {
				body += "\n" + changedStyle.Render(clipped(reason, innerWidth))
			} else if a.resource.Key() != "pods" && (a.items[a.cursor].id == "logs" || a.items[a.cursor].id == "exec") {
				body += "\n" + muted.Render("Pod permissions are checked after selection.")
			}
		}
		if a.permissionUnknown {
			body += "\n" + muted.Render("Permission check unavailable. The API will authorize each request.")
		}
		footer = "↑↓ choose · Enter open · Esc close"
	case "pods", "containers":
		label := "Choose Pod"
		items := a.containers
		if a.stage == "pods" {
			items = nil
			if a.allPodsChoice() {
				items = append(items, "All Pods · merged application containers")
			}
			for _, pod := range a.pods {
				items = append(items, pod.GetName()+" · "+kubernetes.Value(pod.Object, "status", "phase"))
			}
		} else {
			label = "Choose container · " + a.pod.GetName()
		}
		body = titleStyle.Render(label) + "\n\n" + listView(items, a.cursor, innerWidth, max(1, available-2))
		if len(items) == 0 {
			body += "\nNo eligible " + a.stage + " found.\nExec requires a running container."
		}
	case "values":
		items := []string{}
		for _, value := range a.values {
			items = append(items, value.label)
		}
		body = titleStyle.Render("Choose a value") + "\n\n" + listView(items, a.cursor, innerWidth, available-2)
		if len(items) == 0 {
			body += "\nNo values found."
		}
		footer = "Enter reveal selected value · Esc back"
	case "scale":
		body = "Scale replicas\nCurrent desired: " + kubernetes.Value(a.scale.Object, "spec", "replicas") + "\n\nProposed: " + selected.Render(editorLine(a.input, innerWidth-10, true)) + "\n\n" + a.note
		footer = "Enter review · Esc cancel"
	case "export":
		body = "Export YAML to a new file\n\n" + selected.Render(editorLine(a.input, innerWidth, true)) + "\n\nExisting files are never overwritten. Secret values are redacted."
		if a.exportValue {
			body = "Export " + a.container + " as PEM to a new file\n\n" + selected.Render(editorLine(a.input, innerWidth, true)) + "\n\nExisting files are never overwritten."
		}
		footer = "Enter export · Esc cancel"
	case "confirm":
		text := actionWarning(a)
		if a.operation == "delete" && a.note != "" {
			text = a.note + "\n\n" + text
		}
		footer = "←→ choose · Enter confirm · Esc back"
		if a.operation == "scale" {
			text = fmt.Sprintf("Replicas: %s → %d\n\n%s\n\n%s", kubernetes.Value(a.scale.Object, "spec", "replicas"), a.replicas, a.note, text)
		}
		body = styledTextView(text, innerWidth, max(1, available-2), a.scroll) + "\n\n"
		labels := []string{"Back", "Confirm " + a.operation}

		for i, label := range labels {
			if i == a.cursor {
				body += selected.Background(highlight).Render("[ " + label + " ]")
			} else {
				body += muted.Render("  " + label + "  ")
			}
			body += "  "
		}
	case "done":
		body = addedStyle.Render(a.note)
		footer = "Enter or Esc return to details"
		if a.progress != nil {
			body = renderOperationProgress(a, innerWidth, available)
			footer = operationProgressFooter(a.progress, innerWidth)
		}
	}
	if a.busy {
		footer = m.loading("Working…") + " · Esc cancel request"
	}
	if a.err == "" && a.stage == "menu" && a.note != "" {
		body += "\n\n" + muted.Render(a.note)
	}
	content := heading + "\n\n" + lipgloss.NewStyle().Width(innerWidth).Height(available).MaxHeight(available).Render(body)
	if errorNotice != "" {
		content += "\n" + errorNotice
	}
	content += "\n" + muted.Render(clipped(footer, innerWidth))
	window := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).Background(surface).Foreground(foreground).Padding(0, 1).Width(width).Height(height).MaxHeight(height).Render(content)
	return centeredOverlay(base, window)
}
