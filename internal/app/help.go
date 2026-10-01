package app

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type helpState struct {
	previousFocus int
	section       int
	scroll        [3]int
}

// Group commands by the panel they affect. Colon commands work globally in
// navigation; other keys apply to the focused panel or active editor.
var panelHelp = [3]string{
	":n / :new\nNew workspace\n\n/  Search tree\n↑↓  Select\n←→  Fold / expand\nEnter  Select / toggle\nHome / End\nFirst / last\nPgUp / PgDn\nJump ten rows",
	":1 / 11  Overview\n:2 / 22  Explore\n:3 / 33  Activity",
	":c / :chat / cc  Type\n:C / :newchat  New chat\n:H / :openchat  History\n:X / :stopchat  Stop\n/m / /model  Choose model\n\nEnter  Send / run command\nShift+Enter  Newline\nCtrl+J  Newline fallback\nEsc  Leave input; keep draft\n\nRead conversation\n↑↓ / j k  Scroll lines\nPgUp / PgDn  Scroll a page, also while typing\nHome  Oldest message\nEnd  Latest; follow new replies\nHome/End still move the caret while typing.\nNew reply text does not move older messages while reading.\n\nHistory: ↑↓ choose · Enter open\n/  Find replies / search history\nFind: Enter finish · n/N matches\nEsc closes search\n//model  Literal /model\n:p / :restoreprompt  Restore last message\n\nChat scope\nThe header shows the workspace, tool, and selected connection.\nMessages include saved scope and the current page identity.\nRead-only tools can retrieve resources, bounded logs, Activity, and earlier workspace chats.\nReads stay on the captured connection. Namespace limits apply to namespaced resources.\nGeneral chat can search its current conversation only.\nNo arbitrary files or shell commands are available.",
}

const globalHelp = ":s / :settings (ss)   :h / :help (hh)   :q / :quit\n" +
	":b / :sidebar (bb)   :i / :aiinteraction (ii)   Toggle panels\n" +
	"Tab / Shift+Tab  Panels outside overlays   Ctrl+C  Save and exit\n" +
	"bb ii ss cc hh · 11 22 33: navigation only."

func (m *model) closeHelp() {
	if m.help != nil {
		m.focus = m.help.previousFocus
		m.help = nil
		m.ensureVisibleFocus()
	}
}

func (m *model) helpDimensions() (width, height int) {
	margin := 2
	if m.height < 22 {
		margin = 1
	}
	return min(164, m.width-4), min(44, m.height-margin*2)
}

func (m *model) helpSizes() (widths [3]int, topHeight, bottomHeight int) {
	width, height := m.helpDimensions()
	widths[0] = min(26, max(18, width/6))
	widths[2] = min(48, max(24, width*3/10))
	widths[1] = width - widths[0] - widths[2]
	bottomHeight = 7
	topHeight = height - bottomHeight
	return
}

func (m *model) helpBodyHeight(panel, topHeight int) int {
	height := topHeight - 3
	if panel == 1 && m.command != nil {
		height -= 3
	}
	return max(1, height)
}

func (m *model) updateHelp(key string) {
	if key == "esc" {
		m.closeHelp()
		m.status = ""
		return
	}
	if key == "left" {
		m.help.section = (m.help.section + 2) % 3
	} else if key == "right" {
		m.help.section = (m.help.section + 1) % 3
	}
	section := m.help.section
	widths, height, _ := m.helpSizes()
	available := m.helpBodyHeight(section, height)
	lines := strings.Split(ansi.Wrap(m.panelHelpText(section), widths[section]-4, ""), "\n")
	last := max(0, len(lines)-available)
	offset := min(m.help.scroll[section], last)
	switch key {
	case "up":
		offset--
	case "down":
		offset++
	case "pgup":
		offset -= available
	case "pgdown":
		offset += available
	case "home":
		offset = 0
	case "end":
		offset = last
	}
	m.help.scroll[section] = max(0, min(last, offset))
}

func helpBox(title, content string, width, height int, focused bool) string {
	color := borderColor
	if focused {
		color = focusedBorder
	}
	headingStyle := overlayTitleStyle.Background(surface)
	if focused {
		headingStyle = headingStyle.Background(highlight).Width(width - 4)
	}
	heading := headingStyle.Render(clipped(title, width-4))
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(color).
		Foreground(foreground).Background(surface).
		Padding(0, 1).Width(width).Height(height).MaxWidth(width).MaxHeight(height).
		Render(heading + "\n" + content)
}

func (m *model) helpPanels() string {
	widths, topHeight, bottomHeight := m.helpSizes()
	titles := [3]string{"1 Workspaces", "2 Views", "3 AI chat"}
	panels := make([]string, 3)
	for i, width := range widths {
		height := m.helpBodyHeight(i, topHeight)
		body := styledTextView(styledHelpText(m.panelHelpText(i)), width-4, height, m.help.scroll[i])
		if i == 1 && m.command != nil {
			body = lipgloss.NewStyle().Width(width-4).Height(height).Render(body) + "\n" + m.commandBox(width-4)
		}
		panels[i] = helpBox(titles[i], body, width, topHeight, m.help.section == i)
	}
	width, _ := m.helpDimensions()
	global := helpBox("4 Global · Help · Esc close", styledHelpText(globalHelp), width, bottomHeight, false)
	return lipgloss.JoinVertical(lipgloss.Left, lipgloss.JoinHorizontal(lipgloss.Top, panels...), global)
}

func (m *model) panelHelpText(panel int) string {
	text := panelHelp[panel]
	if panel == 1 {
		view := *m
		view.help, view.command, view.focus = nil, nil, 1
		text += "\n\n" + view.viewHelp()
	}
	return text
}

const readerNavigationHelp = "↑↓ / j k / PgUp / PgDn  Scroll\nHome / End  First / last\n/ Find · Enter finish · n/N matches\nv Select · y Copy · Esc clear / back"
const readerHelp = readerNavigationHelp + "\n:z / :zoom  Expand / restore"

const editorHelp = `Plous Editor
:h / :help  Open this reference
PgUp / PgDn  Scroll Help
Esc  Return to the same draft

Modes and movement
NORMAL: movement and editing keys
INSERT: type literal text
SELECT: movement extends selection
Arrows / h j k l  Move cursor
w / b / e  Next / previous word / word end
0 / $ or Home / End  Line start / end
gg / G  Document start / end
PgUp / PgDn  Move a page

Enter text
i  Insert at cursor
a  Append after cursor
o  New line below, then Insert
Esc in Insert  Return to Normal
Tab in Insert  Indent two spaces
Enter in Insert  New line with indentation
Backspace / Delete in Insert  Delete before / at cursor
Paste  One literal undoable edit

Select and edit
v / V  Select characters / whole lines
Esc in Select  Return to Normal
x  Delete character or selection
dd  Cut whole line
d in Select  Cut selection
yy in Normal  Copy line
y in Select  Copy selection
p  Paste last text copied/cut in this editor
u / :u / :undo  Undo
U (Shift+U) / :U / :redo  Redo
> / <  Indent / outdent lines

Find
/  Enter search text
Enter  Finish search input
n / N  Next / previous match
Up / Down while typing  Previous / next match
Esc  Clear search before leaving the editor

Save and close
:w  Save to Kubernetes; validation runs automatically
:v / :review  Optional review without saving
Review uses the same :w Save command.
Saved to Kubernetes confirms the server accepted the change.
:wq  Save and close after success
:q  Close; modified text asks before discarding
:q!  Discard draft and close
Esc in Normal  Focus document actions
Left / Right, Enter  Choose / activate action
Esc in actions  Request Close
!  Full details of the last save or validation error
:quit / Ctrl+C  Exit Plous, without applying draft

View and focus
:z / :zoom  Expand / restore
:L / :linenumbers  Toggle line numbers
Tab in Normal/Select, Shift+Tab  Switch panels
Help, chat and review preserve draft and undo history.
Esc leaves search/selection before restoring an expanded view.
In Insert, press Esc before :h or another command.
Document keys own h, b, i and other app prefix letters.
No Vim counts, registers, macros or arbitrary operators.`

func (m *model) viewHelp() string {
	switch {
	case m.form != nil:
		return "Form\n↑↓ Fields · Enter next / activate\n←→ Choose · Space toggle\nEsc Cancel to previous screen\nText and paste stay literal in fields.\nCommands work from action buttons."
	case m.settingsOverlay != nil:
		return "Settings\n↑↓ Select · Enter open\nApplication: AI connections\nCurrent workspace: name and connections\nEsc Return to previous view"
	case m.resourceTerminalVisible():
		text := "Exec\nCtrl+] Review output (release terminal input)\nCtrl+C Save and exit Plous\n" + readerHelp
		if m.explore.terminal.done {
			return text + "\ne Edit command / choose shell\nR Retry same command · x Close"
		}
		return text + "\nr Resume terminal input\nt Interrupt remote process · x Close"
	case m.documentActive():
		a := m.explore.action
		if a.diagnostic != nil {
			return "Error details\nEsc Back to editor; draft and undo history stay intact\nSave result unknown means check Kubernetes before retrying.\n" + readerHelp + "\n\n" + editorHelp
		}
		if a.stage == "review" {
			return "Optional review\nNothing saved. :w saves these changes to Kubernetes.\n:wq Saves and closes after success\nEsc Back to editor; keeps draft\nClear search/selection and restore expansion first.\n" + readerHelp + "\n[ ] Previous / next change\na Actions · ←→ Choose · Enter activate\nEnter while reading never saves.\n\n" + editorHelp
		}
		return editorHelp
	case m.valueDocumentActive():
		return "Value (read only)\n↑↓ / PgUp / PgDn Scroll\n/ Find · Enter finish · n/N matches\nv Select · y Copy selection / line\na Read / actions · ←→ Choose · Enter activate\nCopy value copies the entire value.\n:z / :zoom · :L / :linenumbers\nEsc Clear search / selection, then hide"
	case m.resourceActionVisible():
		if a := m.explore.action; a.stage == "done" && a.progress != nil {
			return "Operation progress\n↑↓ / j k / PgUp / PgDn Scroll\nHome / End First / last page\ng Check status again after a stopped check\nEnter / Esc Return to resource details\nClosing stops checks, not the accepted operation."
		}
		return "Resource action\n↑↓ Select · Enter next / confirm\nEsc Return to previous step\nType values in the focused input.\nChanges require explicit confirmation."
	case m.activityReviewVisible():
		return "Activity comparison (read only)\n" + readerHelp + "\n[ ] Previous / next change\nEsc Return to saved summary"
	case m.yamlDocumentActive():
		text := "YAML (read only)\n↑↓ / j k / PgUp / PgDn Scroll\n/ Find · Enter finish · n/N matches\nz Select · y Copy · E Edit\n:z / :zoom · :L / :linenumbers\nEsc Clear search / selection, then back"
		if m.explore.resource.Key() == "secrets" {
			text += "\nv Reveal / hide Secret values"
		}
		return text
	case m.logWorkspaceActive():
		return "Pod / workload logs\n" + readerNavigationHelp + "\nz Expand / restore · :z / :zoom also work\nSpace Pause / jump live\ng Reload logs (clears and reloads buffer) · o Choose container\np Previous instance (before a restart)\nu Time range (requested history)\nt Timestamps · w Wrap lines (fit long lines)\nNew logs keep collecting while reading.\nWorkloads default to All Pods; lines start with Pod/container.\nPod Logs also offers All application containers.\no filters containers; workloads filter across Pods.\nMembership refreshes every 3s; failures stay isolated.\nUp to 32 sources; omitted sources are reported."
	case m.exploreVisible():
		e := m.explorer()
		if e.picker != "" || e.input != nil {
			return "Explore selection / filter\nType to search or enter a value\n↑↓ Select · Enter apply\nEsc Cancel\nNamespaces: choose Reload namespaces with an empty search; manual entry available"
		}
		if e.detail != nil {
			return "Resource details\nd Describe · y YAML · e Events · r Related\n[ ] Change section · / Find\nEnter Finish find · n/N Matches\nPgUp / PgDn Scroll · Esc Back" + m.resourceActionHelp()
		}
		return "Resource table\n↑↓ / j k Select · Enter Open\n/ Text filter · Backspace Clear text filter\nf Label selector · S Sort\nn Namespaces · g Refresh\n:k / :kinds  Resource picker\n:N / :namespace  Namespace picker\n:K / :connections  Connection picker\n:g / :refresh  Reload\n:pods / :deployments / :services\nEsc Focus sidebar"
	case m.mode == 2:
		if m.detail != "" {
			return "Activity detail\nd / :v / :review  Compare snapshots\n/ Find · Enter finish · n/N matches\nPgUp / PgDn Scroll · Esc Back"
		}
		return "Activity\n/ Filter changes\n↑↓ Select · Enter Details · Esc Sidebar"
	default:
		return "Overview / connection settings\n/ Find · Enter finish · n/N matches\nPgUp / PgDn Scroll · Esc Back / sidebar\nEmpty workspace: Enter Add tool"
	}
}

func (m *model) renderHelpOverlay() string {
	base := *m
	base.help, base.command = nil, nil
	base.focus = m.help.previousFocus
	base.ensureVisibleFocus()
	lines := strings.Split(base.render(), "\n")
	lines[len(lines)-1] = m.footer()
	return centeredOverlay(strings.Join(lines, "\n"), m.helpPanels())
}
