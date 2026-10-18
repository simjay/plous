package activity

import (
	"encoding/json"
	diffview "github.com/simjay/plous/internal/ui/diff"
	"github.com/simjay/plous/internal/workspace"
)

type ActivityReview struct {
	WorkspaceID, Summary            string
	Before, After, Target, Identity string
	BeforeLabel, AfterLabel         string
	Revision                        uint64
	Open                            bool
	Comparison                      *diffview.Model
}

func WorkspaceSnapshot(w *workspace.Workspace) string {
	if w == nil {
		return ""
	}
	data, _ := json.MarshalIndent(w, "", "  ")
	return string(data) + "\n"
}
