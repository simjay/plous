package monitor

type HelmAction string

const (
	HelmOpenDocument                HelmAction = "open-document"
	HelmPreviewChartChanges         HelmAction = "preview-chart-changes"
	HelmPreviewRollback             HelmAction = "preview-rollback"
	HelmInspectRevisionChart        HelmAction = "inspect-revision-chart"
	HelmReviewRecordedChanges       HelmAction = "review-recorded-changes"
	HelmRefreshHistory              HelmAction = "refresh-history"
	HelmRevealSection               HelmAction = "reveal-section"
	HelmRefreshRelease              HelmAction = "refresh-release"
	HelmReviewResourceDifferences   HelmAction = "review-resource-differences"
	HelmRevealResourceDifferences   HelmAction = "reveal-resource-differences"
	HelmReviewValues                HelmAction = "review-values"
	HelmInspectPreviewSummary       HelmAction = "inspect-preview-summary"
	HelmEditInputs                  HelmAction = "edit-inputs"
	HelmExecuteRollback             HelmAction = "execute-rollback"
	HelmDiscardWorkflow             HelmAction = "discard-workflow"
	HelmResumeWorkflow              HelmAction = "resume-workflow"
	HelmRefreshReleases             HelmAction = "refresh-releases"
	HelmChooseNamespace             HelmAction = "choose-namespace"
	HelmRecordedConfiguration       HelmAction = "recorded-configuration"
	HelmRevealRecordedConfiguration HelmAction = "reveal-recorded-configuration"
	HelmCompareRecordedLive         HelmAction = "compare-recorded-live"
	HelmRevealRecordedLive          HelmAction = "reveal-recorded-live"
)

type HelmMenuItem struct {
	Label  string
	Action HelmAction
}

var helmActionLabels = map[HelmAction]string{
	HelmOpenDocument:                "Open document",
	HelmPreviewChartChanges:         "Preview chart changes",
	HelmPreviewRollback:             "Preview rollback",
	HelmInspectRevisionChart:        "Inspect revision chart",
	HelmReviewRecordedChanges:       "Review recorded changes",
	HelmRefreshHistory:              "Refresh history",
	HelmRevealSection:               "Reveal this section",
	HelmRefreshRelease:              "Refresh release",
	HelmReviewResourceDifferences:   "Review resource differences",
	HelmRevealResourceDifferences:   "Reveal resource differences",
	HelmReviewValues:                "Review values",
	HelmInspectPreviewSummary:       "Inspect preview summary",
	HelmEditInputs:                  "Edit inputs",
	HelmExecuteRollback:             "Execute rollback",
	HelmDiscardWorkflow:             "Discard unfinished action",
	HelmResumeWorkflow:              "Resume unfinished action",
	HelmRefreshReleases:             "Refresh releases",
	HelmChooseNamespace:             "Choose namespace",
	HelmRecordedConfiguration:       "Recorded configuration",
	HelmRevealRecordedConfiguration: "Reveal recorded configuration",
	HelmCompareRecordedLive:         "Compare recorded and live",
	HelmRevealRecordedLive:          "Reveal recorded/live comparison",
}

func (action HelmAction) Item() HelmMenuItem {
	return HelmMenuItem{Action: action, Label: helmActionLabels[action]}
}

func HelmMenuItems(labels []string) []HelmMenuItem {
	items := make([]HelmMenuItem, len(labels))
	for i, label := range labels {
		items[i].Label = label
	}
	return items
}

func (h *HelmState) ActionItems() []HelmMenuItem {
	var actions []HelmAction
	switch {
	case h.WorkflowVisible() && h.Plan == nil:
		actions = []HelmAction{HelmDiscardWorkflow}
	case h.Timeline != nil && h.Detail == nil:
		actions = []HelmAction{HelmInspectRevisionChart, HelmReviewRecordedChanges}
		if revision := h.Timeline.Selected(); revision > 0 && revision < h.LatestRevision() {
			actions = append(actions, HelmPreviewRollback)
		}
		actions = append(actions, HelmRefreshHistory)
	case h.Detail != nil:
		actions = nil
		switch h.Tab {
		case 5:
			actions = []HelmAction{HelmCompareRecordedLive, HelmRevealRecordedConfiguration, HelmRevealRecordedLive}
		case 2:
			actions = []HelmAction{HelmOpenDocument, HelmRevealSection}
		case 1:
			actions = []HelmAction{HelmOpenDocument}
			if h.ChartDocument == 1 || h.ChartDocument == 2 {
				actions = append(actions, HelmRevealSection)
			}
		}
		actions = append(actions, HelmPreviewChartChanges)
		if h.Detail.Version < h.LatestRevision() {
			actions = append(actions, HelmPreviewRollback)
		}
		actions = append(actions, HelmRefreshRelease)
	case h.Plan != nil && h.WorkflowVisible():
		actions = []HelmAction{HelmReviewResourceDifferences, HelmReviewValues, HelmEditInputs}
		if h.Plan.Operation == "rollback" {
			actions = append(actions, HelmExecuteRollback)
		}
		actions = append(actions, HelmDiscardWorkflow, HelmInspectPreviewSummary)
		actions = append(actions, HelmRevealResourceDifferences)
	default:
		actions = []HelmAction{HelmRefreshReleases, HelmChooseNamespace}
	}
	if h.Workflow != nil && !h.WorkflowVisible() {
		actions = append(actions, HelmResumeWorkflow, HelmDiscardWorkflow)
	}
	items := make([]HelmMenuItem, 0, len(actions))
	for _, action := range actions {
		items = append(items, action.Item())
	}
	return items
}
