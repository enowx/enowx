package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Settings → RAG's projects: each project of the host as a card, with its index,
// agreed to before anything is sent. They read a project's status by its folder,
// as the side panel's Index does by its session. A daemon knows a project's files,
// tokens and last index from the runs since it started: only its chunks after a
// restart, and what it doesn't know doesn't show.

// ragProjects are the host's projects' indexes. They belong to the main thread.
type ragProjects struct {
	asked  string // the folders asked about
	status map[string]proto.RAGStatus
	errs   map[string]string
	busy   bool

	details  string         // the project whose details show
	exclude  string         // its Leave out, as typed
	maxFiles string         // its Most files, as typed
	plan     *proto.RAGPlan // asking to agree to send planFor
	planFor  string
	planEx   string // More paths to leave out, as typed
}

// took keeps a project's status as the daemon reports it.
func (r *ragProjects) took(st proto.RAGStatus) {
	if r.status != nil && st.Project != "" {
		r.status[st.Project] = st
	}
}

// loadRAGProjects reads each project's status, once for a list of projects.
func (a *App) loadRAGProjects(st *ragSettingsState) {
	dirs := make([]string, len(a.projects))
	for i, p := range a.projects {
		dirs[i] = p.Path
	}
	key := strings.Join(dirs, "\x00")
	r := &st.projects
	cl := a.client
	if r.asked == key && r.status != nil || cl == nil {
		return
	}
	r.asked, r.status, r.errs = key, map[string]proto.RAGStatus{}, map[string]string{}
	host := st.host
	go func() {
		for _, dir := range dirs {
			got, err := cl.RAGStatus(proto.RAGRef{Project: dir})
			a.onMain(func() {
				if a.ragSet.host != host || r.asked != key {
					return
				}
				if err != nil {
					r.errs[dir] = client.Explain(err)
					return
				}
				r.status[dir] = got
			})
		}
	}()
}

// ragFigures are the page's figures, of what the host knows.
func ragFigures(projects []proto.ProjectInfo, st *ragSettingsState) []widget.Fig {
	on, chunks, files := 0, 0, 0
	var tokens int64
	var last time.Time
	for _, p := range projects {
		s := st.projects.status[p.Path]
		if !s.Settings.On {
			continue
		}
		on++
		chunks += s.Chunks
		files += s.Files
		tokens += s.Tokens
		if s.Indexed.After(last) {
			last = s.Indexed
		}
	}
	unknown := "–"
	filesFig, tokensFig, lastFig := unknown, unknown, unknown
	if files > 0 {
		filesFig = fmt.Sprintf("%d", files)
	}
	if tokens > 0 {
		tokensFig = count(tokens) + " tokens"
	}
	if !last.IsZero() {
		lastFig = ragDay(last, time.Now())
	}
	return []widget.Fig{
		{Label: "Projects indexed", Value: fmt.Sprintf("%d of %d", on, len(projects))},
		{Label: "Chunks", Value: count(chunks)},
		{Label: "Files", Value: filesFig},
		{Label: "Sent to the embedder", Value: tokensFig},
		{Label: "Last indexed", Value: lastFig},
	}
}

// ragDay says the day of t as it reads against now.
func ragDay(t, now time.Time) string {
	t, now = t.Local(), now.Local()
	switch t.Format("2006-01-02") {
	case now.Format("2006-01-02"):
		return "Today"
	case now.AddDate(0, 0, -1).Format("2006-01-02"):
		return "Yesterday"
	}
	return t.Format("2 Jan")
}

// ragProjectCard is a project: its index, or the offer to turn it on.
func (a *App) ragProjectCard(c *ui.Context, st *ragSettingsState, p proto.ProjectInfo) {
	r := &st.projects
	s, read := r.status[p.Path]
	widget.GridCard(c).Children(func() {
		widget.CardTitle(c, nil, p.Name, nil)
		switch {
		case r.errs[p.Path] != "":
			widget.StatusLine(c, theme.Error, false, "Couldn't read it")
			widget.CardNote(c, r.errs[p.Path], 3)
		case !read:
			widget.Spinner(c, 12)
		case !s.Settings.On:
			widget.StatusLine(c, theme.Idle, false, "Off")
			if st.set.Embedder == "" {
				widget.CardNote(c, "Not indexed. Pick an embedder above first.", 0)
			} else {
				widget.CardNote(c, "Not indexed. Nothing is sent until you agree.", 0)
			}
			ui.Spacer(c)
			widget.CardFoot(c, func() {
				b := widget.GhostButton(c, "Turn on…").Disabled(r.busy || st.set.Embedder == "")
				if b.Clicked() {
					a.planRAGProject(st, p.Path)
				}
			})
		default:
			a.ragProjectOn(c, st, p, s)
		}
	})
}

func (a *App) ragProjectOn(c *ui.Context, st *ragSettingsState, p proto.ProjectInfo, s proto.RAGStatus) {
	r := &st.projects
	stats := []widget.Stat{{Value: count(s.Chunks), Label: "chunks"}}
	if s.Files > 0 {
		stats = append(stats, widget.Stat{Value: count(s.Files), Label: "files"})
	}
	if !s.Indexed.IsZero() {
		stats = append(stats, widget.Stat{Value: ragDay(s.Indexed, time.Now()), Label: "indexed"})
	}
	widget.CardStats(c, stats)
	changed := s.Settings.Consent != s.Embedder
	switch {
	case s.Indexing:
		line := "Indexing…"
		if s.Phase != "" {
			line = "Indexing: " + s.Phase + "…"
		}
		widget.StatusLine(c, theme.Working, false, line)
	case s.Error != "":
		widget.StatusLine(c, theme.Error, false, "The last index failed")
		widget.CardNote(c, s.Error, 3)
	case changed:
		widget.StatusLine(c, theme.Warning, false, "The embedder changed to "+s.Embedder)
	default:
		widget.StatusLine(c, theme.Done, false, "Indexed with "+ragEmbedderShort(st.set.Embedder))
	}
	ui.Spacer(c)
	widget.CardFoot(c, func() {
		if widget.GhostButton(c, "Details").Clicked() {
			r.details, r.exclude, r.maxFiles = p.Path, strings.Join(s.Settings.Exclude, ", "), ""
			if s.Settings.MaxFiles > 0 {
				r.maxFiles = strconv.Itoa(s.Settings.MaxFiles)
			}
		}
		if changed && widget.GhostButton(c, "Review…").Disabled(r.busy).Clicked() {
			a.planRAGProject(st, p.Path)
		}
	})
}

// ragEmbedderShort is the embedder's name without its explanation.
func ragEmbedderShort(embedder string) string {
	name, _, _ := strings.Cut(ragEmbedderNames[embedder], " (")
	return name
}

// planRAGProject asks what indexing dir would send, for the user to agree.
func (a *App) planRAGProject(st *ragSettingsState, dir string) {
	cl, r := a.client, &st.projects
	if cl == nil {
		return
	}
	r.busy = true
	go func() {
		plan, err := cl.RAGPlan(proto.RAGRef{Project: dir})
		a.onMain(func() {
			r.busy = false
			if err != nil {
				a.status = "RAG: " + client.Explain(err)
				return
			}
			r.plan, r.planFor, r.planEx = &plan, dir, strings.Join(r.status[dir].Settings.Exclude, ", ")
		})
	}()
}

// setRAGProjectFor keeps a project's settings, then indexes it when asked.
func (a *App) setRAGProjectFor(st *ragSettingsState, dir string, set proto.RAGProject, index bool) {
	cl, r := a.client, &st.projects
	if cl == nil {
		return
	}
	r.busy = true
	go func() {
		out, err := cl.SetRAGProject(proto.RAGProjectParams{Project: dir, Settings: set})
		if err == nil && index {
			out, err = cl.RAGIndex(proto.RAGRef{Project: dir})
		}
		a.onMain(func() {
			r.busy = false
			if err != nil {
				a.status = "RAG: " + client.Explain(err)
			}
			r.took(out)
			if a.ragPanel.dir == dir { // the side panel's, if it shows this project
				a.ragPanel.key = ""
			}
		})
	}()
}

func (a *App) ragProjectDialogs(c *ui.Context, st *ragSettingsState) {
	a.ragConsentSheet(c, st)
	a.ragDetailsSheet(c, st)
}

// ragConsentSheet shows what indexing sends, and where, before it does: nothing
// goes until the user agrees, for this embedder.
func (a *App) ragConsentSheet(c *ui.Context, st *ragSettingsState) {
	r := &st.projects
	if r.plan == nil {
		return
	}
	plan, dir := *r.plan, r.planFor
	open := true
	widget.FormSheet(c, &open, 480, "Send this project to the embedder?", "Nothing is indexed until you agree. Agreeing records consent for this embedder.", func() {
		widget.KV(c, "Project", textfmt.LastPathPart(dir), false)
		widget.KV(c, "Embedder", plan.Embedder+", "+plan.Host, false)
		widget.KV(c, "Files", fmt.Sprintf("%d, about %s tokens", plan.Files, count(plan.Tokens)), false)
		if len(plan.Left) > 0 {
			widget.KV(c, "Never sent", strings.Join(plan.Left, "; "), false)
		}
		widget.Field(c, "More paths to leave out", func() {
			widget.Input(c, &r.planEx, "testdata/, *.lock").FillWidth().Font(theme.MonoFont)
		})
	}, func() {
		if widget.PrimaryButton(c, nil, "Agree and index", "").Clicked() {
			open = false
			set := r.status[dir].Settings
			set.On, set.Consent, set.Exclude = true, plan.Host, splitList(r.planEx)
			if !set.Context && !set.Offer {
				// First turned on: prompts get the parts bearing on them, and
				// decisions are offered as memories (each can be turned off).
				set.Context, set.Offer = true, true
			}
			a.setRAGProjectFor(st, dir, set, true)
		}
	})
	if !open {
		r.plan = nil
	}
}

// ragDetailsSheet is a project's index and what agents do with it.
func (a *App) ragDetailsSheet(c *ui.Context, st *ragSettingsState) {
	r := &st.projects
	dir := r.details
	s, ok := r.status[dir]
	if dir == "" || !ok {
		return
	}
	name := textfmt.LastPathPart(dir)
	open := true
	widget.Sheet(c, &open, 480, func() {
		widget.SheetHead(c, &open, name, "Its index on "+a.host.Name+".")
	}, func() {
		ui.Column(c.Key("facts")).FillWidth().Shrink(0).Children(func() {
			widget.KV(c, "Chunks", count(s.Chunks), false)
			if s.Files > 0 {
				files := count(s.Files)
				if s.Skipped > 0 {
					files += fmt.Sprintf(", %d skipped", s.Skipped)
				}
				widget.KV(c, "Files", files, false)
			}
			if !s.Indexed.IsZero() {
				last := ragDay(s.Indexed, time.Now()) + " " + s.Indexed.Local().Format("15:04")
				widget.KV(c, "Last index", fmt.Sprintf("%s, %d added, %d removed", last, s.Added, s.Removed), false)
			}
			if s.Tokens > 0 {
				widget.KV(c, "Sent to "+ragEmbedderShort(st.set.Embedder), count(s.Tokens)+" tokens", false)
			}
		})
		if s.Error != "" {
			widget.Notice(c, icon.Alert, theme.Error, "The last index failed", s.Error, nil)
		}
		set := s.Settings
		if switchRowKey(c, "ctx", &set.Context, "Add results to a prompt", "At the start of each prompt, the parts that bear on it.") ||
			switchRowKey(c, "offer", &set.Offer, "Offer decisions as memories", "When an answer settles something: “Keep this as a memory of the project?”") {
			s.Settings = set
			r.status[dir] = s
			a.setRAGProjectFor(st, dir, set, false)
		}
		widget.Field(c, "Leave out", func() {
			widget.Input(c, &r.exclude, "testdata/").FillWidth().Font(theme.MonoFont)
		})
		widget.Field(c, "Most files", func() {
			widget.Input(c, &r.maxFiles, "20000").FillWidth()
		})
	}, func() {
		if widget.DangerButton(c, "Turn off…").Disabled(r.busy).Clicked() {
			a.confirm("Turn off project search?", "The index of “"+name+"” on "+a.host.Name+" is deleted and agents here stop finding the project by meaning. Its memories stay.", "Turn off", func() {
				set := r.status[dir].Settings
				set.On, set.Consent = false, ""
				r.details = ""
				a.setRAGProjectFor(st, dir, set, false)
			})
		}
		if widget.GhostButton(c, "Index again").Disabled(r.busy || s.Indexing).Clicked() {
			a.setRAGProjectFor(st, dir, r.detailsForm(s.Settings), true)
		}
		if widget.PrimaryButton(c, nil, "Done", "").Clicked() {
			open = false
			if set := r.detailsForm(s.Settings); !sameRAGLimits(set, s.Settings) {
				a.setRAGProjectFor(st, dir, set, false)
			}
		}
	})
	if !open {
		r.details = ""
	}
}

// detailsForm is set with what the details' fields say.
func (r *ragProjects) detailsForm(set proto.RAGProject) proto.RAGProject {
	set.Exclude = splitList(r.exclude)
	set.MaxFiles, _ = strconv.Atoi(strings.TrimSpace(r.maxFiles))
	return set
}

func sameRAGLimits(a, b proto.RAGProject) bool {
	return a.MaxFiles == b.MaxFiles && strings.Join(a.Exclude, "\x00") == strings.Join(b.Exclude, "\x00")
}
