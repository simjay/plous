package common

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// Target owns a private copy of the selected configuration. It is never serialized.
type Target struct {
	Paths, Context, Namespace, Fingerprint string
	raw                                    clientcmdapi.Config
}

type ContextInventory struct {
	Paths, Current string
	Contexts       []Context
}

func loadConfiguration(paths string) (*clientcmdapi.Config, string, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	rules.MigrationRules = nil
	if paths != "" {
		rules.Precedence = filepath.SplitList(paths)
	}
	resolved := make([]string, 0, len(rules.Precedence))
	for _, path := range rules.Precedence {
		if path == "" {
			continue
		}
		path, err := resolvePath(path)
		if err != nil {
			return nil, "", err
		}
		// Validate bounded input without surfacing parser excerpts or credentials.
		if _, err = readContexts(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, "", err
		}
		resolved = append(resolved, path)
	}
	rules.Precedence = resolved
	raw, err := rules.Load()
	if err != nil {
		return nil, "", errors.New("Cannot load kubeconfig. Check its paths and YAML configuration.")
	}
	return raw, strings.Join(resolved, string(os.PathListSeparator)), nil
}

// DiscoverCurrent reads local metadata only, without clients or authentication.
func DiscoverCurrent(paths string) (ContextInventory, error) {
	raw, paths, err := loadConfiguration(paths)
	if err != nil {
		return ContextInventory{}, err
	}
	result := ContextInventory{Paths: paths, Current: raw.CurrentContext}
	for name, c := range raw.Contexts {
		result.Contexts = append(result.Contexts, Context{Name: name, Namespace: c.Namespace, Cluster: c.Cluster, Source: c.LocationOfOrigin, Kubeconfig: paths})
	}
	sort.Slice(result.Contexts, func(i, j int) bool { return result.Contexts[i].Name < result.Contexts[j].Name })
	return result, nil
}

func ResolveTarget(paths, name, namespace string) (*Target, error) {
	raw, paths, err := loadConfiguration(paths)
	if err != nil {
		return nil, err
	}
	if name == "" {
		name = raw.CurrentContext
	}
	if name == "" {
		return nil, errors.New("No current context is configured. Choose a context or kubeconfig.")
	}
	c := raw.Contexts[name]
	if c == nil || raw.Clusters[c.Cluster] == nil {
		return nil, errors.New("The selected context or its cluster is missing. Choose a context or refresh kubeconfig.")
	}
	raw.CurrentContext = name
	if err = clientcmdapi.MinifyConfig(raw); err != nil {
		return nil, errors.New("Cannot resolve the selected context's configuration.")
	}
	if err = clientcmdapi.FlattenConfig(raw); err != nil {
		return nil, errors.New("Cannot read the selected context's certificate files.")
	}
	if namespace == "" {
		namespace = c.Namespace
	}
	if namespace == "" {
		namespace = "default"
	}
	cluster := raw.Clusters[c.Cluster]
	endpoint, err := url.Parse(cluster.Server)
	if err != nil || endpoint.Host == "" {
		return nil, errors.New("The selected context has an invalid API endpoint.")
	}
	endpoint.User, endpoint.RawQuery, endpoint.Fragment = nil, "", ""
	proxy := ""
	if cluster.ProxyURL != "" {
		parsed, err := url.Parse(cluster.ProxyURL)
		if err != nil || parsed.Host == "" {
			return nil, errors.New("The selected context has an invalid proxy endpoint.")
		}
		parsed.User, parsed.RawQuery, parsed.Fragment = nil, "", ""
		proxy = parsed.String()
	}
	trust := sha256.Sum256(cluster.CertificateAuthorityData)
	identity := struct {
		Version                                               int
		Context, Cluster, User, Server, TLSName, Trust, Proxy string
		Insecure                                              bool
	}{1, name, c.Cluster, c.AuthInfo, endpoint.String(), cluster.TLSServerName, fmt.Sprintf("%x", trust), proxy, cluster.InsecureSkipTLSVerify}
	data, _ := json.Marshal(identity)
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(data))
	return &Target{Paths: paths, Context: name, Namespace: namespace, Fingerprint: fingerprint, raw: *raw.DeepCopy()}, nil
}

func (t *Target) Loader(namespace string) clientcmd.ClientConfig {
	if namespace == "" {
		namespace = t.Namespace
	}
	return clientcmd.NewNonInteractiveClientConfig(*t.raw.DeepCopy(), t.Context, &clientcmd.ConfigOverrides{CurrentContext: t.Context, Context: clientcmdapi.Context{Namespace: namespace}}, nil)
}

func (t *Target) CheckIdentity() error {
	fresh, err := ResolveTarget(t.Paths, t.Context, t.Namespace)
	if err != nil {
		return err
	}
	if fresh.Fingerprint != t.Fingerprint {
		return errors.New("The cluster configuration changed. Reselect the context before writing.")
	}
	return nil
}

func (t *Target) Server() string { return t.raw.Clusters[t.raw.Contexts[t.Context].Cluster].Server }
