package app

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/app/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	diffview "github.com/simjay/plous/internal/ui/diff"
	textview "github.com/simjay/plous/internal/ui/text"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

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
