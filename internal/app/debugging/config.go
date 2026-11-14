package debugging

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"time"
)

const toolTimeout = 150 * time.Second

func (c setupClient) launchPrefix() []string {
	return []string{"mcp", "serve", "--agent", string(c.kind)}
}

func (client setupClient) launchArgs(c configuration) []string {
	args := append(client.launchPrefix(), "--data-dir", c.dataDir)
	for _, option := range []struct{ flag, value string }{
		{"--kubeconfig", c.paths}, {"--context", c.context}, {"--namespace", c.namespace},
	} {
		if option.value != "" {
			args = append(args, option.flag, option.value)
		}
	}
	if c.policy.Exec {
		args = append(args, "--allow-exec")
	}
	if c.policy.Image != "" {
		args = append(args, "--diagnostic-image", c.policy.Image)
	}
	return args
}

type serverLaunch struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// Match the generated launcher across config rewrites by the client.
func (c setupClient) manages(launch serverLaunch, executable string) bool {
	prefix := c.launchPrefix()
	if !filepath.IsAbs(launch.Command) || len(launch.Args) < len(prefix) ||
		!slices.Equal(launch.Args[:len(prefix)], prefix) {
		return false
	}
	if launch.Command != executable && filepath.Base(launch.Command) != "plous" &&
		!sameExecutable(launch.Command, executable) {
		return false
	}
	flags := flag.NewFlagSet("installed Plous MCP launch", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var previous configuration
	previous.bindServeFlags(flags)
	return flags.Parse(launch.Args[len(prefix):]) == nil && flags.NArg() == 0 && filepath.IsAbs(previous.dataDir)
}

func (c setupClient) mergeConfig(old []byte, executable string, args []string) ([]byte, error) {
	launch := serverLaunch{Command: executable, Args: args}
	var updated []byte
	var err error
	switch c.format {
	case tomlConfig:
		updated, err = c.mergeTOMLConfig(old, launch)
	case openCodeConfig:
		updated, err = c.mergeOpenCodeConfig(old, launch)
	default:
		updated, err = c.mergeJSONConfig(old, launch)
	}
	if err != nil {
		return nil, fmt.Errorf("%s configuration: %w", c.kind.Label(), err)
	}
	return updated, nil
}

func unmanagedServerError() error {
	return errors.New("plous_debug already exists with a different launch; edit the existing registration in the client")
}
