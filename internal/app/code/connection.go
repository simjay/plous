package code

import (
	"github.com/simjay/plous/internal/source"
	"github.com/simjay/plous/internal/workspace"
)

func ConfigForConnection(c *workspace.Connection) source.Config {
	config := source.Config{Path: c.Settings["path"]}
	if c.Settings["source_type"] == "Git repository" {
		config.Repository, config.Path = config.Path, ""
		config.RepositoryRoot = true
		config.BrowsePath = c.Settings["browse_path"]
		if _, set := c.Settings["browse_path"]; !set {
			config.BrowsePath = c.Settings["subdirectory"]
		}
		config.Revision, config.Subdirectory = c.Settings["revision"], c.Settings["subdirectory"]
		if branch := c.Settings["branch"]; branch != "" {
			config.Revision = "refs/heads/" + branch
		}
	}
	return config
}
