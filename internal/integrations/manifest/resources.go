package manifest

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"strings"

	kyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

type Resource struct {
	APIVersion, Kind, Namespace, Name, Source string
	Object                                    map[string]any
}

func (r Resource) Key() string {
	return r.APIVersion + " / " + r.Kind + " / " + r.Namespace + " / " + r.Name
}

func (r Resource) ScopedKey(namespace string) string {
	if r.Namespace == "" && !r.ClusterScoped() {
		r.Namespace = namespace
	}
	return r.Key()
}

// Unknown extension resources need API discovery to establish their scope.
func (r Resource) ClusterScoped() bool {
	group, _, namespaced := strings.Cut(r.APIVersion, "/")
	if !namespaced {
		group = ""
	}
	switch group + "/" + r.Kind {
	case "/Namespace", "/Node", "/PersistentVolume",
		"rbac.authorization.k8s.io/ClusterRole", "rbac.authorization.k8s.io/ClusterRoleBinding",
		"apiextensions.k8s.io/CustomResourceDefinition", "apiregistration.k8s.io/APIService",
		"admissionregistration.k8s.io/MutatingWebhookConfiguration", "admissionregistration.k8s.io/ValidatingWebhookConfiguration", "admissionregistration.k8s.io/ValidatingAdmissionPolicy", "admissionregistration.k8s.io/ValidatingAdmissionPolicyBinding",
		"storage.k8s.io/StorageClass", "storage.k8s.io/CSIDriver", "storage.k8s.io/CSINode", "storage.k8s.io/VolumeAttachment",
		"scheduling.k8s.io/PriorityClass", "node.k8s.io/RuntimeClass", "certificates.k8s.io/ClusterTrustBundle":
		return true
	}
	return false
}

func Resources(manifest string) ([]Resource, error) {
	reader := kyaml.NewYAMLReader(bufio.NewReader(strings.NewReader(manifest)))
	var result []Resource
	for {
		data, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		var object map[string]any
		if err = yaml.Unmarshal(data, &object); err != nil {
			return nil, errors.New("invalid rendered resource YAML")
		}
		if len(object) == 0 {
			continue
		}
		source := ""
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "# Source: ") {
				source = strings.TrimPrefix(line, "# Source: ")
				break
			}
		}
		items, err := resourceObjects(object, source, 0)
		if err != nil {
			return nil, err
		}
		result = append(result, items...)
	}
	return result, nil
}

func resourceObjects(object map[string]any, source string, depth int) ([]Resource, error) {
	if depth > 8 {
		return nil, errors.New("rendered Lists are nested too deeply")
	}
	r := Resource{Object: object, Source: source}
	r.APIVersion, _ = object["apiVersion"].(string)
	r.Kind, _ = object["kind"].(string)
	if r.Kind == "" || r.APIVersion == "" {
		return nil, errors.New("rendered document is missing apiVersion or kind")
	}
	if r.Kind == "List" && r.APIVersion == "v1" {
		items, ok := object["items"].([]any)
		if !ok {
			return nil, errors.New("rendered List is missing items")
		}
		var result []Resource
		for _, item := range items {
			child, ok := item.(map[string]any)
			if !ok {
				return nil, errors.New("invalid rendered List item")
			}
			children, err := resourceObjects(child, source, depth+1)
			if err != nil {
				return nil, err
			}
			result = append(result, children...)
		}
		return result, nil
	}
	meta, _ := object["metadata"].(map[string]any)
	r.Name, _ = meta["name"].(string)
	r.Namespace, _ = meta["namespace"].(string)
	return []Resource{r}, nil
}
func YAML(value any, redact bool) string {
	if redact {
		value = Redact(value)
	}
	data, err := yaml.Marshal(value)
	if err != nil {
		return "Cannot display YAML"
	}
	return string(data)
}
func RedactedManifest(manifest string) string {
	resources, err := Resources(manifest)
	if err != nil {
		return "Manifest unavailable: invalid YAML"
	}
	var out []string
	for _, r := range resources {
		out = append(out, "# Source: "+r.Source+"\n"+YAML(r.Object, true))
	}
	return strings.Join(out, "---\n")
}
func Redact(value any) any {
	switch v := value.(type) {
	case map[string]any:
		result := map[string]any{}
		kind, _ := v["kind"].(string)
		for key, child := range v {
			normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "", ".", "").Replace(key))
			sensitive := strings.Contains(normalized, "password") || strings.Contains(normalized, "secret") || strings.Contains(normalized, "token") || strings.Contains(normalized, "credential") || strings.Contains(normalized, "privatekey") || normalized == "apikey" || normalized == "authorization" || normalized == "clientkey" || normalized == "data" || normalized == "stringdata" || normalized == "binarydata" || normalized == "env" || normalized == "args" || normalized == "command" || normalized == "annotations" || normalized == "managedfields" || normalized == "httpheaders" || normalized == "userdata" || strings.EqualFold(kind, "Secret") && key != "kind" && key != "apiVersion" && key != "metadata"
			if sensitive {
				result[key] = "[redacted]"
			} else {
				result[key] = Redact(child)
			}
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, x := range v {
			result[i] = Redact(x)
		}
		return result
	case []string:
		result := make([]any, len(v))
		for i, x := range v {
			result[i] = SafeText(x)
		}
		return result
	case string:
		return SafeText(v)
	default:
		return v
	}
}
func JSON(value any) string { b, _ := json.Marshal(value); return string(b) }
func YAMLString(text string, redact bool) string {
	var value any
	if err := yaml.Unmarshal([]byte(text), &value); err != nil {
		return "Invalid YAML"
	}
	return YAML(value, redact)
}

// Hidden changes keep a visible marker so a masked comparison cannot report
// identical configuration when a credential or opaque data field changed.
func MaskedComparison(before, after any) (string, string) {
	left, right := Redact(before), Redact(after)
	right = markHiddenChanges(before, after, left, right)
	return YAML(left, false), YAML(right, false)
}

func markHiddenChanges(before, after, left, right any) any {
	if text, ok := right.(string); ok && strings.Contains(text, "[redacted]") && JSON(left) == JSON(right) && JSON(before) != JSON(after) {
		return strings.ReplaceAll(text, "[redacted]", "[redacted: changed]")
	}
	switch value := right.(type) {
	case map[string]any:
		b, _ := before.(map[string]any)
		a, _ := after.(map[string]any)
		l, _ := left.(map[string]any)
		for key, child := range value {
			value[key] = markHiddenChanges(b[key], a[key], l[key], child)
		}
	case []any:
		b, _ := before.([]any)
		a, _ := after.([]any)
		l, _ := left.([]any)
		for i := 0; i < min(len(value), len(b), len(a), len(l)); i++ {
			value[i] = markHiddenChanges(b[i], a[i], l[i], value[i])
		}
	}
	return right
}

func ManifestComparison(before, after, namespace string) (string, string) {
	old, oldErr := Resources(before)
	current, err := Resources(after)
	if err != nil || oldErr != nil {
		return RedactedManifest(before), RedactedManifest(after)
	}
	previous := map[string]Resource{}
	for _, r := range old {
		previous[r.ScopedKey(namespace)] = r
	}
	var right []string
	for _, r := range current {
		prior := previous[r.ScopedKey(namespace)]
		_, masked := MaskedComparison(prior.Object, r.Object)
		right = append(right, "# Source: "+r.Source+"\n"+masked)
	}
	return RedactedManifest(before), strings.Join(right, "---\n")
}
