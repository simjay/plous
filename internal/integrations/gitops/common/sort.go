package common

import (
	"fmt"
	"slices"
	"strings"
)

func SortItems(items []Item) {
	slices.SortStableFunc(items, func(a, b Item) int {
		if d := severity(a) - severity(b); d != 0 {
			return d
		}
		return strings.Compare(fmt.Sprintf("%s/%s/%s", a.Ref.Namespace, a.Ref.Name, a.Ref.Kind), fmt.Sprintf("%s/%s/%s", b.Ref.Namespace, b.Ref.Name, b.Ref.Kind))
	})
}

func severity(item Item) int {
	if item.Health == "Degraded" || item.Health == "Missing" || item.Status == "Not ready" || item.Status == "Stalled" || item.Status == "Failed" {
		return 0
	}
	if item.ControllerError {
		return 0
	}
	if item.Sync == "OutOfSync" || item.Sync == "Unknown" || item.Health == "Progressing" || item.Health == "Unknown" || item.Health == "Not reported" || item.Status == "Reconciling" || item.Status == "Awaiting status" || item.Status == "Unknown" || item.Status == "Not reported" {
		return 1
	}
	if item.Status == "Suspended" {
		return 2
	}
	return 3
}
