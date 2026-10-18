package monitor

import (
	"context"
	"fmt"
	"strings"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
)

const maxReadinessSlices = 1000

type EndpointReadiness struct {
	Ready, NotReady, Unknown, Total, Slices, StaleSlices int
	State, Note                                          string
	Partial                                              bool
	ObservedAt                                           time.Time
}

func (r EndpointReadiness) Cell() string {
	if r.State != "" {
		return r.State
	}
	text := fmt.Sprintf("%d/%d", r.Ready, r.Total)
	if r.Unknown > 0 {
		text += fmt.Sprintf(" · %d unknown", r.Unknown)
	}
	if r.Partial {
		text = "Partial " + text
	}
	return text
}

func (c *Client) readinessSlices(ctx context.Context, r kubecommon.Resource, ns, selector string) (Collection, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	var result Collection
	options := metav1.ListOptions{Limit: 200, LabelSelector: selector}
	for {
		list, err := c.resource(r, ns).List(ctx, options)
		if err != nil {
			return Collection{}, err
		}
		result.Objects = append(result.Objects, list.Items...)
		options.Continue = list.GetContinue()
		if len(result.Objects) >= maxReadinessSlices {
			result.Truncated = len(result.Objects) > maxReadinessSlices || options.Continue != ""
			result.Objects = result.Objects[:maxReadinessSlices]
			return result, nil
		}
		if options.Continue == "" {
			return result, nil
		}
	}
}

func (c *Client) serviceEndpointReadiness(ctx context.Context, services []unstructured.Unstructured, target kubecommon.Resource, available bool, ns, selector string) map[string]EndpointReadiness {
	result := map[string]EndpointReadiness{}
	byScope := map[string]unstructured.Unstructured{}
	for _, service := range services {
		key := Identity(service)
		if Value(service.Object, "spec", "type") == "ExternalName" {
			result[key] = EndpointReadiness{State: "Not applicable", Note: "ExternalName uses a DNS alias; EndpointSlice readiness does not describe it.", ObservedAt: time.Now()}
			continue
		}
		result[key] = EndpointReadiness{}
		byScope[service.GetNamespace()+"/"+service.GetName()] = service
	}
	if len(byScope) == 0 {
		return result
	}
	var collection Collection
	var err error
	if available {
		collection, err = c.readinessSlices(ctx, target, ns, selector)
	}
	state, note := "", ""
	switch {
	case !available:
		state, note = "API unavailable", "EndpointSlice API was not discovered."
	case apierrors.IsForbidden(err):
		state, note = "Access denied", "EndpointSlice readiness is unavailable because Kubernetes denied access."
	case err != nil:
		state, note = "Query failed", "EndpointSlice readiness: "+ErrorText(err)
	}
	if state != "" {
		for _, service := range byScope {
			result[Identity(service)] = EndpointReadiness{State: state, Note: note, ObservedAt: time.Now()}
		}
		return result
	}
	addresses := map[string]map[string]string{}
	for _, slice := range collection.Objects {
		service, found := byScope[slice.GetNamespace()+"/"+slice.GetLabels()["kubernetes.io/service-name"]]
		if !found {
			continue
		}
		key := Identity(service)
		readiness := result[key]
		stale := false
		for _, owner := range slice.GetOwnerReferences() {
			if owner.Kind == "Service" && groupOf(owner.APIVersion) == "" && owner.Name == service.GetName() && owner.UID != "" && service.GetUID() != "" && owner.UID != service.GetUID() {
				stale = true
			}
		}
		if stale {
			readiness.StaleSlices++
			result[key] = readiness
			continue
		}
		readiness.Slices++
		result[key] = readiness
		if addresses[key] == nil {
			addresses[key] = map[string]string{}
		}
		for i, endpoint := range Maps(slice.Object, "endpoints") {
			state := Value(endpoint, "conditions", "ready")
			if state != "true" && state != "false" {
				state = "unknown"
			}
			values, _, _ := unstructured.NestedStringSlice(endpoint, "addresses")
			if len(values) == 0 {
				values = []string{fmt.Sprintf("unreported/%s/%d", Identity(slice), i)}
				state = "unknown"
			}
			for _, address := range values {
				identity := Value(slice.Object, "addressType") + "/" + address
				addressState := state
				if previous, found := addresses[key][identity]; found && previous != state {
					addressState = "unknown"
				}
				addresses[key][identity] = addressState
			}
		}
	}
	for _, service := range byScope {
		key := Identity(service)
		readiness := result[key]
		readiness.ObservedAt = time.Now()
		readiness.Total, readiness.Partial = len(addresses[key]), collection.Truncated
		for _, state := range addresses[key] {
			switch state {
			case "true":
				readiness.Ready++
			case "false":
				readiness.NotReady++
			default:
				readiness.Unknown++
			}
		}
		if collection.Truncated {
			readiness.Note = fmt.Sprintf("Partial EndpointSlice coverage: query limited to %d slices. Counts describe only returned addresses.", maxReadinessSlices)
		}
		result[key] = readiness
	}
	return result
}

func (c *Client) serviceReadinessSummary(ctx context.Context, service unstructured.Unstructured, resources []kubecommon.Resource) ([]DetailSection, string) {
	target, available := findResourceIdentity(resources, "endpointslices.discovery.k8s.io")
	selector := labels.SelectorFromSet(map[string]string{"kubernetes.io/service-name": service.GetName()}).String()
	readiness := c.serviceEndpointReadiness(ctx, []unstructured.Unstructured{service}, target, available, service.GetNamespace(), selector)[Identity(service)]
	return EndpointReadinessSummary(readiness)
}

func EndpointReadinessSummary(readiness EndpointReadiness) ([]DetailSection, string) {
	section := DetailSection{Title: "EndpointSlice readiness", Fields: []DetailField{{"Ready / addresses", readiness.Cell()}}}
	if readiness.State == "" {
		section.Fields = append(section.Fields, DetailField{"Reported not ready", fmt.Sprint(readiness.NotReady)}, DetailField{"Readiness unknown", fmt.Sprint(readiness.Unknown)}, DetailField{"Associated slices", fmt.Sprint(readiness.Slices)}, DetailField{"Basis", "Distinct address type/address pairs from slices labeled for this Service. Missing or conflicting ready conditions are unknown. This is not a traffic test."})
		if !readiness.ObservedAt.IsZero() {
			section.Fields = append(section.Fields, DetailField{"Observed", readiness.ObservedAt.Format(time.RFC3339)})
		}
		if readiness.StaleSlices > 0 {
			section.Fields = append(section.Fields, DetailField{"Stale associations excluded", fmt.Sprint(readiness.StaleSlices) + " slices owned by a different Service UID"})
		}
	}
	if readiness.Note != "" {
		section.Fields = append(section.Fields, DetailField{"Coverage", readiness.Note})
	}
	warning := readiness.Note
	if strings.EqualFold(readiness.State, "Not applicable") {
		warning = ""
	}
	return []DetailSection{section}, warning
}
