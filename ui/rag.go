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
// the embedder its projects' text goes to and where their index is kept.
// Each project is turned on, and agreed to, in the side panel's Index.

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

func (a *App) ragSection(c *ui.Context) {
	st := a.ragSettingsOf()
	sectionIntro(c, "RAG", "Agents find a project's code, docs, attached notes and memories by meaning rather than by reading it all. A project is indexed only once turned on in its Index tool, after you agree to send its text to the embedder below; secrets and ignored files never are.")
	switch {
	case st.err != "":
		widget.Notice(c, icon.Alert, theme.Error, st.err, "", nil)
		return
	case !st.loaded:
		widget.Spinner(c, 14)
		return
	}
	s := &st.set
	widget.Field(c, "Embedder", func() {
		hookSelect(c, "Embedder", &s.Embedder, ragEmbedders, ragEmbedderNames)
	})
	switch s.Embedder {
	case proto.RAGVoyage, proto.RAGOpenAI, proto.RAGTEI:
		ui.Row(c).FillWidth().Gap(10).Children(func() {
			ui.Column(c).Grow(1).Children(func() {
				widget.Field(c, "Model", func() { widget.Input(c, &s.Model, ragModelHints[s.Embedder]).FillWidth().Font(theme.MonoFont) })
			})
			ui.Column(c).Width(120).Children(func() {
				widget.Field(c, "Dimensions", func() { widget.Input(c, &st.dim, "auto").FillWidth() })
			})
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
		a.keyFields(c, "rag-key", &st.key, &s.KeyCred, &st.keyVault, hint)
	}
	widget.Field(c, "Index kept", func() {
		cur := 0
		if s.Store == proto.RAGPostgres {
			cur = 1
		}
		if i := widget.Segmented(c, []string{"SQLite on " + a.host.Name, "PostgreSQL (pgvector)"}, cur, nil); i >= 0 {
			s.Store = proto.RAGBuiltin
			if i == 1 {
				s.Store = proto.RAGPostgres
			}
		}
	})
	if s.Store == proto.RAGPostgres {
		hint := "postgres://user:password@host:5432/db"
		if s.HasPostgres {
			hint = "set on " + a.host.Name + ": type to replace"
		}
		a.keyFields(c, "rag-pg", &st.pg, &s.PostgresCred, &st.pgVault, hint)
		ui.Text(c, "A vault credential holding postgres://user:password@host:5432/db. pgvector must be installed there (enowX creates the extension when its user may); enowX keeps its table enowx_rag_points.").FontSize(11.5).TextColor(theme.Muted)
	} else {
		ui.Text(c, "One SQLite file in "+a.host.Name+"'s enowX folder: nothing else to run, fast up to around a hundred thousand parts a project.").FontSize(11.5).TextColor(theme.Muted)
	}
	if s.HasKey || s.HasPostgres || st.move.said != "" {
		a.keysToVaultRow(c, &st.move, a.loadRAGSettings)
	}
	if st.test != "" {
		col, svg := theme.Error, icon.Alert
		if st.testOK {
			col, svg = theme.Done, icon.Check
		}
		widget.Notice(c, svg, col, st.test, "", nil)
	}
	ui.Row(c).FillWidth().Gap(8).Children(func() {
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
				st.test, st.testOK = "Works: vectors of "+strconv.Itoa(res.Dim)+" dimensions.", true
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
			ui.TextAreaBase(c.Key("rag-offer"), &p.Text).FillWidth().MinHeight(90).MaxHeight(240).Padding(9, 11).FontSize(12.5).
				Radius(theme.RadiusControl).Border(1, theme.Border).Label("Memory")
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
		ui.Text(c, st.said).FontSize(12).TextColor(theme.Muted)
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
