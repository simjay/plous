package app

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/diffview"
	docedit "github.com/simjay/plous/internal/editor"
	"github.com/simjay/plous/internal/kubernetes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func documentModel(t *testing.T) (*model, *resourceActionState) {
	t.Helper()
	m := exploreModel(t)
	m.openExploreSelection(0)
	m.explore.detailValid = true
	m.openResourceActions("")
	a := m.explore.action
	a.pending = ""
	a.stage = "edit"
	a.operation = "edit"
	a.initDocument()
	return m, a
}

func TestResourceDocumentOwnsManifestAndValidation(t *testing.T) {
	m, a := documentModel(t)
	if a.input.text != "" || a.document.Text() != kubernetes.YAML(*a.object, true) {
		t.Fatal("resource document was duplicated into the form field")
	}
	a.document = nil
	a.input = newEditor("kind: Pod\nmetadata:\n  name: unrelated\n")
	a.pending = "validate"
	if m.syncResourceAction() != nil || a.busy || a.pending != "" || a.err == "" {
		t.Fatal("validation accepted a form field in place of the document")
	}
}
func TestDocumentInputOwnershipAndFocusPreservesHistory(t *testing.T) {
	m, a := documentModel(t)
	m.draft().insert("chat draft", true)
	typeText(m, "iibbsscc1133:quit")
	if a.document.Mode != docedit.Insert || m.leftHidden || m.rightHidden || m.mode != 1 {
		t.Fatal("document keys escaped to application")
	}
	m.Update(tea.PasteMsg{Content: "\n:quit\n界"})
	if !strings.Contains(a.document.Text(), "ibbsscc1133:quit\n:quit\n界") {
		t.Fatal("paste not literal")
	}
	press(t, m, "esc")
	before := a.document.Text()
	press(t, m, "tab")
	if m.focus != 2 || a.document.Text() != before {
		t.Fatal("normal Tab lost buffer")
	}
	press(t, m, "shift+tab")
	m.execute("help")
	m.closeHelp()
	if m.explore.action != a || a.document.Text() != before || m.draft().text != "chat draft" {
		t.Fatal("Help replaced draft")
	}
	m.execute("undo")
	if a.document.Text() == before {
		t.Fatal("undo history lost on focus change")
	}
	a.review = diffview.New(a.document.Text(), before, a.document.Target, a.document.Revision, true)
	typeText(m, "U")
	if a.document.Text() != before || !a.review.Stale || m.command != nil || a.pending != "" {
		t.Fatal("single-key redo failed to restore the draft and invalidate its old review")
	}
	view := ansi.Strip(m.documentBody(140, 30))
	if !strings.Contains(view, "U Redo") || strings.Contains(view, ":redo") {
		t.Fatal("editor toolbar did not advertise the single-key redo shortcut", view)
	}
}

func TestEditorHelpReferencePreservesDraftSelectionAndUndo(t *testing.T) {
	m, a := documentModel(t)
	before := a.document.Text()
	a.document.Paste("# keep this draft\n")
	typeText(m, "vll")
	text, cursor, revision, selection := a.document.Text(), a.document.Cursor, a.document.Revision, a.document.SelectedText()
	typeText(m, ":h")
	press(t, m, "enter")
	if m.help == nil || m.help.section != 1 {
		t.Fatal("editor help did not open the view section")
	}
	for _, want := range []string{"a  Append after cursor", "o  New line below", "gg / G", "dd  Cut whole line", ":q!", "U (Shift+U) / :U / :redo", "No Vim counts"} {
		if !strings.Contains(m.panelHelpText(1), want) {
			t.Fatalf("editor reference missing %q", want)
		}
	}
	press(t, m, "end")
	if m.help.scroll[1] == 0 || !strings.Contains(ansi.Strip(m.helpPanels()), "arbitrary operators") {
		t.Fatal("full reference cannot be reached by scrolling")
	}
	press(t, m, "tab")
	press(t, m, "esc")
	if m.help != nil || m.explore.action != a || a.document.Text() != text || a.document.Cursor != cursor || a.document.Revision != revision || a.document.SelectedText() != selection {
		t.Fatal("Help changed the draft or selection")
	}
	a.document.Undo()
	if a.document.Text() != before {
		t.Fatal("Help lost undo history")
	}
}

func TestReviewHelpAndEscapePreserveDraftWithoutApplying(t *testing.T) {
	m, a := documentModel(t)
	a.document.Paste("# keep this draft\n")
	a.review = diffview.New("before\n", "after\n", a.document.Target, a.document.Revision, true)
	a.review.BackLabel = "Back to editor"
	a.stage = "review"
	m.viewCommand("zoom")
	text, revision := a.document.Text(), a.document.Revision
	typeText(m, ":h")
	press(t, m, "enter")
	if m.help == nil || !strings.Contains(m.panelHelpText(1), "Esc Back to editor") {
		t.Fatal("review help omitted return navigation")
	}
	press(t, m, "esc")
	if a.stage != "review" || !m.viewExpanded() {
		t.Fatal("closing Help lost expanded review")
	}
	if !strings.Contains(ansi.Strip(m.documentBody(100, 28)), "Esc Restore layout") || !strings.Contains(m.documentHint(), "Esc Restore layout") {
		t.Fatal("expanded review does not explain what Esc does next")
	}
	typeText(m, "/after")
	if !strings.Contains(ansi.Strip(m.documentBody(100, 28)), "Esc Cancel search") {
		t.Fatal("search did not replace the return hint")
	}
	press(t, m, "esc")
	if !m.viewExpanded() || a.review.Reader.Searching || a.review.Reader.Query != "" {
		t.Fatal("Esc did not cancel only the search")
	}
	press(t, m, "esc")
	if a.stage != "review" || m.viewExpanded() {
		t.Fatal("Esc should restore expanded layout before leaving review")
	}
	if !strings.Contains(ansi.Strip(m.documentBody(100, 28)), "Esc Back to editor") {
		t.Fatal("restored review did not expose its return key")
	}
	press(t, m, "esc")
	if a.stage != "edit" || a.pending != "" || a.document.Text() != text || a.document.Revision != revision {
		t.Fatal("review return changed or applied the draft")
	}
}

func TestReviewMessagesDistinguishUnsubmittedAndUnconfirmedChanges(t *testing.T) {
	m, a := documentModel(t)
	a.document.Paste("# keep this draft\n")
	a.review = diffview.New("before\n", "after\n", a.document.Target, a.document.Revision, true)
	a.review.BackLabel = "Back to editor"
	a.stage, a.validatedRevision = "review", a.document.Revision
	a.proposed = a.object.DeepCopy()
	view := ansi.Strip(m.documentBody(100, 28))
	if !strings.Contains(view, "Review only. Nothing saved.") || !strings.Contains(view, ":w saves to Kubernetes.") {
		t.Fatal("unsubmitted review does not explain the decision")
	}
	m.confirmDocument(a.review.ID)
	if !strings.Contains(ansi.Strip(m.documentBody(100, 28)), "Saving to Kubernetes.") || a.review.CanConfirm {
		t.Fatal("submitted review still describes unapplied changes")
	}
	a.pending, a.busy = "", true
	m.updateResourceActionResult(resourceActionResult{action: a, serial: a.serial, job: "apply", err: context.DeadlineExceeded})
	view = ansi.Strip(m.documentBody(100, 28))
	if !strings.Contains(view, "Save result unknown") || !strings.Contains(view, "Plous Editor") || strings.Contains(view, "Saved to Kubernetes") || a.diagnostic != nil {
		t.Fatal("failed confirmation falsely claimed nothing was applied or hid the return path")
	}
	typeText(m, "!")
	if a.diagnostic == nil {
		t.Fatal("full save error was not accessible")
	}
	press(t, m, "esc")
	m.validateDocument()
	a.busy, a.pending, a.runningJob = true, "", "validate"
	press(t, m, "esc")
	view = ansi.Strip(m.documentBody(100, 28))
	if !strings.Contains(view, "Validation cancelled") || strings.Contains(view, "may already have reached") || !a.document.Dirty() {
		t.Fatal("cancelled validation implied a write or lost the draft")
	}
}

func TestDocumentSaveControlsAndConfirmedStatus(t *testing.T) {
	for _, entry := range []string{"toolbar", "w", "wq"} {
		t.Run(entry, func(t *testing.T) {
			m, a := documentModel(t)
			a.document.Paste("# edited\n")
			view := ansi.Strip(m.documentBody(120, 28))
			for _, want := range []string{"Unsaved changes", ":w Save", ":v Review changes", ":wq Save and close"} {
				if !strings.Contains(view, want) {
					t.Fatalf("missing save guidance %q: %s", want, view)
				}
			}
			if entry == "toolbar" {
				press(t, m, "esc")
				if !strings.Contains(ansi.Strip(m.documentBody(120, 28)), ":w Save") || !strings.Contains(m.documentHint(), ":w Save") {
					t.Fatal("action focus hid the save command")
				}
				press(t, m, "right")
				m.updateDocumentKey(tea.KeyPressMsg{Code: tea.KeyEnter})
			} else {
				m.execute(entry)
			}
			if a.pending != "validate" || a.confirmedID != "" || a.document.SavedStatus != "" {
				t.Fatal("Save must check the draft without claiming it was saved")
			}
			proposed := a.object.DeepCopy()
			proposed.SetAnnotations(map[string]string{"test-edit": "saved"})
			a.pending = ""
			m.updateResourceActionResult(resourceActionResult{action: a, serial: a.serial, job: "validate", revision: a.document.Revision, object: proposed})
			if a.pending != "apply" || a.stage != "edit" || a.review != nil || !a.saveRequested || a.saveRevision != a.document.Revision || a.document.SavedStatus != "" {
				t.Fatal("Save did not continue after validation or claimed success too soon")
			}
			a.pending = ""
			m.updateResourceActionResult(resourceActionResult{action: a, serial: a.serial, job: "apply", object: proposed})
			if a.document.Dirty() || a.document.Text() != kubernetes.YAML(*proposed, true) || a.document.SavedStatus != "Saved to Kubernetes" {
				t.Fatal("successful save did not update the baseline and report success")
			}
			if entry == "wq" {
				if m.explore.action != nil || !strings.Contains(m.status, "Saved to Kubernetes") {
					t.Fatal("save and close did not close after success")
				}
				return
			}
			if a.stage != "edit" || !strings.Contains(ansi.Strip(m.documentBody(120, 28)), "Saved to Kubernetes") {
				t.Fatal("save did not return to the editor with confirmed status")
			}
			a.document.Controls = false
			press(t, m, "down")
			if !strings.Contains(ansi.Strip(m.documentBody(120, 28)), "Saved to Kubernetes") {
				t.Fatal("navigation erased saved status")
			}
			a.document.Paste("# next edit\n")
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
			a.document.Paste("# draft\n")
			m.execute("v")
			if a.pending != "validate" || a.saveRequested {
				t.Fatal("review acquired write intent")
			}
			proposed := a.object.DeepCopy()
			proposed.SetAnnotations(map[string]string{"example": "saved"})
			a.pending = ""
			m.updateResourceActionResult(resourceActionResult{action: a, serial: a.serial, job: "validate", revision: a.document.Revision, object: proposed})
			for _, size := range [][2]int{{28, 10}, {42, 14}, {80, 16}, {120, 28}} {
				view := ansi.Strip(m.documentBody(size[0], size[1]))
				if !strings.Contains(view, ":w Save") || strings.Contains(view, ":ap") {
					t.Fatalf("%v hides the shared Save command: %s", size, view)
				}
			}
			m.updateDocumentKey(tea.KeyPressMsg{Code: tea.KeyEnter})
			m.execute("ap")
			m.execute("applychange")
			if a.pending != "" || a.saveRequested {
				t.Fatal("reading or a retired command submitted the review")
			}
			if entry == "button" {
				a.review.Key("a", "a")
				a.review.Key("right", "")
				m.updateDocumentKey(tea.KeyPressMsg{Code: tea.KeyEnter})
			} else {
				m.execute(entry)
			}
			if a.pending != "apply" || !a.saveRequested || a.confirmedID != a.review.ID || a.closeAfterApply != (entry == "wq") {
				t.Fatal("review Save did not authorize the exact comparison")
			}
		})
	}
}

func TestUnchangedDocumentSaveDoesNotCallKubernetes(t *testing.T) {
	m, a := documentModel(t)
	m.execute("w")
	if a.pending != "" || a.busy || a.document.SavedStatus != "No unsaved changes" {
		t.Fatal("unchanged save started a request")
	}
	m.execute("wq")
	if m.explore.action != nil {
		t.Fatal("unchanged save-and-close did not close")
	}
}

func TestDirectSaveRequiresCurrentValidatedRevisionAndExplicitIntent(t *testing.T) {
	for _, failure := range []string{"no intent", "edit during validation", "edit before write", "cancel validation", "cancel write"} {
		t.Run(failure, func(t *testing.T) {
			m, a := documentModel(t)
			a.document.Paste("# draft\n")
			m.execute("wq")
			revision := a.document.Revision
			a.pending = ""
			if failure == "edit during validation" {
				a.document.Paste("# newer\n")
			}
			if failure == "cancel validation" {
				a.busy, a.runningJob = true, "validate"
				serial := a.serial
				m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEscape})
				m.updateResourceActionResult(resourceActionResult{action: a, serial: serial, job: "validate", revision: revision, object: a.object.DeepCopy()})
			} else {
				m.updateResourceActionResult(resourceActionResult{action: a, serial: a.serial, job: "validate", revision: revision, object: a.object.DeepCopy()})
			}
			switch failure {
			case "no intent":
				a.saveRequested = false
			case "edit before write":
				a.document.Paste("# newer\n")
			case "cancel write":
				a.busy, a.pending, a.runningJob = true, "", "apply"
				serial := a.serial
				m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEscape})
				m.updateResourceActionResult(resourceActionResult{action: a, serial: serial, job: "apply", object: a.object.DeepCopy()})
				if a.documentErrorTitle != "Save result unknown" {
					t.Fatal("cancelling a write claimed a known outcome")
				}
			}
			if cmd := m.syncResourceAction(); cmd != nil || a.pending != "" || a.saveRequested || a.closeAfterApply || !a.document.Dirty() || m.explore.action != a {
				t.Fatal("stale/cancelled save wrote, closed, or discarded the draft")
			}
		})
	}
}

func TestDocumentSaveFrames(t *testing.T) {
	for _, width := range []int{80, 120, 160} {
		m, a := documentModel(t)
		m.width, m.height = width, 30
		a.document.Paste("# draft\n")
		proposed := a.object.DeepCopy()
		proposed.SetAnnotations(map[string]string{"example": "saved"})
		for _, stage := range []string{"edit", "review", "saved", "error"} {
			if stage == "review" {
				m.updateResourceActionResult(resourceActionResult{action: a, serial: a.serial, job: "validate", revision: a.document.Revision, object: proposed})
			} else if stage == "saved" {
				m.updateResourceActionResult(resourceActionResult{action: a, serial: a.serial, job: "apply", object: proposed})
			} else if stage == "error" {
				a.document.Paste("# unsaved draft\n")
				a.failDocument("Save failed", "The resource changed since it was opened. Your draft is kept.")
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
	a.document.Paste("# modified\n")
	m.closeDocument()
	if !a.discard || m.explore.action == nil {
		t.Fatal("dirty document silently closed")
	}
	m.updateDocumentKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if a.discard || m.explore.action != a {
		t.Fatal("default close did not keep editing")
	}
	a.review = diffview.New("before\n", "after\n", a.document.Target, a.document.Revision, true)
	a.validatedRevision = a.document.Revision
	a.stage = "review"
	m.updateDocumentKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if a.pending != "" {
		t.Fatal("navigation Enter applied")
	}
	a.document.Paste("changed")
	m.confirmDocument(a.review.ID)
	if a.pending != "" || !a.review.Stale {
		t.Fatal("stale document authorized apply")
	}
}
func TestLateValidationCannotReplaceNewDocumentAndErrorsAreScrollable(t *testing.T) {
	m, a := documentModel(t)
	revision := a.document.Revision
	a.document.Paste("new text\n")
	m.updateResourceActionResult(resourceActionResult{action: a, serial: a.serial, job: "validate", revision: revision, object: a.object.DeepCopy()})
	if a.stage != "edit" || a.proposed != nil {
		t.Fatal("late validation accepted")
	}
	a.showDiagnostic(strings.Repeat("error line\n", 100) + "last error")
	m.updateDocumentKey(tea.KeyPressMsg{Code: tea.KeyEnd})
	if !strings.Contains(ansi.Strip(m.documentBody(60, 20)), "last error") {
		t.Fatal("full error not readable")
	}
	m.updateDocumentKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if a.diagnostic != nil || !strings.HasPrefix(a.document.Text(), "new text") {
		t.Fatal("diagnostic lost draft")
	}
}
func TestDocumentFramesFitAllPanelSizes(t *testing.T) {
	for _, size := range [][2]int{{80, 16}, {120, 30}, {160, 40}, {200, 50}} {
		m, a := documentModel(t)
		m.width, m.height = size[0], size[1]
		for _, stage := range []string{"edit", "review"} {
			a.stage = stage
			a.review = diffview.New("before\n", "after\n", a.document.Target, 0, true)
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
	a.stage = "execcommand"
	a.pod = a.object.DeepCopy()
	a.input = newEditor("/bin/sh")
	a.execChoice = 1
	m.updateExecLaunch(tea.KeyPressMsg{Code: tea.KeyEnter})
	if a.input.text != "/bin/bash" || m.explore.terminal != nil || a.execChoice != 3 {
		t.Fatal("shell selection ran command")
	}
	a.execChoice = 2
	m.updateExecLaunch(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !a.execEditing {
		t.Fatal("custom field not editable")
	}
	m.updateExecLaunch(tea.KeyPressMsg{Code: tea.KeyEscape})
	if a.execEditing || a.stage != "execcommand" {
		t.Fatal("field Esc lost launcher")
	}
	for _, size := range [][2]int{{80, 16}, {120, 30}, {160, 40}} {
		m.width, m.height = size[0], size[1]
		a.execChoice = 3
		view := ansi.Strip(m.render())
		for _, want := range []string{"Custom command", "Command", "/bin/bash", "[› Enter Start ]", "[ Cancel ]"} {
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
	m.explore.terminal = tm
	writeTerminalLines(t, tm, 0, 20)
	tm.beginReview()
	tm.review.viewer.Reader.Key("v", "")
	press(t, m, "tab")
	if m.focus != 2 || !tm.review.viewer.Reader.Selecting {
		t.Fatal("panel navigation lost or trapped terminal selection")
	}
	press(t, m, "shift+tab")
	typeText(m, ":h")
	press(t, m, "enter")
	if m.help == nil || !tm.review.viewer.Reader.Selecting {
		t.Fatal("Help lost terminal review selection")
	}
	m.closeHelp()
	if m.focus != 1 || m.explore.terminal != tm {
		t.Fatal("Help did not restore terminal review")
	}
}
