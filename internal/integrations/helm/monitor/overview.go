package monitor

import (
	"context"
	"sort"
	"strconv"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type OverviewRelease struct {
	Name, Namespace, Status string
	Revision                int
	Updated                 time.Time
}

type Overview struct {
	Releases []OverviewRelease
	ReadAt   time.Time
	Partial  bool
	Error    string
}

type metadataReader interface {
	ListMetadata(context.Context, kubecommon.Resource, string, string) ([]metav1.PartialObjectMetadata, bool, error)
}

// ReadOverview uses storage labels only; opening a release reads its full record.
func ReadOverview(ctx context.Context, reader metadataReader, driver string) Overview {
	if driver != "secret" && driver != "configmap" {
		return Overview{ReadAt: time.Now(), Error: "Helm storage must be secret or configmap"}
	}
	r := kubecommon.Resource{Version: "v1", Name: "secrets", Kind: "Secret", Namespaced: true}
	if driver == "configmap" {
		r.Name, r.Kind = "configmaps", "ConfigMap"
	}
	items, partial, err := reader.ListMetadata(ctx, r, "", "owner=helm")
	result := Overview{ReadAt: time.Now(), Partial: partial}
	if err != nil {
		result.Error = kubernetes.ErrorText(err)
	}
	latest := map[string]OverviewRelease{}
	for _, item := range items {
		labels := item.Labels
		if labels["owner"] != "helm" {
			continue
		}
		revision, err := strconv.Atoi(labels["version"])
		if err != nil || revision < 1 || validRelease(labels["name"]) != nil || item.Namespace == "" || validNamespace(item.Namespace) != nil {
			result.Partial = true
			continue
		}
		release := OverviewRelease{Name: labels["name"], Namespace: item.Namespace, Revision: revision, Status: labels["status"]}
		if release.Status == "" {
			release.Status = "Unknown"
			result.Partial = true
		}
		// Helm's modification label is a storage observation, not a deployment time.
		stamp := labels["modifiedAt"]
		if stamp == "" {
			stamp = labels["createdAt"]
		}
		if seconds, err := strconv.ParseInt(stamp, 10, 64); err == nil && seconds > 0 {
			release.Updated = time.Unix(seconds, 0)
		}
		key := release.Namespace + "/" + release.Name
		if old, ok := latest[key]; !ok || revision > old.Revision {
			latest[key] = release
		}
	}
	for _, release := range latest {
		result.Releases = append(result.Releases, release)
	}
	sort.Slice(result.Releases, func(i, j int) bool {
		a, b := result.Releases[i], result.Releases[j]
		return a.Namespace+"/"+a.Name < b.Namespace+"/"+b.Name
	})
	return result
}
