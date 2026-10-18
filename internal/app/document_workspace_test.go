package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	diffview "github.com/simjay/plous/internal/ui/diff"
	docedit "github.com/simjay/plous/internal/ui/editor"
	textview "github.com/simjay/plous/internal/ui/text"
)

func documentModel(t *testing.T) (*model, *monitor.ResourceActionState) {
	t.Helper()
	m := exploreModel(t)
	m.openExploreSelection(0)
	m.explore.DetailValid = true
	m.openResourceActions("")
	a := m.explore.Action
	a.Pending = ""
	a.Stage = "edit"
	a.Operation = "edit"
	a.InitDocument()
	return m, a
}

func TestResourceDocumentOwnsManifestAndValidation(t *testing.T) {
	m, a := documentModel(t)
	if a.Input.Text != "" || a.Document.Text() != kubernetes.YAML(*a.Object, true) {
		t.Fatal("resource document was duplicated into the form field")
	}
	a.Document = nil
	a.Input = textview.NewField("kind: Pod\nmetadata:\n  name: unrelated\n")
	a.Pending = "validate"
	if m.syncResourceAction() != nil || a.Busy || a.Pending != "" || a.Err == "" {
		t.Fatal("validation accepted a form field in place of the document")
	}
}

func TestDocumentInputOwnershipAndFocusPreservesHistory(t *testing.T) {
	m, a := documentModel(t)
	m.draft().Insert("chat draft", true)
	typeText(m, "iibbsscc1133:quit")
	if a.Document.Mode != docedit.Insert || m.leftHidden || m.rightHidden || m.mode != 1 {
		t.Fatal("document keys escaped to application")
	}
	m.Update(tea.PasteMsg{Content: "\n:quit\n界"})
	if !strings.Contains(a.Document.Text(), "ibbsscc1133:quit\n:quit\n界") {
		t.Fatal("paste not literal")
	}
	press(t, m, "esc")
	before := a.Document.Text()
	press(t, m, "alt+right")
	if m.focus != 2 || a.Document.Text() != before {
		t.Fatal("panel travel lost the Normal-mode buffer")
	}
	press(t, m, "alt+left")
	m.execute("help")
	m.closeHelp()
	if m.explore.Action != a || a.Document.Text() != before || m.draft().Text != "chat draft" {
		t.Fatal("Help replaced draft")
	}
	m.execute("undo")
	if a.Document.Text() == before {
		t.Fatal("undo history lost on focus change")
	}
	a.Review = diffview.New(a.Document.Text(), before, a.Document.Target, a.Document.Revision, true)
	typeText(m, "U")
	if a.Document.Text() != before || !a.Review.Stale || m.command != nil || a.Pending != "" {
		t.Fatal("single-key redo failed to restore the draft and invalidate its old review")
	}
	view := ansi.Strip(m.documentBody(140, 30))
	if !strings.Contains(view, "[U] Redo") || strings.Contains(view, ":redo") {
		t.Fatal("editor toolbar did not advertise the single-key redo shortcut", view)
	}
}

func TestEditorHelpReferencePreservesDraftSelectionAndUndo(t *testing.T) {
	m, a := documentModel(t)
	before := a.Document.Text()
	a.Document.Paste("# keep this draft\n")
	typeText(m, "vll")
	text, cursor, revision, selection := a.Document.Text(), a.Document.Cursor, a.Document.Revision, a.Document.SelectedText()
	typeText(m, ":h")
	press(t, m, "enter")
	if m.help == nil || m.help.section != 1 {
		t.Fatal("editor help did not open the view section")
	}
	for _, want := range []string{"a Append after cursor", "o New line below", "gg / G", "d Cut selection", ":q!", ":U / :redo", "Document keys own"} {
		if !strings.Contains(m.panelHelpText(1), want) {
			t.Fatalf("editor reference missing %q", want)
		}
	}
	press(t, m, "end")
	if m.help.scroll[1] == 0 || !strings.Contains(ansi.Strip(m.helpPanels()), ":redo") {
		t.Fatal("full reference cannot be reached by scrolling")
	}
	press(t, m, "tab")
	press(t, m, "esc")
	if m.help != nil || m.explore.Action != a || a.Document.Text() != text || a.Document.Cursor != cursor || a.Document.Revision != revision || a.Document.SelectedText() != selection {
		t.Fatal("Help changed the draft or selection")
	}
	a.Document.Undo()
	if a.Document.Text() != before {
		t.Fatal("Help lost undo history")
	}
}

func TestReviewHelpAndEscapePreserveDraftWithoutApplying(t *testing.T) {
	m, a := documentModel(t)
	a.Document.Paste("# keep this draft\n")
	a.Review = diffview.New("before\n", "after\n", a.Document.Target, a.Document.Revision, true)
	a.Review.BackLabel = "Back to editor"
	a.Stage = "review"
	m.viewCommand("fullscreen")
	text, revision := a.Document.Text(), a.Document.Revision
	typeText(m, ":h")
	press(t, m, "enter")
	if m.help == nil || !strings.Contains(m.panelHelpText(1), "Esc Back to editor") {
		t.Fatal("review help omitted return navigation")
	}
	press(t, m, "esc")
	if a.Stage != "review" || !m.viewExpanded() {
		t.Fatal("closing Help lost expanded review")
	}
	if !strings.Contains(ansi.Strip(m.documentBody(100, 28)), "[Esc] Restore layout") || !strings.Contains(m.documentHint(), "Esc Restore layout") {
		t.Fatal("expanded review does not explain what Esc does next")
	}
	typeText(m, "/after")
	if !strings.Contains(ansi.Strip(m.documentBody(100, 28)), "[Esc] Cancel search") {
		t.Fatal("search did not replace the return hint")
	}
	press(t, m, "esc")
	if !m.viewExpanded() || a.Review.Reader.Searching || a.Review.Reader.Query != "" {
		t.Fatal("Esc did not cancel only the search")
	}
	press(t, m, "esc")
	if a.Stage != "review" || m.viewExpanded() {
		t.Fatal("Esc should restore expanded layout before leaving review")
	}
	if !strings.Contains(ansi.Strip(m.documentBody(100, 28)), "[Esc] Back to editor") {
		t.Fatal("restored review did not expose its return key")
	}
	press(t, m, "esc")
	if a.Stage != "edit" || a.Pending != "" || a.Document.Text() != text || a.Document.Revision != revision {
		t.Fatal("review return changed or applied the draft")
	}
}

func TestReviewMessagesDistinguishUnsubmittedAndUnconfirmedChanges(t *testing.T) {
	m, a := documentModel(t)
	a.Document.Paste("# keep this draft\n")
	a.Review = diffview.New("before\n", "after\n", a.Document.Target, a.Document.Revision, true)
	a.Review.BackLabel = "Back to editor"
	a.Stage, a.ValidatedRevision = "review", a.Document.Revision
	a.Proposed = a.Object.DeepCopy()
	view := ansi.Strip(m.documentBody(100, 28))
	if !strings.Contains(view, "Review only. Nothing saved.") || !strings.Contains(view, ":w saves to Kubernetes.") {
		t.Fatal("unsubmitted review does not explain the decision")
	}
	m.confirmDocument(a.Review.ID)
	if !strings.Contains(ansi.Strip(m.documentBody(100, 28)), "Saving to Kubernetes.") || a.Review.CanConfirm {
		t.Fatal("submitted review still describes unapplied changes")
	}
	a.Pending, a.Busy = "", true
	m.updateResourceActionResult(monitor.ResourceActionResult{Action: a, Serial: a.Serial, Job: "apply", Err: context.DeadlineExceeded})
	view = ansi.Strip(m.documentBody(100, 28))
	if !strings.Contains(view, "Save result unknown") || !strings.Contains(view, "Plous Editor") || strings.Contains(view, "Saved to Kubernetes") || a.Diagnostic != nil {
		t.Fatal("failed confirmation falsely claimed nothing was applied or hid the return path")
	}
	typeText(m, "!")
	if a.Diagnostic == nil {
		t.Fatal("full save error was not accessible")
	}
	press(t, m, "esc")
	m.validateDocument()
	a.Busy, a.Pending, a.RunningJob = true, "", "validate"
	press(t, m, "esc")
	view = ansi.Strip(m.documentBody(100, 28))
	if !strings.Contains(view, "Validation cancelled") || strings.Contains(view, "may already have reached") || !a.Document.Dirty() {
		t.Fatal("cancelled validation implied a write or lost the draft")
	}
}

func TestDocumentSaveControlsAndConfirmedStatus(t *testing.T) {
	for _, entry := range []string{"toolbar", "w", "wq"} {
		t.Run(entry, func(t *testing.T) {
			m, a := documentModel(t)
			a.Document.Paste("# edited\n")
			view := ansi.Strip(m.documentBody(120, 28))
			for _, want := range []string{"Unsaved changes", "[:w] Save", "[:v] Review changes", "[:wq] Save and close"} {
				if !strings.Contains(view, want) {
					t.Fatalf("missing save guidance %q: %s", want, view)
				}
			}
			if entry == "toolbar" {
				press(t, m, "esc")
				if !strings.Contains(ansi.Strip(m.documentBody(120, 28)), "[:w] Save") || !strings.Contains(m.documentHint(), ":w Save") {
					t.Fatal("action focus hid the save command")
				}
				press(t, m, "right")
				m.updateDocumentKey(tea.KeyPressMsg{Code: tea.KeyEnter})
			} else {
				m.execute(entry)
			}
			if a.Pending != "validate" || a.ConfirmedID != "" || a.Document.SavedStatus != "" {
				t.Fatal("Save must check the draft without claiming it was saved")
			}
			proposed := a.Object.DeepCopy()
			proposed.SetAnnotations(map[string]string{"test-edit": "saved"})
			a.Pending = ""
			m.updateResourceActionResult(monitor.ResourceActionResult{Action: a, Serial: a.Serial, Job: "validate", Revision: a.Document.Revision, Object: proposed})
			if a.Pending != "apply" || a.Stage != "edit" || a.Review != nil || !a.SaveRequested || a.SaveRevision != a.Document.Revision || a.Document.SavedStatus != "" {
				t.Fatal("Save did not continue after validation or claimed success too soon")
			}
			a.Pending = ""
			m.updateResourceActionResult(monitor.ResourceActionResult{Action: a, Serial: a.Serial, Job: "apply", Object: proposed})
			if a.Document.Dirty() || a.Document.Text() != kubernetes.YAML(*proposed, true) || a.Document.SavedStatus != "Saved to Kubernetes" {
				t.Fatal("successful save did not update the baseline and report success")
			}
			if entry == "wq" {
				if m.explore.Action != nil || !strings.Contains(m.status, "Saved to Kubernetes") {
					t.Fatal("save and close did not close after success")
				}
				return
			}
			if a.Stage != "edit" || !strings.Contains(ansi.Strip(m.documentBody(120, 28)), "Saved to Kubernetes") {
				t.Fatal("save did not return to the editor with confirmed status")
			}
			a.Document.Controls = false
			press(t, m, "down")
			if !strings.Contains(ansi.Strip(m.documentBody(120, 28)), "Saved to Kubernetes") {
				t.Fatal("navigation erased saved status")
			}
			a.Document.Paste("# next edit\n")
			view = ansi.Strip(m.documentBody(120, 28))
			if !strings.Contains(view, "Unsaved changes") || strings.Contains(view, "Saved to Kubernetes") {
				t.Fatal("a new edit was still presented as saved")
			}
		})
	}
}

func TestOptionalReviewUsesSaveAndNeverNavigationEnter(t *testing.T) {
	for _, entry := range []string{"w", "wq", "button"} {
		t.Run(entry, func(t *testing.T) {
			m, a := documentModel(t)
			a.Document.Paste("# draft\n")
			m.execute("v")
			if a.Pending != "validate" || a.SaveRequested {
				t.Fatal("review acquired write intent")
			}
			proposed := a.Object.DeepCopy()
			proposed.SetAnnotations(map[string]string{"example": "saved"})
			a.Pending = ""
			m.updateResourceActionResult(monitor.ResourceActionResult{Action: a, Serial: a.Serial, Job: "validate", Revision: a.Document.Revision, Object: proposed})
			for _, size := range [][2]int{{28, 10}, {42, 14}, {80, 16}, {120, 28}} {
				view := ansi.Strip(m.documentBody(size[0], size[1]))
				if !strings.Contains(view, "[:w] Save") || strings.Contains(view, ":ap") {
					t.Fatalf("%v hides the shared Save command: %s", size, view)
				}
			}
			m.updateDocumentKey(tea.KeyPressMsg{Code: tea.KeyEnter})
			m.execute("ap")
			m.execute("applychange")
			if a.Pending != "" || a.SaveRequested {
				t.Fatal("reading or a retired command submitted the review")
			}
			if entry == "button" {
				a.Review.Key("a", "a")
				a.Review.Key("right", "")
				m.updateDocumentKey(tea.KeyPressMsg{Code: tea.KeyEnter})
			} else {
				m.execute(entry)
			}
			if a.Pending != "apply" || !a.SaveRequested || a.ConfirmedID != a.Review.ID || a.CloseAfterApply != (entry == "wq") {
				t.Fatal("review Save did not authorize the exact comparison")
			}
		})
	}
}

func TestUnchangedDocumentSaveDoesNotCallKubernetes(t *testing.T) {
	m, a := documentModel(t)
	m.execute("w")
	if a.Pending != "" || a.Busy || a.Document.SavedStatus != "No unsaved changes" {
		t.Fatal("unchanged save started a request")
	}
	m.execute("wq")
	if m.explore.Action != nil {
		t.Fatal("unchanged save-and-close did not close")
	}
}

func TestDirectSaveRequiresCurrentValidatedRevisionAndExplicitIntent(t *testing.T) {
	for _, failure := range []string{"no intent", "edit during validation", "edit before write", "cancel validation", "cancel write"} {
		t.Run(failure, func(t *testing.T) {
			m, a := documentModel(t)
			a.Document.Paste("# draft\n")
			m.execute("wq")
			revision := a.Document.Revision
			a.Pending = ""
			if failure == "edit during validation" {
				a.Document.Paste("# newer\n")
			}
			if failure == "cancel validation" {
				a.Busy, a.RunningJob = true, "validate"
				serial := a.Serial
				m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEscape})
				m.updateResourceActionResult(monitor.ResourceActionResult{Action: a, Serial: serial, Job: "validate", Revision: revision, Object: a.Object.DeepCopy()})
			} else {
				m.updateResourceActionResult(monitor.ResourceActionResult{Action: a, Serial: a.Serial, Job: "validate", Revision: revision, Object: a.Object.DeepCopy()})
			}
			switch failure {
			case "no intent":
				a.SaveRequested = false
			case "edit before write":
				a.Document.Paste("# newer\n")
			case "cancel write":
				a.Busy, a.Pending, a.RunningJob = true, "", "apply"
				serial := a.Serial
				m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEscape})
				m.updateResourceActionResult(monitor.ResourceActionResult{Action: a, Serial: serial, Job: "apply", Object: a.Object.DeepCopy()})
				if a.DocumentErrorTitle != "Save result unknown" {
					t.Fatal("cancelling a write claimed a known outcome")
				}
			}
			if cmd := m.syncResourceAction(); cmd != nil || a.Pending != "" || a.SaveRequested || a.CloseAfterApply || !a.Document.Dirty() || m.explore.Action != a {
				t.Fatal("stale/cancelled save wrote, closed, or discarded the draft")
			}
		})
	}
}

func TestDocumentSaveFrames(t *testing.T) {
	for _, width := range []int{80, 120, 160} {
		m, a := documentModel(t)
		m.width, m.height = width, 30
		a.Document.Paste("# draft\n")
		proposed := a.Object.DeepCopy()
		proposed.SetAnnotations(map[string]string{"example": "saved"})
		for _, stage := range []string{"edit", "review", "saved", "error"} {
			if stage == "review" {
				m.updateResourceActionResult(monitor.ResourceActionResult{Action: a, Serial: a.Serial, Job: "validate", Revision: a.Document.Revision, Object: proposed})
			} else if stage == "saved" {
				m.updateResourceActionResult(monitor.ResourceActionResult{Action: a, Serial: a.Serial, Job: "apply", Object: proposed})
			} else if stage == "error" {
				a.Document.Paste("# unsaved draft\n")
				a.FailDocument("Save failed", "The resource changed since it was opened. Your draft is kept.")
			}
			view := m.render()
			if strings.Count(view, "\n") >= m.height {
				t.Fatalf("%d %s frame exceeds window height", width, stage)
			}
			for _, line := range strings.Split(view, "\n") {
				if ansi.StringWidth(line) > m.width {
					t.Fatalf("%d %s frame exceeds window width", width, stage)
				}
			}
			if dir := os.Getenv("PLOUS_REVIEW_DIR"); dir != "" {
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("save-%d-%s.ansi", width, stage)), []byte(view), 0600); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}

func TestDocumentCloseRequiresDiscardAndReviewRequiresCurrentRevision(t *testing.T) {
	m, a := documentModel(t)
	a.Document.Paste("# modified\n")
	m.closeDocument()
	if !a.Discard || m.explore.Action == nil {
		t.Fatal("dirty document silently closed")
	}
	m.updateDocumentKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if a.Discard || m.explore.Action != a {
		t.Fatal("default close did not keep editing")
	}
	a.Review = diffview.New("before\n", "after\n", a.Document.Target, a.Document.Revision, true)
	a.ValidatedRevision = a.Document.Revision
	a.Stage = "review"
	m.updateDocumentKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if a.Pending != "" {
		t.Fatal("navigation Enter applied")
	}
	a.Document.Paste("changed")
	m.confirmDocument(a.Review.ID)
	if a.Pending != "" || !a.Review.Stale {
		t.Fatal("stale document authorized apply")
	}
}

func TestLateValidationCannotReplaceNewDocumentAndErrorsAreScrollable(t *testing.T) {
	m, a := documentModel(t)
	revision := a.Document.Revision
	a.Document.Paste("new text\n")
	m.updateResourceActionResult(monitor.ResourceActionResult{Action: a, Serial: a.Serial, Job: "validate", Revision: revision, Object: a.Object.DeepCopy()})
	if a.Stage != "edit" || a.Proposed != nil {
		t.Fatal("late validation accepted")
	}
	a.ShowDiagnostic(strings.Repeat("error line\n", 100) + "last error")
	m.updateDocumentKey(tea.KeyPressMsg{Code: tea.KeyEnd})
	if !strings.Contains(ansi.Strip(m.documentBody(60, 20)), "last error") {
		t.Fatal("full error not readable")
	}
	m.updateDocumentKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if a.Diagnostic != nil || !strings.HasPrefix(a.Document.Text(), "new text") {
		t.Fatal("diagnostic lost draft")
	}
}

func TestDocumentFramesFitAllPanelSizes(t *testing.T) {
	for _, size := range [][2]int{{80, 16}, {120, 30}, {160, 40}, {200, 50}} {
		m, a := documentModel(t)
		m.width, m.height = size[0], size[1]
		for _, stage := range []string{"edit", "review"} {
			a.Stage = stage
			a.Review = diffview.New("before\n", "after\n", a.Document.Target, 0, true)
			for _, help := range []bool{false, true} {
				if help {
					m.execute("help")
					m.updateHelp("end")
				}
				view := m.render()
				if len(strings.Split(view, "\n")) > m.height {
					t.Fatalf("%v height overflow", size)
				}
				for _, line := range strings.Split(view, "\n") {
					if ansi.StringWidth(line) > m.width {
						t.Fatalf("%v width overflow", size)
					}
				}
				m.closeHelp()
			}
		}
	}
}

func TestExecLaunchRequiresExplicitStartAndKeepsCancelledFailure(t *testing.T) {
	m, a := documentModel(t)
	a.Stage = "execcommand"
	a.Pod = a.Object.DeepCopy()
	a.Input = textview.NewField("/bin/sh")
	a.ExecChoice = 1
	m.updateExecLaunch(tea.KeyPressMsg{Code: tea.KeyEnter})
	if a.Input.Text != "/bin/bash" || m.explore.Terminal != nil || a.ExecChoice != 3 {
		t.Fatal("shell selection ran command")
	}
	a.ExecChoice = 2
	m.updateExecLaunch(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !a.ExecEditing {
		t.Fatal("custom field not editable")
	}
	m.updateExecLaunch(tea.KeyPressMsg{Code: tea.KeyEscape})
	if a.ExecEditing || a.Stage != "execcommand" {
		t.Fatal("field Esc lost launcher")
	}
	for _, size := range [][2]int{{80, 16}, {120, 30}, {160, 40}} {
		m.width, m.height = size[0], size[1]
		a.ExecChoice = 3
		view := ansi.Strip(m.render())
		for _, want := range []string{"Custom command", "Command", "/bin/bash", "› [Enter] Start", "[ Cancel ]"} {
			if !strings.Contains(view, want) {
				t.Fatalf("%v launcher hid %q: %s", size, want, view)
			}
		}
		if len(strings.Split(view, "\n")) > size[1] {
			t.Fatalf("%v launcher height overflow", size)
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("%v launcher width overflow", size)
			}
		}
	}
}

func TestTerminalReviewSelectionAllowsPanelNavigationAndCommands(t *testing.T) {
	m := exploreModel(t)
	tm := reviewTerminal(t, 40, 10)
	m.explore.Terminal = tm
	writeTerminalLines(t, tm, 0, 20)
	tm.BeginReview()
	tm.Review.Viewer.Reader.Key("v", "")
	press(t, m, "alt+right")
	if m.focus != 2 || !tm.Review.Viewer.Reader.Selecting {
		t.Fatal("panel navigation lost or trapped terminal selection")
	}
	press(t, m, "alt+left")
	typeText(m, ":h")
	press(t, m, "enter")
	if m.help == nil || !tm.Review.Viewer.Reader.Selecting {
		t.Fatal("Help lost terminal review selection")
	}
	m.closeHelp()
	if m.focus != 1 || m.explore.Terminal != tm {
		t.Fatal("Help did not restore terminal review")
	}
}
