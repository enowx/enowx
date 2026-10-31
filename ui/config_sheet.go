package ui

import (
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/vault"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The dialog making or changing an agent's config: its kind (a model API,
// another account, a key of the vault's), what that kind needs, its
// effort, extra variables and flags, its name and badge color.

// configForm is a config being made (id "") or changed.
type configForm struct {
	open    bool
	id      string
	agent   string
	kinds   []string // the kinds the agent can have
	kind    int
	name    string
	cred    string // an API key config's credential
	keyCred string // a model API's key's credential
	effort  string
	env     string // NAME=VALUE lines
	args    string // flags, by spaces
	color   string
	api     *agentAPIForm // a model API's address, protocol, models
	err     string
}

// newConfigForm opens the dialog for agentID's config c (a new one when
// its ID is empty, its fields the start of it).
func (a *App) newConfigForm(agentID string, c proto.Account) *configForm {
	f := &configForm{open: true, id: c.ID, agent: agent.Base(agentID), kinds: a.configKinds(agentID), name: c.Name, cred: c.Cred,
		effort: c.Effort, env: strings.Join(c.Env, "\n"), args: strings.Join(c.Args, " "), color: c.Color}
	if c.ID == "" && c.Name == "" {
		f.color = widget.SwatchColors[len(a.accountsOf().accountsOfAgent(agentID))%len(widget.SwatchColors)]
	}
	info, _ := apiInfoOf(a.agentAPIsOf(), f.agent)
	f.api = &agentAPIForm{agent: info, effort: -1}
	switch {
	case c.Kind == proto.ConfigKindAPI:
		f.kinds = []string{kindAPI} // a config stays its kind
	case c.ID != "" && c.Cred != "":
		f.kinds = []string{kindKey}
	case c.ID != "":
		f.kinds = []string{kindAccount}
	case c.Cred != "":
		f.kind = max(0, slices.Index(f.kinds, kindKey))
	}
	if api := c.API; api != nil {
		f.api.url, f.api.small, f.keyCred = api.BaseURL, api.SmallModel, api.KeyCred
		f.api.protocol = max(0, slices.Index(info.Protocols, api.Protocol))
		for _, m := range api.Models {
			f.api.models = append(f.api.models, modelRow{id: m.ID, context: numText(m.Context), output: numText(m.Output), vision: m.Vision,
				effort: slices.Index(info.Caps.Efforts, m.Effort)})
		}
	}
	if len(f.api.models) == 0 && !info.NoModel {
		f.api.models = []modelRow{{effort: -1}}
	}
	return f
}

func (f *configForm) kindName() string {
	if f.kind < len(f.kinds) {
		return f.kinds[f.kind]
	}
	return ""
}

func (a *App) configSheet(c *ui.Context, st *accountsState) {
	f := st.edit
	if f == nil || !f.open {
		st.edit = nil
		return
	}
	title := a.agentName(f.agent) + ": a new config"
	if f.id != "" {
		title = a.agentName(f.agent) + ": " + f.name
	}
	about := "Saved apart from " + a.agentName(f.agent) + "'s own settings, which stay as they are; a session runs with it when picked at New session. Configs are the same on every host of yours."
	widget.FormSheet(c, &f.open, 600, title, about, func() {
		if len(f.kinds) > 1 {
			widget.Field(c, "Kind", func() {
				ui.Row(c).Children(func() {
					if i := widget.Segmented(c, f.kinds, f.kind, nil); i >= 0 {
						f.kind = i
					}
				})
			})
		}
		widget.Field(c, "Name", func() { widget.Input(c, &f.name, "GLM via OpenRouter").FillWidth().AutoFocus() })
		switch f.kindName() {
		case kindAPI:
			a.configAPIFields(c, f)
		case kindAccount:
			ui.Text(c, "Its own sign-in, in a home of its own sharing "+a.agentName(f.agent)+"'s settings, skills and conversations. Once saved, it signs in on "+a.host.Name+"; on another host, sign it in there from its menu.").FontSize(11.5).TextColor(theme.Muted)
		case kindKey:
			widget.Field(c, "The vault's credential", func() {
				widget.Input(c, &f.cred, "anthropic-work").FillWidth().Font(theme.MonoFont)
			})
			ui.Text(c, "Its value stays in the vault: sessions on this config get it as the vault lends credentials, never shown.").FontSize(11.5).TextColor(theme.Muted)
		}
		a.configExtras(c, f)
		if f.err != "" {
			ui.Text(c, f.err).FontSize(12).TextColor(theme.Error)
		}
	}, func() {
		label := "Save"
		if st.busy {
			label = "Saving…"
		}
		if widget.PrimaryButton(c, icon.Check, label, "").Disabled(st.busy || strings.TrimSpace(f.name) == "").Clicked() {
			a.saveConfig(st, f)
		}
	})
}

// configAPIFields are a model API's: its address, how it is spoken, its
// key's credential, its models.
func (a *App) configAPIFields(c *ui.Context, f *configForm) {
	ag := f.api.agent
	widget.Field(c, "Base URL", func() {
		widget.Input(c, &f.api.url, "https://openrouter.ai/api").FillWidth().Font(theme.MonoFont)
	})
	if len(ag.Protocols) > 1 {
		widget.Field(c, "It speaks", func() {
			labels := make([]string, len(ag.Protocols))
			for i, p := range ag.Protocols {
				labels[i] = map[string]string{"anthropic": "Anthropic", "openai": "OpenAI (chat)", "openai-responses": "OpenAI (responses)", "gemini": "Gemini"}[p]
			}
			ui.Row(c).Children(func() {
				if i := widget.Segmented(c, labels, f.api.protocol, nil); i >= 0 {
					f.api.protocol = i
				}
			})
		})
	}
	widget.Field(c, "Its key: the vault's credential", func() {
		widget.Input(c, &f.keyCred, "openrouter").FillWidth().Font(theme.MonoFont)
		a.vaultKeyNames(c, &f.keyCred)
	})
	if !ag.NoModel {
		a.agentAPIModels(c, f.api)
	}
	if ag.Small {
		widget.Field(c, "Model for quick work (optional)", func() { widget.Input(c, &f.api.small, "").FillWidth().Font(theme.MonoFont) })
	}
}

// vaultKeyNames offers the vault's API keys' names for value.
func (a *App) vaultKeyNames(c *ui.Context, value *string) {
	var names []string
	if v := a.vaultOf(); v.data != nil {
		for _, cr := range v.data.Credentials {
			if !cr.Revoked && vault.Lookup(cr.Platform).Type == vault.TypeAPIKey {
				names = append(names, cr.Name)
			}
		}
	}
	if len(names) == 0 {
		ui.Text(c, "Keep the key in the vault first (Vault → Add), then name it here: its value never leaves the vault but for the session.").FontSize(11).TextColor(theme.Muted)
		return
	}
	ui.Row(c).FillWidth().Wrap().Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "In the vault:").FontSize(11.5).TextColor(theme.Muted)
		for _, n := range names {
			b := ui.ButtonBase(c.Key("cred:"+n)).Height(24).PaddingX(8).Radius(12).Border(1, theme.Border).Cursor(ui.CursorPointer).Label("Use " + n)
			if *value == n {
				b.Background(theme.Selected)
			} else if b.Hovered() {
				b.Background(theme.Hover)
			}
			b.Children(func() { ui.Text(c, n).FontSize(11.5).TextColor(theme.TextSoft) })
			if widget.Pressable(b).Clicked() {
				*value = n
			}
		}
	})
}

// configExtras are any config's: effort, variables, flags, badge.
func (a *App) configExtras(c *ui.Context, f *configForm) {
	if efforts := agent.SettingsOf(f.agent).Efforts; len(efforts) > 0 {
		widget.Field(c, "Effort", func() {
			labels, cur := []string{"its own"}, 0
			for i, e := range efforts {
				labels = append(labels, e.Label)
				if e.ID == f.effort {
					cur = i + 1
				}
			}
			ui.Row(c).Children(func() {
				if i := widget.Segmented(c, labels, cur, nil); i >= 0 {
					f.effort = ""
					if i > 0 {
						f.effort = efforts[i-1].ID
					}
				}
			})
		})
	}
	widget.Field(c, "Variables its sessions get (NAME=VALUE, a line each)", func() {
		ui.TextAreaBase(c, &f.env).FillWidth().MinHeight(48).MaxHeight(140).Padding(9, 11).FontSize(12).Font(theme.MonoFont).
			Label("Variables").Radius(theme.RadiusControl).Background(theme.Sunken).Border(1, theme.Border).Placeholder("DISABLE_TELEMETRY=1")
	})
	widget.Field(c, "Flags added to its command line", func() {
		widget.Input(c, &f.args, "--verbose").FillWidth().Font(theme.MonoFont)
	})
	widget.Field(c, "Badge", func() { widget.Swatches(c, widget.SwatchColors, &f.color) })
}

// saveConfig keeps f's config.
func (a *App) saveConfig(st *accountsState, f *configForm) {
	cl := a.client
	if cl == nil {
		return
	}
	cfg := proto.Account{ID: f.id, Agent: f.agent, Name: strings.TrimSpace(f.name), Effort: f.effort, Color: f.color, Args: strings.Fields(f.args)}
	for _, l := range strings.Split(f.env, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			cfg.Env = append(cfg.Env, l)
		}
	}
	switch f.kindName() {
	case kindAPI:
		cfg.Kind = proto.ConfigKindAPI
		ag := f.api.agent
		api := &proto.ConfigAPI{BaseURL: strings.TrimSpace(f.api.url), KeyCred: strings.TrimSpace(f.keyCred), SmallModel: strings.TrimSpace(f.api.small)}
		if f.api.protocol < len(ag.Protocols) {
			api.Protocol = ag.Protocols[f.api.protocol]
		}
		for _, r := range f.api.models {
			if strings.TrimSpace(r.id) == "" {
				continue
			}
			m := proto.AgentAPIModel{ID: strings.TrimSpace(r.id), Vision: r.vision}
			m.Context, _ = strconv.Atoi(strings.TrimSpace(r.context))
			m.Output, _ = strconv.Atoi(strings.TrimSpace(r.output))
			if r.effort >= 0 && r.effort < len(ag.Caps.Efforts) {
				m.Effort = ag.Caps.Efforts[r.effort]
			}
			api.Models = append(api.Models, m)
		}
		if len(api.Models) > 0 {
			api.Model = api.Models[0].ID
		}
		cfg.API = api
	case kindKey:
		cfg.Cred = strings.TrimSpace(f.cred)
	}
	isNew := f.id == ""
	st.busy, f.err = true, ""
	go func() {
		saved, err := cl.SaveConfig(cfg)
		a.onMain(func() {
			st.busy = false
			if err != nil {
				f.err = client.Explain(err)
				return
			}
			f.open = false
			a.loadAccounts()
			if isNew && saved.Kind == proto.ConfigKindAccount && saved.Cred == "" {
				a.signInAccount(saved.Agent, saved) // signed in once, here
			}
		})
	}()
}
