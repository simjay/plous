package monitor

type HelmNamespaceResult struct {
	State *HelmState
	NamespaceResult
}

func (h *HelmState) NamespaceScope() string {
	if h.Preference.Namespace == "*" {
		return ""
	}
	return h.Preference.Namespace
}

func (h *HelmState) StopNamespacePicker() {
	h.Namespaces.Cancel()
	h.Namespaces = nil
}
