// Package code inspects and renders Helm source without a cluster connection.
package code

import (
	"context"
	"errors"
	"strings"

	"github.com/simjay/plous/internal/integrations/helm/common"
)

type Renderer interface {
	Render(context.Context, common.TemplateRequest) ([]byte, error)
	Acquire(context.Context, string, string, string) (*common.Artifact, error)
}
type Engine struct{}

func (Engine) Render(ctx context.Context, request common.TemplateRequest) ([]byte, error) {
	if request.Release == "" {
		request.Release = "preview"
	}
	if strings.HasPrefix(request.Release, "-") || strings.ContainsAny(request.Release, "\r\n\x00") {
		return nil, errors.New("invalid release name")
	}
	return common.Render(ctx, request)
}
func (Engine) Acquire(ctx context.Context, reference, repository, version string) (*common.Artifact, error) {
	return common.Acquire(ctx, common.RepositoryConfig{}, reference, repository, version)
}
