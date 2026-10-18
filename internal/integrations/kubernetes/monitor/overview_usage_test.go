package monitor

import (
	"context"
	"errors"
	"testing"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
)

type concurrentOverviewUsageReader struct {
	nodesStarted, metricsStarted chan struct{}
}

func (r concurrentOverviewUsageReader) List(ctx context.Context, resource kubecommon.Resource, _, _ string) (Collection, error) {
	if resource.Key() != "nodes" {
		return Collection{}, errors.New("usage fetched unrelated inventory")
	}
	close(r.nodesStarted)
	select {
	case <-r.metricsStarted:
		return Collection{}, nil
	case <-ctx.Done():
		return Collection{}, ctx.Err()
	}
}

func (r concurrentOverviewUsageReader) UsageList(ctx context.Context, _ kubecommon.Resource, _ string) (map[string]Usage, error) {
	close(r.metricsStarted)
	select {
	case <-r.nodesStarted:
		return nil, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestOverviewUsageReadsOnlyNodesAndMetricsConcurrently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r := concurrentOverviewUsageReader{make(chan struct{}), make(chan struct{})}
	result := readOverviewUsage(ctx, r, overviewCatalog())
	if ctx.Err() != nil || result.Error != "Metrics incomplete: 0/0 observed Nodes" || result.CPU != nil {
		t.Fatal("usage serialized requests or treated an empty Node set as measured zero", result)
	}
}
