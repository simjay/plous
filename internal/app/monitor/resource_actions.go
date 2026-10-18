package monitor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"slices"
	"unicode/utf8"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	diffview "github.com/simjay/plous/internal/ui/diff"
	docedit "github.com/simjay/plous/internal/ui/editor"
	textview "github.com/simjay/plous/internal/ui/text"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type ResourceAction struct {
	Id, Label, Alias, Command, Verb, Subresource string
}

type ResourceValue struct{ Label, Text string }

func ResourceValues(r kubecommon.Resource, object unstructured.Unstructured) []ResourceValue {
	var values []ResourceValue
	if r.Key() == "controllerrevisions.apps" {
		payload, found, _ := unstructured.NestedFieldCopy(object.Object, "data")
		if found {
			if text, err := json.MarshalIndent(payload, "", "  "); err == nil {
				values = append(values, ResourceValue{"Revision snapshot", string(text)})
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
				values = append(values, ResourceValue{field.name, string(decoded)})
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
			values = append(values, ResourceValue{label, value})
		}
	}
	return values
}

var ResourceActionDefinitions = []ResourceAction{
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

type ResourceActionState struct {
	ValueDocument                                 *docedit.Model
	Document                                      *docedit.Model
	Review                                        *diffview.Model
	Diagnostic                                    *textview.Viewer
	ValidatedRevision                             uint64
	ConfirmedID                                   string
	SaveRequested                                 bool
	SaveRevision                                  uint64
	RunningJob, DocumentErrorTitle                string
	Yank                                          string
	DocumentAction                                int
	Discard                                       bool
	CloseAfterApply                               bool
	ExecChoice                                    int
	ExecEditing                                   bool
	PreviousTerminal                              *ResourceTerminal
	Progress                                      *ResourceProgress
	Resource                                      kubecommon.Resource
	Object, Proposed, Scale                       *unstructured.Unstructured
	Client                                        *kubernetes.Client
	Resources                                     []kubecommon.Resource
	Context, Stage, Operation, Pending, Err, Note string
	Items                                         []ResourceAction
	Reasons                                       map[string]string
	Input                                         textview.Field
	Cursor, Scroll                                int
	Busy                                          bool
	Cancel                                        context.CancelFunc
	Serial                                        uint64
	Pods                                          []unstructured.Unstructured
	Pod                                           *unstructured.Unstructured
	Containers                                    []string
	Container                                     string
	Replicas                                      int64
	Requested                                     bool
	Direct                                        bool
	TargetChecked                                 bool
	PermissionUnknown                             bool
	Values                                        []ResourceValue
	ExportText                                    string
	ExportValue                                   bool
}

type ResourceActionResult struct {
	Revision          uint64
	Action            *ResourceActionState
	Serial            uint64
	Job               string
	Object, Scale     *unstructured.Unstructured
	Pods              []unstructured.Unstructured
	Reasons           map[string]string
	Note              string
	PermissionUnknown bool
	Err               error
}

func ManagedResource(r kubecommon.Resource) bool {
	return slices.Contains([]string{"events", "events.events.k8s.io", "leases.coordination.k8s.io", "leasecandidates.coordination.k8s.io", "csinodes.storage.k8s.io", "csistoragecapacities.storage.k8s.io", "volumeattachments.storage.k8s.io", "resourceslices.resource.k8s.io", "endpointslices.discovery.k8s.io", "controllerrevisions.apps", "volumesnapshotcontents.snapshot.storage.k8s.io"}, r.Key())
}

func ActionsFor(r kubecommon.Resource, object *unstructured.Unstructured) []ResourceAction {
	var result []ResourceAction
	for _, item := range ResourceActionDefinitions {
		include := true
		switch item.Id {
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
				item.Label = "Edit metadata"
			}
			if r.Key() == "secrets" {
				item.Label += " (reveals values)"
			}
		case "namespace":
			include = r.Key() == "namespaces"
		case "value":
			include = slices.Contains([]string{"configmaps", "secrets", "certificatesigningrequests.certificates.k8s.io", "controllerrevisions.apps"}, r.Key())
			if r.Key() == "secrets" {
				item.Label = "Reveal/copy selected value"
			}
			if r.Key() == "certificatesigningrequests.certificates.k8s.io" {
				item.Label = "Inspect/copy/export request or certificate"
			}
			if r.Key() == "controllerrevisions.apps" {
				item.Label = "Inspect/copy revision snapshot"
			}
		}
		if include {
			if item.Id == "edit" || item.Id == "delete" {
				if ManagedResource(r) {
					item.Label += " (advanced)"
				}
			}
			result = append(result, item)
		}
	}
	return result
}
