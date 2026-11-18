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

type configAdapter interface {
	merge(old []byte, launch serverLaunch, owns func(serverLaunch) bool) ([]byte, error)
}

// Some clients select a config from several files under the locked directory.
type configFileSelector interface {
	selectFile(path string, owns func(serverLaunch) bool) (string, error)
}

func (c setupClient) configFile(path, executable string) (string, error) {
	if selector, ok := c.config.(configFileSelector); ok {
		return selector.selectFile(path, func(previous serverLaunch) bool {
			return c.manages(previous, executable)
		})
	}
	return path, nil
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
	updated, err := c.config.merge(old, launch, func(previous serverLaunch) bool {
		return c.manages(previous, executable)
	})
	if err != nil {
		return nil, fmt.Errorf("%s configuration: %w", c.kind.Label(), err)
	}
	return updated, nil
}

func unmanagedServerError() error {
	return errors.New("plous_debug already exists with a different launch; edit the existing registration in the client")
}
