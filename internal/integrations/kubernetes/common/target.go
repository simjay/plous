package common

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
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
	resolved, err := normalizeConfigPaths(rules.Precedence)
	if err != nil {
		return nil, "", err
	}
	rules.Precedence = resolved
	configPaths := strings.Join(resolved, string(os.PathListSeparator))
	found := false
	for _, path := range rules.Precedence {
		// Validate bounded input without surfacing parser excerpts or credentials.
		if _, err = readContexts(path); errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, "", err
		}
		found = true
	}
	if !found {
		return nil, "", fmt.Errorf("No kubeconfig files found in %q. Choose an existing kubeconfig path.", configPaths)
	}
	raw, err := rules.Load()
	if err != nil {
		return nil, "", fmt.Errorf("Cannot load kubeconfig from %q. Check the Kubernetes config structure.", configPaths)
	}
	return raw, configPaths, nil
}

// NormalizeConfigPaths anchors explicitly supplied relative paths without loading credentials.
func NormalizeConfigPaths(paths string) (string, error) {
	resolved, err := normalizeConfigPaths(filepath.SplitList(paths))
	if err != nil {
		return "", err
	}
	if paths != "" && len(resolved) == 0 {
		return "", errors.New("Choose a kubeconfig path.")
	}
	return strings.Join(resolved, string(os.PathListSeparator)), nil
}

func normalizeConfigPaths(paths []string) ([]string, error) {
	var resolved []string
	for _, path := range paths {
		if path == "" {
			continue
		}
		absolute, err := resolvePath(path)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, absolute)
	}
	return resolved, nil
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
		return nil, fmt.Errorf("No current context is configured in kubeconfig %q. Choose a named context or another kubeconfig.", paths)
	}
	c := raw.Contexts[name]
	if c == nil {
		return nil, fmt.Errorf("Context %q was not found in kubeconfig %q.", name, paths)
	}
	if raw.Clusters[c.Cluster] == nil {
		return nil, fmt.Errorf("Context %q references a missing cluster %q in kubeconfig %q.", name, c.Cluster, paths)
	}
	raw.CurrentContext = name
	if err = clientcmdapi.MinifyConfig(raw); err != nil {
		return nil, fmt.Errorf("Cannot resolve context %q. Check its user and cluster references in kubeconfig %q.", name, paths)
	}
	if err = clientcmdapi.FlattenConfig(raw); err != nil {
		return nil, fmt.Errorf("Cannot read certificate or key files for context %q. Check the paths and permissions in kubeconfig %q.", name, paths)
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
		return nil, fmt.Errorf("Context %q has an invalid API endpoint in kubeconfig %q.", name, paths)
	}
	endpoint.User, endpoint.RawQuery, endpoint.Fragment = nil, "", ""
	proxy := ""
	if cluster.ProxyURL != "" {
		parsed, err := url.Parse(cluster.ProxyURL)
		if err != nil || parsed.Host == "" {
			return nil, fmt.Errorf("Context %q has an invalid proxy endpoint in kubeconfig %q.", name, paths)
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

// SameConfig compares snapshots without exposing credentials or config contents.
func (t *Target) SameConfig(other *Target) bool {
	return t != nil && other != nil && t.Fingerprint == other.Fingerprint &&
		t.Paths == other.Paths && t.Namespace == other.Namespace && reflect.DeepEqual(t.raw, other.raw)
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
