package monitor

func (h *HelmState) WorkflowVisible() bool {
	return h.Workflow != nil && h.Workflow.Active
}

func (h *HelmState) BeginWorkflow() {
	if h.Detail != nil {
		h.SaveInspectorPosition()
	}
	h.Workflow = &HelmWorkflow{Active: true, Preference: h.Preference, Detail: h.Detail, Timeline: h.Timeline, Artifact: h.Artifact, Cursor: h.Cursor, Tab: h.Tab, Subcursor: h.Subcursor}
	h.Workflow.ValuesVariant, h.Workflow.ChartDocument = h.ValuesVariant, h.ChartDocument
	h.Workflow.InspectorPositions = h.InspectorPositions
	h.StopTimelineLoad()
	h.Detail, h.Artifact = nil, nil
	h.Timeline = nil
	h.Find = nil
	h.Subcursor = 0
}
