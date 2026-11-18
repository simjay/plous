package monitor

import (
	"regexp"
	"strings"

	"github.com/simjay/plous/internal/integrations/manifest"
)

type resourceMask uint8

const (
	objectMask resourceMask = iota
	// Selected scalars use the same additional masking as projected manifest fields.
	manifestMask
)

func isSecretObject(object map[string]any) bool {
	kind, _ := object["kind"].(string)
	return strings.EqualFold(kind, "Secret")
}

func (m resourceMask) hidesKey(key string) bool {
	normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "", ".", "").Replace(key))
	switch normalized {
	case "data", "binarydata", "stringdata", "annotations", "managedfields", "env", "command", "args", "userdata",
		"token", "accesstoken", "refreshtoken", "apikey", "accesskey", "clientkey", "clientcertificate", "authorization", "secret":
		return true
	}
	return strings.Contains(normalized, "password") || strings.Contains(normalized, "credential") ||
		strings.Contains(normalized, "privatekey") || m == manifestMask && manifest.SensitiveKey(key)
}

func (e *Projector) maskedFieldText(value string, mask resourceMask) string {
	text := e.sanitizeText(value)
	if mask == manifestMask {
		text = manifest.SafeText(text)
	}
	return text
}

var credentialAssignment = regexp.MustCompile(`(?i)((?:password|passwd|token|secret|api[_-]?key|authorization|credential|client[_-]?key|access[_-]?key)["']?\s*[=:]\s*)(?:"[^"\n]*"|'[^'\n]*'|[^\s,;]+)`)
var authorizationCredential = regexp.MustCompile(`(?i)\b(Bearer|Basic)\s+[^\s"',;]+`)
var jwtCredential = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]+\b`)
var privateKeyMaterial = regexp.MustCompile(`(?s)-----BEGIN [^-]*PRIVATE KEY-----.*?(?:-----END [^-]*PRIVATE KEY-----|$)`)
var credentialURL = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)[^\s/@]+:[^\s/@]+@`)

func (e *Projector) sanitizeText(value string) string {
	for _, credential := range e.credentials {
		if credential != "" {
			value = strings.ReplaceAll(value, credential, "[redacted]")
		}
	}
	value = privateKeyMaterial.ReplaceAllString(value, "[redacted private key]")
	value = authorizationCredential.ReplaceAllString(value, "${1} [redacted]")
	value = jwtCredential.ReplaceAllString(value, "[redacted token]")
	value = credentialAssignment.ReplaceAllString(value, `${1}"[redacted]"`)
	return credentialURL.ReplaceAllString(value, "${1}[redacted]@")
}

func (e *Projector) sanitize(value any) any {
	switch item := value.(type) {
	case map[string]any:
		if isSecretObject(item) {
			return map[string]any{"kind": "Secret", "redacted": true}
		}
		result := make(map[string]any, len(item))
		for key, child := range item {
			if objectMask.hidesKey(key) {
				result[key] = "[redacted]"
				continue
			}
			result[key] = e.sanitize(child)
		}
		return result
	case []any:
		result := make([]any, len(item))
		for i, child := range item {
			result[i] = e.sanitize(child)
		}
		return result
	case string:
		return e.maskedFieldText(item, objectMask)
	default:
		return value
	}
}
