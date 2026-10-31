package ui

import (
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

// A model API written into an agent's own settings (core/agentapi), for
// the agents that take one no other way, in Settings → Agents: the
// settings backed up first and put back on asking. Agents that take one
// per session have it as a config (agents.go); one set in their files
// before shows here until it moved to a config.

type agentAPIState struct {
	host   string
	list   []proto.AgentAPIInfo
	loaded bool
	busy   bool
	err    string
	edit   *agentAPIForm
	menus  map[string]*widget.ContextMenu
}

// agentAPIForm is an agent's API being set: its models as typed, the
// first the default.
type agentAPIForm struct {
	open            bool
	agent           proto.AgentAPIInfo
	url, key, small string
	effort          int // in Caps.Efforts, -1 for the agent's own
	protocol        int
	models          []modelRow
	err             string
}

// modelRow is a model of the form, its numbers as typed.
type modelRow struct {
	id, context, output string
	vision              bool
	effort              int // in Caps.Efforts, -1 for the model's own
}

func numText(n int) string {
	if n <= 0 {
		return ""
	}
	return strconv.Itoa(n)
}

func (a *App) agentAPIsOf() *agentAPIState {
	if a.agentAPIs == nil || a.agentAPIs.host != a.host.ID {
		a.agentAPIs = &agentAPIState{host: a.host.ID, menus: map[string]*widget.ContextMenu{}}
		a.loadAgentAPIs()
	}
	return a.agentAPIs
}

func (a *App) loadAgentAPIs() {
	st, cl := a.agentAPIs, a.client
	if cl == nil {
		return
	}
	go func() {
		list, err := cl.AgentAPIs()
		a.onMain(func() {
			st.loaded = true
			if err != nil {
				st.err = client.Explain(err)
				return
			}
			st.err, st.list = "", list
		})
	}()
}

// agentAPIDo runs a change off the main thread and shows the agent as it
// is after.
func (a *App) agentAPIDo(st *agentAPIState, do func(*client.Client) (proto.AgentAPIInfo, error), done func()) {
	cl := a.client
	if cl == nil || st.busy {
		return
	}
	st.busy, st.err = true, ""
	go func() {
		info, err := do(cl)
		a.onMain(func() {
			st.busy = false
			if err != nil {
				st.err = client.Explain(err)
				if st.edit != nil && st.edit.open {
					st.edit.err = st.err
				}
				return
			}
			for i := range st.list {
				if st.list[i].Agent == info.Agent {
					st.list[i] = info
				}
			}
			if done != nil {
				done()
			}
		})
	}()
}

func (a *App) agentAPIRow(c *ui.Context, st *agentAPIState, ag proto.AgentAPIInfo) {
	widget.Card(c).FillWidth().Children(func() {
		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
			agentTile(c, ag.Agent, 24)
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, ag.Name).FontSize(13)
				detail := "Its own API"
				if cur := ag.Current; cur != nil {
					detail = cur.BaseURL
					if cur.Model != "" {
						detail += " · " + cur.Model
					}
					if n := len(cur.Models); n > 1 {
						detail += " +" + strconv.Itoa(n-1)
					}
					if cur.APIKey != "" {
						detail += " · key " + cur.APIKey
					}
				}
				col := theme.Muted
				if ag.Current != nil {
					col = theme.TextSoft
				}
				ui.Text(c, detail).FontSize(11.5).TextColor(col).SingleLine().Shrink(1)
				if ag.Error != "" {
					ui.Text(c, ag.Error).FontSize(11).TextColor(theme.Error)
				}
			})
			label := "Use an API…"
			if ag.Current != nil {
				label = "Change…"
			}
			if ag.PerSession && ag.Current != nil {
				// It moves to a config once the vault took its key.
				widget.Tag(c, "becomes a config", theme.Warning)
			}
			if !ag.PerSession && widget.Button(c, widget.Secondary, nil, label, "").Disabled(st.busy).Clicked() {
				f := &agentAPIForm{open: true, agent: ag, effort: -1}
				if cur := ag.Current; cur != nil {
					f.url, f.small = cur.BaseURL, cur.SmallModel
					f.protocol = max(0, slices.Index(ag.Protocols, cur.Protocol))
					f.effort = slices.Index(ag.Caps.Efforts, cur.Effort)
					for _, m := range cur.Models {
						f.models = append(f.models, modelRow{id: m.ID, context: numText(m.Context), output: numText(m.Output), vision: m.Vision,
							effort: slices.Index(ag.Caps.Efforts, m.Effort)})
					}
				}
				if len(f.models) == 0 && !ag.NoModel {
					f.models = []modelRow{{effort: -1}}
				}
				st.edit = f
			}
			if ag.Current != nil || len(ag.Backups) > 0 {
				m := st.menus[ag.Agent]
				if m == nil {
					m = &widget.ContextMenu{}
					st.menus[ag.Agent] = m
				}
				widget.MoreMenu(c, m, "api:"+ag.Agent, func(done func()) {
					if ag.Current != nil && widget.MenuItem(c, icon.Restore, "Back to its own API", false) {
						a.agentAPIDo(st, func(cl *client.Client) (proto.AgentAPIInfo, error) { return cl.ClearAgentAPI(ag.Agent) }, nil)
						done()
					}
					if len(ag.Backups) > 0 {
						widget.MenuSeparator(c)
						widget.PopupLabel(c, "Restore its settings as they were")
					}
					for _, b := range ag.Backups {
						label := b.Time.Local().Format("Jan 2 15:04") + " · before " + strings.TrimPrefix(b.Note, "restoring ")
						if b.Original {
							label = "As you had them (" + b.Time.Local().Format("Jan 2 15:04") + ")"
						}
						if widget.MenuItem(c, icon.Restore, label, false) {
							id := b.ID
							a.agentAPIDo(st, func(cl *client.Client) (proto.AgentAPIInfo, error) { return cl.RestoreAgentAPI(ag.Agent, id) }, nil)
							done()
						}
					}
				})
			}
		})
	})
}

func (a *App) agentAPISheet(c *ui.Context, st *agentAPIState) {
	f := st.edit
	if f == nil || !f.open {
		st.edit = nil
		return
	}
	ag := f.agent
	about := "Written into " + strings.Join(ag.Files, ", ") + " on " + a.host.Name + ", which are kept first. Sessions of " + ag.Name + " started afterwards use it."
	widget.FormSheet(c, &f.open, 560, ag.Name+": a model API", about, func() {
		widget.Field(c, "Base URL", func() {
			widget.Input(c, &f.url, "https://openrouter.ai/api/v1").FillWidth().Font(theme.MonoFont)
		})
		if len(ag.Protocols) > 1 {
			widget.Field(c, "It speaks", func() {
				labels := make([]string, len(ag.Protocols))
				for i, p := range ag.Protocols {
					labels[i] = map[string]string{"anthropic": "Anthropic", "openai": "OpenAI (chat)", "openai-responses": "OpenAI (responses)", "gemini": "Gemini"}[p]
				}
				ui.Row(c).Children(func() {
					if i := widget.Segmented(c, labels, f.protocol, nil); i >= 0 {
						f.protocol = i
					}
				})
			})
		}
		key := "API key"
		if ag.Current != nil && ag.Current.APIKey != "" {
			key = "API key (empty keeps " + ag.Current.APIKey + ")"
		}
		widget.Field(c, key, func() {
			widget.Input(c, &f.key, "sk-…").FillWidth().Password()
			a.vaultAPIKeys(c, f)
		})
		if !ag.NoModel {
			a.agentAPIModels(c, f)
		}
		caps := ag.Caps
		if ag.Small || caps.Effort == "global" {
			ui.Row(c).FillWidth().Gap(10).AlignItems(ui.End).Children(func() {
				if ag.Small {
					ui.Column(c).Grow(1).Children(func() {
						widget.Field(c, "Model for quick work (optional)", func() { widget.Input(c, &f.small, "").FillWidth().Font(theme.MonoFont) })
					})
				}
				if caps.Effort == "global" {
					widget.Field(c, "Reasoning effort, every model", func() { effortPicker(c, caps.Efforts, &f.effort) })
				}
			})
		}
		if ag.Note != "" {
			ui.Text(c, ag.Note).FontSize(11.5).TextColor(theme.Muted)
		}
		if f.err != "" {
			ui.Text(c, f.err).FontSize(12).TextColor(theme.Error)
		}
	}, func() {
		label := "Use it"
		if st.busy {
			label = "Saving…"
		}
		if widget.PrimaryButton(c, icon.Check, label, "").Disabled(st.busy || strings.TrimSpace(f.url) == "").Clicked() {
			api := proto.AgentAPI{BaseURL: f.url, APIKey: f.key, SmallModel: f.small}
			if f.effort >= 0 && f.effort < len(ag.Caps.Efforts) {
				api.Effort = ag.Caps.Efforts[f.effort]
			}
			for _, r := range f.models {
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
			if f.protocol < len(ag.Protocols) {
				api.Protocol = ag.Protocols[f.protocol]
			}
			if api.APIKey == "" && ag.Current != nil {
				api.APIKey = proto.AgentAPIKeepKey // the daemon keeps the key it has
			}
			id := ag.Agent
			a.agentAPIDo(st, func(cl *client.Client) (proto.AgentAPIInfo, error) { return cl.SetAgentAPI(id, api) }, func() { f.open = false })
		}
	})
}

// agentAPIModels are the form's models, each with what the agent takes of
// it; the first is the default.
func (a *App) agentAPIModels(c *ui.Context, f *agentAPIForm) {
	caps := f.agent.Caps
	ui.Column(c).FillWidth().Gap(8).Children(func() {
		title := "Model"
		if caps.Multi {
			title = "Models: the first is the default"
		}
		ui.Text(c, title).FontSize(12).TextColor(theme.TextSoft)
		for i := range f.models {
			r := &f.models[i]
			ui.Column(c.Key(fmt.Sprint("m", i))).FillWidth().Gap(6).Padding(10).Radius(theme.RadiusControl).Background(theme.Sunken).Children(func() {
				ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
					widget.Input(c, &r.id, "the model's id, as the API names it").Grow(1).Font(theme.MonoFont)
					if i > 0 && widget.IconButton(c, icon.ArrowUp, "Make it the default", false).Clicked() {
						f.models[0], f.models[i] = f.models[i], f.models[0]
					}
					if len(f.models) > 1 && widget.IconButton(c, icon.Trash, "Remove", false).Clicked() {
						f.models = slices.Delete(f.models, i, i+1)
					}
				})
				// Each option only where the agent takes it: per model, or the
				// default's for all.
				perModel := func(scope string) bool { return scope == "model" || (scope == "global" && i == 0) }
				if !caps.Vision && !perModel(caps.Context) && !perModel(caps.Output) && caps.Effort != "model" {
					return
				}
				if caps.Vision || perModel(caps.Context) || perModel(caps.Output) {
					ui.Row(c).FillWidth().Gap(14).AlignItems(ui.Center).Children(func() {
						if perModel(caps.Context) {
							tokens(c, "Context", &r.context, caps.Context == "global")
						}
						if perModel(caps.Output) {
							tokens(c, "Output", &r.output, caps.Output == "global")
						}
						if caps.Vision {
							widget.Checkbox(c, &r.vision, "Vision")
						}
					})
				}
				if caps.Effort == "model" {
					ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Text(c, "Effort").FontSize(11.5).TextColor(theme.Muted).Shrink(0)
						effortPicker(c, caps.Efforts, &r.effort)
					})
				}
			})
		}
		if caps.Multi && widget.Button(c, widget.Secondary, icon.Plus, "Add a model", "").Clicked() {
			f.models = append(f.models, modelRow{effort: -1})
		}
	})
}

// tokens is a field of a number of tokens.
func tokens(c *ui.Context, label string, value *string, all bool) {
	tip := label + ", in tokens"
	if all {
		tip += ": the agent takes one for every model"
	}
	ui.Row(c).Gap(6).AlignItems(ui.Center).Tooltip(tip).Children(func() {
		ui.Text(c, label).FontSize(11.5).TextColor(theme.Muted).Shrink(0)
		widget.Input(c, value, "auto").Width(96).Font(theme.MonoFont)
	})
}

// effortPicker picks a reasoning effort, or the agent's own.
func effortPicker(c *ui.Context, efforts []string, value *int) {
	labels := append([]string{"auto"}, efforts...)
	if i := widget.Segmented(c, labels, *value+1, nil); i >= 0 {
		*value = i - 1
	}
}
