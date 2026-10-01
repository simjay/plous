package kubernetes

import (
	"errors"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestLogErrorsGiveRelevantRecoveryWithoutLeakingServerText(t *testing.T) {
	sensitive := "https://user:private-token@example.invalid/log"
	cases := []struct {
		err      error
		previous bool
		want     string
	}{
		{apierrors.NewForbidden(schema.GroupResource{Resource: "pods/log"}, "pod", errors.New(sensitive)), false, "pods/log"},
		{apierrors.NewBadRequest(sensitive), true, "retained previous instance"},
		{apierrors.NewBadRequest(sensitive), false, "Describe and Events"},
		{apierrors.NewUnauthorized(sensitive), false, "Renew your credentials"},
		{apierrors.NewTooManyRequests(sensitive, 1), false, "Wait briefly"},
		{TargetChangedError(), false, "Reopen its details"},
		{errors.New(sensitive), false, "Check this connection"},
	}
	for _, tc := range cases {
		got := LogErrorText(tc.err, tc.previous)
		if !strings.Contains(got, tc.want) || strings.Contains(got, "private-token") || strings.Contains(got, "Retrying") {
			t.Fatalf("unhelpful or unsafe log error: %s", got)
		}
	}
	if LogErrorText(nil, false) != "" {
		t.Fatal("nil error became a failure")
	}
}
