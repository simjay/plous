package monitor

import (
	"context"
	"strings"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func resourceListReferences(r kubecommon.Resource, object unstructured.Unstructured, resources []kubecommon.Resource) []Reference {
	var refs []Reference
	add := func(target kubecommon.Resource, ns, relation string) {
		if !target.Namespaced {
			ns = ""
		}
		refs = append(refs, Reference{Resource: target, Namespace: ns, Relation: relation, List: true})
	}
	named := func(key, ns, relation string) {
		if target, ok := findResourceIdentity(resources, key); ok {
			add(target, ns, relation)
		}
	}
	o := object.Object
	switch r.Key() {
	case "namespaces":
		for _, target := range resources {
			if target.Namespaced {
				add(target, object.GetName(), "Browse namespace resources")
			}
		}
	case "resourcequotas", "limitranges":
		for _, key := range []string{"pods", "deployments.apps", "statefulsets.apps", "jobs.batch", "persistentvolumeclaims"} {
			named(key, object.GetNamespace(), "Browse namespace scope")
		}
	case "customresourcedefinitions.apiextensions.k8s.io":
		named(Value(o, "spec", "names", "plural")+"."+Value(o, "spec", "group"), "", "Browse instances from served discovery")
	case "apiservices.apiregistration.k8s.io":
		for _, target := range resources {
			if target.Group == Value(o, "spec", "group") && target.Version == Value(o, "spec", "version") {
				add(target, "", "Browse registered API resources")
			}
		}
	case "validatingadmissionpolicies.admissionregistration.k8s.io", "mutatingadmissionpolicies.admissionregistration.k8s.io":
		kind := nestedMap(o, "spec", "paramKind")
		gv, err := schema.ParseGroupVersion(Value(kind, "apiVersion"))
		if err == nil {
			for _, target := range resources {
				if target.Group == gv.Group && target.Kind == Value(kind, "kind") {
					add(target, "", "Browse parameter kind")
				}
			}
		}
	case "referencegrants.gateway.networking.k8s.io":
		for _, to := range Maps(o, "spec", "to") {
			if Value(to, "name") != "" {
				continue
			}
			for _, target := range resources {
				if target.Group == Value(to, "group") && target.Kind == Value(to, "kind") {
					add(target, object.GetNamespace(), "Browse permitted target kind")
				}
			}
		}
	}
	return refs
}

func (p *relationCollector) admissionParameters(r kubecommon.Resource, object unstructured.Unstructured) {
	if r.Key() != "validatingadmissionpolicybindings.admissionregistration.k8s.io" && r.Key() != "mutatingadmissionpolicybindings.admissionregistration.k8s.io" {
		return
	}
	if p.client == nil {
		return
	}
	policyKey := "validatingadmissionpolicies.admissionregistration.k8s.io"
	if strings.HasPrefix(r.Key(), "mutating") {
		policyKey = "mutatingadmissionpolicies.admissionregistration.k8s.io"
	}
	policyResource, ok := findResourceIdentity(p.resources, policyKey)
	if !ok {
		return
	}
	name := Value(object.Object, "spec", "policyName")
	if name == "" {
		return
	}
	policy, err := p.client.Get(p.ctx, policyResource, "", name)
	if err != nil {
		p.warnings = append(p.warnings, "Policy parameter resolution: "+ErrorText(err))
		return
	}
	kind := nestedMap(policy.Object, "spec", "paramKind")
	gv, err := schema.ParseGroupVersion(Value(kind, "apiVersion"))
	if err != nil {
		return
	}
	for _, target := range p.resources {
		if target.Group != gv.Group || target.Kind != Value(kind, "kind") {
			continue
		}
		ref := nestedMap(object.Object, "spec", "paramRef")
		ns := Value(ref, "namespace")
		if target.Namespaced && ns == "" {
			p.warnings = append(p.warnings, "Parameter namespace depends on each admission request; no fixed parameter object can be linked.")
			return
		}
		if parameter := Value(ref, "name"); parameter != "" {
			p.named(target.Key(), ns, parameter, "", "Admission parameter reference")
		} else if selector := nestedMap(ref, "selector"); selector != nil {
			p.query(target.Key(), ns, "Admission parameter selector match", func(candidate unstructured.Unstructured) bool {
				return selectorMatches(selector, candidate.GetLabels(), false)
			})
		}
		return
	}
}

func (c *Client) customDefinitionReference(ctx context.Context, r kubecommon.Resource, object unstructured.Unstructured, resources []kubecommon.Resource) []Reference {
	if c == nil || r.Group == "" {
		return nil
	}
	definition, ok := findResourceIdentity(resources, "customresourcedefinitions.apiextensions.k8s.io")
	if !ok {
		return nil
	}
	crd, err := c.Get(ctx, definition, "", r.Name+"."+r.Group)
	if err != nil || Value(crd.Object, "spec", "group") != r.Group || Value(crd.Object, "spec", "names", "plural") != r.Name {
		return nil
	}
	return []Reference{{Resource: definition, Name: crd.GetName(), UID: string(crd.GetUID()), Relation: "Custom resource definition"}}
}
