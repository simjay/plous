package common

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func Acquire(ctx context.Context, config RepositoryConfig, reference, repository, version string) (*Artifact, error) {
	if reference == "" || strings.HasPrefix(reference, "-") || strings.ContainsAny(reference, "\x00\r\n") {
		return nil, errors.New("an explicit chart reference is required")
	}
	if version != "" && strings.HasPrefix(version, "-") {
		return nil, errors.New("invalid chart version")
	}
	remoteReference := strings.Contains(reference, "://")
	if remoteReference {
		u, err := url.Parse(reference)
		if err != nil || u.Host == "" || u.User != nil || u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "oci" {
			return nil, errors.New("chart reference must be an HTTP(S) or OCI URL without embedded credentials")
		}
	}
	if repository != "" {
		u, e := url.Parse(repository)
		if e != nil || u.Host == "" || u.User != nil || u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "oci" {
			return nil, errors.New("repository must be an HTTP(S) or OCI URL without embedded credentials")
		}
	}
	dir, err := os.MkdirTemp("", "plous-chart-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	local, _ := ExpandPaths(reference)
	info, statErr := os.Stat(local)
	localSource := repository == "" && !remoteReference
	if localSource && statErr != nil && (filepath.IsAbs(local) || strings.HasPrefix(reference, ".") || strings.HasPrefix(reference, "~/")) {
		return nil, errors.New("cannot read the selected local chart")
	}
	if localSource && statErr == nil {
		if info.IsDir() {
			if err = CheckChartDirectory(local); err != nil {
				return nil, err
			}
			if err = Package(ctx, local, dir); err != nil {
				return nil, err
			}
		} else {
			if err = CopyRegular(local, filepath.Join(dir, "chart.tgz"), MaxOutput); err != nil {
				return nil, err
			}
		}
	} else {
		pullReference := reference
		ociRepository := strings.HasPrefix(repository, "oci://")
		if ociRepository {
			pullReference = strings.TrimSuffix(repository, "/") + "/" + reference
		}
		if version == "" {
			return nil, errors.New("select an exact version for a remote chart")
		}
		pullRepository := repository
		if ociRepository {
			pullRepository = ""
		}
		if err = pullChart(ctx, config, pullReference, pullRepository, version, dir); err != nil {
			return nil, err
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var archive []byte
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".tgz") {
			if archive != nil {
				return nil, errors.New("chart acquisition produced multiple archives")
			}
			info, e := entry.Info()
			if e != nil || info.Size() > MaxOutput {
				return nil, errors.New("chart archive exceeds limit")
			}
			archive, err = os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				return nil, err
			}
		}
	}
	if archive == nil {
		return nil, errors.New("chart archive was not produced")
	}
	artifact, err := ReadArtifact(archive)
	if err != nil {
		return nil, err
	}
	artifact.Reference, artifact.Repository = reference, repository
	if version != "" && fmt.Sprint(artifact.Metadata["version"]) != strings.TrimPrefix(version, "v") && fmt.Sprint(artifact.Metadata["version"]) != version {
		return nil, errors.New("downloaded chart does not match the selected version")
	}
	return artifact, nil
}
