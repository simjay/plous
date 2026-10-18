package manifest

import (
	"regexp"
	"strings"
)

var credentialText = regexp.MustCompile(`(?i)((?:password|token|secret|api[_-]?key|authorization|credential)["']?\s*[=:]\s*)(?:"[^"\n]*"|'[^'\n]*'|[^\s,;]+)`)
var authText = regexp.MustCompile(`(?i)\b(Bearer|Basic)\s+[^\s"',;]+`)
var urlCredential = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)[^\s/@]+:[^\s/@]+@`)

func SafeText(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, s)
	s = credentialText.ReplaceAllString(s, `${1}"[redacted]"`)
	s = authText.ReplaceAllString(s, "${1} [redacted]")
	return urlCredential.ReplaceAllString(s, "${1}[redacted]@")
}
