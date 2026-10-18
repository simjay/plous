package monitor

import (
	"context"
	"sync"
	"time"

	gitops "github.com/simjay/plous/internal/integrations/gitops/common"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type Reader interface {
	Discover(context.Context) ([]kubecommon.Resource, string, error)
	List(context.Context, kubecommon.Resource, string, string) (kubernetes.Collection, error)
	Get(context.Context, kubecommon.Resource, string, string) (*unstructured.Unstructured, error)
}

type Definition struct {
	Roots           []gitops.Reference
	OptionalRoots   bool
	Allows          func(gitops.Reference) bool
	SupportsVersion func(kubecommon.Resource) bool
	Parse           func(unstructured.Unstructured, time.Time) gitops.Item
}
type Client struct {
	reader     Reader
	definition Definition
}

func New(reader Reader, definition Definition) *Client {
	return &Client{reader: reader, definition: definition}
}

func (c *Client) Load(ctx context.Context, namespace string) gitops.Snapshot {
	catalog, warning, err := c.reader.Discover(ctx)
	if err != nil {
		failure := ClassifyError(err)
		return gitops.Snapshot{State: failure.State, Message: failure.Message, ReadAt: time.Now()}
	}
	return c.LoadWithCatalog(ctx, namespace, catalog, warning)
}

func (c *Client) LoadWithCatalog(ctx context.Context, namespace string, catalog []kubecommon.Resource, warning string) gitops.Snapshot {
	refs := c.definition.Roots
	if c.definition.OptionalRoots && warning == "" {
		refs = nil
		for _, ref := range c.definition.Roots {
			if _, ok := gitops.FindResource(catalog, ref); ok {
				refs = append(refs, ref)
			}
		}
	}
	if len(refs) == 0 {
		return gitops.Snapshot{State: gitops.APIUnavailable, Message: "This GitOps view is not supported.", ReadAt: time.Now()}
	}
	type result struct {
		collection kubernetes.Collection
		resource   kubecommon.Resource
		failure    *gitops.ReadError
	}
	results := make([]result, len(refs))
	var wg sync.WaitGroup
	for i, ref := range refs {
		resource, ok := gitops.FindResource(catalog, ref)
		if !ok {
			results[i].failure = &gitops.ReadError{State: gitops.APIUnavailable, Message: ref.Kind + " API is unavailable or could not be discovered."}
			continue
		}
		if failure := c.unsupportedVersion(resource); failure != nil {
			results[i].failure = failure
			continue
		}
		results[i].resource = resource
		wg.Go(func() {
			collection, err := c.reader.List(ctx, resource, namespace, "")
			results[i].collection = collection
			if err != nil {
				results[i].failure = ClassifyError(err)
			}
		})
	}
	wg.Wait()
	snapshot := gitops.Snapshot{State: gitops.Available, ReadAt: time.Now()}
	readable := 0
	for i, result := range results {
		if result.failure != nil {
			snapshot.Issues = append(snapshot.Issues, gitops.Issue{Kind: refs[i].Kind, State: result.failure.State, Message: result.failure.Message})
			continue
		}
		readable++
		snapshot.Truncated = snapshot.Truncated || result.collection.Truncated
		for _, object := range result.collection.Objects {
			if object.GetKind() == "" {
				object.SetKind(result.resource.Kind)
			}
			if object.GetAPIVersion() == "" {
				object.SetAPIVersion(result.resource.GVR().GroupVersion().String())
			}
			snapshot.Items = append(snapshot.Items, c.definition.Parse(object, snapshot.ReadAt))
		}
	}
	gitops.SortItems(snapshot.Items)
	switch {
	case readable == 0:
		snapshot.State = gitops.APIUnavailable
		for _, issue := range snapshot.Issues {
			if issue.State == gitops.ReadDenied || issue.State == gitops.ConnectionUnavailable {
				snapshot.State = issue.State
				break
			}
		}
		snapshot.Message = accessMessage(snapshot.State)
	case len(snapshot.Issues) > 0:
		snapshot.State = gitops.Partial
		snapshot.Message = "Some resource types could not be read. Available results are shown."
	case len(snapshot.Items) == 0:
		snapshot.State = gitops.Empty
		snapshot.Message = "No objects found in this namespace scope."
	}
	if warning != "" {
		snapshot.Issues = append(snapshot.Issues, gitops.Issue{State: gitops.APIUnavailable, Message: warning})
		if snapshot.State == gitops.Available || snapshot.State == gitops.Empty {
			snapshot.State = gitops.Partial
			snapshot.Message = "API discovery was incomplete. Available results are shown."
		}
	}
	return snapshot
}

func (c *Client) Get(ctx context.Context, ref gitops.Reference) (gitops.Item, error) {
	if c.definition.Allows == nil || !c.definition.Allows(ref) || ref.Name == "" || ref.Namespace == "" {
		return gitops.Item{}, &gitops.ReadError{State: gitops.APIUnavailable, Message: "This GitOps reference is not supported."}
	}
	catalog, _, err := c.reader.Discover(ctx)
	if err != nil {
		return gitops.Item{}, ClassifyError(err)
	}
	resource, ok := gitops.FindResource(catalog, ref)
	if !ok {
		return gitops.Item{}, &gitops.ReadError{State: gitops.APIUnavailable, Message: ref.Kind + " API is unavailable or could not be discovered."}
	}
	if failure := c.unsupportedVersion(resource); failure != nil {
		return gitops.Item{}, failure
	}
	object, err := c.reader.Get(ctx, resource, ref.Namespace, ref.Name)
	if err != nil {
		failure := ClassifyError(err)
		if apierrors.IsNotFound(err) {
			failure.Message = ref.Label() + " is no longer available. Refresh its parent view."
		}
		return gitops.Item{}, failure
	}
	if object.GetKind() == "" {
		object.SetKind(resource.Kind)
	}
	if object.GetAPIVersion() == "" {
		object.SetAPIVersion(resource.GVR().GroupVersion().String())
	}
	return c.definition.Parse(*object, time.Now()), nil
}

func ClassifyError(err error) *gitops.ReadError {
	state := gitops.ConnectionUnavailable
	if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
		state = gitops.ReadDenied
	} else if apierrors.IsNotFound(err) || apierrors.IsMethodNotSupported(err) || apierrors.IsGone(err) {
		state = gitops.APIUnavailable
	}
	return &gitops.ReadError{State: state, Message: accessMessage(state)}
}

func accessMessage(state gitops.AccessState) string {
	switch state {
	case gitops.ReadDenied:
		return "Read denied. Check this saved context's permissions and authentication."
	case gitops.APIUnavailable:
		return "API unavailable. The controller API is not installed or could not be discovered."
	default:
		return "Connection unavailable. Check the saved context and refresh."
	}
}

func (c *Client) unsupportedVersion(resource kubecommon.Resource) *gitops.ReadError {
	if c.definition.SupportsVersion == nil || !c.definition.SupportsVersion(resource) {
		return &gitops.ReadError{State: gitops.APIUnavailable, Message: resource.Kind + " API version " + resource.Version + " is not supported by this view."}
	}
	return nil
}
func Supports(catalog []kubecommon.Resource, roots []gitops.Reference) bool {
	for _, ref := range roots {
		if _, ok := gitops.FindResource(catalog, ref); ok {
			return true
		}
	}
	return false
}
