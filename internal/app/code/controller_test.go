package code

import (
	"testing"

	catalog "github.com/simjay/plous/internal/integrations/catalog"
	"github.com/simjay/plous/internal/source"
)

func TestSavedCodePreferenceTakesPrecedenceOverLegacySelection(t *testing.T) {
	config := source.Config{Repository: "file:///fixture", Revision: "configured"}
	for _, tc := range []struct {
		name       string
		preference CodePreference
		revision   string
	}{
		{"empty selection", CodePreference{Revision: "selected", Fingerprint: config.Fingerprint(), File: "README.md"}, "selected"},
		{"changed connection", CodePreference{Revision: "selected", Fingerprint: "previous configuration", File: "README.md"}, config.Revision},
		{"scanned empty source", CodePreference{Fingerprint: config.Fingerprint()}, config.Revision},
	} {
		t.Run(tc.name, func(t *testing.T) {
			controller := Controller{Preferences: map[string]CodePreference{"workspace/connection": tc.preference}}
			state := controller.StateFor("workspace", "connection", config, &LegacySelection{Open: "old", Revision: "legacy"})
			if state.Preference.Revision != tc.revision || state.Preference.Selected != "" || state.Preference.File != tc.preference.File {
				t.Fatal("legacy selection replaced the saved Code preference", state.Preference)
			}
		})
	}
}

func TestRootBrowsingMigratesLegacyRelativeSelectionsOnce(t *testing.T) {
	old := source.Config{Repository: "file:///fixture", Revision: "main", Subdirectory: "deploy"}
	saved := CodePreference{Selected: "kustomize:prod", File: "prod/kustomization.yaml", Revision: "saved-tag", Fingerprint: old.Fingerprint()}
	config := old
	config.RepositoryRoot = true
	config.BrowsePath = "deploy"
	c := Controller{Preferences: map[string]CodePreference{"w/c": saved}}
	s := c.StateFor("w", "c", config, nil)
	if s.Preference.Selected != "kustomize:deploy/prod" || s.Preference.File != "deploy/prod/kustomization.yaml" || s.Preference.Revision != "saved-tag" || s.FileRoot != "deploy" {
		t.Fatal(s.Preference)
	}
	c.Remember(s)
	c.States = nil
	again := c.StateFor("w", "c", config, nil)
	if again.Preference != s.Preference {
		t.Fatal("migration repeated", again.Preference)
	}
	if saved.Fingerprint != old.Fingerprint() {
		t.Fatal("older preferences changed")
	}
}

func TestCatalogArrowsFollowRowsAcrossPathGroups(t *testing.T) {
	s := &CodeState{View: "code-helm", Inventory: &source.Inventory{Catalog: []catalog.Item{
		{Technology: "helm", Group: "one", Name: "a"}, {Technology: "helm", Group: "two", Name: "b"}, {Technology: "helm", Group: "two", Name: "c"}, {Technology: "helm", Group: "three", Name: "d"},
	}}}
	s.MoveCatalog("down")
	if s.Cursor != 1 {
		t.Fatal("down skipped a short group", s.Cursor)
	}
	s.MoveCatalog("right")
	if s.Cursor != 2 {
		t.Fatal(s.Cursor)
	}
	s.MoveCatalog("down")
	if s.Cursor != 3 {
		t.Fatal(s.Cursor)
	}
	s.MoveCatalog("up")
	if s.Cursor != 2 {
		t.Fatal(s.Cursor)
	}
}
