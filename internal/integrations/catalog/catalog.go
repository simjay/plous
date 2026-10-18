package catalog

import (
	"bufio"
	"bytes"
	"io"
	"path"
	"slices"
	"strings"

	kyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

// Item identifies a configuration in the captured source, not a live object.
// A document can appear in multiple browsers without becoming a render target.
type Item struct {
	ID, Technology, Group, Name, Kind, File, EntryID string
	Version, Location, Detail                        string
	External                                         bool
}

type Document struct {
	File   string
	Index  int
	Object map[string]any
}

func Documents(files map[string][]byte) []Document {
	var docs []Document
	paths := make([]string, 0, len(files))
	for file := range files {
		paths = append(paths, file)
	}
	slices.Sort(paths)
	for _, file := range paths {
		ext := strings.ToLower(path.Ext(file))
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			continue
		}
		reader := kyaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(files[file])))
		for n := 1; ; n++ {
			data, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}
			var object map[string]any
			if yaml.Unmarshal(data, &object) == nil && len(object) > 0 {
				docs = append(docs, Document{file, n, object})
			}
		}
	}
	return docs
}

func Object(m map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		m, _ = m[key].(map[string]any)
	}
	return m
}

func Text(m map[string]any, keys ...string) string {
	for _, key := range keys[:len(keys)-1] {
		m, _ = m[key].(map[string]any)
	}
	value, _ := m[keys[len(keys)-1]].(string)
	return value
}
