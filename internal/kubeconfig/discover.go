// Package kubeconfig discovers context references without contacting clusters,
// loading credentials into Plous state, or running kubeconfig auth plugins.
package kubeconfig

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

const maxFileSize = 16 << 20

// Context contains the metadata for selecting a kubecontext. Source is the
// absolute path of its defining file. Kubeconfig is the full path list, which
// may include other files defining the referenced cluster and user.
type Context struct {
	Name       string
	Namespace  string
	Cluster    string
	Source     string
	Kubeconfig string
}

// Discover returns contexts in name order. An explicit path reads only that
// file. An empty path uses KUBECONFIG's platform-specific path list, or
// ~/.kube/config when KUBECONFIG is empty. A leading ~/ is expanded. Missing
// files in a KUBECONFIG list are skipped; other read or parse errors are reported.
// When several files define the same context, the first definition wins.
func Discover(path string) ([]Context, error) {
	paths, optional, err := sourcePaths(path)
	if err != nil {
		return nil, err
	}
	contexts := make(map[string]Context)
	seenPaths := make(map[string]bool)
	var effectivePaths []string
	for _, source := range paths {
		resolved, err := resolvePath(source)
		if err != nil {
			return nil, err
		}
		if seenPaths[resolved] {
			continue
		}
		seenPaths[resolved] = true
		found, err := readContexts(resolved)
		if err != nil {
			if optional && errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		effectivePaths = append(effectivePaths, resolved)
		for _, context := range found {
			if _, exists := contexts[context.Name]; !exists {
				contexts[context.Name] = context
			}
		}
	}
	if len(effectivePaths) == 0 {
		return nil, errors.New("no kubeconfig files found in KUBECONFIG; choose an existing kubeconfig path")
	}
	if len(contexts) == 0 {
		return nil, errors.New("no contexts found in kubeconfig; choose a file containing named contexts")
	}
	result := make([]Context, 0, len(contexts))
	kubeconfig := strings.Join(effectivePaths, string(os.PathListSeparator))
	for _, context := range contexts {
		context.Kubeconfig = kubeconfig
		result = append(result, context)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func sourcePaths(path string) ([]string, bool, error) {
	if path != "" {
		return []string{path}, false, nil
	}
	if value := os.Getenv("KUBECONFIG"); value != "" {
		var paths []string
		for _, entry := range filepath.SplitList(value) {
			if entry != "" {
				paths = append(paths, entry)
			}
		}
		return paths, true, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, false, fmt.Errorf("locate home directory for kubeconfig: %w", err)
	}
	return []string{filepath.Join(home, ".kube", "config")}, false, nil
}

func resolvePath(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expand kubeconfig home directory: %w", err)
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, path[2:])
		}
	}
	resolved, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve kubeconfig path: %w", err)
	}
	return resolved, nil
}

func readContexts(path string) ([]Context, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("read kubeconfig %q (choose an existing kubeconfig path): %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("kubeconfig %q must be a regular file", path)
	}
	if info.Size() > maxFileSize {
		return nil, fmt.Errorf("kubeconfig %q exceeds the 16 MiB input limit", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open kubeconfig %q: %w", path, err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("read kubeconfig %q: %w", path, err)
	}
	if len(data) > maxFileSize {
		return nil, fmt.Errorf("kubeconfig %q exceeds the 16 MiB input limit", path)
	}
	// Exclude credentials and authentication plugins from discovery results.
	var config struct {
		Contexts []struct {
			Name    string `yaml:"name"`
			Context struct {
				Namespace string `yaml:"namespace"`
				Cluster   string `yaml:"cluster"`
			} `yaml:"context"`
		} `yaml:"contexts"`
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&config); err != nil && !errors.Is(err, io.EOF) {
		// Parser errors can quote input values, including credentials. Keep them
		// out of UI messages and persisted activity.
		return nil, fmt.Errorf("invalid kubeconfig YAML in %q", path)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("kubeconfig %q must contain one YAML document", path)
	}
	contexts := make([]Context, 0, len(config.Contexts))
	for i, entry := range config.Contexts {
		if strings.TrimSpace(entry.Name) == "" {
			return nil, fmt.Errorf("kubeconfig %q has an unnamed context at entry %d", path, i+1)
		}
		contexts = append(contexts, Context{
			Name: entry.Name, Namespace: entry.Context.Namespace,
			Cluster: entry.Context.Cluster, Source: path,
		})
	}
	return contexts, nil
}
