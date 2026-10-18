package app

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	gitops "github.com/simjay/plous/internal/integrations/gitops/common"
	helm "github.com/simjay/plous/internal/integrations/helm/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOverviewFullApplicationComposition(t *testing.T) {
	m, v := overviewModel(t)
	now := time.Now()
	k := kubernetes.Overview{ReadAt: now, MetricsAt: now, MetricsEnd: now, MetricsObserved: 13, MetricsExpected: 13, Reads: map[string]kubernetes.OverviewRead{}, NodesReady: 13, PodsRunning: 323, PodsPending: 8, PodsFailed: 3, PodsSucceeded: 13, Restarts: 82, HPACurrent: 28, HPADesired: 31, HPAUp: 1, HPAStable: 5, HPALastScale: now.Add(-2 * time.Minute)}
	k.CPU = kubernetes.OverviewMetric{Used: new(49.9), Allocatable: new(118.79), Requests: new(70.12), Limits: new(79.68)}
	k.Memory = kubernetes.OverviewMetric{Used: new(178.3 * (1 << 30)), Allocatable: new(569.5 * (1 << 30)), Requests: new(275.4 * (1 << 30)), Limits: new(298.7 * (1 << 30))}
	for key, count := range map[string]int{"nodes": 13, "pods": 347, "deployments.apps": 73, "statefulsets.apps": 21, "daemonsets.apps": 8, "horizontalpodautoscalers.autoscaling": 6} {
		k.Reads[key] = kubernetes.OverviewRead{Count: count, ReadAt: now}
	}
	dep := kubecommon.Resource{Group: "apps", Version: "v1", Name: "deployments", Kind: "Deployment", Namespaced: true}
	pod := kubecommon.Resource{Version: "v1", Name: "pods", Kind: "Pod", Namespaced: true}
	for i, name := range []string{"payment-service", "user-api", "frontend", "analytics", "notification", "postgresql", "redis", "ingress-nginx"} {
		k.Changes = append(k.Changes, kubernetes.OverviewEntry{Resource: dep, Namespace: "production", Name: name, UID: name, Revision: fmt.Sprint(72 - i), Outcome: "Available", At: now.Add(-time.Duration(3+i*4) * time.Minute)})
	}
	for i, cause := range []string{"CrashLoopBackOff", "ImagePullBackOff", "Readiness gate blocked"} {
		k.Attention = append(k.Attention, kubernetes.OverviewEntry{Resource: pod, Namespace: "production", Name: []string{"user-api-676d9c87-xy212", "notification-worker-78f99", "payment-service-56fc2f"}[i], Summary: cause, Reason: cause, UID: fmt.Sprintf("failing-pod-%d", i), Severity: 2, At: now.Add(-time.Duration(2+i*3) * time.Minute)})
	}
	data := monitor.OverviewData{Kubernetes: k, Helm: &helm.Overview{ReadAt: now, Releases: []helm.OverviewRelease{{Name: "analytics", Namespace: "production", Status: "failed", Revision: 4, Updated: now.Add(-12 * time.Minute)}, {Name: "postgresql", Namespace: "infra", Status: "deployed", Revision: 9, Updated: now.Add(-time.Hour)}}}, Argo: &gitops.Snapshot{ReadAt: now, State: gitops.Available, Items: []gitops.Item{{Ref: gitops.Reference{Kind: "Application", Name: "payments", Namespace: "argocd"}, Sync: "Synced", Health: "Healthy", Status: "Synced / Healthy", Fields: []gitops.Field{{Label: "Latest operation", Value: "Succeeded"}, {Label: "Operation finished", Value: now.Add(-time.Minute).Format(time.RFC3339)}, {Label: "Successful operation revision", Value: "a13bf92"}}}}}, Flux: []gitops.Snapshot{{ReadAt: now, State: gitops.Available, Items: []gitops.Item{{Ref: gitops.Reference{Kind: "Kustomization", Name: "platform", Namespace: "flux-system"}, Status: "Not ready", Reason: "HealthCheckFailed", ObservedAt: now.Add(-5 * time.Minute)}}}, {ReadAt: now, State: gitops.Empty}, {ReadAt: now, State: gitops.Empty}}, Notices: []string{"Network: no telemetry provider configured"}}
	m.width, m.height, m.focus = 220, 50, 1
	saved := v.State
	v.State = monitor.OverviewState{}
	v.State.Apply(data)
	captureOverviewScreen(t, m, "overview-visual-first-load-220x50")
	v.State = monitor.OverviewState{}
	for i := 0; i < 4; i++ {
		sample := data
		at := now.Add(-time.Duration(3-i) * 15 * time.Second)
		sample.Kubernetes.ReadAt, sample.Kubernetes.MetricsAt = at, at
		sample.Kubernetes.CPU.Used = new([]float64{5.5, 6.8, 5.9, 6.5}[i])
		v.State.Apply(sample)
	}
	for _, size := range [][2]int{{150, 40}, {220, 50}} {
		m.width, m.height = size[0], size[1]
		captureOverviewScreen(t, m, fmt.Sprintf("overview-visual-startup-%dx%d", size[0], size[1]))
	}
	v.State = saved
	for i := 0; i < 60; i++ {
		at := now.Add(-time.Duration(59-i) * 15 * time.Second)
		data.Kubernetes.ReadAt, data.Kubernetes.MetricsAt = at, at
		data.Kubernetes.Reads = map[string]kubernetes.OverviewRead{}
		for key, read := range k.Reads {
			read.ReadAt = at
			data.Kubernetes.Reads[key] = read
		}
		restarts, replicas := int64(79), int64(26)
		data.Kubernetes.Attention = k.Attention[:2]
		if i == 59 {
			restarts, replicas = 82, 28
			data.Kubernetes.Attention = k.Attention
		}
		data.Kubernetes.Restarts = int(restarts)
		data.Kubernetes.HPACurrent = replicas
		data.Kubernetes.RestartsByContainer = map[string]int64{"observed-pod/containers/worker": restarts}
		data.Kubernetes.ReplicasByHPA = map[string]int64{"observed-hpa": replicas}
		data.Kubernetes.CPU.Used = new(40.0 + float64(i%9)*2)
		data.Kubernetes.Memory.Used = new((150 + float64(i%13)*3) * (1 << 30))
		v.State.Apply(data)
	}
	for _, size := range [][2]int{{80, 24}, {109, 40}, {110, 40}, {149, 40}, {150, 40}, {180, 45}, {220, 50}, {260, 60}} {
		m.width, m.height = size[0], size[1]
		m.focus = 1
		m.leftHidden, m.rightHidden = false, false
		v.State.Pane = 0
		captureOverviewScreen(t, m, fmt.Sprintf("overview-visual-%dx%d", size[0], size[1]))
		if size[0] >= 150 {
			plain := ansi.Strip(m.render())
			for _, label := range []string{"Pods", "Autoscaling", "Infrastructure Snapshot"} {
				if !strings.Contains(plain, label) {
					t.Fatalf("normal %v hides %s", size, label)
				}
			}
		}
		if size[0] >= 220 {
			view := ansi.Strip(m.render())
			for _, text := range []string{"Recent Deployments", "Needs Attention", "Infrastructure Snapshot", "Autoscaling", "Helm", "Argo CD", "1 kustomization", "0 Helm releases", "4 integrations"} {
				if !strings.Contains(view, text) {
					t.Fatalf("normal composition %v missing %s", size, text)
				}
			}
		}
		for _, pane := range []int{2, 3} {
			v.State.Pane = pane
			captureOverviewScreen(t, m, fmt.Sprintf("overview-visual-%dx%d-pane%d", size[0], size[1], pane))
		}
	}
	m.width, m.height = 220, 50
	m.execute("fullscreen")
	captureOverviewScreen(t, m, "overview-visual-fullscreen")
	m.execute("fullscreen")
	v.State = monitor.OverviewState{}
	onlyKube := data
	onlyKube.Helm, onlyKube.Argo, onlyKube.Flux = nil, nil, nil
	onlyKube.Notices = []string{"Argo CD API not detected here", "Flux APIs not detected in this cluster", "Network: no telemetry provider configured"}
	v.State.Apply(onlyKube)
	captureOverviewScreen(t, m, "overview-visual-kubernetes-only-220x50")
	partial := data
	partial.Flux = append([]gitops.Snapshot(nil), data.Flux...)
	partial.Flux[1] = gitops.Snapshot{ReadAt: now, State: gitops.ReadDenied}
	partial.Notices = []string{"Flux HelmReleases: read denied", "Network: no telemetry provider configured"}
	v.State = monitor.OverviewState{}
	v.State.Apply(partial)
	captureOverviewScreen(t, m, "overview-visual-partial-220x50")
	v.State = monitor.OverviewState{}
	for i := 0; i < 60; i++ {
		at := now.Add(-time.Duration(59-i) * 15 * time.Second)
		data.Kubernetes.ReadAt, data.Kubernetes.MetricsAt = at, at
		data.Kubernetes.CPU.Used = new(3.5 + float64(i%7)*0.28)
		data.Kubernetes.Memory.Used = new((165 + float64(i%9)*2) * (1 << 30))
		v.State.Apply(data)
	}
	for _, size := range [][2]int{{150, 40}, {220, 50}} {
		m.width, m.height = size[0], size[1]
		captureOverviewScreen(t, m, fmt.Sprintf("overview-visual-low-usage-%dx%d", size[0], size[1]))
	}
}

func captureOverviewScreen(t *testing.T, m *model, name string) {
	t.Helper()
	out := m.render()
	plain := ansi.Strip(out)
	rows := strings.Split(plain, "\n")
	if len(rows) != m.height {
		t.Fatalf("%s height=%d", name, len(rows))
	}
	for _, row := range rows {
		if ansi.StringWidth(row) > m.width {
			t.Fatalf("%s overflow", name)
		}
	}
	if directory := os.Getenv("PLOUS_REVIEW_DIR"); directory != "" {
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
		for suffix, content := range map[string]string{"ansi": out, "txt": plain} {
			if err := os.WriteFile(filepath.Join(directory, name+"."+suffix), []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
}
