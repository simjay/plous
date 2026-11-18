package monitor

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
)

const MaxObjects = 10000

type Collection struct {
	Objects          []unstructured.Unstructured
	Version          string
	Columns          []string
	Cells            map[string][]string
	Truncated        bool
	ServiceEndpoints map[string]EndpointReadiness
	Usage            map[string]Usage
}
type Snapshot struct {
	Collection *Collection
	Status     string
	Err        error
}
type Client struct {
	target    *kubecommon.Target
	dynamic   dynamic.Interface
	rest      rest.Interface
	discovery rest.Interface
	config    *rest.Config
	Namespace string
	catalogMu sync.RWMutex
	catalog   []kubecommon.Resource
}

func New(paths, contextName, namespace string) (*Client, error) {
	target, err := kubecommon.ResolveTarget(paths, contextName, namespace)
	if err != nil {
		return nil, err
	}
	return NewTarget(target, namespace)
}

func NewTarget(target *kubecommon.Target, namespace string) (*Client, error) {
	if target == nil {
		return nil, errors.New("No captured Kubernetes target is available.")
	}
	config, err := target.Loader(target.Namespace).ClientConfig()
	if err != nil {
		return nil, err
	}
	if err := PrepareExecCredentials(config); err != nil {
		return nil, err
	}
	c, err := NewForConfig(config)
	if err != nil {
		return nil, errors.New("Cannot initialize Kubernetes authentication or TLS. Check the selected kubeconfig.")
	}
	if namespace == "" {
		namespace = target.Namespace
	}
	c.Namespace = namespace
	c.target = target
	return c, nil
}

func NewForConfig(config *rest.Config) (*Client, error) {
	copy := dynamic.ConfigFor(config)
	copy.Timeout = 0
	copy.UserAgent = "plous"
	raw, err := rest.UnversionedRESTClientFor(copy)
	if err != nil {
		return nil, err
	}
	// Discovery has its own burst allowance so catalog reads do not compete
	// with resource requests for the normal client's small token bucket.
	catalog, err := discovery.NewDiscoveryClientForConfig(copy)
	if err != nil {
		return nil, err
	}
	return &Client{dynamic: dynamic.New(raw), rest: raw, discovery: catalog.RESTClient(), config: rest.CopyConfig(copy), Namespace: "default"}, nil
}

func (c *Client) Discover(ctx context.Context) ([]kubecommon.Resource, string, error) {
	catalog, err := discoverCatalog(ctx, func(ctx context.Context, path, accept string, target any) error {
		request := c.discovery.Get().AbsPath(path).Timeout(12 * time.Second)
		if accept != "" {
			request.SetHeader("Accept", accept)
		}
		data, err := request.Do(ctx).Raw()
		if err != nil {
			return err
		}
		return json.Unmarshal(data, target)
	}, "")
	if err != nil {
		return nil, "", err
	}
	resources := catalog.Listable()
	if len(resources) == 0 {
		return nil, "", errors.New("The cluster returned no listable resource types.")
	}
	slices.SortFunc(resources, CompareResources)
	c.catalogMu.Lock()
	c.catalog = slices.Clone(resources)
	c.catalogMu.Unlock()
	warning := ""
	if len(catalog.Missing) > 0 {
		warning = "Some API groups could not be discovered. Refresh to retry."
	}
	return resources, warning, nil
}

func (c *Client) resource(r kubecommon.Resource, ns string) dynamic.ResourceInterface {
	api := c.dynamic.Resource(r.GVR())
	if r.Namespaced {
		return api.Namespace(ns)
	}
	return api
}
func resourcePath(r kubecommon.Resource, ns string) string {
	path := "/api/" + r.Version
	if r.Group != "" {
		path = "/apis/" + r.Group + "/" + r.Version
	}
	if r.Namespaced && ns != "" {
		path += "/namespaces/" + ns
	}
	return path + "/" + r.Name
}

func (c *Client) List(ctx context.Context, r kubecommon.Resource, ns, selector string) (Collection, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	collection := Collection{Cells: map[string][]string{}}
	options := metav1.ListOptions{Limit: 500, LabelSelector: selector}
	for {
		list, err := c.resource(r, ns).List(ctx, options)
		if err != nil {
			return Collection{}, err
		}
		collection.Objects = append(collection.Objects, list.Items...)
		collection.Version = list.GetResourceVersion()
		options.Continue = list.GetContinue()
		if len(collection.Objects) >= MaxObjects {
			collection.Truncated = len(collection.Objects) > MaxObjects || options.Continue != ""
			collection.Objects = collection.Objects[:MaxObjects]
			break
		}
		if options.Continue == "" {
			break
		}
	}
	for i := range collection.Objects {
		setType(&collection.Objects[i], r)
	}
	if r.Key() == "services" && len(collection.Objects) > 0 {
		c.catalogMu.RLock()
		target, available := findResourceIdentity(c.catalog, "endpointslices.discovery.k8s.io")
		c.catalogMu.RUnlock()
		collection.ServiceEndpoints = c.serviceEndpointReadiness(ctx, collection.Objects, target, available, ns, "")
	}
	if !HasPresenter(r) && len(collection.Objects) > 0 {
		var table metav1.Table
		data, err := c.rest.Get().AbsPath(resourcePath(r, ns)).SetHeader("Accept", "application/json;as=Table;g=meta.k8s.io;v=v1").Param("includeObject", "Metadata").Param("limit", fmt.Sprint(MaxObjects)).Param("labelSelector", selector).Timeout(8 * time.Second).Do(ctx).Raw()
		if err == nil && json.Unmarshal(data, &table) == nil && table.Kind == "Table" {
			for _, column := range table.ColumnDefinitions {
				collection.Columns = append(collection.Columns, column.Name)
			}
			for _, row := range table.Rows {
				var object unstructured.Unstructured
				if json.Unmarshal(row.Object.Raw, &object.Object) != nil {
					continue
				}
				cells := make([]string, len(row.Cells))
				for i, cell := range row.Cells {
					cells[i] = fmt.Sprint(cell)
				}
				collection.Cells[Identity(object)] = cells
			}
		}
	}
	return collection, nil
}

func (c *Client) Get(ctx context.Context, r kubecommon.Resource, ns, name string) (*unstructured.Unstructured, error) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	object, err := c.resource(r, ns).Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		setType(object, r)
	}
	return object, err
}

func setType(object *unstructured.Unstructured, r kubecommon.Resource) {
	if object.GetKind() == "" {
		object.SetKind(r.Kind)
	}
	if object.GetAPIVersion() == "" {
		object.SetAPIVersion(r.GVR().GroupVersion().String())
	}
}

func pause(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

type LogOptions struct {
	Namespace, Pod, Container string
	UID                       types.UID
	Previous, Follow          bool
	SinceSeconds, TailLines   *int64
	Timestamps                *bool
	OnConnect                 func()
}

func (c *Client) Logs(ctx context.Context, options LogOptions, emit func(string)) error {
	if !validPathPart(options.Namespace) || !validPathPart(options.Pod) || options.Container == "" {
		return operationError("Choose a namespace, Pod, and container before opening logs.")
	}
	if options.UID != "" {
		pod, err := c.Get(ctx, kubecommon.Resource{Name: "pods", Version: "v1", Kind: "Pod", Namespaced: true}, options.Namespace, options.Pod)
		if err != nil {
			return err
		}
		if pod.GetUID() != options.UID {
			return TargetChangedError()
		}
	}
	timestamps, tail := true, int64(200)
	if options.Timestamps != nil {
		timestamps = *options.Timestamps
	}
	if options.TailLines != nil {
		tail = *options.TailLines
	}
	if tail < 0 || options.SinceSeconds != nil && *options.SinceSeconds <= 0 {
		return operationError("Log range must be positive and tail count cannot be negative.")
	}
	request := c.rest.Get().AbsPath("/api/v1/namespaces", options.Namespace, "pods", options.Pod, "log").Param("container", options.Container).Param("timestamps", fmt.Sprint(timestamps)).Param("previous", fmt.Sprint(options.Previous)).Param("follow", fmt.Sprint(options.Follow && !options.Previous))
	if options.TailLines != nil || options.SinceSeconds == nil {
		request.Param("tailLines", fmt.Sprint(tail))
	}
	if options.SinceSeconds != nil {
		request.Param("sinceSeconds", fmt.Sprint(*options.SinceSeconds))
	}
	stream, err := request.Stream(ctx)
	if err != nil {
		return err
	}
	defer stream.Close()
	if options.OnConnect != nil {
		options.OnConnect()
	}
	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		emit(scanner.Text() + "\n")
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return scanner.Err()
}

func LogErrorText(err error, previous bool) string {
	switch {
	case apierrors.IsForbidden(err):
		return "Log access denied by Kubernetes RBAC. Reading logs requires access to the Pod and pods/log in this namespace."
	case apierrors.IsBadRequest(err):
		if previous {
			return "Kubernetes could not return previous-instance logs. The container may not have a retained previous instance. Try current logs or choose another container."
		}
		return "Kubernetes could not return logs for this container. Check its state in Describe and Events, or choose another container."
	default:
		return OperationErrorText(err)
	}
}

func ErrorText(err error) string {
	switch {
	case err == nil:
		return ""
	case apierrors.IsForbidden(err):
		return "Access denied by Kubernetes RBAC. Choose an allowed namespace or resource."
	case apierrors.IsUnauthorized(err):
		return "Kubernetes authentication failed. Renew your credentials, then refresh."
	case apierrors.IsNotFound(err):
		return "This resource or API is no longer available. Refresh to discover it again."
	case apierrors.IsBadRequest(err):
		return "Kubernetes rejected the request. Check the filter or selected container."
	case apierrors.IsTooManyRequests(err):
		return "The API server is busy. Wait briefly before trying again."
	case errors.Is(err, context.DeadlineExceeded), apierrors.IsTimeout(err), apierrors.IsServerTimeout(err):
		return "Kubernetes request timed out. Check the connection and refresh."
	case errors.Is(err, context.Canceled):
		return "Request cancelled."
	default:
		return "Cannot reach Kubernetes or complete authentication/TLS. Check this connection and refresh."
	}
}

func Identity(object unstructured.Unstructured) string {
	if object.GetUID() != "" {
		return string(object.GetUID())
	}
	return object.GetNamespace() + "/" + object.GetName()
}

func (c *Client) checkTarget() error {
	if c.target != nil {
		return c.target.CheckIdentity()
	}
	return nil
}
