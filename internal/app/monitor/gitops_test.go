package monitor

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	gitops "github.com/simjay/plous/internal/integrations/gitops/common"
)

func gitOpsKey(value string) tea.KeyPressMsg {
	key := tea.Key{Text: value}
	switch value {
	case "enter":
		key.Code, key.Text = tea.KeyEnter, ""
	case "esc":
		key.Code, key.Text = tea.KeyEscape, ""
	case "backspace":
		key.Code, key.Text = tea.KeyBackspace, ""
	case "down":
		key.Code, key.Text = tea.KeyDown, ""
	case "end":
		key.Code, key.Text = tea.KeyEnd, ""
	case "pgdown":
		key.Code, key.Text = tea.KeyPgDown, ""
	default:
		if len([]rune(value)) == 1 {
			key.Code = []rune(value)[0]
		}
	}
	return tea.KeyPressMsg(key)
}

func gitOpsApplication(name, namespace, sync, health string) gitops.Item {
	return gitops.Item{Ref: gitops.Reference{Group: "argoproj.io", Version: "v1alpha1", Kind: "Application", Namespace: namespace, Name: name}, Sync: sync, Health: health}
}

func TestGitOpsStatusGroupingAndSelectionSurviveRefresh(t *testing.T) {
	healthy := gitOpsApplication("healthy", "argocd", "Synced", "Healthy")
	drifted := gitOpsApplication("drifted", "argocd", "OutOfSync", "Healthy")
	degraded := gitOpsApplication("degraded", "argocd", "Synced", "Degraded")
	s := NewGitOpsState(gitops.ArgoCD, "")
	s.Apply(gitops.Snapshot{State: gitops.Available, Items: []gitops.Item{healthy, drifted, degraded}})
	rows := s.Rows()
	if rows[0].Ref.Name != "degraded" || rows[1].Ref.Name != "drifted" || rows[2].Ref.Name != "healthy" {
		t.Fatalf("status distinctions lost: %#v", rows)
	}
	s.SelectKey(drifted.Ref.Key())
	drifted.Health = "Degraded"
	s.Apply(gitops.Snapshot{State: gitops.Available, Items: []gitops.Item{drifted, degraded, healthy}})
	if s.SelectedKey() != drifted.Ref.Key() {
		t.Fatal("refresh changed selected object")
	}
	s.Open()
	body := ansi.Strip(s.Body(100, 40))
	if !strings.Contains(body, "OutOfSync") || !strings.Contains(body, "Degraded") {
		t.Fatalf("details combined distinct sync and health: %s", body)
	}
}

func TestGitOpsFailedRefreshRetainsPreviousObservations(t *testing.T) {
	readAt := time.Date(2026, 10, 4, 10, 0, 0, 0, time.UTC)
	s := NewGitOpsState(gitops.ArgoCD, "")
	s.Apply(gitops.Snapshot{State: gitops.Available, ReadAt: readAt, Items: []gitops.Item{gitOpsApplication("payments", "argocd", "Synced", "Degraded")}})
	s.Apply(gitops.Snapshot{State: gitops.ReadDenied, Message: "Application listing is forbidden", ReadAt: readAt.Add(time.Minute)})
	if len(s.Rows()) != 1 || !s.Snapshot.ReadAt.Equal(readAt) || s.Err == "" {
		t.Fatalf("failed refresh lost earlier evidence: %#v", s)
	}
	if body := ansi.Strip(s.Body(100, 30)); !strings.Contains(body, "Previous observations") || !strings.Contains(body, "forbidden") {
		t.Fatalf("old rows are not marked as retained: %s", body)
	}
	s.Apply(gitops.Snapshot{State: gitops.Empty})
	if len(s.Rows()) != 0 || s.Err != "" {
		t.Fatal("successful empty read did not replace earlier observations")
	}
}

func TestGitOpsDependencyBackPreservesParentAndList(t *testing.T) {
	parent := gitops.Item{Ref: gitops.Reference{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Kind: "Kustomization", Namespace: "flux-system", Name: "storefront"}, Status: "Not ready", Reason: "Dependency platform is not ready"}
	dependency := parent
	dependency.Ref.Name, dependency.Reason = "platform", "Health check failed"
	parent.Dependencies = []gitops.Reference{dependency.Ref}
	s := NewGitOpsState(gitops.Flux, "")
	s.Apply(gitops.Snapshot{State: gitops.Available, Items: []gitops.Item{dependency, parent}})
	s.Query = "storefront"
	s.Open()
	s.Scroll, s.ConfigurationExpanded = 3, true
	action := s.Open()
	if action == nil || action.Kind != "detail" || action.Ref.Key() != dependency.Ref.Key() {
		t.Fatalf("dependency did not create an explicit cluster read: %#v", action)
	}
	s.TakeAction()
	s.ApplyDetail(dependency)
	if s.Detail.Ref.Name != "platform" || s.Scroll != 0 {
		t.Fatal("dependency did not open at its own starting position")
	}
	if !s.Back() || s.Detail.Ref.Name != "storefront" || s.Scroll != 3 || !s.ConfigurationExpanded {
		t.Fatal("back lost the parent detail state")
	}
	if !s.Back() || s.Detail != nil || s.Query != "storefront" || s.SelectedKey() != parent.Ref.Key() {
		t.Fatal("back lost list identity or filter")
	}
}

func TestGitOpsFilterOwnsLiteralKeysAndPaste(t *testing.T) {
	s := NewGitOpsState(gitops.ArgoCD, "")
	s.Apply(gitops.Snapshot{State: gitops.Available, Items: []gitops.Item{gitOpsApplication("payments", "argocd", "Synced", "Healthy")}})
	s.Key(gitOpsKey("/"))
	for _, key := range "ff:ng" {
		s.Key(gitOpsKey(string(key)))
	}
	s.Paste(";kubectl reconcile")
	if s.Input.Text != "ff:ng;kubectl reconcile" || s.Pending != nil || !s.FilterInput() {
		t.Fatalf("filter triggered navigation: %#v", s)
	}
	s.Key(gitOpsKey("esc"))
	if s.Input != nil || s.Query != "" || s.Detail != nil {
		t.Fatal("cancelled search changed navigation")
	}
	s.Key(gitOpsKey("/"))
	s.Paste("payments")
	s.Key(gitOpsKey("enter"))
	if s.Detail == nil || s.Query != "payments" || s.FilterInput() {
		t.Fatal("Enter did not open the filtered object")
	}
	s.Key(gitOpsKey("/"))
	s.Body(60, 20)
	s.Paste("ff:n")
	if s.Find.Query != "ff:n" || s.Pending != nil {
		t.Fatal("detail find did not own literal paste")
	}
	s.Key(gitOpsKey("esc"))
	if s.Find != nil || s.Detail == nil {
		t.Fatal("Esc left details before clearing find")
	}
	s.Back()
	s.Key(gitOpsKey("backspace"))
	if s.Query != "" || s.Namespace != "" {
		t.Fatal("clearing filter changed namespace")
	}
}

func TestGitOpsSourcesAndResourcesKeepTypedDestinations(t *testing.T) {
	item := gitops.Item{Ref: gitops.Reference{Kind: "HelmRelease", Namespace: "flux-system", Name: "metrics"}, Status: "Ready", Destination: gitops.Destination{Remote: true, Namespace: "monitoring"}, Sources: []gitops.SourceReference{{Ref: gitops.Reference{Kind: "OCIRepository", Group: "source.toolkit.fluxcd.io", Namespace: "flux-system", Name: "charts"}}}, Resources: []gitops.ManagedResource{{Ref: gitops.Reference{Kind: "Deployment", Group: "apps", Namespace: "monitoring", Name: "metrics"}}}}
	s := NewGitOpsState(gitops.Flux, "")
	s.ApplyDetail(item)
	actions := s.Actions()
	if len(actions) != 3 || actions[0].Kind != "detail" || actions[0].Ref.Kind != "OCIRepository" || actions[1].Kind != "resource" || !actions[1].Destination.Remote {
		t.Fatalf("source and destination routing lost: %#v", actions)
	}
	s.Select(1)
	if got := actions[1]; got.Ref.Namespace != "monitoring" || got.Destination.Namespace != "monitoring" {
		t.Fatal("resource link silently used the controller namespace")
	}
	if got := s.Open(); got != nil || s.Pending != nil {
		t.Fatal("remote controller credentials became a live resource link")
	}
	s.Select(2)
	s.Open()
	if !s.ConfigurationExpanded {
		t.Fatal("configuration control is not reachable by selection")
	}
	s.Key(gitOpsKey("g"))
	if action := s.TakeAction(); action == nil || action.Kind != "detail" || action.Ref.Key() != item.Ref.Key() {
		t.Fatal("detail refresh read the deployment list instead of the current object")
	}
}

func TestGitOpsNarrowViewsFitAndMissingHealthStaysUnknown(t *testing.T) {
	item := gitOpsApplication("payments-with-a-long-name", "argocd", "Synced", "Degraded")
	item.Reason = "Deployment/payments exceeded its progress deadline. " + strings.Repeat("Long controller message. ", 20)
	item.Resources = []gitops.ManagedResource{{Ref: gitops.Reference{Kind: "Deployment", Group: "apps", Name: "payments", Namespace: "shop"}}}
	item.Fields = []gitops.Field{{Label: "Compared revision", Value: "main@sha1:123456789"}}
	for _, width := range []int{16, 28, 40, 60, 100} {
		s := NewGitOpsState(gitops.ArgoCD, "")
		s.Apply(gitops.Snapshot{State: gitops.Available, Items: []gitops.Item{item}})
		for _, detail := range []bool{false, true} {
			if detail {
				s.Open()
			}
			body := s.Body(width, 18)
			if lipgloss.Width(body) > width || lipgloss.Height(body) > 18 {
				t.Fatalf("view overflow at width %d detail %v: %dx%d", width, detail, lipgloss.Width(body), lipgloss.Height(body))
			}
		}
		all := ansi.Strip(strings.Join(s.detailLines(100), "\n"))
		if !strings.Contains(all, "Health Not reported") {
			t.Fatal("missing resource health was asserted healthy")
		}
		s.Key(gitOpsKey("end"))
		s.Key(gitOpsKey("enter"))
		s.Body(width, 18)
		if !s.ConfigurationExpanded {
			t.Fatal("narrow details lost the configuration control")
		}
	}
}

func TestGitOpsTablePreviewKeepsSingleRowsAndFullMessageInDetails(t *testing.T) {
	s := NewGitOpsState(gitops.ArgoCD, "")
	item := gitOpsApplication("payments", "argocd", "Synced", "Degraded")
	item.Destination.Namespace = "shop"
	item.Reason = strings.Repeat("Controller reports a long error. ", 30) + "END OF FULL MESSAGE"
	s.Apply(gitops.Snapshot{State: gitops.Available, Items: []gitops.Item{item}, ReadAt: time.Now()})
	body := ansi.Strip(s.Body(120, 22))
	if !strings.Contains(body, "Namespace") || !strings.Contains(body, "Filter") || !strings.Contains(body, "Enter Details") || !strings.Contains(body, "Plous read:") {
		t.Fatalf("missing shared collection controls: %s", body)
	}
	var row string
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "payments") && strings.Contains(line, "Synced") {
			row = line
		}
	}
	if !strings.Contains(row, "argocd") || !strings.Contains(row, "Degraded") || !strings.Contains(row, "shop") {
		t.Fatalf("identity/status not in one row: %q", row)
	}
	if strings.Contains(body, "END OF FULL MESSAGE") || strings.Count(body, "Controller reports") > 4 {
		t.Fatal("full message expanded in preview")
	}
	s.Open()
	if !strings.Contains(ansi.Strip(strings.Join(s.detailLines(120), "\n")), "END OF FULL MESSAGE") {
		t.Fatal("preview discarded full message")
	}
}

func TestGitOpsEveryCategoryFitsSmallAndWidePanels(t *testing.T) {
	for _, tool := range []gitops.Tool{gitops.ArgoCD, gitops.Flux} {
		for _, section := range gitops.Sections(tool) {
			s := NewGitOpsState(tool, "")
			s.Section = section
			s.Loaded = true
			s.Snapshot = gitops.Snapshot{State: gitops.Available, ReadAt: time.Now(), Items: []gitops.Item{{Ref: gitops.Reference{Name: strings.Repeat("long-name-", 7), Namespace: "controllers"}, Reason: strings.Repeat("message ", 60), Status: "Not ready"}}}
			for _, width := range []int{16, 28, 42, 60, 80, 140} {
				for _, height := range []int{5, 10, 16, 30} {
					body := s.Body(width, height)
					if lipgloss.Width(body) > width || lipgloss.Height(body) > height {
						t.Fatalf("%s overflow at %dx%d: %dx%d", section, width, height, lipgloss.Width(body), lipgloss.Height(body))
					}
				}
			}
		}
	}
}

func TestGitOpsProjectRelationsOpenApplicationAndReturn(t *testing.T) {
	s := NewGitOpsState(gitops.ArgoCD, "")
	s.Section = gitops.Projects
	ref := gitops.Reference{Group: "argoproj.io", Kind: "Application", Namespace: "argocd", Name: "payments"}
	s.ApplyDetail(gitops.Item{Ref: gitops.Reference{Group: "argoproj.io", Kind: "AppProject", Name: "shop", Namespace: "argocd"}, Related: []gitops.ManagedResource{{Ref: ref, Sync: "Synced", Health: "Degraded"}}, RelatedMessage: "Applications in namespace argocd."})
	body := ansi.Strip(s.Body(100, 30))
	if !strings.Contains(body, "payments") || strings.Contains(body, "Managed resources") || strings.Contains(body, "Status  Not reported") {
		t.Fatalf("project rendered as deployment: %s", body)
	}
	action := s.Open()
	if action == nil || action.Kind != "detail" || action.Ref != ref {
		t.Fatal("related application was not an Argo detail link")
	}
	s.ApplyDetail(gitOpsApplication("payments", "argocd", "Synced", "Degraded"))
	s.Back()
	if s.Detail.Ref.Kind != "AppProject" {
		t.Fatal("Back lost project")
	}
}
