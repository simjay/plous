package code

import (
	"github.com/simjay/plous/internal/source"
	"path"
	"strings"
)

type Controller struct {
	States      map[string]*CodeState
	Preferences map[string]CodePreference
}

type LegacySelection struct {
	Open, Revision string
}

func (c *Controller) StateFor(workspace, connection string, config source.Config, legacy *LegacySelection) *CodeState {
	key := workspace + "/" + connection + "/" + config.Fingerprint()
	if c.States == nil {
		c.States = map[string]*CodeState{}
	}
	if c.Preferences == nil {
		c.Preferences = map[string]CodePreference{}
	}
	s := c.States[key]
	if s != nil {
		return s
	}
	p := c.Preferences[workspace+"/"+connection]
	// Older view migrations saved empty placeholders before opening Code.
	saved := p != (CodePreference{})
	oldConfig := config
	oldConfig.RepositoryRoot = false
	migrate := config.RepositoryRoot && config.Subdirectory != "" && (p.Fingerprint == oldConfig.Fingerprint() || !saved && legacy != nil)
	if p.Fingerprint == oldConfig.Fingerprint() && config.RepositoryRoot {
		p.Fingerprint = config.Fingerprint()
	}
	if p.Fingerprint != config.Fingerprint() {
		p.Revision, p.Fingerprint = config.Revision, config.Fingerprint()
	}
	if p.Revision == "" {
		p.Revision = config.Revision
	}
	if !saved && legacy != nil {
		p.Selected, p.Revision = "kustomize:"+legacy.Open, legacy.Revision
	}
	if migrate {
		if kind, relative, ok := strings.Cut(p.Selected, ":"); ok {
			p.Selected = kind + ":" + path.Join(config.Subdirectory, relative)
		}
		if p.File != "" {
			p.File = path.Join(config.Subdirectory, p.File)
		}
	}
	// Output and details reopen explicitly; saved source identities are retained.
	p.Section = 0
	s = &CodeState{Key: key, Workspace: workspace, Connection: connection, Config: config, Preference: p, Builds: map[string]*source.Result{}, Failures: map[string]string{}, Inputs: map[string]source.Inputs{}}
	s.InspectorArea = 0
	if config.BrowsePath != "" && config.BrowsePath != "." {
		s.FileRoot = config.BrowsePath
	}
	c.States[key] = s
	return s
}

func (c *Controller) Remember(s *CodeState) {
	if c.Preferences == nil {
		c.Preferences = map[string]CodePreference{}
	}
	c.Preferences[s.Workspace+"/"+s.Connection] = s.Preference
}

func (c *Controller) Stop() {
	for _, s := range c.States {
		s.Stop()
	}
}
