package code

import (
	"fmt"

	kustomizecommon "github.com/simjay/plous/internal/integrations/kustomize/common"
	"github.com/simjay/plous/internal/source"
)

func (s *CodeState) CapturedStage() *kustomizecommon.Stage {
	b := s.Builds[s.Preference.Selected]
	if b == nil || b.Observation == nil || s.Stage <= 0 || s.Stage > len(b.Observation.Stages) {
		return nil
	}
	return &b.Observation.Stages[s.Stage-1]
}

func (s *CodeState) StageItems() []string {
	items := []string{"Final output"}
	b := s.Builds[s.Preference.Selected]
	if b != nil && b.Observation != nil {
		for n, stage := range b.Observation.Stages {
			items = append(items, fmt.Sprintf("%d · %s · %s", n+1, source.TransformationLabel(stage.Kind), stage.Scope))
		}
	}
	return items
}

func (s *CodeState) SelectFinalResource() {
	if stage := s.CapturedStage(); stage != nil {
		if current, ok := s.CurrentResource(); ok {
			for id, r := range stage.After {
				if r.Key() == current.Key() {
					for key, finalID := range s.Builds[s.Preference.Selected].Observation.FinalIDs {
						if id == finalID {
							s.Preference.Resource = key
							break
						}
					}
					break
				}
			}
		}
	}
	s.Stage = 0
	s.FieldResource = ""
	s.RestoreCursor()
}
