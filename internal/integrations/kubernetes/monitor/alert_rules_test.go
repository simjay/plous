package monitor

import (
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestAlertRulesRequireObservedEndpointReadiness(t *testing.T) {
	object := ResourceCheck{CheckTarget: CheckTarget{Kind: "Service", Name: "checkout"}}
	sample := CheckResult{Coverage: map[string]string{}}
	evaluateAlertRules(&sample, &object, unstructured.Unstructured{}, EndpointReadiness{}, nil, false)
	if len(object.Signals) != 0 || sample.Coverage["Service/checkout"] == "" {
		t.Fatal("missing endpoint data was treated as zero ready endpoints")
	}
	sample = CheckResult{Coverage: map[string]string{}}
	readiness := EndpointReadiness{ObservedAt: time.Now()}
	evaluateAlertRules(&sample, &object, unstructured.Unstructured{}, readiness, nil, false)
	if len(object.Signals) != 1 || object.Signals[0].Rule != noReadyEndpoints || !object.Signals[0].Bad {
		t.Fatalf("confirmed zero ready endpoints did not report a fault: %+v", object.Signals)
	}
}

func TestAlertRulesDaemonSetAvailabilityHonorsUpdateBudget(t *testing.T) {
	o := summaryFixture(t, `{
		"metadata":{"generation":2},
		"spec":{"updateStrategy":{"type":"RollingUpdate","rollingUpdate":{"maxUnavailable":"25%"}}},
		"status":{"observedGeneration":2,"desiredNumberScheduled":12,"numberAvailable":9,"updatedNumberScheduled":7}
	}`)
	sample := CheckResult{Coverage: map[string]string{}}
	object := ResourceCheck{CheckTarget: CheckTarget{Kind: "DaemonSet", Name: "logger"}}
	workloadAlertSignals(&sample, &object, o)
	if len(object.Signals) != 1 || object.Signals[0].Bad {
		t.Fatalf("availability within rollout budget alerted: %+v", object.Signals)
	}
	if err := unstructured.SetNestedField(o.Object, int64(8), "status", "numberAvailable"); err != nil {
		t.Fatal(err)
	}
	object.Signals = nil
	workloadAlertSignals(&sample, &object, o)
	if len(object.Signals) != 1 || !object.Signals[0].Bad {
		t.Fatalf("availability below rollout budget did not alert: %+v", object.Signals)
	}
}

func TestAlertRulesMissingContainerStateCannotResolveCrashLoop(t *testing.T) {
	running := summaryFixture(t, `{"spec":{"containers":[{"name":"api"}]},"status":{"containerStatuses":[{"name":"api","state":{"running":{}}}]}}`)
	unobserved := summaryFixture(t, `{"spec":{"containers":[{"name":"api"}]}}`)
	if signals, partial := crashLoopSignals([]unstructured.Unstructured{running, unobserved}); !partial || len(signals) != 0 {
		t.Fatalf("unobserved container yielded a recovery signal: %+v (partial=%v)", signals, partial)
	}
}

func TestAlertRulesMissingContainerStatePreservesObservedCrash(t *testing.T) {
	crashing := summaryFixture(t, `{"spec":{"containers":[{"name":"api"}]},"status":{"containerStatuses":[{"name":"api","state":{"waiting":{"reason":"CrashLoopBackOff"}}}]}}`)
	unobserved := summaryFixture(t, `{"spec":{"containers":[{"name":"api"}]}}`)
	signals, partial := crashLoopSignals([]unstructured.Unstructured{crashing, unobserved})
	if !partial || len(signals) != 1 || !signals[0].Bad || signals[0].Container != "api" {
		t.Fatalf("missing status masked an observed crash: %+v (partial=%v)", signals, partial)
	}
}
