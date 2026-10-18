package monitor

import (
	"context"
	"fmt"
	"sync"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
)

// OverviewUsage binds consumption and capacity to the same readable Node set.
type OverviewUsage struct {
	ReadAt, SourceStart, SourceEnd    time.Time
	CPU, Memory                       *float64
	CPUAllocatable, MemoryAllocatable *float64
	Observed, Expected                int
	Error                             string
}

func (c *Client) OverviewUsage(ctx context.Context, catalog []kubecommon.Resource) OverviewUsage {
	return readOverviewUsage(ctx, c, catalog)
}

func readOverviewUsage(ctx context.Context, reader overviewReader, catalog []kubecommon.Resource) OverviewUsage {
	nodes, nodesOK := Find(catalog, "nodes")
	metrics, metricsOK := UsageResource(kubecommon.Resource{Name: "nodes"}, catalog)
	if !nodesOK {
		return OverviewUsage{ReadAt: time.Now(), Error: "Metrics Node inventory unavailable"}
	}
	var collection Collection
	var samples map[string]Usage
	var nodesErr, metricsErr error
	var wg sync.WaitGroup
	wg.Go(func() { collection, nodesErr = reader.List(ctx, nodes, "", "") })
	if metricsOK {
		wg.Go(func() { samples, metricsErr = reader.UsageList(ctx, metrics, "") })
	}
	wg.Wait()
	result := OverviewUsage{ReadAt: time.Now(), Expected: len(collection.Objects)}
	if nodesErr != nil {
		result.Error = "Metrics Node inventory: " + ErrorText(nodesErr)
		return result
	}
	if !collection.Truncated {
		result.CPUAllocatable = overviewQuantitySum(collection.Objects, "status", "allocatable", "cpu")
		result.MemoryAllocatable = overviewQuantitySum(collection.Objects, "status", "allocatable", "memory")
	}
	if !metricsOK {
		result.Error = "Metrics API unavailable"
		return result
	}
	if metricsErr != nil {
		result.Error = UsageErrorText(metricsErr)
		return result
	}
	var cpu, memory float64
	for _, node := range collection.Objects {
		sample, exists := samples[UsageKey(node)]
		if !exists || !sample.Matches(node, result.ReadAt) {
			continue
		}
		result.Observed++
		cpu += sample.CPU
		memory += sample.Memory
		if result.SourceStart.IsZero() || sample.Timestamp.Before(result.SourceStart) {
			result.SourceStart = sample.Timestamp
		}
		if sample.Timestamp.After(result.SourceEnd) {
			result.SourceEnd = sample.Timestamp
		}
	}
	if collection.Truncated || result.Expected == 0 || result.Observed != result.Expected {
		result.Error = fmt.Sprintf("Metrics incomplete: %d/%d observed Nodes", result.Observed, result.Expected)
	} else {
		result.CPU, result.Memory = &cpu, &memory
	}
	return result
}

func (u OverviewUsage) Apply(o *Overview) {
	o.CPU.Used, o.Memory.Used = u.CPU, u.Memory
	o.CPU.Allocatable, o.Memory.Allocatable = u.CPUAllocatable, u.MemoryAllocatable
	o.MetricsAt, o.MetricsEnd, o.MetricsReadAt = u.SourceStart, u.SourceEnd, u.ReadAt
	o.MetricsObserved, o.MetricsExpected, o.MetricsError = u.Observed, u.Expected, u.Error
}
