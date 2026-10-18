package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	helmcommon "github.com/simjay/plous/internal/integrations/helm/common"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/release"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// SDKRunner preserves the bounded operation adapter used by the UI and guarded reads.
// Its arguments are internal requests, never shell commands.
type SDKRunner struct{}

// Helm storage drivers can panic while decoding malformed release records.
// Keep read failures inside the adapter without exposing raw storage contents.
func sdkReleaseRead[T any](read func() (T, error)) (value T, err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("Helm release storage could not be decoded")
		}
	}()
	return read()
}

type sdkRequest struct {
	words []string
	flags map[string][]string
}

func parseSDKRequest(args []string) (sdkRequest, error) {
	r := sdkRequest{flags: map[string][]string{}}
	bools := strings.Fields("all all-namespaces include-crds is-upgrade short no-hooks skip-tests generate-name debug devel")
	values := strings.Fields("max offset output revision namespace kube-context values set kube-version api-versions timeout name-template")
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			r.words = append(r.words, arg)
			continue
		}
		key := strings.TrimLeft(arg, "-")
		if key == "f" {
			key = "values"
		}
		known := false
		for _, b := range bools {
			if key == b {
				r.flags[key] = []string{"true"}
				known = true
				break
			}
		}
		if known {
			continue
		}
		for _, v := range values {
			if key == v {
				known = true
				break
			}
		}
		if !known || i+1 >= len(args) {
			return r, fmt.Errorf("unsupported Helm option %s", arg)
		}
		i++
		r.flags[key] = append(r.flags[key], args[i])
	}
	if len(r.words) == 0 {
		return r, errors.New("choose a Helm operation")
	}
	return r, nil
}
func (r sdkRequest) value(key string) string {
	v := r.flags[key]
	if len(v) > 0 {
		return v[len(v)-1]
	}
	return ""
}
func (r sdkRequest) number(key string) int { n, _ := strconv.Atoi(r.value(key)); return n }
func (r sdkRequest) has(key string) bool   { return len(r.flags[key]) > 0 }

func (SDKRunner) Run(ctx context.Context, config Config, args []string, input string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r, err := parseSDKRequest(args)
	if err != nil {
		return nil, err
	}
	if r.value("kube-context") != "" && r.value("kube-context") != config.Context {
		return nil, errors.New("Helm request changed its captured context")
	}
	ns := r.value("namespace")
	if ns == "" {
		ns = config.Namespace
	}
	if ns == "" {
		ns = "default"
	}
	var result any
	var text string
	word := func(n int) string {
		if n < len(r.words) {
			return r.words[n]
		}
		return ""
	}
	switch word(0) {
	case "version":
		return []byte(helmcommon.EngineVersion), nil
	case "template":
		name, chartPath := word(1), word(2)
		if r.has("generate-name") {
			chartPath = word(1)
			name = "release-name"
		}
		return helmcommon.Render(ctx, helmcommon.TemplateRequest{ChartPath: chartPath, Release: name, Namespace: ns,
			ValuesFiles: r.flags["values"], Overrides: r.flags["set"], ValuesInput: input,
			APIVersions: r.flags["api-versions"], KubeVersion: r.value("kube-version"), NameTemplate: r.value("name-template"),
			IsUpgrade: r.has("is-upgrade"), IncludeCRDs: r.has("include-crds"), NoHooks: r.has("no-hooks"), SkipTests: r.has("skip-tests")})
	case "lint":
		return helmcommon.Lint(ctx, word(1), r.flags["values"], r.flags["set"], input)
	case "list", "get", "history", "status", "rollback":
		cfg, e := newSDKConfiguration(ctx, config, ns, r.has("all-namespaces"))
		if e != nil {
			return nil, e
		}
		switch word(0) {
		case "list":
			l := action.NewList(cfg)
			l.All = true
			l.StateMask = action.ListAll
			l.Limit = r.number("max")
			l.Offset = r.number("offset")
			l.AllNamespaces = r.has("all-namespaces")
			releases, e := sdkReleaseRead(l.Run)
			if e != nil {
				return nil, e
			}
			rows := []map[string]string{}
			for _, release := range releases {
				if release == nil || release.Info == nil {
					return nil, errors.New("Helm release storage is missing release information")
				}
				rows = append(rows, map[string]string{"name": release.Name, "namespace": release.Namespace, "revision": strconv.Itoa(release.Version), "status": release.Info.Status.String(), "updated": release.Info.LastDeployed.Format(time.RFC3339)})
			}
			result = rows
		case "get":
			if word(1) == "metadata" {
				g := action.NewGetMetadata(cfg)
				g.Version = r.number("revision")
				result, err = sdkReleaseRead(func() (*action.Metadata, error) { return g.Run(word(2)) })
			} else if word(1) == "values" {
				g := action.NewGetValues(cfg)
				g.Version = r.number("revision")
				g.AllValues = r.has("all")
				result, err = sdkReleaseRead(func() (map[string]interface{}, error) { return g.Run(word(2)) })
			} else {
				return nil, errors.New("unsupported Helm read")
			}
		case "status":
			g := action.NewGet(cfg)
			g.Version = r.number("revision")
			result, err = sdkReleaseRead(func() (*release.Release, error) { return g.Run(word(1)) })
		case "history":
			h := action.NewHistory(cfg)
			h.Max = r.number("max")
			releases, e := sdkReleaseRead(func() ([]*release.Release, error) { return h.Run(word(1)) })
			if e != nil {
				return nil, e
			}
			rows := []Revision{}
			for _, release := range releases {
				if release == nil || release.Info == nil || release.Chart == nil || release.Chart.Metadata == nil {
					return nil, errors.New("Helm release history is missing release or chart information")
				}
				rows = append(rows, Revision{Revision: release.Version, Updated: release.Info.LastDeployed.Format(time.RFC3339), Status: release.Info.Status.String(), Chart: release.Chart.Metadata.Name + "-" + release.Chart.Metadata.Version, Description: release.Info.Description})
			}
			result = rows
		case "rollback":
			rollback := action.NewRollback(cfg)
			rollback.Version, _ = strconv.Atoi(word(2))
			rollback.Timeout = 5 * time.Minute
			err = rollback.Run(word(1))
		}
	default:
		return nil, errors.New("unsupported Helm operation")
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	data := []byte(text)
	if result != nil {
		data, err = json.Marshal(result)
	}
	if len(data) > helmcommon.MaxOutput {
		return nil, helmcommon.ErrOutputLimit
	}
	return data, err
}

type sdkGetter struct {
	loader clientcmd.ClientConfig
	ctx    context.Context
}

func (g sdkGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig { return g.loader }
func (g sdkGetter) ToRESTConfig() (*rest.Config, error) {
	c, err := g.loader.ClientConfig()
	if err != nil {
		return nil, err
	}
	if c.ExecProvider != nil {
		c.ExecProvider.StdinUnavailable = true
		c.ExecProvider.StdinUnavailableMessage = "Authenticate outside Plous, then refresh."
	}
	c.Timeout = 30 * time.Second
	c.Wrap(func(rt http.RoundTripper) http.RoundTripper {
		return helmcommon.OperationTransport{Context: g.ctx, Base: rt}
	})
	return c, nil
}
func (g sdkGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	c, err := g.ToRESTConfig()
	if err != nil {
		return nil, err
	}
	d, err := discovery.NewDiscoveryClientForConfig(c)
	if err != nil {
		return nil, err
	}
	return memory.NewMemCacheClient(d), nil
}
func (g sdkGetter) ToRESTMapper() (meta.RESTMapper, error) {
	d, err := g.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}
	return restmapper.NewDeferredDiscoveryRESTMapper(d), nil
}

func newSDKConfiguration(ctx context.Context, c Config, ns string, all bool) (*action.Configuration, error) {
	if c.Context == "" {
		return nil, errors.New("select an explicit Helm kubecontext")
	}
	if c.Driver == "" {
		c.Driver = "secret"
	}
	if c.Driver != "secret" && c.Driver != "configmap" {
		return nil, errors.New("unsupported Helm release storage")
	}
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	rules.MigrationRules = nil
	rules.Precedence = filepath.SplitList(c.Kubeconfig)
	getter := sdkGetter{ctx: ctx, loader: clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{CurrentContext: c.Context, Context: clientcmdapi.Context{Namespace: ns}})}
	if c.Target != nil {
		getter.loader = c.Target.Loader(ns)
	}
	cfg := &action.Configuration{}
	storageNS := ns
	if all {
		storageNS = ""
	}
	if err := cfg.Init(getter, storageNS, c.Driver, func(string, ...interface{}) {}); err != nil {
		return nil, err
	}
	return cfg, nil
}
