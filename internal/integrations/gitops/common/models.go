package common

import (
	"time"
)

type Tool string

const (
	ArgoCD Tool = "argocd"
	Flux   Tool = "flux"
)

type AccessState string

const (
	Available             AccessState = "available"
	Empty                 AccessState = "empty"
	Partial               AccessState = "partial"
	ReadDenied            AccessState = "read-denied"
	APIUnavailable        AccessState = "api-unavailable"
	ConnectionUnavailable AccessState = "connection-unavailable"
)

type Reference struct {
	Group, Version, Kind, Namespace, Name string
}

func (r Reference) Key() string { return r.Group + "/" + r.Kind + "/" + r.Namespace + "/" + r.Name }

func (r Reference) Label() string { return r.Kind + "/" + r.Name }

type Destination struct {
	Name, Server, Namespace string
	// Remote identifies Flux objects that use another cluster's kubeconfig.
	// Plous never reads those credentials to resolve the destination.
	Remote bool
}

type SourceReference struct {
	Ref                        Reference
	URL, Path, Revision, Chart string
}

type ManagedResource struct {
	Ref                   Reference
	Sync, Health, Message string
}

type Field struct{ Label, Value string }

type Condition struct {
	Type, Status, Reason, Message string
	ObservedGeneration            int64
	ChangedAt                     time.Time
	Stale                         bool
}

type Item struct {
	Ref                          Reference
	UID                          string
	Sync, Health, Status, Reason string
	Conditions                   []Condition
	Destination                  Destination
	Sources                      []SourceReference
	Dependencies                 []Reference
	Related                      []ManagedResource
	RelatedMessage               string
	Resources                    []ManagedResource
	InventoryReported            bool
	Fields                       []Field
	ObservedAt, ReadAt           time.Time
	ObservedLabel                string
	Suspended, Stale             bool
	ControllerError              bool
}

type Issue struct {
	Kind    string
	State   AccessState
	Message string
}

type Snapshot struct {
	Items     []Item
	State     AccessState
	Message   string
	Issues    []Issue
	ReadAt    time.Time
	Truncated bool
}

type ReadError struct {
	State   AccessState
	Message string
}

func (e *ReadError) Error() string { return e.Message }
