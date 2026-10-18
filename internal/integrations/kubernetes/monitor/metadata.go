package monitor

import (
	"context"
	"encoding/json"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FirstMetadata requests one matching object's metadata, without its contents.
func (c *Client) FirstMetadata(ctx context.Context, r kubecommon.Resource, namespace, selector string) (*metav1.PartialObjectMetadata, error) {
	data, err := c.rest.Get().AbsPath(resourcePath(r, namespace)).
		SetHeader("Accept", "application/json;as=PartialObjectMetadataList;g=meta.k8s.io;v=v1").
		Param("labelSelector", selector).Param("limit", "1").Timeout(10 * time.Second).Do(ctx).Raw()
	if err != nil {
		return nil, err
	}
	var list metav1.PartialObjectMetadataList
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	if len(list.Items) == 0 {
		return nil, nil
	}
	return &list.Items[0], nil
}

// ListMetadata reads a bounded inventory without retaining Secret payloads.
func (c *Client) ListMetadata(ctx context.Context, r kubecommon.Resource, namespace, selector string) ([]metav1.PartialObjectMetadata, bool, error) {
	if err := c.checkTarget(); err != nil {
		return nil, false, err
	}
	var items []metav1.PartialObjectMetadata
	continuation := ""
	for {
		data, err := c.rest.Get().AbsPath(resourcePath(r, namespace)).
			SetHeader("Accept", "application/json;as=PartialObjectMetadataList;g=meta.k8s.io;v=v1").
			Param("labelSelector", selector).Param("limit", "500").Param("continue", continuation).Timeout(10 * time.Second).Do(ctx).Raw()
		if err != nil {
			return items, len(items) > 0, err
		}
		var list metav1.PartialObjectMetadataList
		if err := json.Unmarshal(data, &list); err != nil {
			return items, len(items) > 0, err
		}
		remaining := MaxObjects - len(items)
		items = append(items, list.Items[:min(remaining, len(list.Items))]...)
		if len(list.Items) > remaining || len(items) == MaxObjects && list.Continue != "" {
			return items, true, nil
		}
		continuation = list.Continue
		if continuation == "" {
			return items, false, nil
		}
	}
}
