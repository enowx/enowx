package ui

import (
	"fmt"
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

// The side panel's Index: the selected session's project as RAG knows it:
// turned on (after agreeing to what goes to the embedder), indexed, its
// search, its memories.

const panelIndex = "index"

// ragPanelState is the Index tool's, for one project. It belongs to the
// main thread.
type ragPanelState struct {
	key     string // host and session it was read for
	session string
	dir     string // the project
	loaded  bool
	st      proto.RAGStatus
	err     string
	busy    bool

	query     string
	searched  string
	hits      []proto.RAGHit
	searching bool

	mems     []proto.RAGMemory
	newMem   string
	editing  string // a memory's ID, being changed
	editText string

	plan    *proto.RAGPlan // asking to agree
	exclude string
}

func (a *App) ragPanelOf(s proto.SessionInfo) *ragPanelState {
	if key := a.host.ID + "\x00" + s.ID; a.ragPanel.key != key {
		a.ragPanel = ragPanelState{key: key, session: s.ID}
		a.loadRAGPanel()
	}
	return &a.ragPanel
}

func (a *App) loadRAGPanel() {
	cl, key, ref := a.client, a.ragPanel.key, proto.RAGRef{Session: a.ragPanel.session}
	if cl == nil {
		return
	}
	go func() {
		st, err := cl.RAGStatus(ref)
		mems, _ := cl.RAGMemories(ref)
		a.onMain(func() {
			p := &a.ragPanel
			if p.key != key {
				return
			}
			p.loaded, p.st, p.dir, p.mems, p.err = true, st, st.Project, mems, ""
			p.exclude = strings.Join(st.Settings.Exclude, ", ")
			if err != nil {
				p.err = client.Explain(err)
			}
		})
	}()
}

func (a *App) indexTab(c *ui.Context, s proto.SessionInfo) {
	p := a.ragPanelOf(s)
	ui.Scroll(c).Grow(1).FillWidth().Children(func() {
		ui.Column(c).FillWidth().Padding(14, 16).Gap(14).Children(func() {
			switch {
			case p.err != "":
				widget.Notice(c, icon.Alert, theme.Error, p.err, "", nil)
			case !p.loaded:
				widget.Spinner(c, 14)
			case !p.st.Ready:
				widget.EmptyState(c, icon.Search, "No embedder set", "Pick the embedder a project's text goes to in Settings → RAG, then turn the project on here.", func() {
					if widget.GhostButton(c, "Open Settings").Clicked() {
						a.page, a.settingsSection = pageSettings, sectionRAG
					}
				})
			case !p.st.Settings.On:
				a.ragOff(c, p)
			default:
				a.ragOn(c, s, p)
			}
		})
	})
	a.ragConsentDialog(c, p)
}

// ragOff offers to turn the project on.
func (a *App) ragOff(c *ui.Context, p *ragPanelState) {
	widget.EmptyState(c, icon.Search, "“"+textfmt.LastPathPart(p.dir)+"” isn't indexed", "Indexed, agents here find its code, docs, attached notes and memories by meaning, and may get the parts bearing on a prompt. Its text goes to "+p.st.Embedder+" to be embedded; you see what first.", func() {
		if widget.PrimaryButton(c, icon.Search, "Index this project…", "").Disabled(p.busy).Clicked() {
			a.planRAG(p)
		}
	})
}

func (a *App) planRAG(p *ragPanelState) {
	cl, key := a.client, p.key
	p.busy = true
	go func() {
		plan, err := cl.RAGPlan(proto.RAGRef{Session: p.session})
		a.onMain(func() {
			if a.ragPanel.key != key {
				return
			}
			p.busy = false
			if err != nil {
				p.err = client.Explain(err)
				return
			}
			p.plan = &plan
		})
	}()
}

// ragConsentDialog shows what indexing sends, and where, before it does.
func (a *App) ragConsentDialog(c *ui.Context, p *ragPanelState) {
	if p.plan == nil {
		return
	}
	plan := *p.plan
	open := true
	widget.Dialog(c, &open, 500, 12, func() {
		widget.DialogHeader(c, "Send this project's text to "+plan.Host+"?", "To index “"+textfmt.LastPathPart(p.dir)+"”, enowX sends its text to "+plan.Embedder+" to be embedded, again as it changes. The index stays on "+a.host.Name+".", false)
		widget.Row(c, "Files", "", fmt.Sprintf("%d, about %s tokens", plan.Files, count(plan.Tokens)), nil)
		if len(plan.Left) > 0 {
			widget.Field(c, "Never sent", func() {
				for _, l := range plan.Left {
					ui.Text(c, "· "+l).FontSize(12).TextColor(theme.TextSoft)
				}
			})
		}
		widget.Field(c, "Leave out too (paths or globs, comma-separated)", func() {
			widget.Input(c, &p.exclude, "fixtures/, *.min.js").FillWidth().Font(theme.MonoFont)
		})
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			ui.Spacer(c)
			if widget.GhostButton(c, "Cancel").Clicked() {
				p.plan = nil
			}
			if widget.PrimaryButton(c, icon.Check, "Agree and index", "").Clicked() {
				p.plan = nil
				set := p.st.Settings
				set.On, set.Consent, set.Exclude = true, plan.Host, splitList(p.exclude)
				a.setRAGProject(p, set, true)
			}
		})
	})
	if !open {
		p.plan = nil
	}
}

// count is n for reading: 1,234 or 12.3k.
func count[T int | int64](n T) string {
	if n < 10000 {
		return fmt.Sprint(n)
	}
	return fmt.Sprintf("%.1fk", float64(n)/1000)
}

// setRAGProject keeps the project's settings, then indexes it when asked.
func (a *App) setRAGProject(p *ragPanelState, set proto.RAGProject, index bool) {
	cl, key, dir := a.client, p.key, p.dir
	p.busy = true
	go func() {
		st, err := cl.SetRAGProject(proto.RAGProjectParams{Project: dir, Settings: set})
		if err == nil && index {
			st, err = cl.RAGIndex(proto.RAGRef{Project: dir})
		}
		a.onMain(func() {
			if a.ragPanel.key != key {
				return
			}
			p.busy = false
			if err != nil {
				a.status = "RAG: " + client.Explain(err)
			}
			if st.Project != "" {
				p.st = st
			}
		})
	}()
}

// ragOn is an indexed project: its state, search, memories, settings.
func (a *App) ragOn(c *ui.Context, s proto.SessionInfo, p *ragPanelState) {
	st := p.st
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, textfmt.LastPathPart(p.dir)).FontSize(13.5).SingleLine()
			ui.Text(c, ragStatusLine(st)).FontSize(11.5).TextColor(theme.Muted)
		})
		if st.Indexing {
			widget.Spinner(c, 12)
		} else if widget.GhostButton(c, "Reindex").Disabled(p.busy).Clicked() {
			a.setRAGProject(p, st.Settings, true)
		}
	})
	if st.Error != "" {
		widget.Notice(c, icon.Alert, theme.Error, "The last index failed", st.Error, nil)
	}
	if st.Settings.Consent != st.Embedder {
		widget.Notice(c, icon.Alert, theme.Warning, "The embedder changed to "+st.Embedder, "Agree again to send this project's text there before it is indexed.", func() {
			if widget.GhostButton(c, "Review…").Clicked() {
				a.planRAG(p)
			}
		})
	}
	// Search.
	ui.Row(c).FillWidth().Gap(6).Children(func() {
		widget.SearchField(c, &p.query, "Search by meaning: where are invoices totalled", icon.Search)
	})
	if q := strings.TrimSpace(p.query); q != "" && q != p.searched && !p.searching {
		a.searchRAG(p, q)
	}
	for i, h := range p.hits {
		a.ragHitRow(c.Key(fmt.Sprint("hit", i)), s, h)
	}
	if p.searched != "" && len(p.hits) == 0 && !p.searching {
		ui.Text(c, "Nothing found.").FontSize(12).TextColor(theme.Muted)
	}
	a.ragMemories(c, p)
	widget.Field(c, "Agents here", func() {
		set := st.Settings
		if switchRowKey(c, "ctx", &set.Context, "Give prompts the parts that bear on them", "Up to four results, added as the prompt starts (agents with hooks).") ||
			switchRowKey(c, "offer", &set.Offer, "Offer to keep decisions as memories", "As a session ends, what its agent said last, to edit and keep.") {
			a.setRAGProject(p, set, false)
		}
	})
	ui.Row(c).FillWidth().Children(func() {
		ui.Spacer(c)
		if widget.DangerButton(c, "Turn off and delete the index").Disabled(p.busy).Clicked() {
			a.confirm("Turn off project search?", "The index of “"+textfmt.LastPathPart(p.dir)+"” on "+a.host.Name+" is deleted and agents here stop finding the project by meaning. Its memories stay.", "Turn off", func() {
				set := p.st.Settings
				set.On, set.Consent = false, ""
				a.setRAGProject(p, set, false)
			})
		}
	})
}

func ragStatusLine(st proto.RAGStatus) string {
	switch {
	case st.Indexing && st.Phase != "":
		return "Indexing: " + st.Phase + "…"
	case st.Indexing:
		return "Indexing…"
	}
	line := fmt.Sprintf("%s parts", count(st.Chunks))
	if st.Files > 0 {
		line += fmt.Sprintf(" of %s files", count(st.Files))
	}
	if !st.Indexed.IsZero() {
		line += ", indexed " + st.Indexed.Local().Format("Jan 2 15:04")
	}
	return line + " · " + st.Embedder
}

func (a *App) searchRAG(p *ragPanelState, q string) {
	cl, key := a.client, p.key
	p.searching, p.searched = true, q
	go func() {
		time.Sleep(250 * time.Millisecond) // typing
		hits, err := cl.RAGSearch(proto.RAGSearchParams{RAGRef: proto.RAGRef{Session: p.session}, Query: q, K: 8})
		a.onMain(func() {
			if a.ragPanel.key != key {
				return
			}
			p.searching = false
			if strings.TrimSpace(p.query) != q {
				return // typed on: searched again next frame
			}
			p.hits = hits
			if err != nil {
				a.status = "Search: " + client.Explain(err)
			}
		})
	}()
}

// ragHitRow is a result; a file's opens it.
func (a *App) ragHitRow(c *ui.Context, s proto.SessionInfo, h proto.RAGHit) {
	where := h.File
	if h.Start > 0 {
		where += fmt.Sprintf(":%d", h.Start)
	}
	if h.Symbol != "" {
		where += "  " + h.Symbol
	}
	if where == "" {
		where = h.Source
	}
	b := ui.ButtonBase(c).FillWidth().Padding(8, 10).Radius(theme.RadiusControl).Border(1, theme.Border).Cursor(ui.CursorPointer).Label(where)
	if b.Hovered() {
		b.Background(theme.Hover)
	}
	b.Children(func() {
		ui.Column(c).FillWidth().Gap(4).Children(func() {
			ui.Row(c).FillWidth().Gap(6).Children(func() {
				ui.Text(c, where).FontSize(12).Font(theme.MonoFont).SingleLine().Grow(1)
				widget.Tag(c, h.Source, theme.Muted)
			})
			snip := strings.TrimSpace(h.Snippet)
			if len(snip) > 280 {
				snip = snip[:280] + "…"
			}
			ui.Text(c, snip).FontSize(11.5).TextColor(theme.TextSoft).Font(theme.MonoFont).MaxLines(4)
		})
	})
	if widget.Pressable(b).Clicked() && h.File != "" {
		a.openFileAt(s, h.File, h.Start, h.End)
	}
}

// ragMemories lists the project's memories, to add, change and remove.
func (a *App) ragMemories(c *ui.Context, p *ragPanelState) {
	widget.Field(c, fmt.Sprintf("Memories (%d)", len(p.mems)), func() {
		for _, m := range p.mems {
			ui.Row(c.Key(m.ID)).FillWidth().Gap(6).AlignItems(ui.Start).Children(func() {
				if p.editing == m.ID {
					widget.Input(c, &p.editText, "").Grow(1).AutoFocus()
					if widget.GhostButton(c, "Save").Clicked() {
						a.forgetRAG(p, m.ID, strings.TrimSpace(p.editText))
						p.editing = ""
					}
					return
				}
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, m.Text).FontSize(12.5)
					ui.Text(c, m.By+" · "+m.Time.Local().Format("Jan 2 15:04")).FontSize(11).TextColor(theme.Muted)
				})
				if widget.IconButton(c, icon.Pencil, "Change", false).Clicked() {
					p.editing, p.editText = m.ID, m.Text
				}
				if widget.IconButton(c, icon.Trash, "Delete", false).Clicked() {
					id := m.ID
					a.confirm("Delete this memory?", "Agents won't find “"+m.Text+"” in this project any more.", "Delete", func() { a.forgetRAG(p, id, "") })
				}
			})
		}
		ui.Row(c).FillWidth().Gap(6).Children(func() {
			widget.Input(c, &p.newMem, "A decision or gotcha agents should know").Grow(1)
			if widget.GhostButton(c, "Keep").Disabled(strings.TrimSpace(p.newMem) == "").Clicked() {
				a.rememberRAG(proto.RAGRememberParams{RAGRef: proto.RAGRef{Project: p.dir}, Text: strings.TrimSpace(p.newMem)})
				p.newMem = ""
			}
		})
	})
}

// forgetRAG removes memory id, or changes its text.
func (a *App) forgetRAG(p *ragPanelState, id, text string) {
	cl, dir := a.client, p.dir
	go func() {
		err := cl.RAGForget(proto.RAGForgetParams{RAGRef: proto.RAGRef{Project: dir}, ID: id, Text: text})
		a.onMain(func() {
			if err != nil {
				a.status = "Memory: " + client.Explain(err)
				return
			}
			if a.ragPanel.dir == dir {
				a.loadRAGPanel()
			}
		})
	}()
}
