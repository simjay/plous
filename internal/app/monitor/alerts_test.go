package monitor

import (
	"testing"
	"time"

	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
)

func TestAlertRulesResetCoverageStreakAndKeepOpenAlert(t *testing.T) {
	var state State
	var rules alertRules
	now := time.Now()
	check := func(bad bool) kubernetes.CheckResult {
		return kubernetes.CheckResult{Objects: []kubernetes.ResourceCheck{{
			CheckTarget: kubernetes.CheckTarget{Kind: "Deployment", Name: "checkout"}, UID: "checkout-uid",
			Signals: []kubernetes.AlertSignal{{Rule: kubernetes.AlertRule{Name: "container crash loop", Checks: 2}, Container: "app", Bad: bad}},
		}}}
	}
	pulse := func(sample kubernetes.CheckResult) []Alert {
		now = now.Add(time.Minute)
		return rules.apply(&state, sample, now)
	}
	gap := kubernetes.CheckResult{Coverage: map[string]string{"Deployment": "denied"}}
	for _, sample := range []kubernetes.CheckResult{check(true), gap, check(true)} {
		if changed := pulse(sample); len(changed) != 0 {
			t.Fatalf("incomplete streak triggered an alert: %+v", changed)
		}
	}
	opened := pulse(check(true))
	if len(opened) != 1 || opened[0].State != "open" || opened[0].PresentAtStart {
		t.Fatalf("bad streak did not open an alert: %+v", opened)
	}
	for _, sample := range []kubernetes.CheckResult{check(true), check(false), gap, check(false)} {
		if changed := pulse(sample); len(changed) != 0 || state.Alerts[0].State != "open" {
			t.Fatalf("alert changed without confirmed recovery: %+v", changed)
		}
	}
	recovered := pulse(check(false))
	if len(recovered) != 1 || recovered[0].ID != opened[0].ID || recovered[0].State != "recovered" {
		t.Fatalf("healthy streak did not close the same alert: %+v", recovered)
	}
}

func TestAlertRulesDistinguishRemovalAndExistingAlerts(t *testing.T) {
	var state State
	var rules alertRules
	data := kubernetes.CheckResult{Objects: []kubernetes.ResourceCheck{{
		CheckTarget: kubernetes.CheckTarget{Kind: "Deployment", Name: "checkout"}, UID: "uid",
		Signals: []kubernetes.AlertSignal{{Rule: kubernetes.AlertRule{Name: "stalled rollout", Checks: 1}, Bad: true}},
	}}}
	now := time.Now()
	opened := rules.apply(&state, data, now)
	if len(opened) != 1 || !opened[0].PresentAtStart {
		t.Fatalf("first observed problem did not open immediately as present at start: %+v", opened)
	}
	if changed := rules.apply(&state, data, now.Add(time.Minute)); len(changed) != 0 {
		t.Fatalf("unchanged rollout emitted another transition: %+v", changed)
	}
	removed := rules.apply(&state, kubernetes.CheckResult{}, now.Add(2*time.Minute))
	if len(removed) != 1 || removed[0].State != "removed" {
		t.Fatalf("confirmed deletion did not close the alert as removed: %+v", removed)
	}
}

func TestAlertRulesCloseRemovedContainerAfterCompleteInventory(t *testing.T) {
	var state State
	var rules alertRules
	rule := kubernetes.AlertRule{Name: "container crash loop", Checks: 2}
	object := kubernetes.ResourceCheck{
		CheckTarget: kubernetes.CheckTarget{Kind: "Deployment", Name: "checkout"}, UID: "checkout-uid",
		Signals: []kubernetes.AlertSignal{{Rule: rule, Container: "sidecar", Bad: true}},
	}
	now := time.Now()
	check := kubernetes.CheckResult{Objects: []kubernetes.ResourceCheck{object}}
	rules.apply(&state, check, now)
	rules.apply(&state, check, now.Add(time.Minute))
	if state.OpenAlerts() != 1 {
		t.Fatal("the confirmed crash did not open an alert")
	}
	check.Objects[0].Signals = nil
	if changed := rules.apply(&state, check, now.Add(2*time.Minute)); len(changed) != 0 || state.OpenAlerts() != 1 {
		t.Fatal("missing signals closed the alert without a complete inventory")
	}
	check.Objects[0].CompleteRules = []kubernetes.AlertRule{rule}
	changed := rules.apply(&state, check, now.Add(3*time.Minute))
	if len(changed) != 1 || changed[0].State != "removed" || state.OpenAlerts() != 0 {
		t.Fatalf("the removed container's alert stayed open: %+v", changed)
	}
}
