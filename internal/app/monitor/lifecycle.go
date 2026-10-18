package monitor

func (e *ExploreState) Stop() {
	if e != nil {
		e.StopUsage()
		e.StopAction()
		e.StopTerminal()
		e.StopNamespacePicker()
		if e.Stream != nil {
			e.Stream.Cancel()
			e.Stream = nil
		}
		if e.DetailRun != nil {
			e.DetailRun.Cancel()
			e.DetailRun = nil
		}
		if e.Log != nil {
			e.Log.Cancel()
			e.Log = nil
		}
		e.DetailKey = ""
		e.LogKey = ""
	}
}

func (e *ExploreState) StopUsage() {
	if e != nil && e.Usage != nil {
		e.Usage.Cancel()
		e.Usage = nil
	}
}

func (e *ExploreState) StopAction() {
	if e != nil && e.Action != nil {
		e.Action.ClearValueDocument()
		if cancel := e.Action.Cancel; cancel != nil {
			cancel()
		}
		e.Action = nil
	}
}

func (e *ExploreState) StopTerminal() {
	if e != nil && e.Terminal != nil {
		e.Terminal.Close()
		e.Terminal = nil
	}
}

func (e *ExploreState) StopNamespacePicker() {
	if e != nil && e.Namespaces != nil {
		if e.Namespaces.Run != nil {
			e.Namespaces.Run.Cancel()
		}
		e.Namespaces = nil
	}
}

func (h *HelmState) Stop() {
	h.StopNamespacePicker()
	h.StopTimelineLoad()
	if h.Cancel != nil {
		h.Cancel()
	}
}
