package app

func (m *model) startTraceAcrossOverlays() {
	s := m.coder()
	s.CaptureTraceReference()
	if s.TraceReference == nil || s.TraceReference.Result.SourceIdentity != s.Inventory.Source.Identity {
		m.status = "Select a field in current rendered output first."
		return
	}
	s.Pending = "trace-all"
}
