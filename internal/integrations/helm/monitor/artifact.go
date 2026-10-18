package monitor

import (
	"context"

	helmcommon "github.com/simjay/plous/internal/integrations/helm/common"
)

func (c *Client) Acquire(ctx context.Context, reference, repository, version string) (*helmcommon.Artifact, error) {
	return helmcommon.Acquire(ctx, helmcommon.RepositoryConfig{RepositoryConfig: c.Config.RepositoryConfig, RepositoryCache: c.Config.RepositoryCache, RegistryConfig: c.Config.RegistryConfig}, reference, repository, version)
}
