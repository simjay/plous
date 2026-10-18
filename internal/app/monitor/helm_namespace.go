package monitor

type HelmNamespaceResult struct {
	State *HelmState
	Run   *NamespaceRequest
	Names []string
	Err   error
}

func (h *HelmState) NamespaceScope() string {
	if h.Preference.Namespace == "*" {
		return ""
	}
	return h.Preference.Namespace
}

func (h *HelmState) StopNamespacePicker() {
	if h.Namespaces != nil && h.Namespaces.Run != nil {
		h.Namespaces.Run.Cancel()
	}
	h.Namespaces = nil
}
