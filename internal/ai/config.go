// Package ai provides model configuration and Eino provider adapters.
package ai

import (
	"errors"
	"net"
	"net/url"
	"regexp"
	"strings"
)

type Config struct {
	Name          string `json:"name"`
	Provider      string `json:"provider"`
	Endpoint      string `json:"endpoint"`
	Model         string `json:"model,omitempty"`
	KeyEnv        string `json:"key_env"`
	CredentialRef string `json:"credential_ref,omitempty"`
	Instructions  string `json:"instructions"`
}

// Connection removes a request's model selection while preserving its vendor settings.
func (c Config) Connection() Config {
	c.Model = ""
	return c
}

func DefaultConfig() Config {
	return Config{Name: "Assistant", Provider: "openai", KeyEnv: "OPENAI_API_KEY"}
}
func (c Config) BaseURL() string {
	if c.Endpoint != "" {
		return strings.TrimRight(c.Endpoint, "/")
	}
	switch c.Provider {
	case "anthropic":
		return "https://api.anthropic.com"
	}
	return "https://api.openai.com/v1"
}

var credentialID = regexp.MustCompile(`^[a-f0-9]{32}$`)

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Validate checks saved connection settings without contacting a provider.
func (c Config) Validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return errors.New("AI connection name is required")
	}
	switch c.Provider {
	case "openai", "anthropic":
	default:
		return errors.New("choose a supported provider in Settings")
	}
	if c.KeyEnv != "" && !envName.MatchString(c.KeyEnv) {
		return errors.New("credential must be an environment-variable name, not an API key")
	}
	if c.CredentialRef != "" && !credentialID.MatchString(c.CredentialRef) {
		return errors.New("invalid saved credential reference")
	}
	u, err := url.Parse(c.BaseURL())
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("endpoint must be a base URL without credentials, query, or fragment")
	}
	ip := net.ParseIP(u.Hostname())
	local := u.Hostname() == "localhost" || (ip != nil && ip.IsLoopback())
	if u.Scheme != "https" && !(u.Scheme == "http" && local) {
		return errors.New("endpoint must use HTTPS (HTTP is allowed for localhost)")
	}
	return nil
}
func (c Config) Ready(key string) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if err := c.Supported(); err != nil {
		return err
	}
	if (c.CredentialRef != "" || c.KeyEnv != "" || c.Endpoint == "") && key == "" {
		return errors.New("enter an API key in Settings (:s)")
	}
	return nil
}

type Message struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

type Usage struct {
	Input  int64 `json:"input_tokens"`
	Output int64 `json:"output_tokens"`
	Total  int64 `json:"total_tokens"`
}
type Result struct {
	Text   string
	Usage  Usage
	Finish string
}
