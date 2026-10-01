package kubernetes

import (
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func gatewayRouteRelation(route unstructured.Unstructured, resource Resource, gateway unstructured.Unstructured) string {
	var attachments []string
	for _, ref := range Maps(route.Object, "spec", "parentRefs") {
		if !gatewayReferenceMatches(ref, route.GetNamespace(), "gateway.networking.k8s.io", "Gateway", resource, gateway) {
			continue
		}
		listener := fallback(Value(ref, "sectionName"), "listener unspecified")
		if port := Value(ref, "port"); port != "" {
			listener += ":" + port
		}
		var statuses []string
		for _, parent := range Maps(route.Object, "status", "parents") {
			statusRef := nestedMap(parent, "parentRef")
			if !gatewayReferenceMatches(statusRef, route.GetNamespace(), "gateway.networking.k8s.io", "Gateway", resource, gateway) || Value(statusRef, "sectionName") != Value(ref, "sectionName") || Value(statusRef, "port") != Value(ref, "port") {
				continue
			}
			var conditions []string
			accepted := false
			for _, condition := range Maps(parent, "conditions") {
				kind := Value(condition, "type")
				accepted = accepted || kind == "Accepted"
				text := kind + "=" + fallback(Value(condition, "status"), "Unknown")
				if reason := Value(condition, "reason"); reason != "" {
					text += " (" + reason + ")"
				}
				if observed, found, _ := unstructured.NestedInt64(condition, "observedGeneration"); found && observed < route.GetGeneration() {
					text += " [stale]"
				}
				conditions = append(conditions, text)
			}
			if !accepted {
				conditions = append([]string{"Accepted=Not reported"}, conditions...)
			}
			status := strings.Join(conditions, ", ")
			if controller := Value(parent, "controllerName"); controller != "" {
				status += " · " + controller
			}
			statuses = append(statuses, status)
		}
		if len(statuses) == 0 {
			statuses = []string{"Accepted=Not reported"}
		}
		attachments = append(attachments, listener+": "+strings.Join(statuses, "; "))
	}
	if len(attachments) == 0 {
		return ""
	}
	return "Gateway parent reference · " + strings.Join(attachments, " | ")
}
