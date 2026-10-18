package monitor

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	manifestutil "github.com/simjay/plous/internal/integrations/manifest"
	"github.com/simjay/plous/internal/integrations/observation"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type Projector struct {
	client *Client
	config Config
}

func NewProjector(client *Client, namespace string) (*Projector, error) {
	if client == nil {
		return nil, errors.New("Helm client is unavailable")
	}
	scope := client.Config
	if namespace != "" {
		scope.Namespace = namespace
	}
	if err := ValidateRead(Command{Operation: "list"}, scope); err != nil {
		return nil, err
	}
	captured := *client
	return &Projector{client: &captured, config: scope}, nil
}
func (e *Projector) Namespace() string { return e.config.Namespace }
func releaseRow(r Release) observation.Row {
	return observation.Row{Identity: observation.Identity{API: "helm", Kind: "Release", Namespace: r.Namespace, Name: r.Name, Revision: strconv.Itoa(r.Revision)}, Facts: map[string]observation.Value{
		"revision": observation.Number(int64(r.Revision)), "status": observation.Text(r.Status), "chart": observation.Text(r.Chart), "version": observation.Text(r.Version), "app_version": observation.Text(r.AppVersion), "updated": observation.Text(r.Updated),
	}}
}
func (e *Projector) Execute(ctx context.Context, c Command) (r observation.Result, err error) {
	r = observation.New("helm.release.v1", c.Operation, time.Now().UTC())
	r.Connection, r.Namespace = e.config.ConnectionID, e.config.Namespace
	r.Selection = map[string]string{"release": c.Name}
	defer func() {
		r.Coverage.Observed = len(r.Rows)
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		err = r.FinishRead(projectedReadError(err))
		r.Facts["observed_total"] = observation.Number(int64(len(r.Rows)))
		if err == nil && r.Outcome == "completed" {
			r.Complete()
		}
		err = errors.Join(err, r.Validate())
	}()
	if err := ValidateRead(c, e.config); err != nil {
		return r, err
	}
	switch c.Operation {
	case "list":
		releases, readErr := e.client.List(ctx, r.Namespace)
		for _, release := range releases {
			r.Rows = append(r.Rows, releaseRow(release))
			if release.Error != "" {
				r.Partial("release metadata unavailable")
			}
		}
		if readErr != nil {
			return r, readErr
		}
	case "status":
		revision := c.Revision
		if revision == 0 {
			release, err := e.client.Metadata(ctx, c.Name, r.Namespace, 0)
			if err != nil {
				return r, err
			}
			revision = release.Revision
		}
		s, err := e.client.RecordedSnapshot(ctx, c.Name, r.Namespace, revision)
		if err != nil {
			return r, err
		}
		row := releaseRow(s.Release())
		row.Facts["description"] = observation.Text(manifestutil.SafeText(s.Info.Description))
		r.Rows = []observation.Row{row}
	case "metadata":
		release, err := e.client.Metadata(ctx, c.Name, r.Namespace, c.Revision)
		if err != nil {
			return r, err
		}
		r.Rows = []observation.Row{releaseRow(release)}
	case "history":
		revisions, err := e.client.History(ctx, c.Name, r.Namespace)
		if err != nil {
			return r, err
		}
		for _, v := range revisions {
			r.Rows = append(r.Rows, observation.Row{Identity: observation.Identity{API: "helm", Kind: "Revision", Namespace: r.Namespace, Name: c.Name, UID: c.Name + "/" + r.Namespace + "/" + strconv.Itoa(v.Revision), Revision: strconv.Itoa(v.Revision)}, Facts: map[string]observation.Value{"revision": observation.Number(int64(v.Revision)), "status": observation.Text(v.Status), "chart": observation.Text(v.Chart), "updated": observation.Text(v.Updated), "description": observation.Text(manifestutil.SafeText(v.Description))}})
		}
		if len(revisions) == 1000 {
			r.Partial("retained revision limit reached")
		}
	default:
		revision := c.Revision
		if revision == 0 {
			v, err := e.client.Metadata(ctx, c.Name, r.Namespace, 0)
			if err != nil {
				return r, err
			}
			revision = v.Revision
		}
		read := e.client.RecordedSnapshot
		if c.Operation == "values" {
			read = e.client.Snapshot
		}
		s, err := read(ctx, c.Name, r.Namespace, revision)
		if err != nil {
			return r, err
		}
		r.Facts["release"] = observation.Text(c.Name)
		r.Facts["revision"] = observation.Number(int64(revision))
		switch c.Operation {
		case "compare":
			var before *Snapshot
			beforeRevision := revision - 1
			if c.CompareRevision != nil {
				beforeRevision = *c.CompareRevision
			}
			if beforeRevision > 0 {
				before, err = e.client.RecordedSnapshot(ctx, c.Name, r.Namespace, beforeRevision)
				if err != nil {
					return r, err
				}
			}
			changes, err := CompareRevisions(before, s)
			if err != nil {
				return r, err
			}
			r.Facts["before_revision"] = observation.Number(int64(beforeRevision))
			for _, change := range changes.Fields {
				r.Rows = append(r.Rows, observation.Row{Identity: observation.Identity{Kind: "FieldChange", Name: change.Pointer}, Facts: map[string]observation.Value{"before": change.Before, "after": change.After, "had_before": observation.Boolean(change.HadBefore), "has_after": observation.Boolean(change.HasAfter)}})
			}
			if !changes.FieldsComplete {
				r.Partial("comparison field limit reached")
			}
			r.Coverage.Limitations = append(r.Coverage.Limitations, "recorded revisions only; does not compare live resources")
		case "manifest", "hooks":
			manifests := []string{s.Manifest}
			if c.Operation == "hooks" {
				manifests = nil
				for _, h := range s.Hooks {
					manifests = append(manifests, h.Manifest)
					r.Rows = append(r.Rows, observation.Row{Identity: observation.Identity{Kind: "Hook", Name: h.Name}, Facts: map[string]observation.Value{"events": observation.Text(strings.Join(h.Events, ", "))}})
				}
			}
			for _, text := range manifests {
				resources, err := manifestutil.Resources(text)
				if err != nil {
					return r, err
				}
				for _, v := range resources {
					identity := observation.Identity{API: v.APIVersion, Kind: v.Kind, Namespace: v.Namespace, Name: v.Name}
					r.Rows = append(r.Rows, observation.Row{Identity: identity, Facts: map[string]observation.Value{"source": observation.Text(v.Source)}})
					fields, complete := manifestutil.Fields(v.Object)
					for _, field := range fields {
						fieldID := identity
						fieldID.UID = v.Key() + field.Pointer
						r.Rows = append(r.Rows, observation.Row{Identity: fieldID, Facts: map[string]observation.Value{"field": observation.Text(field.Pointer), "value": field.Value}})
					}
					if !complete {
						r.Partial("manifest field limit reached")
					}
				}
			}
			r.Facts["detail"] = observation.Text("rendered resource identities and masked scalar fields")
		case "values", "chart":
			var value any = s.Effective
			if c.Operation == "chart" {
				value = s.Chart.Metadata
			}
			projectConfiguration(&r, value)
		}
	}
	return r, nil
}

func projectedReadError(err error) error {
	var read *readError
	if !errors.As(err, &read) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	message := "The Helm read failed; check the selected connection and retained release revision."
	denied := false
	switch {
	case apierrors.IsForbidden(err):
		message, denied = "Access denied by Kubernetes RBAC while reading Helm releases.", true
	case apierrors.IsUnauthorized(err):
		message, denied = "Kubernetes authentication failed while reading Helm releases.", true
	case apierrors.IsNotFound(err):
		message = "The Helm release or revision is no longer available."
	case apierrors.IsTooManyRequests(err):
		message = "The API server is busy; the Helm read could not finish."
	case apierrors.IsTimeout(err), apierrors.IsServerTimeout(err):
		message = "The Helm read timed out."
	}
	return &observation.ReadFailure{Message: message, Denied: denied}
}

func projectConfiguration(r *observation.Result, value any) {
	fields, complete := manifestutil.Fields(value)
	if len(fields) > 1000 {
		fields = fields[:1000]
		complete = false
	}
	for _, field := range fields {
		r.Rows = append(r.Rows, observation.Row{Identity: observation.Identity{API: "helm", Kind: "Configuration", Name: field.Pointer}, Facts: map[string]observation.Value{"value": field.Value}})
	}
	if !complete {
		r.Partial("configuration field limit reached or oversized values omitted")
	}
}
