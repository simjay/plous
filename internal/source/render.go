package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	helm "github.com/simjay/plous/internal/integrations/helm/code"
	helmcommon "github.com/simjay/plous/internal/integrations/helm/common"
	kubecode "github.com/simjay/plous/internal/integrations/kubernetes/code"
	kustomize "github.com/simjay/plous/internal/integrations/kustomize/code"
	kustomizecommon "github.com/simjay/plous/internal/integrations/kustomize/common"
	manifest "github.com/simjay/plous/internal/integrations/manifest"
	"sigs.k8s.io/yaml"
)

type Inputs struct {
	Release, Namespace, KubeVersion string
	Values, Overrides, APIVersions  []string
}
type BuildResult struct {
	Path, SourceIdentity, Revision, Engine string
	Built                                  time.Time
	Resources                              []manifest.Resource
	Observation                            *kustomizecommon.Observation
}

type Result struct {
	*BuildResult
	ID, InputIdentity, OperationID string
	Inputs                         Inputs
	Dependencies                   map[string]string
	Origins                        map[string]string
	Lineages                       map[string]string
	Findings                       []Finding
	captured                       *Inventory
	ContentIdentity                string
}

// CapturedSource exposes the immutable inputs associated with this render.
func (r *Result) CapturedSource() *Inventory { return r.captured }

type Renderer struct {
	Helm      helm.Renderer
	Kustomize kustomize.Builder
}

func (r Renderer) helm() helm.Renderer {
	if r.Helm != nil {
		return r.Helm
	}
	return helm.Engine{}
}
func inputIdentity(in Inputs) string {
	b, _ := json.Marshal(in)
	return fmt.Sprintf("%x", sha256.Sum256(b))
}
func (in Inputs) Identity() string { return inputIdentity(in) }
func materialize(s *Snapshot) (string, func(), error) {
	dir, err := os.MkdirTemp("", "plous-code-")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { os.RemoveAll(dir) }
	for _, name := range s.Paths() {
		if !fs.ValidPath(name) {
			cleanup()
			return "", func() {}, errors.New("invalid source path")
		}
		file := filepath.Join(dir, filepath.FromSlash(name))
		if err = os.MkdirAll(filepath.Dir(file), 0700); err == nil {
			err = os.WriteFile(file, s.Files[name], 0600)
		}
		if err != nil {
			cleanup()
			return "", func() {}, err
		}
	}
	return dir, cleanup, nil
}
func (r Renderer) Render(ctx context.Context, i *Inventory, id string, in Inputs) (*Result, error) {
	if i == nil {
		return nil, errors.New("load a source first")
	}
	e := i.Entry(id)
	if e == nil {
		return nil, errors.New("entry point is unavailable")
	}
	if e.Component {
		return nil, errors.New("render a consuming kustomization for this component")
	}
	sourceIdentity := i.Source.Identity
	captured, err := captureInputs(ctx, i.Source, *e, in)
	if err != nil {
		return nil, err
	}
	i = captured
	e = i.Entry(id)
	if e == nil {
		return nil, errors.New("selected configuration changed; refresh discovery")
	}
	b := &BuildResult{Path: e.Path, SourceIdentity: sourceIdentity, Revision: i.Source.Revision, Built: time.Now().UTC()}
	out := &Result{BuildResult: b, ID: id, InputIdentity: inputIdentity(in), Inputs: in, Dependencies: map[string]string{}, Origins: map[string]string{}}
	var data []byte
	out.captured, out.ContentIdentity = i, i.Source.Identity
	switch e.Kind {
	case "plain":
		data = i.Source.Files[e.File]
		b.Engine = "YAML parser"
	case "helm":
		for _, gap := range i.Source.Gaps {
			if e.Path == "." || strings.HasPrefix(gap, e.Path+"/") {
				return nil, errors.New("chart contains omitted inputs: " + gap)
			}
		}
		dir, cleanup, err := materialize(i.Source)
		if err != nil {
			return nil, err
		}
		defer cleanup()
		release := in.Release
		if release == "" {
			release = "preview"
		}
		ns := in.Namespace
		if ns == "" {
			ns = "default"
		}
		if strings.HasPrefix(release, "-") || strings.ContainsAny(release, "\r\n\x00") {
			return nil, errors.New("invalid release name")
		}
		request := helmcommon.TemplateRequest{Release: release, Namespace: ns, ChartPath: filepath.Join(dir, filepath.FromSlash(e.Path)), IncludeCRDs: true, Overrides: in.Overrides, KubeVersion: in.KubeVersion, APIVersions: in.APIVersions}
		for _, file := range in.Values {
			if !fs.ValidPath(file) || i.Source.Files[file] == nil {
				return nil, fmt.Errorf("values file unavailable: %s", file)
			}
			request.ValuesFiles = append(request.ValuesFiles, filepath.Join(dir, filepath.FromSlash(file)))
		}
		data, err = r.helm().Render(ctx, request)
		if err != nil {
			return nil, err
		}
		b.Engine = "helm " + helmcommon.EngineVersion
	case "kustomize", "kustomize+helm":
		prepared := *i.Source
		prepared.Files = maps.Clone(i.Source.Files)
		if err = r.prepareCharts(ctx, i, e.Path, &prepared, out.Dependencies, map[string]bool{}); err != nil {
			return nil, err
		}
		ks := kustomize.Discover(prepared.Files)
		built, buildErr := kustomize.BuildHelm(ctx, ks, e.Path, r.Kustomize)
		if buildErr != nil {
			return nil, buildErr
		}
		b.Resources, b.Engine = built.Resources, built.Engine
		b.Observation = built.Observation
		if len(out.Dependencies) > 0 || e.Kind == "kustomize+helm" {
			b.Engine += " + helm " + helmcommon.EngineVersion
		}
		// Optional instrumentation is accepted only when it reproduces every
		// authoritative resource byte after removing newly added annotations.
		root := ks.Entry(e.Path)
		config := maps.Clone(root.Configuration)
		meta, _ := config["buildMetadata"].([]any)
		if !slices.Contains(meta, any("originAnnotations")) {
			config["buildMetadata"] = append(slices.Clone(meta), "originAnnotations")
			prepared.Files[root.File], err = yaml.Marshal(config)
			if err == nil {
				observed, traceErr := kustomize.BuildHelm(ctx, kustomize.Discover(prepared.Files), e.Path, r.Kustomize)
				if traceErr == nil {
					out.Origins = verifiedOrigins(i, e.Path, b.Resources, observed.Resources)
				}
			}
		}

	default:
		return nil, errors.New("rendering is unavailable for this entry type")
	}
	if data != nil {
		b.Resources, err = manifest.Resources(string(data))
		if err != nil {
			return nil, err
		}
	}
	if err := kubecode.Validate(b.Resources); err != nil {
		return nil, err
	}
	for _, resource := range b.Resources {
		if e.Kind == "plain" {
			out.Origins[resource.Key()] = e.File
		}
		if e.Kind == "helm" && resource.Source != "" {
			if file := capturedHelmOrigin(i, e, resource.Source); file != "" {
				out.Origins[resource.Key()] = file
			}
		}
		inspectImages(manifest.Redact(resource.Object), "", func(field, value string) {
			if !imageHasDigest(value) {
				out.Findings = append(out.Findings, Finding{Severity: "warning", Rule: "mutable-image", Target: id, File: e.File, Resource: resource.Key(), Field: field, Summary: "Image has no content digest · " + resource.Kind + "/" + resource.Name + " · " + path.Base(value), Message: resource.Kind + "/" + resource.Name + " " + field + ": " + value + " has no supported content digest"})
			}
		})
	}
	out.identifyLineages(i)
	out.inspectOverwrites(i)
	identity, _ := json.Marshal(struct {
		Source, Inputs, Engine, Content string
		Dependencies                    map[string]string
		Resources                       []manifest.Resource
	}{b.SourceIdentity, out.InputIdentity, b.Engine, out.ContentIdentity, out.Dependencies, b.Resources})
	out.OperationID = fmt.Sprintf("%x", sha256.Sum256(identity))
	return out, nil
}

func capturedHelmOrigin(i *Inventory, e *Entry, origin string) string {
	var chart struct {
		Name string `json:"name"`
	}
	if yaml.Unmarshal(i.Source.Files[e.File], &chart) != nil || chart.Name == "" {
		return ""
	}
	if relative, ok := strings.CutPrefix(origin, chart.Name+"/"); ok && fs.ValidPath(relative) {
		file := path.Join(e.Path, relative)
		if _, captured := i.Source.Files[file]; captured {
			return file
		}
	}
	return ""
}

func imageHasDigest(value string) bool {
	name, digest, ok := strings.Cut(value, "@")
	if !ok || name == "" {
		return false
	}
	algorithm, encoded, _ := strings.Cut(digest, ":")
	size := map[string]int{"sha256": 32, "sha384": 48, "sha512": 64}[algorithm]
	decoded, err := hex.DecodeString(encoded)
	return size > 0 && err == nil && len(decoded) == size
}

func inspectImages(value any, field string, found func(string, string)) {
	switch v := value.(type) {
	case map[string]any:
		for k, child := range v {
			p := field + "/" + k
			if k == "containers" || k == "initContainers" || k == "ephemeralContainers" {
				containers, _ := child.([]any)
				for n, container := range containers {
					object, _ := container.(map[string]any)
					if image, ok := object["image"].(string); ok {
						found(fmt.Sprintf("%s/%d/image", p, n), image)
					}
				}
			} else {
				inspectImages(child, p, found)
			}
		}
	case []any:
		for n, child := range v {
			inspectImages(child, fmt.Sprintf("%s/%d", field, n), found)
		}
	}
}
func (r Renderer) prepareCharts(ctx context.Context, i *Inventory, name string, s *Snapshot, digests map[string]string, seen map[string]bool) error {
	if seen[name] {
		return nil
	}
	seen[name] = true
	e := i.Kustomize.Entry(name)
	if e == nil {
		return fmt.Errorf("missing kustomization %s", name)
	}
	for _, ref := range e.References {
		if ref.Entry {
			if err := r.prepareCharts(ctx, i, ref.Path, s, digests, seen); err != nil {
				return err
			}
		}
	}
	globals, _ := e.Configuration["helmGlobals"].(map[string]any)
	if home, _ := globals["configHome"].(string); home != "" {
		return errors.New("helmGlobals.configHome is unsupported; rendering uses private Helm configuration")
	}
	home, _ := globals["chartHome"].(string)
	if home == "" {
		home = "charts"
	}
	if !fs.ValidPath(home) {
		return errors.New("chartHome must stay inside the kustomization directory")
	}
	charts, _ := e.Configuration["helmCharts"].([]any)
	for _, v := range charts {
		chart, ok := v.(map[string]any)
		if !ok {
			return errors.New("invalid helmCharts entry")
		}
		chartName, _ := chart["name"].(string)
		version, _ := chart["version"].(string)
		repo, _ := chart["repo"].(string)
		if chartName == "" || path.Base(chartName) != chartName || !fs.ValidPath(chartName) || strings.HasPrefix(chartName, "-") {
			return errors.New("chart name must be a simple directory name")
		}
		prefix := path.Join(e.Path, home, chartName)
		if s.Files[path.Join(prefix, "Chart.yaml")] != nil {
			for _, gap := range s.Gaps {
				if strings.HasPrefix(gap, prefix+"/") {
					return fmt.Errorf("chart has omitted input: %s", gap)
				}
			}
			h := sha256.New()
			for _, file := range s.Paths() {
				if strings.HasPrefix(file, prefix+"/") {
					fmt.Fprintf(h, "%s\x00", file)
					h.Write(s.Files[file])
				}
			}
			digests[prefix] = fmt.Sprintf("%x", h.Sum(nil))
			continue
		}
		if repo == "" || version == "" || strings.ContainsAny(version, "*^~<>= ,") {
			return fmt.Errorf("%s requires a captured chart or an exact chart version and repository", chartName)
		}
		a, err := r.helm().Acquire(ctx, chartName, repo, version)
		if err != nil {
			return err
		}
		total := 0
		for _, data := range s.Files {
			total += len(data)
		}
		for file, data := range a.Files {
			total += len(data)
			if total > 32<<20 || len(s.Files) >= 4096 {
				return errors.New("captured dependencies exceed 32 MiB or 4,096 files")
			}
			s.Files[path.Join(prefix, file)] = data
		}
		digests[prefix] = a.Digest
	}
	return nil
}

func verifiedOrigins(i *Inventory, entry string, authoritative, observed []manifest.Resource) map[string]string {
	origins := map[string]string{}
	if len(authoritative) != len(observed) {
		return origins
	}
	originals := map[string]manifest.Resource{}
	for _, r := range authoritative {
		originals[r.Key()] = r
	}
	for _, r := range observed {
		original, ok := originals[r.Key()]
		if !ok {
			return map[string]string{}
		}
		metadata, _ := r.Object["metadata"].(map[string]any)
		annotations, _ := metadata["annotations"].(map[string]any)
		value, _ := annotations["config.kubernetes.io/origin"].(string)
		originalMetadata, _ := original.Object["metadata"].(map[string]any)
		originalAnnotations, _ := originalMetadata["annotations"].(map[string]any)
		if old, exists := originalAnnotations["config.kubernetes.io/origin"]; exists {
			annotations["config.kubernetes.io/origin"] = old
		} else {
			delete(annotations, "config.kubernetes.io/origin")
		}
		if len(annotations) == 0 && originalMetadata["annotations"] == nil {
			delete(metadata, "annotations")
		}
		if manifest.JSON(r.Object) != manifest.JSON(original.Object) {
			return map[string]string{}
		}
		var origin struct {
			Path string `json:"path"`
		}
		if yaml.Unmarshal([]byte(value), &origin) == nil && origin.Path != "" {
			file := path.Join(entry, origin.Path)
			if fs.ValidPath(file) && i.Source.Files[file] != nil {
				origins[r.Key()] = file
			}
		}
	}
	return origins
}
