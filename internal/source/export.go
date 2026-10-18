package source

import (
	"errors"
	"fmt"
	"os"
	"strings"

	manifest "github.com/simjay/plous/internal/integrations/manifest"
)

// Export writes the captured final output, independent of UI filters and stages.
// O_EXCL protects existing files, including symlinks, from replacement.
func Export(result *Result, name string) (string, error) {
	if result == nil || result.BuildResult == nil || len(result.Resources) == 0 {
		return "", errors.New("render output is empty")
	}
	if strings.TrimSpace(name) == "" {
		return "", errors.New("enter a new file path")
	}
	filePath, err := ResolveDirectory(name, "")
	if err != nil {
		return "", err
	}
	var documents []string
	for _, resource := range result.Resources {
		documents = append(documents, manifest.YAML(resource.Object, true))
	}
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", fmt.Errorf("cannot create export: %w", err)
	}
	_, writeErr := file.WriteString(strings.Join(documents, "---\n"))
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return "", fmt.Errorf("export incomplete; inspect %s: %w", filePath, err)
	}
	return filePath, nil
}
