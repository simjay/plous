package common

import "slices"

type Section string

const (
	Applications    Section = "Applications"
	ApplicationSets Section = "ApplicationSets"
	Projects        Section = "Projects"
	Kustomizations  Section = "Kustomizations"
	HelmReleases    Section = "HelmReleases"
	Sources         Section = "Sources"
)

func Sections(tool Tool) []Section {
	if tool == ArgoCD {
		return []Section{Applications, ApplicationSets, Projects}
	}
	return []Section{Kustomizations, HelmReleases, Sources}
}

func NormalizeSection(tool Tool, section Section) Section {
	sections := Sections(tool)
	if slices.Contains(sections, section) {
		return section
	}
	return sections[0]
}

func SectionFor(ref Reference) Section {
	switch ref.Kind {
	case "Application":
		return Applications
	case "ApplicationSet":
		return ApplicationSets
	case "AppProject":
		return Projects
	case "Kustomization":
		return Kustomizations
	case "HelmRelease":
		return HelmReleases
	default:
		return Sources
	}
}

func FieldValue(item Item, label string) string {
	for _, field := range item.Fields {
		if field.Label == label {
			return field.Value
		}
	}
	return ""
}
