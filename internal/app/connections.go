package app

import (
	appcode "github.com/simjay/plous/internal/app/code"
	"github.com/simjay/plous/internal/workspace"
	"maps"
	"os"
	"path/filepath"
	"strings"
)

func monitorCluster(tool string) bool { return tool == "kubernetes" || tool == "helm" }

func sourceConnection(tool string) bool { return tool == "code" || tool == "kustomize" }

func connectionArea(tool string) string {
	if appcode.IsBrowser(tool) || sourceConnection(tool) || tool == "files" || tool == "manifests" || tool == "tfcode" {
		return "code"
	}
	return "monitor"
}

func clusterSettings(c workspace.Connection) map[string]string {
	if !monitorCluster(c.Tool) {
		return nil
	}
	settings := map[string]string{}
	for key, value := range c.Settings {
		if value != "" {
			settings[key] = value
		}
	}
	paths := settings["kubeconfig"]
	if paths == "" && c.Tool == "kubernetes" {
		paths = os.Getenv("KUBECONFIG")
	}
	if paths == "" {
		paths = "~/.kube/config"
	}
	resolved, seen := []string{}, map[string]bool{}
	for _, path := range filepath.SplitList(paths) {
		if path == "" {
			continue
		}
		if strings.HasPrefix(path, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				path = filepath.Join(home, path[2:])
			}
		}
		if absolute, err := filepath.Abs(path); err == nil {
			path = absolute
		}
		if real, err := filepath.EvalSymlinks(path); err == nil {
			path = real
		}
		if !seen[path] {
			resolved, seen[path] = append(resolved, path), true
		}
	}
	settings["kubeconfig"] = strings.Join(resolved, string(os.PathListSeparator))
	if settings["driver"] == "" {
		settings["driver"] = "secret"
	}
	return settings
}

func sameClusterTarget(a, b workspace.Connection) bool {
	x, y := clusterSettings(a), clusterSettings(b)
	return x != nil && y != nil && x["context"] != "" && x["context"] == y["context"] && x["kubeconfig"] == y["kubeconfig"]
}

// Legacy records remain addressable in Settings, Activity, and saved scopes.
// The tree groups only equivalent records with the same display name.
func (m *model) sidebarConnections(w workspace.Workspace) []workspace.Connection {
	connections := []workspace.Connection{}
	identities := []map[string]string{}
	for _, c := range w.Connections {
		identity, duplicate := clusterSettings(c), -1
		if identity != nil && identity["context"] != "" {
			for n, previous := range connections {
				if previous.Name == c.Name && maps.Equal(identities[n], identity) {
					duplicate = n
					break
				}
			}
		}
		if duplicate >= 0 {
			if w.ID == m.active && c.ID == m.connection {
				connections[duplicate] = c
			}
			continue
		}
		connections, identities = append(connections, c), append(identities, identity)
	}
	return connections
}
