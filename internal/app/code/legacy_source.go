package code

// LegacyPreference preserves the saved fields of earlier Kustomize screens.
type LegacyPreference struct {
	Fingerprint string          `json:"fingerprint,omitempty"`
	Selected    string          `json:"selected,omitempty"`
	Open        string          `json:"open,omitempty"`
	Tab         int             `json:"tab,omitempty"`
	Folded      map[string]bool `json:"folded,omitempty"`
	Descending  bool            `json:"descending,omitempty"`
	Revision    string          `json:"revision,omitempty"`
}
