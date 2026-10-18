// Package common owns Helm chart data and operations that do not access a cluster.
package common

import (
	"context"
	"fmt"
	"strings"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
)

const EngineVersion = "v3.22.0 (Go SDK)"

type TemplateRequest struct {
	ChartPath, Release, Namespace, KubeVersion, NameTemplate string
	ValuesFiles, Overrides, APIVersions                      []string
	ValuesInput                                              string
	IsUpgrade, IncludeCRDs, NoHooks, SkipTests               bool
}

func Render(ctx context.Context, request TemplateRequest) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	values, e := ReadValues(request.ValuesFiles, request.Overrides, request.ValuesInput)
	if e != nil {
		return nil, e
	}
	chartPath, name, ns := request.ChartPath, request.Release, request.Namespace
	if ns == "" {
		ns = "default"
	}
	chart, e := loader.Load(chartPath)
	if e != nil {
		return nil, e
	}
	if e = action.CheckDependencies(chart, chart.Metadata.Dependencies); e != nil {
		return nil, e
	}
	cfg := &action.Configuration{Log: func(string, ...interface{}) {}}
	install := action.NewInstall(cfg)
	install.ClientOnly, install.DryRun, install.Replace = true, true, true
	install.DryRunOption = "client"
	install.Namespace, install.ReleaseName = ns, name
	install.IsUpgrade = request.IsUpgrade
	install.IncludeCRDs, install.DisableHooks = request.IncludeCRDs, request.NoHooks
	install.APIVersions = chartutil.VersionSet(request.APIVersions)
	if request.NameTemplate != "" {
		install.NameTemplate = request.NameTemplate
		name, _, e = install.NameAndChart([]string{chartPath})
		if e != nil {
			return nil, e
		}
		install.ReleaseName = name
	}
	if version := request.KubeVersion; version != "" {
		install.KubeVersion, e = chartutil.ParseKubeVersion(version)
		if e != nil {
			return nil, e
		}
	}
	release, e := install.RunWithContext(ctx, chart, values)
	if e != nil {
		return nil, e
	}
	text := strings.TrimSpace(release.Manifest) + "\n"
	if !install.DisableHooks {
		for _, hook := range release.Hooks {
			test := false
			for _, event := range hook.Events {
				if string(event) == "test" {
					test = true
				}
			}
			if request.SkipTests && test {
				continue
			}
			text += "---\n# Source: " + hook.Path + "\n" + hook.Manifest + "\n"
		}
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(text) > MaxOutput {
		return nil, ErrOutputLimit
	}
	return []byte(text), nil
}
func Lint(ctx context.Context, chartPath string, files, overrides []string, input string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	values, err := ReadValues(files, overrides, input)
	if err != nil {
		return nil, err
	}
	findings := action.NewLint().Run([]string{chartPath}, values)
	var lines []string
	for _, finding := range findings.Messages {
		lines = append(lines, finding.Error())
	}
	text := strings.Join(lines, "\n")
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(findings.Errors) > 0 {
		return nil, fmt.Errorf("chart validation failed: %s", text)
	}
	if len(text) > MaxOutput {
		return nil, ErrOutputLimit
	}
	return []byte(text), nil
}
func Package(ctx context.Context, chartPath, destination string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p := action.NewPackage()
	p.Destination = destination
	_, err := p.Run(chartPath, nil)
	if err != nil {
		return err
	}
	return ctx.Err()
}
