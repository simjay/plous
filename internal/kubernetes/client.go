package kubernetes

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	apiVersion "k8s.io/apimachinery/pkg/version"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const MaxObjects = 10000

type Resource struct {
	Group, Version, Name, Kind string
	Namespaced                 bool
	Verbs, ShortNames          []string
	Subresources               map[string][]string
}

func (r Resource) Supports(verb, subresource string) bool {
	if subresource == "" {
		return slices.Contains(r.Verbs, verb)
	}
	return slices.Contains(r.Subresources[subresource], verb)
}

func (r Resource) GVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: r.Group, Version: r.Version, Resource: r.Name}
}
func (r Resource) Key() string {
	if r.Group == "" {
		return r.Name
	}
	return r.Name + "." + r.Group
}
func (r Resource) Matches(name string) bool {
	return name == r.Key() || name == r.Name || strings.EqualFold(name, r.Kind) || slices.Contains(r.ShortNames, name)
}

type Collection struct {
	Objects          []unstructured.Unstructured
	Version          string
	Columns          []string
	Cells            map[string][]string
	Truncated        bool
	ServiceEndpoints map[string]EndpointReadiness
}
type Snapshot struct {
	Collection *Collection
	Status     string
	Err        error
}
type Client struct {
	dynamic   dynamic.Interface
	rest      rest.Interface
	config    *rest.Config
	Namespace string
	catalogMu sync.RWMutex
	catalog   []Resource
}

func New(paths, contextName, namespace string) (*Client, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	// Loading must not migrate or write the owner's kubeconfig files.
	rules.MigrationRules = nil
	if paths != "" {
		rules.Precedence = nil
		for _, path := range filepath.SplitList(paths) {
			if strings.HasPrefix(path, "~/") {
				home, err := os.UserHomeDir()
				if err != nil {
					return nil, errors.New("Cannot locate kubeconfig home directory.")
				}
				path = filepath.Join(home, path[2:])
			}
			if path != "" {
				rules.Precedence = append(rules.Precedence, path)
			}
		}
	}
	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{CurrentContext: contextName})
	config, err := loader.ClientConfig()
	if err != nil {
		return nil, errors.New("Cannot load this kubecontext. Check its cluster, credentials, and kubeconfig paths in Settings.")
	}
	if namespace == "" {
		namespace, _, err = loader.Namespace()
		if err != nil {
			return nil, errors.New("Cannot read the kubecontext namespace.")
		}
	}
	if config.ExecProvider != nil {
		config.ExecProvider.StdinUnavailable = true
		config.ExecProvider.StdinUnavailableMessage = "Authenticate outside Plous, then refresh Explore."
	}
	c, err := NewForConfig(config)
	if err != nil {
		return nil, errors.New("Cannot initialize Kubernetes authentication or TLS. Check the selected kubeconfig.")
	}
	c.Namespace = namespace
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
	return &Client{dynamic: dynamic.New(raw), rest: raw, config: rest.CopyConfig(copy), Namespace: "default"}, nil
}

func (c *Client) read(ctx context.Context, path string, into any) error {
	data, err := c.rest.Get().AbsPath(path).Timeout(12 * time.Second).Do(ctx).Raw()
	if err != nil {
		return err
	}
	return json.Unmarshal(data, into)
}

func (c *Client) Discover(ctx context.Context) ([]Resource, string, error) {
	var groups metav1.APIGroupList
	groupErr := c.read(ctx, "/apis", &groups)
	versions := []schema.GroupVersion{{Version: "v1"}}
	preferred := map[string]string{"": "v1"}
	for _, group := range groups.Groups {
		gv, err := schema.ParseGroupVersion(group.PreferredVersion.GroupVersion)
		if err == nil && gv.Version != "" {
			preferred[gv.Group] = gv.Version
			versions = append(versions, gv)
		}
		for _, version := range group.Versions {
			gv, err := schema.ParseGroupVersion(version.GroupVersion)
			if err == nil && gv.Version != "" && !slices.Contains(versions, gv) {
				versions = append(versions, gv)
			}
		}
	}
	type discoveryResult struct {
		list metav1.APIResourceList
		err  error
	}
	results := make([]discoveryResult, len(versions))
	var failures []error
	if groupErr != nil {
		failures = append(failures, groupErr)
	}
	var wg sync.WaitGroup
	limit := make(chan struct{}, 6)
	for i, gv := range versions {
		wg.Go(func() {
			select {
			case limit <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-limit }()
			path := "/api/" + gv.Version
			if gv.Group != "" {
				path = "/apis/" + gv.String()
			}
			results[i].err = c.read(ctx, path, &results[i].list)
		})
	}
	wg.Wait()
	if ctx.Err() != nil {
		return nil, "", ctx.Err()
	}
	selected := map[string]Resource{}
	for i, result := range results {
		if result.err != nil {
			failures = append(failures, result.err)
			continue
		}
		gv := versions[i]
		subresources := map[string]map[string][]string{}
		for _, item := range result.list.APIResources {
			parent, subresource, ok := strings.Cut(item.Name, "/")
			if !ok {
				continue
			}
			if subresources[parent] == nil {
				subresources[parent] = map[string][]string{}
			}
			subresources[parent][subresource] = slices.Clone(item.Verbs)
		}
		for _, item := range result.list.APIResources {
			if strings.Contains(item.Name, "/") || !slices.Contains(item.Verbs, "list") {
				continue
			}
			resource := Resource{Group: gv.Group, Version: gv.Version, Name: item.Name, Kind: item.Kind, Namespaced: item.Namespaced, Verbs: item.Verbs, ShortNames: item.ShortNames}
			resource.Subresources = subresources[item.Name]
			previous, exists := selected[resource.Key()]
			// A group's preferred version may omit a resource served by another
			// version. Prefer it only when it actually advertises this resource.
			if !exists || resource.Version == preferred[gv.Group] || (previous.Version != preferred[gv.Group] && apiVersion.CompareKubeAwareVersionStrings(resource.Version, previous.Version) > 0) {
				selected[resource.Key()] = resource
			}
		}
	}
	resources := make([]Resource, 0, len(selected))
	for _, resource := range selected {
		resources = append(resources, resource)
	}
	if len(resources) == 0 {
		if len(failures) > 0 {
			return nil, "", failures[0]
		}
		return nil, "", errors.New("The cluster returned no listable resource types.")
	}
	slices.SortFunc(resources, CompareResources)
	c.catalogMu.Lock()
	c.catalog = slices.Clone(resources)
	c.catalogMu.Unlock()
	warning := ""
	if len(failures) > 0 {
		warning = "Some API groups could not be discovered. Refresh to retry."
	}
	return resources, warning, nil
}

func (c *Client) resource(r Resource, ns string) dynamic.ResourceInterface {
	api := c.dynamic.Resource(r.GVR())
	if r.Namespaced {
		return api.Namespace(ns)
	}
	return api
}
func resourcePath(r Resource, ns string) string {
	path := "/api/" + r.Version
	if r.Group != "" {
		path = "/apis/" + r.Group + "/" + r.Version
	}
	if r.Namespaced && ns != "" {
		path += "/namespaces/" + ns
	}
	return path + "/" + r.Name
}

func (c *Client) List(ctx context.Context, r Resource, ns, selector string) (Collection, error) {
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

func (c *Client) Get(ctx context.Context, r Resource, ns, name string) (*unstructured.Unstructured, error) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	object, err := c.resource(r, ns).Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		setType(object, r)
	}
	return object, err
}

func setType(object *unstructured.Unstructured, r Resource) {
	if object.GetKind() == "" {
		object.SetKind(r.Kind)
	}
	if object.GetAPIVersion() == "" {
		object.SetAPIVersion(r.GVR().GroupVersion().String())
	}
}

// Watch notifications coalesce into a fresh table so CRD printer columns stay current.
func (c *Client) Observe(ctx context.Context, r Resource, ns, selector string, emit func(Snapshot)) {
	backoff := time.Second
	for ctx.Err() == nil {
		collection, err := c.List(ctx, r, ns, selector)
		if err != nil {
			emit(Snapshot{Err: err, Status: "Disconnected; retrying"})
			if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) || apierrors.IsNotFound(err) {
				return
			}
			if !pause(ctx, backoff) {
				return
			}
			backoff = min(backoff*2, 30*time.Second)
			continue
		}
		backoff = time.Second
		status := "Live"
		if !slices.Contains(r.Verbs, "watch") {
			status = "Refreshing every 5s"
		}
		emit(Snapshot{Collection: &collection, Status: status})
		if !slices.Contains(r.Verbs, "watch") {
			if !pause(ctx, 5*time.Second) {
				return
			}
			continue
		}
		seconds := int64(300)
		stream, err := c.resource(r, ns).Watch(ctx, metav1.ListOptions{LabelSelector: selector, ResourceVersion: collection.Version, TimeoutSeconds: &seconds, AllowWatchBookmarks: true})
		if err != nil {
			if apierrors.IsForbidden(err) {
				r.Verbs = slices.DeleteFunc(slices.Clone(r.Verbs), func(v string) bool { return v == "watch" })
				emit(Snapshot{Status: "Watch denied; refreshing every 5s"})
				if !pause(ctx, 5*time.Second) {
					return
				}
				continue
			}
			emit(Snapshot{Err: err, Status: "Watch disconnected; retrying"})
			if !pause(ctx, backoff) {
				return
			}
			backoff = min(backoff*2, 30*time.Second)
			continue
		}
		err = c.consume(ctx, stream, r, ns, selector, emit)
		stream.Stop()
		if ctx.Err() != nil {
			return
		}
		if err != nil && !apierrors.IsResourceExpired(err) && !apierrors.IsGone(err) {
			emit(Snapshot{Err: err, Status: "Watch disconnected; retrying"})
		}
		if !pause(ctx, time.Second) {
			return
		}
	}
}

func (c *Client) consume(ctx context.Context, stream watch.Interface, r Resource, ns, selector string, emit func(Snapshot)) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	dirty := false
	lastRead := time.Now()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-stream.ResultChan():
			if !ok {
				return nil
			}
			if event.Type == watch.Error {
				return apierrors.FromObject(event.Object)
			}
			if event.Type != watch.Bookmark {
				dirty = true
			}
		case <-ticker.C:
			if r.Key() == "services" && time.Since(lastRead) >= 5*time.Second {
				dirty = true
			}
			if !dirty {
				continue
			}
			dirty = false
			lastRead = time.Now()
			collection, err := c.List(ctx, r, ns, selector)
			if err != nil {
				return err
			}
			emit(Snapshot{Collection: &collection, Status: "Live"})
		}
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
		pod, err := c.Get(ctx, Resource{Name: "pods", Version: "v1", Kind: "Pod", Namespaced: true}, options.Namespace, options.Pod)
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
