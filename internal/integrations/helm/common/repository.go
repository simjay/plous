package common

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/repo"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
	"sigs.k8s.io/yaml"
)

type RepositoryConfig struct{ RepositoryConfig, RepositoryCache, RegistryConfig string }

func pullChart(ctx context.Context, config RepositoryConfig, reference, repository, version, destination string) error {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	var data []byte
	var err error
	if strings.HasPrefix(reference, "oci://") {
		data, err = pullOCI(ctx, config, reference, version)
	} else {
		data, err = pullHTTPChart(ctx, config, reference, repository, version)
	}
	if err != nil {
		return err
	}
	if len(data) > MaxOutput {
		return ErrOutputLimit
	}
	return os.WriteFile(filepath.Join(destination, "chart.tgz"), data, 0600)
}
func readBoundedFile(name string, limit int64) ([]byte, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.New("input must be a bounded regular file")
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if int64(len(data)) > limit {
		return nil, ErrOutputLimit
	}
	return data, err
}

func pullHTTPChart(ctx context.Context, c RepositoryConfig, reference, repository, version string) ([]byte, error) {
	entry := &repo.Entry{URL: repository}
	var index *repo.IndexFile
	if repository == "" && !strings.Contains(reference, "://") {
		parts := strings.SplitN(reference, "/", 2)
		if len(parts) != 2 || parts[0] == "." || parts[0] == ".." {
			return nil, errors.New("use a configured repository/chart reference")
		}
		configPath, err := ExpandPaths(c.RepositoryConfig)
		if err != nil {
			return nil, err
		}
		config, err := LoadRepositoryConfig(configPath)
		if err != nil {
			return nil, err
		}
		entry = config.Get(parts[0])
		if entry == nil {
			return nil, errors.New("chart repository is not configured")
		}
		reference = parts[1]
		cache, err := ExpandPaths(c.RepositoryCache)
		if err != nil {
			return nil, err
		}
		index, err = LoadBoundedIndex(filepath.Join(cache, parts[0]+"-index.yaml"))
		if err != nil {
			return nil, err
		}
	}
	if entry.URL != "" {
		if index == nil {
			indexURL, err := repo.ResolveReferenceURL(entry.URL, "index.yaml")
			if err != nil {
				return nil, err
			}
			data, err := chartHTTPGet(ctx, indexURL, entry)
			if err != nil {
				return nil, err
			}
			dir, err := os.MkdirTemp("", "plous-chart-index-")
			if err != nil {
				return nil, err
			}
			defer os.RemoveAll(dir)
			name := filepath.Join(dir, "index.yaml")
			if err := os.WriteFile(name, data, 0600); err != nil {
				return nil, err
			}
			index, err = repo.LoadIndexFile(name)
			if err != nil {
				return nil, err
			}
		}
		chart, err := index.Get(reference, version)
		if err != nil {
			return nil, err
		}
		if len(chart.URLs) == 0 {
			return nil, errors.New("chart has no download URL")
		}
		reference, err = repo.ResolveReferenceURL(entry.URL, chart.URLs[0])
		if err != nil {
			return nil, err
		}
	}
	return chartHTTPGet(ctx, reference, entry)
}

func LoadRepositoryConfig(name string) (*repo.File, error) {
	data, err := readBoundedFile(name, 2<<20)
	if err != nil {
		return nil, err
	}
	config := &repo.File{}
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, errors.New("invalid chart repository configuration")
	}
	for _, entry := range config.Repositories {
		if entry == nil {
			return nil, errors.New("invalid chart repository entry")
		}
	}
	return config, nil
}

func LoadBoundedIndex(name string) (*repo.IndexFile, error) {
	// Helm normalizes and validates index entries when loading the copied file.
	data, err := readBoundedFile(name, MaxOutput)
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "plous-chart-index-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	copy := filepath.Join(dir, "index.yaml")
	if err := os.WriteFile(copy, data, 0600); err != nil {
		return nil, err
	}
	return repo.LoadIndexFile(copy)
}

func chartHTTPGet(ctx context.Context, address string, entry *repo.Entry) ([]byte, error) {
	u, err := url.Parse(address)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errors.New("chart download requires an HTTP(S) URL without embedded credentials")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: entry.InsecureSkipTLSverify}
	if entry.CAFile != "" {
		data, err := readBoundedFile(entry.CAFile, 2<<20)
		if err != nil {
			return nil, err
		}
		roots, err := x509.SystemCertPool()
		if err != nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM(data) {
			return nil, errors.New("invalid chart repository CA")
		}
		tlsConfig.RootCAs = roots
	}
	if entry.CertFile != "" || entry.KeyFile != "" {
		certificate, err := tls.LoadX509KeyPair(entry.CertFile, entry.KeyFile)
		if err != nil {
			return nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{certificate}
	}
	transport.TLSClientConfig = tlsConfig
	origin, _ := url.Parse(entry.URL)
	authenticate := func(request *http.Request) {
		request.Header.Del("Authorization")
		if entry.PassCredentialsAll || origin != nil && request.URL.Scheme == origin.Scheme && request.URL.Host == origin.Host {
			if entry.Username != "" {
				request.SetBasicAuth(entry.Username, entry.Password)
			}
		}
	}
	client := &http.Client{Transport: transport, Timeout: 90 * time.Second, CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("too many chart download redirects")
		}
		if request.URL.User != nil || (request.URL.Scheme != "https" && request.URL.Scheme != "http") {
			return errors.New("unsupported chart redirect")
		}
		authenticate(request)
		return nil
	}}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	authenticate(request)
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("chart repository returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, MaxOutput+1))
	if len(data) > MaxOutput {
		return nil, ErrOutputLimit
	}
	return data, err
}

func pullOCI(ctx context.Context, c RepositoryConfig, reference, version string) ([]byte, error) {
	store := credentials.NewMemoryStore()
	if c.RegistryConfig != "" {
		name, err := ExpandPaths(c.RegistryConfig)
		if err != nil {
			return nil, err
		}
		data, err := readBoundedFile(name, 2<<20)
		if err != nil {
			return nil, err
		}
		store, err = credentials.NewMemoryStoreFromDockerConfig(data)
		if err != nil {
			return nil, errors.New("invalid registry credentials file")
		}
	}
	httpClient := &http.Client{Timeout: 90 * time.Second, Transport: boundedRegistryTransport{OperationTransport{Context: ctx, Base: http.DefaultTransport}}}
	// An explicit authorizer uses only captured inline credentials. Helm's default
	// authorizer consults Docker and may execute credential-helper programs.
	authorizer := auth.Client{Client: httpClient, Credential: credentials.Credential(store), Cache: auth.NewCache()}
	dir, err := os.MkdirTemp("", "plous-registry-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	client, err := registry.NewClient(registry.ClientOptCredentialsFile(filepath.Join(dir, "config.json")), registry.ClientOptAuthorizer(authorizer), registry.ClientOptHTTPClient(httpClient))
	if err != nil {
		return nil, err
	}
	uri, err := url.Parse(reference)
	if err != nil {
		return nil, err
	}
	resolved, err := client.ValidateReference(reference, version, uri)
	if err != nil {
		return nil, err
	}
	result, err := client.Pull(strings.TrimPrefix(resolved.String(), "oci://"))
	if err != nil {
		return nil, err
	}
	return result.Chart.Data, nil
}

type boundedRegistryTransport struct{ base http.RoundTripper }

func (t boundedRegistryTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	response.Body = &boundedRegistryBody{ReadCloser: response.Body, remaining: MaxOutput}
	return response, nil
}

type boundedRegistryBody struct {
	io.ReadCloser
	remaining int
}

func (b *boundedRegistryBody) Read(p []byte) (int, error) {
	if b.remaining < 0 {
		return 0, ErrOutputLimit
	}
	n, err := b.ReadCloser.Read(p[:min(len(p), b.remaining+1)])
	b.remaining -= n
	if b.remaining < 0 {
		return n, ErrOutputLimit
	}
	return n, err
}
