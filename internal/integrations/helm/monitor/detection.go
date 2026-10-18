package monitor

import (
	"context"
	"errors"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// Detect finds retained Helm release metadata. No Helm binary or release payload is read.
func Detect(ctx context.Context, client *kubernetes.Client, driver string) (string, error) {
	drivers := []string{driver}
	if driver == "" {
		drivers = []string{"secret", "configmap"}
	} else if driver != "secret" && driver != "configmap" {
		return "", errors.New("Helm storage must be secret or configmap")
	}
	var failures []error
	for _, candidate := range drivers {
		resource := kubecommon.Resource{Version: "v1", Name: "secrets", Kind: "Secret", Namespaced: true}
		if candidate == "configmap" {
			resource.Name, resource.Kind = "configmaps", "ConfigMap"
		}
		item, err := client.FirstMetadata(ctx, resource, "", "owner=helm")
		if apierrors.IsForbidden(err) && client.Namespace != "" && client.Namespace != "*" {
			// Namespace-limited credentials can still identify their own releases.
			failures = append(failures, err)
			item, err = client.FirstMetadata(ctx, resource, client.Namespace, "owner=helm")
		}
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if item != nil && item.Labels["owner"] == "helm" {
			return candidate, nil
		}
	}
	return "", errors.Join(failures...)
}
