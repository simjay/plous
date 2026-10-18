// Package monitor owns captured Helm connections and live release operations.
package monitor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	helmcommon "github.com/simjay/plous/internal/integrations/helm/common"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	manifest "github.com/simjay/plous/internal/integrations/manifest"
	"k8s.io/client-go/tools/clientcmd"
)

type Config struct {
	TargetCaptured                                            bool               `json:"-"`
	Target                                                    *kubecommon.Target `json:"-"`
	TargetFingerprint                                         string             `json:"target_fingerprint,omitempty"`
	ConnectionID, Kubeconfig, Context, Namespace              string
	Driver, RepositoryConfig, RepositoryCache, RegistryConfig string
}

func (c Config) Fingerprint() string {
	if c.Target != nil {
		c.TargetFingerprint = c.Target.Fingerprint
	}
	data, _ := json.Marshal(c)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

type Runner interface {
	Run(context.Context, Config, []string, string) ([]byte, error)
}
type Client struct {
	Config         Config
	Runner         Runner
	targetIdentity func() (string, error)
}

func New(config Config) (*Client, error) {
	if config.Context == "" {
		return nil, errors.New("select an explicit Helm kubecontext")
	}
	if config.Driver == "" {
		config.Driver = "secret"
	}
	if config.Driver != "secret" && config.Driver != "configmap" {
		return nil, errors.New("Helm storage must be secret or configmap")
	}
	var err error
	config.Kubeconfig, err = helmcommon.ExpandPaths(config.Kubeconfig)
	if err != nil {
		return nil, err
	}
	if config.Kubeconfig == "" {
		home, e := os.UserHomeDir()
		if e != nil {
			return nil, e
		}
		config.Kubeconfig = filepath.Join(home, ".kube", "config")
	}
	if config.Target == nil && config.TargetCaptured {
		return nil, errors.New("The selected cluster configuration is unavailable. Reselect the connection.")
	}
	if config.Target == nil {
		config.Target, err = kubecommon.ResolveTarget(config.Kubeconfig, config.Context, config.Namespace)
		if err != nil {
			return nil, err
		}
	}
	config.TargetFingerprint = config.Target.Fingerprint
	if config.Namespace == "" {
		config.Namespace = config.Target.Namespace
	}
	if err := validNamespace(config.Namespace); err != nil {
		return nil, err
	}
	return &Client{Config: config, Runner: SDKRunner{}, targetIdentity: func() (string, error) {
		currentRules := clientcmd.NewDefaultClientConfigLoadingRules()
		currentRules.MigrationRules = nil
		currentRules.Precedence = filepath.SplitList(config.Kubeconfig)
		current, e := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(currentRules, &clientcmd.ConfigOverrides{CurrentContext: config.Context}).RawConfig()
		if e != nil || current.Contexts[config.Context] == nil {
			return "", errors.New("cannot recheck the selected kubeconfig")
		}
		data, _ := json.Marshal(current)
		return fmt.Sprintf("%x", sha256.Sum256(data)), nil
	}}, nil
}

func (c *Client) captureTarget() (string, error) {
	if c.Config.Target != nil {
		if err := c.Config.Target.CheckIdentity(); err != nil {
			return "", err
		}
	}
	if c.targetIdentity == nil {
		return "", nil
	}
	return c.targetIdentity()
}

func SafeText(s string) string { return manifest.SafeText(s) }

// Keep service error details for the UI while distinguishing failed acquisition
// from invalid response data at the projection boundary.
type readError struct{ error }

func (e *readError) Unwrap() error { return e.error }

func (c *Client) run(ctx context.Context, ns string, args ...string) ([]byte, error) {
	if ns == "" {
		ns = c.Config.Namespace
	}
	if err := validNamespace(ns); err != nil {
		return nil, err
	}
	args = append(args, "--kube-context", c.Config.Context, "--namespace", ns)
	data, err := c.Runner.Run(ctx, c.Config, args, "")
	if err != nil {
		return nil, &readError{err}
	}
	return data, nil
}

type Release struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Revision   int    `json:"revision"`
	Chart      string `json:"chart"`
	Version    string `json:"version"`
	AppVersion string `json:"appVersion"`
	Status     string `json:"status"`
	Updated    string `json:"deployedAt"`
	Error      string `json:"error,omitempty"`
}

func (r Release) Key() string { return r.Namespace + "/" + r.Name }
func (r Release) ChartKey() string {
	if r.Error != "" {
		return "Metadata unavailable / " + r.Key()
	}
	return r.Chart + " / " + r.Version
}
func (c *Client) List(ctx context.Context, namespace string) ([]Release, error) {
	var result []Release
	for offset := 0; offset < 10000; offset += 100 {
		args := []string{"list", "--all", "--max", "100", "--offset", strconv.Itoa(offset), "--output", "json"}
		ns := namespace
		if ns == "*" {
			args = append(args, "--all-namespaces")
			ns = c.Config.Namespace
		}
		data, err := c.run(ctx, ns, args...)
		if err != nil {
			return result, err
		}
		var rows []struct{ Name, Namespace, Revision, Chart, Status, Updated string }
		if err = json.Unmarshal(data, &rows); err != nil {
			return result, errors.New("invalid Helm release list")
		}
		page := make([]Release, len(rows))
		for i, row := range rows {
			if err := validRelease(row.Name); err != nil || validNamespace(row.Namespace) != nil || namespace != "*" && row.Namespace != ns {
				return result, errors.New("Helm returned a release outside the requested scope")
			}
			rev, e := strconv.Atoi(row.Revision)
			if e != nil || rev < 1 {
				return result, errors.New("invalid Helm revision")
			}
			page[i] = Release{Name: row.Name, Namespace: row.Namespace, Revision: rev, Status: row.Status, Updated: row.Updated}
		}
		jobs := make(chan int, len(page))
		for i := range page {
			jobs <- i
		}
		close(jobs)
		var group sync.WaitGroup
		for range min(4, len(page)) {
			group.Add(1)
			go func() {
				defer group.Done()
				for i := range jobs {
					if ctx.Err() != nil {
						page[i].Error = "metadata request cancelled"
						continue
					}
					r := page[i]
					metadata, e := c.Metadata(ctx, r.Name, r.Namespace, r.Revision)
					if e != nil {
						page[i].Error = e.Error()
					} else {
						page[i] = metadata
					}
				}
			}()
		}
		group.Wait()
		if err := ctx.Err(); err != nil {
			return append(result, page...), err
		}
		result = append(result, page...)
		if len(rows) < 100 {
			return result, nil
		}
	}
	return result, errors.New("release inventory reached 10,000 entries; select a namespace")
}
func (c *Client) Metadata(ctx context.Context, name, ns string, revision int) (Release, error) {
	var r Release
	if err := validRelease(name); err != nil {
		return r, err
	}
	args := []string{"get", "metadata", name, "--output", "json"}
	if revision > 0 {
		args = append(args, "--revision", strconv.Itoa(revision))
	}
	data, err := c.run(ctx, ns, args...)
	if err != nil {
		return r, err
	}
	if err = json.Unmarshal(data, &r); err != nil {
		return r, errors.New("invalid Helm metadata")
	}
	if r.Name != name || r.Namespace != ns || revision > 0 && r.Revision != revision {
		return r, errors.New("Helm returned a different release identity")
	}
	return r, nil
}

type Revision struct {
	Revision    int    `json:"revision"`
	Updated     string `json:"updated"`
	Status      string `json:"status"`
	Chart       string `json:"chart"`
	Description string `json:"description"`
}

func (c *Client) History(ctx context.Context, name, ns string) ([]Revision, error) {
	if err := validRelease(name); err != nil {
		return nil, err
	}
	data, err := c.run(ctx, ns, "history", name, "--max", "1000", "--output", "json")
	if err != nil {
		return nil, err
	}
	var out []Revision
	if err = json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	seen := map[int]bool{}
	for _, r := range out {
		if r.Revision < 1 || seen[r.Revision] {
			return nil, errors.New("invalid Helm revision history")
		}
		seen[r.Revision] = true
	}
	return out, nil
}

type Snapshot struct {
	Name      string           `json:"name"`
	Namespace string           `json:"namespace"`
	Version   int              `json:"version"`
	Chart     helmcommon.Chart `json:"chart"`
	Config    map[string]any   `json:"config"`
	Manifest  string           `json:"manifest"`
	Info      struct {
		Status       string `json:"status"`
		Notes        string `json:"notes"`
		Description  string `json:"description"`
		LastDeployed string `json:"last_deployed"`
	} `json:"info"`
	Hooks []struct {
		Name     string   `json:"name"`
		Manifest string   `json:"manifest"`
		Events   []string `json:"events"`
	} `json:"hooks"`
	Effective map[string]any `json:"-"`
	History   []Revision     `json:"-"`
}

func (s Snapshot) Release() Release {
	appVersion, _ := s.Chart.Metadata["appVersion"].(string)
	return Release{Name: s.Name, Namespace: s.Namespace, Revision: s.Version, Chart: fmt.Sprint(s.Chart.Metadata["name"]), Version: fmt.Sprint(s.Chart.Metadata["version"]), AppVersion: appVersion, Status: s.Info.Status, Updated: s.Info.LastDeployed}
}
func (c *Client) Snapshot(ctx context.Context, name, ns string, revision int) (*Snapshot, error) {
	s, err := c.RecordedSnapshot(ctx, name, ns, revision)
	if err != nil {
		return nil, err
	}
	data, err := c.run(ctx, ns, "get", "values", name, "--revision", strconv.Itoa(revision), "--all", "--output", "json")
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err = decoder.Decode(&s.Effective); err != nil {
		return nil, err
	}
	s.History, err = c.History(ctx, name, ns)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// RecordedSnapshot reads one exact retained record without querying current history.
func (c *Client) RecordedSnapshot(ctx context.Context, name, ns string, revision int) (*Snapshot, error) {
	if err := validRelease(name); err != nil {
		return nil, err
	}
	if revision < 1 {
		return nil, errors.New("an exact retained revision is required")
	}
	args := []string{"status", name, "--revision", strconv.Itoa(revision), "--output", "json"}
	data, err := c.run(ctx, ns, args...)
	if err != nil {
		return nil, err
	}
	var s Snapshot
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err = decoder.Decode(&s); err != nil {
		return nil, errors.New("invalid Helm release snapshot")
	}
	if s.Name != name || s.Namespace != ns || s.Version != revision {
		return nil, errors.New("Helm returned a different snapshot identity")
	}
	if len(s.Chart.Metadata) == 0 {
		stored, e := c.retainedSnapshot(ctx, name, ns, revision)
		if e != nil {
			return nil, e
		}
		s = *stored
	}
	return &s, nil
}
