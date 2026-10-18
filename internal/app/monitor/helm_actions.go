package monitor

func (h *HelmState) ActionItems() []string {
	var items []string
	switch {
	case h.WorkflowVisible() && h.Plan == nil:
		items = []string{"Discard unfinished action"}
	case h.Timeline != nil && h.Detail == nil:
		items = []string{"Inspect revision chart", "Review recorded changes"}
		if revision := h.Timeline.Selected(); revision > 0 && revision < h.LatestRevision() {
			items = append(items, "Preview rollback")
		}
		items = append(items, "Refresh history")
	case h.Detail != nil:
		items = nil
		switch h.Tab {
		case 5:
			items = []string{"Compare recorded and live", "Reveal recorded configuration", "Reveal recorded/live comparison"}
		case 2:
			items = []string{"Open document", "Reveal this section"}
		case 1:
			items = []string{"Open document"}
			if h.ChartDocument == 1 || h.ChartDocument == 2 {
				items = append(items, "Reveal this section")
			}
		}
		items = append(items, "Preview chart changes")
		if h.Detail.Version < h.LatestRevision() {
			items = append(items, "Preview rollback")
		}
		items = append(items, "Refresh release")
	case h.Plan != nil && h.WorkflowVisible():
		items = []string{"Review resource differences", "Review values", "Edit inputs"}
		if h.Plan.Operation == "rollback" {
			items = append(items, "Execute "+h.Plan.Operation)
		}
		items = append(items, "Discard unfinished action", "Inspect preview summary")
		items = append(items, "Reveal resource differences")
	default:
		items = []string{"Refresh releases", "Choose namespace"}
	}
	if h.Workflow != nil && !h.WorkflowVisible() {
		items = append(items, "Resume unfinished action", "Discard unfinished action")
	}
	return items
}
