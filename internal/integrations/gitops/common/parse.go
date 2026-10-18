package common

import (
	"net/url"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func BaseItem(object unstructured.Unstructured, readAt time.Time) Item {
	gvk := object.GroupVersionKind()
	return Item{Ref: Reference{Group: gvk.Group, Version: gvk.Version, Kind: object.GetKind(), Namespace: object.GetNamespace(), Name: object.GetName()}, UID: string(object.GetUID()), ReadAt: readAt}
}

func ParseConditions(m map[string]any, generation int64, checkGeneration bool) []Condition {
	var conditions []Condition
	for _, c := range Maps(m, "status", "conditions") {
		observed, _, _ := unstructured.NestedInt64(c, "observedGeneration")
		conditions = append(conditions, Condition{Type: Str(c, "type"), Status: Str(c, "status"), Reason: Str(c, "reason"), Message: Str(c, "message"), ObservedGeneration: observed, ChangedAt: Timestamp(Str(c, "lastTransitionTime")), Stale: checkGeneration && generation > 0 && observed < generation})
	}
	return conditions
}

func Str(m map[string]any, path ...string) string {
	value, _, _ := unstructured.NestedString(m, path...)
	return value
}

func Maps(m map[string]any, path ...string) []map[string]any {
	values, _, _ := unstructured.NestedSlice(m, path...)
	var result []map[string]any
	for _, value := range values {
		if object, ok := value.(map[string]any); ok {
			result = append(result, object)
		}
	}
	return result
}

func StringsAt(m map[string]any, path ...string) []string {
	values, _, _ := unstructured.NestedStringSlice(m, path...)
	return values
}

func Fallback(value, otherwise string) string {
	if value == "" {
		return otherwise
	}
	return value
}

func AddField(item *Item, label, value string) {
	if value != "" {
		item.Fields = append(item.Fields, Field{Label: label, Value: value})
	}
}

func Timestamp(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339, value)
	return parsed
}

func SafeURL(raw string) string {
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "URL not displayed"
	}
	parsed.User = nil
	parsed.RawQuery, parsed.Fragment = "", ""
	return parsed.String()
}
