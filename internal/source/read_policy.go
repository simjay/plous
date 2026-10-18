package source

import (
	"errors"
	"io"
	"path/filepath"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

var sourceCredentialAssignment = regexp.MustCompile(`(?im)(?:^|[\s,{])["']?(?:password|passwd|token|access[_-]?token|refresh[_-]?token|api[_-]?key|private[_-]?key|client[_-]?key|authorization|credential)["']?\s*(?::|=)\s*\S+`)
var sourceCredentialURL = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://[^\s/@:]+:[^\s/@]+@`)
var sourceSensitiveWord = regexp.MustCompile(`(?i)password|passwd|token|secret|credential|private[\s_-]*key|client[\s_-]*(?:key|certificate)|api[\s_-]*key|access[\s_-]*key|authorization|users\s*:`)

var sourceSecretKind = regexp.MustCompile(`(?im)["']?kind["']?\s*:\s*["']?Secret(?:["'\s,}]|$)`)

// Reject known credential structures while preserving exact permitted bytes.
// This is a conservative read policy, not a claim to recognize every secret a
// user could embed in otherwise ordinary source code.
func checkSourceContent(path, text string) error {
	denied := func() error { return denySourceRead(errors.New("credential-bearing source content is unavailable")) }
	if strings.Contains(text, "PRIVATE KEY-----") || sourceCredentialAssignment.MatchString(text) || sourceCredentialURL.MatchString(text) || sourceSecretKind.MatchString(text) {
		return denied()
	}
	extension := strings.ToLower(filepath.Ext(path))
	structured := extension == ".yaml" || extension == ".yml" || extension == ".json"
	if !structured {
		return nil
	}
	decoder := yaml.NewDecoder(strings.NewReader(text))
	for documents := 0; ; documents++ {
		if documents > 4096 {
			return denySourceRead(errors.New("structured source exceeds the document limit"))
		}
		var document yaml.Node
		err := decoder.Decode(&document)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			// Templates and broken documents are useful diagnostic evidence. When
			// syntax prevents structural inspection, reject any credential marker.
			if sourceSensitiveWord.MatchString(text) || strings.Contains(text, "\\") {
				return denySourceRead(errors.New("unparsed source contains potential credential content"))
			}
			return nil
		}
		pending := []*yaml.Node{&document}
		visited := map[*yaml.Node]bool{}
		for len(pending) > 0 {
			node := pending[len(pending)-1]
			pending = pending[:len(pending)-1]
			if node == nil || visited[node] {
				continue
			}
			visited[node] = true
			if len(visited) > 100000 {
				return denySourceRead(errors.New("structured source exceeds the node limit"))
			}
			if node.Kind == yaml.MappingNode {
				fields := map[string]*yaml.Node{}
				for i := 0; i+1 < len(node.Content); i += 2 {
					key := sourceScalar(node.Content[i])
					value := node.Content[i+1]
					fields[key] = value
					if sensitiveSourceKey(key) && sourceNonempty(value) {
						return denied()
					}
				}
				if strings.EqualFold(sourceScalar(fields["kind"]), "Secret") {
					return denied()
				}
				if strings.EqualFold(sourceScalar(fields["kind"]), "Config") && (fields["users"] != nil || fields["clusters"] != nil) {
					return denied()
				}
				if sensitiveSourceKey(sourceScalar(fields["name"])) && sourceNonempty(fields["value"]) {
					return denied()
				}
			}
			pending = append(pending, node.Content...)
			if node.Alias != nil {
				pending = append(pending, node.Alias)
			}
		}
	}
}

func sourceScalar(node *yaml.Node) string {
	for depth := 0; node != nil && depth < 64; depth++ {
		if node.Kind == yaml.ScalarNode {
			return node.Value
		}
		if node.Kind != yaml.AliasNode {
			return ""
		}
		node = node.Alias
	}
	return ""
}
func sourceNonempty(node *yaml.Node) bool {
	if node == nil {
		return false
	}
	if node.Kind == yaml.ScalarNode {
		return node.Tag != "!!null" && node.Value != ""
	}
	return true
}
func sensitiveSourceKey(key string) bool {
	key = strings.ToLower(strings.NewReplacer("_", "", "-", "", ".", "").Replace(key))
	if strings.Contains(key, "password") || strings.Contains(key, "credential") || strings.Contains(key, "privatekey") {
		return true
	}
	switch key {
	case "passwd", "token", "accesstoken", "refreshtoken", "apikey", "accesskey", "clientkey", "clientkeydata", "clientcertificate", "clientcertificatedata", "authorization", "auth", "auths", "secret":
		return true
	}
	return false
}
