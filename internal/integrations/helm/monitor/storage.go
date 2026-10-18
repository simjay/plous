package monitor

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"io"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	// Read the exact retained record when an adapter omits its chart body, using
	// the connection's selected storage driver.
	helmcommon "github.com/simjay/plous/internal/integrations/helm/common"
)

func (c *Client) retainedSnapshot(ctx context.Context, name, namespace string, revision int) (*Snapshot, error) {
	if err := validRelease(name); err != nil {
		return nil, err
	}
	if err := validNamespace(namespace); err != nil {
		return nil, err
	}
	if revision < 1 {
		return nil, errors.New("retained revision is required")
	}
	resource := kubecommon.Resource{Version: "v1", Name: "secrets", Kind: "Secret", Namespaced: true}
	if c.Config.Driver == "configmap" {
		resource.Name, resource.Kind = "configmaps", "ConfigMap"
	}
	client, err := kubernetes.NewTarget(c.Config.Target, namespace)
	if err != nil {
		return nil, err
	}
	object, err := client.Get(ctx, resource, namespace, fmt.Sprintf("sh.helm.release.v1.%s.v%d", name, revision))
	if err != nil {
		return nil, errors.New("cannot read the retained Helm record: " + client.ExecErrorText(err))
	}
	encoded, found, err := unstructured.NestedString(object.Object, "data", "release")
	if err != nil || !found || len(encoded) > 2*helmcommon.MaxOutput {
		return nil, errors.New("invalid retained Helm record")
	}
	if resource.Name == "secrets" {
		outer, e := base64.StdEncoding.DecodeString(encoded)
		if e != nil {
			return nil, errors.New("invalid Helm Secret encoding")
		}
		encoded = string(outer)
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("invalid Helm release encoding")
	}
	if len(data) > 3 && bytes.Equal(data[:3], []byte{0x1f, 0x8b, 0x08}) {
		reader, e := gzip.NewReader(bytes.NewReader(data))
		if e != nil {
			return nil, errors.New("invalid Helm release compression")
		}
		defer reader.Close()
		data, err = io.ReadAll(io.LimitReader(reader, helmcommon.MaxOutput+1))
		if err != nil {
			return nil, errors.New("cannot decode retained release")
		}
	}
	if len(data) > helmcommon.MaxOutput {
		return nil, errors.New("retained release exceeds 16 MiB")
	}
	var snapshot Snapshot
	if err = json.Unmarshal(data, &snapshot); err != nil {
		return nil, errors.New("invalid retained release JSON")
	}
	if snapshot.Name != name || snapshot.Namespace != namespace || snapshot.Version != revision {
		return nil, errors.New("retained release identity differs from request")
	}
	return &snapshot, nil
}
