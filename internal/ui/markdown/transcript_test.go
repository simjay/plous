package markdown

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	textview "github.com/simjay/plous/internal/ui/text"
)

func TestTranscriptTablesListsLinksAndCode(t *testing.T) {
	source := "## Check\n\n- Parent\n  - Child\n\n| Release | Image |\n| --- | --- |\n| frontend | `app:v2` |\n| backend | `app:v1` |\n\n[Registry](https://example.com/registry)\n\n```yaml\nspec:\n  replicas: 2\n# | Code | untouched |\n# | --- | --- |\n```"
	view := ansi.Strip(Transcript(source, textview.DefaultTheme()))
	for _, want := range []string{"Check", "Parent", "Child", "Release: frontend", "Image: app:v2", "Release: backend", "Registry", "https://example.com/registry", "  replicas: 2", "# | Code | untouched |"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in %s", want, view)
		}
	}
	for _, raw := range []string{"## Check", "[Registry](", "| Release | Image |"} {
		if strings.Contains(view, raw) {
			t.Fatal("unrendered markup", view)
		}
	}
	if got := Transcript("one\ntwo\nthree", textview.DefaultTheme()); !strings.Contains(ansi.Strip(got), "one\ntwo\nthree") {
		t.Fatal("logical newlines were collapsed", got)
	}
}

func TestTranscriptTablesDoNotInterpretCodeOrEscapedPipes(t *testing.T) {
	for _, source := range []string{"```\n| A | B |\n| --- | --- |\n| x | y |\n```", "    | A | B |\n    | --- | --- |\n    | x | y |"} {
		if transcriptTables(source) != source {
			t.Fatal("code was rewritten")
		}
	}
	source := "Before\n\nA | B\n--- | ---\nx\\|y | z\n\nAfter"
	view := ansi.Strip(Transcript(source, textview.DefaultTheme()))
	for _, want := range []string{"Before", "A: x|y", "B: z", "After"} {
		if !strings.Contains(view, want) {
			t.Fatal("lost table or surrounding text", want, view)
		}
	}
}
