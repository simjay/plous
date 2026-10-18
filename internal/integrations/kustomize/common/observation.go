package common

import (
	manifest "github.com/simjay/plous/internal/integrations/manifest"
)

// Stage contains actual builtin input/output, scoped to its accumulator.
// Resource IDs follow engine objects through renaming without modifying them.
type Stage struct {
	Scope, Kind, Path string
	ConfigurationName string
	External          bool
	Before, After     map[int]manifest.Resource
}

type Observation struct {
	Stages    []Stage
	FinalIDs  map[string]int
	Truncated bool
}
