package ai

import (
	"errors"
	"fmt"
	"github.com/simjay/plous/internal/workspace"
	"go.yaml.in/yaml/v3"
	"io"
	"os"
	"path/filepath"
	"time"
)

// RuntimeConfig is read once at operation start. Credentials remain in the
// existing credential store or the named environment variable.
type RuntimeConfig struct {
	MaxSteps       int `json:"max_calls" yaml:"max_calls"`
	MaxTokens      int `json:"max_tokens" yaml:"max_tokens"`
	TimeoutSeconds int `json:"timeout_seconds" yaml:"timeout_seconds"`
	Embedding      struct {
		Connection string `json:"-" yaml:"connection"`
		Model      string `json:"model" yaml:"model"`
	} `json:"embedding" yaml:"embedding"`
}

func DefaultRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{MaxSteps: 128, MaxTokens: 1000000, TimeoutSeconds: 900}
}
func (c RuntimeConfig) Timeout() time.Duration { return time.Duration(c.TimeoutSeconds) * time.Second }

func loadRuntimeConfig(store *workspace.Store, override *RuntimeConfig) (RuntimeConfig, error) {
	c := DefaultRuntimeConfig()
	root, err := store.DataRoot()
	if err != nil {
		return c, err
	}
	path := os.Getenv("PLOUS_AGENT_CONFIG")
	if path == "" {
		path = filepath.Join(root, "agent", "config.yaml")
	}
	f, err := os.Open(path)
	if err == nil {
		defer f.Close()
		d := yaml.NewDecoder(f)
		d.KnownFields(true)
		if err = d.Decode(&c); err != nil {
			return c, fmt.Errorf("agent configuration (supported settings: max_calls, max_tokens, timeout_seconds, embedding): %w", err)
		}
		var extra any
		if err = d.Decode(&extra); err != io.EOF {
			return c, errors.New("agent configuration must contain one YAML document")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return c, err
	} else if os.Getenv("PLOUS_AGENT_CONFIG") != "" {
		return c, fmt.Errorf("agent configuration (supported settings: max_calls, max_tokens, timeout_seconds, embedding): %w", err)
	}
	if override != nil {
		c = *override
	}

	if c.MaxSteps < 1 || c.MaxTokens < 1 || c.TimeoutSeconds < 1 {
		return c, errors.New("turn budgets must be positive")
	}
	if (c.Embedding.Connection == "") != (c.Embedding.Model == "") {
		return c, errors.New("embedding connection and model must be configured together")
	}
	return c, nil
}
