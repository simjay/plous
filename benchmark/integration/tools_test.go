package integration

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	bench "github.com/simjay/plous/benchmark"
	"github.com/simjay/plous/internal/ai/agent"
	"github.com/simjay/plous/internal/integrations/observation"
)

func workload(t testing.TB) (bench.Suite, bench.Report) {
	t.Helper()
	s, err := bench.LoadSuite("../fixtures/tools.json")
	if err != nil {
		t.Fatal(err)
	}
	files, fixtures := map[string]string{}, map[string]string{}
	for _, root := range []string{"../../internal", "../../cmd", ".."} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if filepath.Ext(path) != ".go" && filepath.Ext(path) != ".json" {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			absolute, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			base, err := filepath.Abs("../..")
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(base, absolute)
			if err != nil {
				return err
			}
			files[rel] = bench.Hash(string(body))
			if strings.HasPrefix(rel, "benchmark/fixtures/") {
				fixtures[rel] = files[rel]
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"go.mod", "go.sum"} {
		body, err := os.ReadFile("../../" + name)
		if err != nil {
			t.Fatal(err)
		}
		files[name] = bench.Hash(string(body))
	}
	return s, bench.NewReport(s, bench.Hash(fixtures), bench.Hash(files))
}

func retain(t testing.TB, r bench.Report) {
	t.Helper()
	if path := os.Getenv("PLOUSBENCH_REPORT"); path != "" {
		if err := bench.WriteReport(path, r); err != nil {
			t.Error(err)
		}
	}
}

func TestPlousBenchTools(t *testing.T) {
	s, report := workload(t)
	defer func() { retain(t, report) }()
	for _, c := range s.Cases {
		var pair []bench.Result
		for _, transport := range []string{"direct", "mcp"} {
			t.Run(c.ID+"/"+transport, func(t *testing.T) {
				r := execute(t, c, transport)
				report.Results = append(report.Results, r)
				pair = append(pair, r)
				if !r.Passed() {
					t.Fatalf("%s", bench.JSON(r))
				}
			})
		}
		if len(pair) != 2 {
			continue
		}
		failParity := func(detail string) {
			t.Errorf("%s: %s", c.ID, detail)
			for i := len(report.Results) - 2; i < len(report.Results); i++ {
				report.Results[i].Failures = append(report.Results[i].Failures, detail)
			}
		}
		if pair[0].SchemaHash != pair[1].SchemaHash {
			failParity("direct and MCP schemas differ")
		}
		for i, call := range pair[0].Calls {
			if call.Tool != "ui_read" && !strings.HasSuffix(call.Tool, "_show") {
				continue
			}
			var a, b agent.UIResult
			_ = json.Unmarshal(call.Body, &a)
			_ = json.Unmarshal(pair[1].Calls[i].Body, &b)
			// Errors may contain fixture ports; saved refs belong to isolated profiles.
			a.Reason, b.Reason = "", ""
			for _, result := range []*agent.UIResult{&a, &b} {
				if result.View != nil {
					result.View.Ref = ""
				}
			}
			if !reflect.DeepEqual(a, b) {
				failParity(call.ID + ": direct and MCP presentation results differ")
			}
		}
		// Fixed cluster fixtures permit complete result comparisons. Source and
		// history identifiers differ across isolated profiles and have explicit checks.
		if c.Fixture == "source" || c.Fixture == "general" || c.ID == "workspace-scope" {
			continue
		}
		for i := range pair[0].Calls {
			var a, b observation.Delivery
			_ = json.Unmarshal(pair[0].Calls[i].Body, &a)
			_ = json.Unmarshal(pair[1].Calls[i].Body, &b)
			if a.Observation == nil || b.Observation == nil {
				continue
			}
			a.Ref, b.Ref = "", ""
			a.Observation.ObservedAt, b.Observation.ObservedAt = time.Time{}, time.Time{}
			// Resource age and collection time follow the real observation clock.
			for _, d := range []*observation.Delivery{&a, &b} {
				for i := range d.Observation.Rows {
					delete(d.Observation.Rows[i].Facts, "age_seconds")
					delete(d.Observation.Rows[i].Facts, "collected_at")
				}
			}
			if !reflect.DeepEqual(a, b) {
				failParity(pair[0].Calls[i].ID + ": direct and MCP facts differ")
			}
		}
	}
}

func BenchmarkPlousBenchTools(b *testing.B) {
	s, report := workload(b)
	defer func() { retain(b, report) }()
	for _, c := range s.Cases {
		for _, transport := range []string{"direct", "mcp"} {
			var samples []bench.Result
			b.Run(c.ID+"/"+transport, func(b *testing.B) {
				samples = nil
				var ns, bytes, reads int64
				for i := 0; i < b.N; i++ {
					r := execute(b, c, transport)
					samples = append(samples, r)
					for _, call := range r.Calls {
						ns += call.DurationNS
						bytes += int64(call.OutputBytes)
						reads += int64(call.APIReads)
					}
					if !r.Passed() {
						b.Fatal(string(bench.JSON(r)))
					}
				}
				b.ReportMetric(float64(ns)/float64(b.N), "ns/op")
				b.ReportMetric(float64(bytes)/float64(b.N), "response-B/op")
				b.ReportMetric(float64(reads)/float64(b.N), "api-reads/op")
			})
			report.Results = append(report.Results, samples...)
		}
	}
}
