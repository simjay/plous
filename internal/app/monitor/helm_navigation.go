package monitor

func (h *HelmState) CollectionSelectionKey() string {
	keys := h.CollectionKeys()
	if h.Cursor >= 0 && h.Cursor < len(keys) {
		return keys[h.Cursor]
	}
	return ""
}

func (h *HelmState) CollectionKeys() []string {
	var keys []string
	for _, row := range h.ReleaseRows() {
		keys = append(keys, row.Key())
	}
	return keys
}

func (h *HelmState) RestoreCollectionSelection(key string) {
	h.Cursor = 0
	for i, candidate := range h.CollectionKeys() {
		if candidate == key {
			h.Cursor = i
			break
		}
	}
}
