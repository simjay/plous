// Package monitor describes native Kustomize monitoring capabilities.
package monitor

// Supported is false because Kustomize is a source renderer without a live controller.
// Flux Kustomization observations belong to the Flux Monitor interface.
const Supported = false
const UnavailableReason = "Kustomize has no native live controller. Monitor Flux Kustomizations through Flux."
