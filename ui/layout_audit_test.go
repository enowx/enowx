package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/notesview"
	"github.com/enowx/enowx/ui/wrapcheck"
)

// The layout audit draws the whole app, each page at a small, a common
// and a large window, and fails on layouts that break: text squeezed to a
// letter a line, text out of the window, a page's content huddled in a
// corner of a wide window. With SHOTS set it keeps a screenshot of each.

var auditSizes = [][2]int{{1200, 720}, {1280, 800}, {1920, 1080}}

// auditPage is a page to draw, set up by open.
type auditPage struct {
	name string
	open func(a *App)
	// fill: the page's content should use the window's width (not a
	// column kept at a readable width, centered).
	fill bool
	// after sets what a host's answers to open would otherwise overwrite.
	after func(h *notesHarness)
}

func auditPages() []auditPage {
	settings := func(section string) auditPage {
		return auditPage{name: "settings-" + strings.ToLower(strings.ReplaceAll(section, " ", "-")), open: func(a *App) {
			a.page, a.settingsSection = pageSettings, section
		}, fill: true}
	}
	pages := []auditPage{
		{name: "sessions", open: func(a *App) { a.page = "" }},
		{name: "notes", open: func(a *App) { a.page = pageNotes }, fill: true},
		{name: "notes-page", open: func(a *App) {
			a.page = pageNotes
			a.notes.SetAssistant(false)
			a.notes.Select(a.notesOptions(), a.noteList.list[0].ID)
		}},
		{name: "notes-page-agent", open: func(a *App) {
			a.page = pageNotes
			a.notes.Select(a.notesOptions(), a.noteList.list[0].ID)
			a.notes.SetAssistant(true)
		}},
		{name: "notes-dock-panel", open: func(a *App) {
			a.page = pageNotes
			a.notes.Select(a.notesOptions(), a.noteList.list[0].ID)
			a.selectSession(a.sessions[0])
			a.page = pageNotes
			a.showTool(panelFiles)
		}},
		{name: "notes-dock-agent-panel", open: func(a *App) {
			a.page = pageNotes
			a.notes.Select(a.notesOptions(), a.noteList.list[0].ID)
			a.notes.SetAssistant(true)
			a.selectSession(a.sessions[0])
			a.page = pageNotes
			a.showTool(panelChanges)
		}, after: func(h *notesHarness) {
			h.settle("the note", func() bool { return h.app.notes.Opened() })
		}},
		{name: "notes-dock-no-session", open: func(a *App) {
			a.page, a.selected, a.state.Panel.Open = pageNotes, "", false
			a.notes.Select(a.notesOptions(), a.noteList.list[0].ID)
		}},
		{name: "hosts", open: func(a *App) { a.page = pageHosts }},
		{name: "usage", open: func(a *App) { a.page = pageUsage }, fill: true},
		{name: "vault", open: func(a *App) { a.page = pageVault }},
		{name: "canvas", open: func(a *App) { a.page = pageCanvas }, fill: true},
		{name: "appearance", open: func(a *App) { a.page = pageAppearance }},
		{name: "credits", open: func(a *App) { a.page = pageCredits }},
	}
	for _, s := range settingsSections() {
		pages = append(pages, settings(s))
	}
	pages = append(pages,
		auditPage{name: "settings-hooks-log", open: func(a *App) { a.page, a.settingsSection = pageSettings, sectionHooks; a.hooksOf().tab = 1 }, fill: true},
		auditPage{name: "settings-decisions-uses", open: func(a *App) { a.page, a.settingsSection = pageSettings, sectionDecide; a.decideOf().tab = 1 }, fill: true},
		auditPage{name: "settings-decisions-log", open: func(a *App) { a.page, a.settingsSection = pageSettings, sectionDecide; a.decideOf().tab = 2 }, fill: true},
	)
	session := func(a *App) {
		a.page, a.state.Panel.Open = "", false
		if len(a.sessions) > 0 {
			a.selectSession(a.sessions[0])
		}
	}
	for _, tool := range []string{panelChanges, panelFiles, panelPorts, panelActivity, panelChecks, panelBrowser, panelTraffic, panelAndroid} {
		pages = append(pages, auditPage{name: "session-panel-" + tool, open: func(a *App) {
			session(a)
			a.openPanel(tool)
		}})
	}
	pages = append(pages, auditPage{name: "session", open: session})
	pages = append(pages, auditToolPages()...)
	for name, tab := range map[string]string{
		"check":   centerCheck + "test",
		"browser": centerBrowser,
		"diff":    centerDiff + "README.md",
		"file":    fileTabPrefix + "README.md",
	} {
		pages = append(pages, auditPage{name: "session-center-" + name, open: func(a *App) {
			session(a)
			a.openPanel(map[string]string{"check": panelChecks, "browser": panelBrowser, "diff": panelChanges, "file": panelFiles}[name])
			a.openCenter(a.sessions[0], tab)
		}})
	}
	pages = append(pages, chatAuditPages()...)
	pages = append(pages,
		auditPage{name: "session-by-project", open: func(a *App) { session(a); a.state.SessionsByProject = true; a.state.RailWide = true }},
		auditPage{name: "session-by-state", open: func(a *App) { session(a); a.state.SessionsByProject = false; a.state.RailWide = true }},
		auditPage{name: "session-folded", open: func(a *App) { session(a); a.state.RailWide = false }},
	)
	project := func(a *App) proto.ProjectInfo { return a.projects[0] }
	note := func(a *App) proto.NoteInfo { return a.noteList.list[0] }
	dialogs := []auditPage{
		{name: "dialog-new-session", open: func(a *App) { a.page = ""; a.openNewSession(project(a)) }},
		{name: "dialog-clone", open: func(a *App) { a.page = ""; a.openClone() }},
		{name: "dialog-add-host", open: func(a *App) { a.page = pageHosts; a.openAddHost() }},
		{name: "dialog-new-vps", open: func(a *App) { a.page = pageHosts; a.openCreateServer() }},
		{name: "dialog-import-notes", open: func(a *App) { a.page = pageNotes; a.openNoteImport(nil) }},
		{name: "dialog-export-notes", open: func(a *App) {
			a.page = pageNotes
			a.openNoteExport(notesview.ExportPick{All: true, Title: "All notes"})
		}},
		{name: "dialog-attach", open: func(a *App) { a.page = pageNotes; a.openAttach(note(a)) }},
		{name: "dialog-links", open: func(a *App) { a.page = ""; a.openLinks(project(a)) }},
		{name: "dialog-copy", open: func(a *App) { a.page = ""; a.openCopy(project(a)) }},
		{name: "dialog-clone-to", open: func(a *App) { a.page = ""; a.openCloneTo(project(a)) }},
		{name: "dialog-project-sync", open: func(a *App) { a.page = ""; a.openProjectSync(project(a)) }},
		{name: "dialog-import-notion", open: func(a *App) {
			a.page = pageNotes
			a.openNoteImport(nil)
			a.noteImport.notionAPI = true
		}},
		{name: "dialog-feature-ask", open: func(a *App) {
			a.page = pageSettings
			a.featAsks = []featureAsk{{req: proto.FeatureRequest{ID: "traffic", Reason: "to mock the payment API while testing the checkout"}, on: "this Mac"}}
		}},
	}
	// Last: it makes the vault, which the vault page above shows locked.
	dialogs = append(dialogs, auditPage{name: "dialog-vault-add-github", open: func(a *App) {
		a.page = pageVault
		if a.vaultOf().data == nil {
			_ = a.vaultCreate("audit password", false)
		}
		a.vaultOf().view.OpenAdd("github")
	}})
	return append(append(append(append(pages, dialogs...), auditCloudPages()...), auditSettingsPages()...), auditOverlayPages()...)
}

// closeAudit closes what the pages opened, for the next.
func closeAudit(a *App) {
	a.newSession.open = false
	a.notes.SetAssistant(false)
	a.state.Panel.Open, a.panelPage = false, ""
	a.clone, a.featAsks, a.acctAsks, a.decisions = nil, nil, nil, nil
	a.addHost.open = false
	a.noteImport, a.noteExport = nil, nil
	a.linksForm, a.attaching, a.copying, a.projectSyncing, a.creating, a.cloningTo = nil, nil, nil, nil, nil, nil
	a.cloudSet, a.ragSet, a.rulesEditing, a.ragOffers, a.cloudSigningIn = nil, ragSettingsState{}, nil, nil, false
	a.projects = a.projects[:min(1, len(a.projects))]
	closeOverlayAudit(a)
}

func TestLayoutAudit(t *testing.T) {
	t.Setenv("ANDROID_HOME", stubAndroidSDK(t))
	h := newNotesHarness(t)
	seedAudit(t, h)
	shots := os.Getenv("SHOTS")
	var problems []string
	for _, p := range auditPages() {
		if !auditWanted(p.name) {
			continue
		}
		for _, size := range auditDrawSizes() {
			w, hgt := size[0], size[1]
			closeAudit(h.app)
			p.open(h.app)
			tt := ui.NewTester(func(c *ui.Context) { h.app.view(c) }, w, hgt)
			h.tt = tt
			for range 6 {
				h.settle("", func() bool { return true })
			}
			if p.after != nil {
				p.after(h)
				for range 4 {
					h.settle("", func() bool { return true })
				}
			}
			name := fmt.Sprintf("%s-%dx%d", p.name, w, hgt)
			if shots != "" {
				shot(t, tt, filepath.Join("audit", name+".png"))
			}
			for _, pr := range layoutProblems(tt, float32(w), float32(hgt), p.fill) {
				problems = append(problems, name+": "+pr)
			}
		}
	}
	if len(problems) > 0 {
		t.Errorf("%d layout problems:\n%s", len(problems), strings.Join(problems, "\n"))
	}
}

// layoutProblems are what is wrong with the frame tt drew in a w×h
// window.
func layoutProblems(tt *ui.Tester, w, h float32, fill bool) []string {
	var out []string
	seen := map[string]bool{}
	for _, text := range tt.Texts() {
		if seen[text] || strings.TrimSpace(text) == "" {
			continue
		}
		seen[text] = true
		r, ok := tt.Find(text)
		if !ok || r.W < 12 || r.H <= 0 || r.W <= 44 && r.H <= 44 {
			continue // an icon button's label or a handle's, not a text
		}
		if wrapcheck.FullWidth(tt, text) > r.W+wrapSlackPoints {
			continue // cut by what scrolls it, as the last card of a strip: it isn't squeezed
		}
		short := text
		if len(short) > 40 {
			short = short[:40] + "…"
		}
		words := strings.Fields(text)
		longest := 0
		for _, wd := range words {
			longest = max(longest, len([]rune(wd)))
		}
		// A word broken over lines: its box is narrower than the word
		// could be at the smallest text, about 5 points a letter.
		// Or a text squeezed to a word or two a line.
		lines := r.H / 15
		perLine := float32(len([]rune(text))) / max(lines, 1)
		// Short enough for one line at this width, it can't have wrapped: the
		// label of a taller control, perhaps cut by what scrolls it.
		oneLine := float32(len([]rune(text)))*7 < r.W
		if !oneLine && (longest >= 5 && r.W < float32(longest)*4.5 && r.H > 26 || lines >= 3 && perLine < 9 && len(words) > 1 && r.W < 120) {
			out = append(out, fmt.Sprintf("%q squeezed to %.0f wide, %.0f tall", short, r.W, r.H))
		}
		if r.X+r.W > w+2 && r.X < w {
			out = append(out, fmt.Sprintf("%q runs out of the window (%.0f > %.0f)", short, r.X+r.W, w))
		}
	}
	return append(out, outOfParents(tt)...)
}

// wrapSlackPoints is what rounding leaves between two measures of one box.
const wrapSlackPoints = 1.5

// seedAudit gives the app things to show: notes, a project, sessions.
func seedAudit(t *testing.T, h *notesHarness) {
	t.Helper()
	for _, n := range []string{"Release plan", "Groceries", "A note with a rather long title to see it wrap"} {
		h.cl.SaveNote(proto.Note{NoteInfo: proto.NoteInfo{Title: n}, Body: "# Heading\n\n- [ ] one\n- [x] two\n\nSome text."})
	}
	dir := t.TempDir()
	if _, err := h.cl.AddProject(dir); err != nil {
		t.Fatal(err)
	}
	h.app.setProjects([]proto.ProjectInfo{{Path: dir, Name: "antares"}})
	info, err := h.cl.CreateSession(proto.SessionCreateParams{Agent: "shell", Project: dir, Name: "a shell", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	h.app.upsert(info)
	h.settle("the notes", func() bool { return len(h.app.noteList.list) >= 3 })
}

// auditWanted says whether a page is to be drawn: AUDIT is a regular
// expression (a part of a name, or several with |) over the pages' names.
func auditWanted(name string) bool {
	only := os.Getenv("AUDIT")
	if only == "" {
		return true
	}
	re, err := regexp.Compile(only)
	if err != nil {
		return strings.Contains(name, only)
	}
	return re.MatchString(name)
}

// auditDrawSizes are the audit's sizes, one alone for pictures: they are slow.
func auditDrawSizes() [][2]int {
	return drawSizes(auditSizes)
}

// drawSizes are sizes a test draws at; with SHOTS set one alone, as pictures
// are slow: SHOTS_SIZE=1440x900 draws them at the design's size, to compare.
func drawSizes(sizes [][2]int) [][2]int {
	if os.Getenv("SHOTS") != "" {
		var w, h int
		if _, err := fmt.Sscanf(os.Getenv("SHOTS_SIZE"), "%dx%d", &w, &h); err == nil {
			return [][2]int{{w, h}}
		}
		return [][2]int{{1280, 800}}
	}
	return sizes
}
