package ui

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Settings → RAG: the connected host's RAG (proto.RAGSettings),
// the embedder its projects' text goes to and where their index is kept, and
// each project's index, turned on and agreed to here or in the side panel's
// Index.

const sectionRAG = "RAG"

// ragSettingsState is the section's. It belongs to the main thread.
type ragSettingsState struct {
	host    string
	asked   bool
	loaded  bool
	set     proto.RAGSettings
	dim     string
	err     string
	testing bool
	test    string
	testOK  bool
	saved   bool
	// key and pg are the embedder's key and the PostgreSQL URL typed here
	// (kept on the host alone), unless keyVault / pgVault: a vault
	// credential instead.
	key, pg           string
	keyVault, pgVault bool
	move              keysMove
	projects          ragProjects // each project's index (rag_projects.go)
}

var ragEmbedders = []string{"", proto.RAGVoyage, proto.RAGOpenAI, proto.RAGTEI}

var ragEmbedderNames = map[string]string{
	"":              "None: nothing is indexed",
	proto.RAGVoyage: "Voyage AI",
	proto.RAGOpenAI: "OpenAI-compatible (OpenAI, Ollama, OpenRouter…)",
	proto.RAGTEI:    "Text Embeddings Inference (self-hosted)",
}

var ragModelHints = map[string]string{proto.RAGVoyage: "voyage-code-3", proto.RAGOpenAI: "text-embedding-3-small", proto.RAGTEI: "BAAI/bge-small-en-v1.5"}

func (a *App) ragSettingsOf() *ragSettingsState {
	if a.ragSet.host != a.host.ID || !a.ragSet.asked {
		a.ragSet = ragSettingsState{host: a.host.ID, asked: true}
		a.loadRAGSettings()
	}
	return &a.ragSet
}

func (a *App) loadRAGSettings() {
	cl, host := a.client, a.host.ID
	if cl == nil {
		return
	}
	go func() {
		set, err := cl.RAGSettings()
		a.onMain(func() {
			st := &a.ragSet
			if st.host != host {
				return
			}
			st.loaded, st.set, st.err = true, set, ""
			if err != nil {
				st.err = client.Explain(err)
			}
			if set.Dim > 0 {
				st.dim = strconv.Itoa(set.Dim)
			}
			st.keyVault = set.KeyCred != "" && !set.HasKey
			st.pgVault = set.PostgresCred != "" && !set.HasPostgres
		})
	}()
}

// ragSection is the page: the figures of the host's projects, the embedder and
// the index's store, then each project's card.
func (a *App) ragSection(c *ui.Context) {
	st := a.ragSettingsOf()
	widget.PageHeader(c, "RAG", "Agents find a project by meaning", nil)
	switch {
	case st.err != "":
		widget.PageBody(c, true, func() { widget.Notice(c, icon.Alert, theme.Error, st.err, "", nil) })
		return
	case !st.loaded:
		widget.PageBody(c, true, func() { widget.Spinner(c, 14) })
		return
	}
	a.loadRAGProjects(st)
	widget.FigsBand(c, ragFigures(a.projects, st))
	cols := 4
	if w, _ := c.Size(); w >= 1400 {
		cols = 5
	}
	widget.PageBodyGrid(c, cols, len(a.projects), 190, func() {
		a.ragForm(c, st)
		ui.Row(c.Key("rag-projects")).FillWidth().Shrink(0).Margin(theme.S1, 0, 0, 0).Children(func() { widget.Heading(c, "Projects") })
	}, func() {
		for _, p := range a.projects {
			a.ragProjectCard(c.Key(p.Path), st, p)
		}
	}, nil)
	a.ragProjectDialogs(c, st)
}

// ragForm is the host's settings: its cards, then testing and saving them.
func (a *App) ragForm(c *ui.Context, st *ragSettingsState) {
	s := &st.set
	keys := s.HasKey || s.HasPostgres || st.move.said != ""
	cols := 2
	if keys {
		cols = 3
	}
	widget.Cols(c, cols).Children(func() {
		widget.Card(c.Key("embedder")).Children(func() { a.ragEmbedderCard(c, st) })
		widget.Card(c.Key("store")).Children(func() { a.ragStoreCard(c, st) })
		if keys {
			widget.Card(c.Key("keys")).Children(func() { a.ragKeysCard(c, st) })
		}
	})
	ui.Row(c.Key("rag-actions")).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
		ui.Spacer(c)
		if widget.GhostButton(c, "Test").Disabled(st.testing || s.Embedder == "").Clicked() { // the store's connection too
			a.testRAG(st)
		}
		label := "Save"
		if st.saved {
			label = "Saved"
		}
		if widget.PrimaryButton(c, icon.Check, label, "").Clicked() {
			a.saveRAGSettings(st)
		}
	})
	switch {
	case st.test == "":
	case !st.testOK:
		widget.Notice(c, icon.Alert, theme.Error, st.test, "", nil)
	case st.testing:
		widget.Banner(c, theme.Now, st.test, nil)
	default:
		widget.Banner(c, theme.Done, st.test, nil)
	}
}

func (a *App) ragEmbedderCard(c *ui.Context, st *ragSettingsState) {
	s := &st.set
	widget.CardTitle(c, nil, "Embedder", nil)
	options := make([]string, len(ragEmbedders))
	for i, e := range ragEmbedders {
		options[i] = ragEmbedderNames[e]
	}
	if i := widget.Choice(c, "Embedder", options, slices.Index(ragEmbedders, s.Embedder), 0); i >= 0 {
		s.Embedder = ragEmbedders[i]
	}
	if s.Embedder == "" {
		widget.CardNote(c, "Nothing is indexed, and agents read files as they would.", 0)
		return
	}
	widget.FieldCols(c, func() {
		widget.Field(c, "Model", func() { widget.Input(c, &s.Model, ragModelHints[s.Embedder]).FillWidth().Font(theme.MonoFont) })
	}, func() {
		widget.Field(c, "Dimensions", func() { widget.Input(c, &st.dim, "auto").FillWidth() })
	})
	if s.Embedder != proto.RAGVoyage {
		hint := "https://api.openai.com/v1"
		if s.Embedder == proto.RAGTEI {
			hint = "http://localhost:8080"
		}
		widget.Field(c, "Address", func() { widget.Input(c, &s.BaseURL, hint).FillWidth().Font(theme.MonoFont) })
	}
	hint := "pa-… / sk-…"
	if s.HasKey {
		hint = "set on " + a.host.Name + ": type to replace"
	}
	a.ragKeySelect(c, "API key", &st.key, &s.KeyCred, &st.keyVault, hint)
}

func (a *App) ragStoreCard(c *ui.Context, st *ragSettingsState) {
	s := &st.set
	widget.CardTitle(c, nil, "Where the index is kept", nil)
	cur := 0
	if s.Store == proto.RAGPostgres {
		cur = 1
	}
	if i := widget.SegmentedFit(c, []string{"SQLite on " + a.host.Name, "PostgreSQL (pgvector)"}, cur); i >= 0 {
		s.Store = proto.RAGBuiltin
		if i == 1 {
			s.Store = proto.RAGPostgres
		}
	}
	if s.Store != proto.RAGPostgres {
		widget.CardNote(c, "One SQLite file in "+a.host.Name+"'s enowX folder: nothing else to run.", 0)
		return
	}
	hint := "postgres://user:password@host:5432/db"
	if s.HasPostgres {
		hint = "set on " + a.host.Name + ": type to replace"
	}
	a.ragKeySelect(c, "Credential", &st.pg, &s.PostgresCred, &st.pgVault, hint)
	widget.CardNote(c, "A vault credential holding postgres://user:password@host:5432/db. pgvector must be installed.", 0)
}

func (a *App) ragKeysCard(c *ui.Context, st *ragSettingsState) {
	widget.CardTitle(c, nil, "Keys", nil)
	widget.Banner(c, theme.Warning, "Some keys here are on this host alone", nil)
	note := "The app asks you about each one, and moves it to the vault."
	if st.move.said != "" {
		note = st.move.said
	}
	widget.CardNote(c, note, 0)
	widget.CardFoot(c, func() {
		b := widget.GhostButton(c, "Keep them in the vault")
		if st.move.busy {
			b.Disabled(true)
		} else if b.Clicked() {
			a.moveKeysToVault(&st.move, a.loadRAGSettings)
		}
	})
}

// ragKeySelect picks where a key comes from, a credential of the vault or typed
// for this host alone, with what that takes.
func (a *App) ragKeySelect(c *ui.Context, label string, typed, cred *string, fromVault *bool, hint string) {
	const typedHere = "Typed here, on this host alone"
	var names, options []string
	if v := a.vaultOf(); v.data != nil {
		for _, cr := range v.data.Credentials {
			if !cr.Revoked && !slices.Contains(names, cr.Name) {
				names, options = append(names, cr.Name), append(options, "The vault's credential: "+cr.Name)
			}
		}
	}
	if *cred != "" && !slices.Contains(names, *cred) {
		names, options = append(names, *cred), append(options, *cred+" (not in the vault)")
	}
	options = append(options, typedHere)
	cur := len(options) - 1
	if i := slices.Index(names, *cred); *fromVault && i >= 0 {
		cur = i
	}
	widget.Field(c.Key(label), label, func() {
		if i := widget.Choice(c, label, options, cur, 0); i >= 0 {
			cur = i
			*fromVault = i < len(names)
			if *fromVault {
				*cred = names[i]
			}
		}
		if cur == len(options)-1 {
			widget.Input(c.Key("typed"), typed, hint).FillWidth().Password()
			widget.CardNote(c, "Kept on "+a.host.Name+" alone, readable by its user only; never synced or shown again.", 0)
		}
	})
}

// ragCredSelect picks a credential of the vault by name.
func (a *App) ragCredSelect(c *ui.Context, key string, value *string) {
	names := map[string]string{"": "None"}
	opts := []string{""}
	if v := a.vaultOf(); v.data != nil {
		for _, cr := range v.data.Credentials {
			if !cr.Revoked && names[cr.Name] == "" {
				names[cr.Name], opts = cr.Name, append(opts, cr.Name)
			}
		}
	}
	if *value != "" && names[*value] == "" {
		names[*value], opts = *value+" (not in the vault)", append(opts, *value)
	}
	hookSelect(c.Key(key), "Credential", value, opts, names)
}

func (st *ragSettingsState) form() proto.RAGSettings {
	s := st.set
	s.Dim, _ = strconv.Atoi(strings.TrimSpace(st.dim))
	s.Model, s.BaseURL = strings.TrimSpace(s.Model), strings.TrimSpace(s.BaseURL)
	if s.Model == "" {
		s.Model = ragModelHints[s.Embedder]
	}
	if !st.keyVault {
		s.KeyCred = "" // the key typed here
	}
	if !st.pgVault {
		s.PostgresCred = ""
	}
	return s
}

func (a *App) testRAG(st *ragSettingsState) {
	cl, set, keys := a.client, st.form(), st.keysToSave()
	st.testing, st.test = true, "Testing…"
	st.testOK = true
	go func() {
		var err error
		for _, k := range keys { // a key typed is kept first: the test uses it
			if _, err = cl.SetRAGKey(k.Which, k.Value); err != nil {
				break
			}
		}
		res := proto.RAGTestResult{}
		if err == nil {
			res, err = cl.RAGTest(set)
		}
		a.onMain(func() {
			st.testing = false
			switch {
			case err != nil:
				st.test, st.testOK = client.Explain(err), false
			case res.Error != "":
				st.test, st.testOK = res.Error, false
			default:
				st.test, st.testOK = "Works: vectors of "+strconv.Itoa(res.Dim)+" dimensions, the store answers.", true
			}
		})
	}()
}

func (a *App) saveRAGSettings(st *ragSettingsState) {
	cl, set, keys := a.client, st.form(), st.keysToSave()
	go func() {
		var err error
		for _, k := range keys {
			if _, err = cl.SetRAGKey(k.Which, k.Value); err != nil {
				break
			}
		}
		out := set
		if err == nil {
			out, err = cl.SetRAGSettings(set)
		}
		a.onMain(func() {
			if err != nil {
				st.test, st.testOK = client.Explain(err), false
				return
			}
			st.set, st.saved, st.key, st.pg = out, true, "", ""
			a.ragPanel.key = "" // read the project's anew: its embedder may have changed
		})
	}()
}

// ragEvent follows a project's index as it goes, and the daemon's offers
// to keep what a session decided.
func (a *App) ragEvent(ev proto.Event) {
	switch ev.Name {
	case proto.EventRAGProgress:
		var st proto.RAGStatus
		if json.Unmarshal(ev.Data, &st) != nil {
			return
		}
		a.onMain(func() {
			if a.ragPanel.dir == st.Project {
				a.ragPanel.st = st
			}
		})
	case proto.EventRAGOffer:
		var p proto.RAGRememberParams
		if json.Unmarshal(ev.Data, &p) != nil {
			return
		}
		a.onMain(func() { a.ragOffers = append(a.ragOffers, &p) })
	}
}

// ragAsk offers to keep what a session's agent said last as a memory of its
// project, edited first.
func (a *App) ragAsk(c *ui.Context) *widget.Ask {
	if len(a.ragOffers) == 0 {
		return nil
	}
	p := a.ragOffers[0]
	done := func() {
		a.ragOffers = slices.DeleteFunc(a.ragOffers, func(x *proto.RAGRememberParams) bool { return x == p })
	}
	return &widget.Ask{Key: fmt.Sprintf("rag:%p", p), Icon: icon.Check, Title: "Keep this as a memory of the project?",
		About: a.sessionLabel(p.Session) + " ended. Agents searching the project will find it; trim it to the decision.",
		Body: func() {
			area := ui.TextAreaBase(c.Key("rag-offer"), &p.Text).FillWidth().MinHeight(90).MaxHeight(240).Padding(theme.S2, theme.S3).FontSize(theme.TextUI).
				Radius(theme.RadiusCard).Label("Memory")
			widget.Well(area, area.Focused())
		},
		Buttons: []widget.AskButton{
			{Kind: widget.Secondary, Label: "Dismiss", Do: done},
			{Kind: widget.Primary, Icon: icon.Check, Label: "Save memory", Disabled: strings.TrimSpace(p.Text) == "", Do: func() {
				done()
				a.rememberRAG(proto.RAGRememberParams{RAGRef: proto.RAGRef{Project: p.Project}, Text: strings.TrimSpace(p.Text)})
			}},
		}}
}

func (a *App) rememberRAG(p proto.RAGRememberParams) {
	cl := a.client
	if cl == nil {
		return
	}
	go func() {
		_, err := cl.RAGRemember(p)
		a.onMain(func() {
			if err != nil {
				a.status = "Couldn't keep the memory: " + client.Explain(err)
				return
			}
			a.notify("Kept as a memory of the project")
			if a.ragPanel.dir == p.Project {
				a.loadRAGPanel()
			}
		})
	}()
}

// keysToSave are the keys to keep on the host as the form has them: one
// typed here, or none any more where the vault's credential is picked.
func (st *ragSettingsState) keysToSave() []proto.RAGKey {
	var out []proto.RAGKey
	for _, k := range []struct {
		which, typed string
		vault, has   bool
	}{{proto.RAGKeyEmbedder, st.key, st.keyVault, st.set.HasKey}, {proto.RAGKeyPostgres, st.pg, st.pgVault, st.set.HasPostgres}} {
		switch {
		case k.vault && k.has:
			out = append(out, proto.RAGKey{Which: k.which}) // the vault's from now on
		case !k.vault && strings.TrimSpace(k.typed) != "":
			out = append(out, proto.RAGKey{Which: k.which, Value: strings.TrimSpace(k.typed)})
		}
	}
	return out
}

// keysToVaultRow offers to move the keys typed on this host to the vault,
// for the user's other hosts: done runs after, on the main thread.
func (a *App) keysToVaultRow(c *ui.Context, st *keysMove, done func()) {
	widget.Notice(c, icon.Key, theme.Warning, "Some keys here are on this host alone", "Your other hosts get these settings but not the keys. Keep them in the vault, without typing them again: the app asks you first, and each host the first time it uses one.", func() {
		b := widget.GhostButton(c, "Keep them in the vault")
		if st.busy {
			b.Disabled(true)
		} else if b.Clicked() {
			a.moveKeysToVault(st, done)
		}
	})
	if st.said != "" {
		ui.Text(c, st.said).FontSize(theme.TextLabel).TextColor(theme.Muted)
	}
}

// keysMove is keysToVaultRow's, the main thread's.
type keysMove struct {
	busy bool
	said string
}

func (a *App) moveKeysToVault(st *keysMove, done func()) {
	cl := a.client
	if cl == nil {
		return
	}
	st.busy, st.said = true, "The app asks you about each one…"
	go func() {
		res, err := cl.KeysToVault()
		a.onMain(func() {
			st.busy = false
			if err != nil {
				st.said = client.Explain(err)
				return
			}
			var parts []string
			for _, m := range res.Moved {
				parts = append(parts, m.What+": now "+m.Credential)
			}
			for _, f := range res.Failed {
				parts = append(parts, f.What+" stays here: "+f.Err)
			}
			st.said = strings.Join(parts, ". ")
			done()
		})
	}()
}
