package libraryview

import (
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/envsync"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Claude Code's settings.json shows as its permissions and hooks, edited
// as lists (envsync.ReadClaudeSettings), the file itself a tab away.

const (
	tabPermissions = 0
	tabHooks       = 1
	tabFile        = 2
)

// claudeModes are the permission modes a session starts in.
var claudeModes = []struct{ id, name string }{
	{"", "Default"}, {"acceptEdits", "Accept edits"}, {"plan", "Plan"}, {"bypassPermissions", "Bypass"},
}

// settingsEditor is what the page edits of a settings.json.
type settingsEditor struct {
	forID   string
	tab     int
	err     string
	perm    envsync.ClaudePermissions
	hooks   []envsync.ClaudeHook
	dirty   bool
	newRule [3]string // what is typed to add to allow, ask, deny
	hook    envsync.ClaudeHook
	timeout string
}

func isClaudeSettings(it proto.EnvItem) bool {
	return it.Agent == "claude" && it.Kind == proto.EnvSettings
}

// settingsKind shows an agent's settings file as a card of the frame's whole
// width, as Claude Code's needs: there are few of them, so a drop-down picks
// one rather than a list beside it.
func (v *View) settingsKind(c *ui.Context, o Options) {
	items := v.items(proto.EnvSettings, "")
	ui.Row(c.Key("tools")).FillWidth().Shrink(0).MinHeight(theme.ControlHeight).Gap(theme.S3).Children(func() {
		aboutLine(c, v.kindAbout())
		if !slices.ContainsFunc(items, isClaudeSettings) {
			if widget.Button(c, widget.Secondary, nil, "Make Claude Code's settings.json", "").Tooltip("To set its permissions and hooks").Clicked() {
				v.createClaudeSettings(o)
			}
		}
		if len(items) > 1 {
			ids := make([]string, len(items))
			names := map[string]string{}
			for i, it := range items {
				ids[i], names[it.ID] = it.ID, agentName(it.Agent)+" · "+it.Name
			}
			ui.Row(c).Width(280).Shrink(0).Children(func() {
				pickOne(c, "Settings file", 280, ids, func(id string) string { return names[id] }, &v.selected)
			})
		}
		if widget.Button(c, widget.Secondary, icon.Sync, "Refresh", "").Clicked() {
			v.Reload(o)
		}
	})
	v.noticeLine(c)
	if len(items) == 0 {
		noneHere(c, o)
		return
	}
	v.autoPick(items)
	v.detail(c, o)
}

// claudeSettings shows the settings.json picked as lists to edit, heading the
// card with what it is and the tabs.
func (v *View) claudeSettings(c *ui.Context, o Options, p proto.EnvPayload) {
	e := &v.settings
	if e.forID != p.Item.ID || len(p.Files) == 0 {
		*e = settingsEditor{forID: p.Item.ID, hook: envsync.ClaudeHook{Event: "PreToolUse"}}
		if len(p.Files) > 0 {
			var err error
			e.perm, e.hooks, err = envsync.ReadClaudeSettings(p.Files[0].Data)
			if err != nil {
				e.err, e.tab = err.Error(), tabFile
			}
		}
	}
	ui.Row(c.Key("settings-head")).FillWidth().Shrink(0).MinHeight(theme.ControlHeight).Gap(theme.S2).Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Gap(theme.S3).Children(func() {
			ui.Row(c).MinWidth(0).Shrink(1).Children(func() {
				widget.Running(c, p.Item.Name, widget.RunStyle{Size: theme.TextRead, Weight: 600, Always: true})
			})
			ui.Row(c).MinWidth(0).Shrink(100).Children(func() {
				widget.Running(c.Key("where"), agentName(p.Item.Agent)+" · "+p.Item.Detail, widget.RunStyle{Color: theme.TextSoft})
			})
		})
		if i := widget.Segmented(c.Key("tab"), []string{"Permissions", "Hooks", "File"}, e.tab, nil); i >= 0 {
			e.tab = i
		}
		if e.dirty && widget.Button(c, widget.Secondary, nil, "Discard", "").Clicked() {
			e.forID = "" // read again
		}
		if widget.PrimaryButton(c, nil, "Save", "").Disabled(!e.dirty).Clicked() && e.dirty {
			data, err := envsync.WriteClaudeSettings(p.Files[0].Data, e.perm, e.hooks)
			if err != nil {
				e.err = err.Error()
			} else {
				out := p
				f := p.Files[0]
				f.Data = data
				out.Files, out.Home = []proto.EnvFile{f}, v.scan.Home
				e.forID = ""
				v.save(o, out, "Saved "+p.Item.Name)
			}
		}
		v.actions(c, o, p.Item, true)
	})
	v.itemNotes(c, p.Item)
	if e.err != "" {
		ui.Text(c.Key("settings-error"), e.err).FontSize(theme.TextUI).TextColor(theme.Error).FillWidth().Shrink(0)
	}
	switch e.tab {
	case tabPermissions:
		v.permissions(c, e)
	case tabHooks:
		v.hookList(c, e)
	default:
		v.content(c, o, p)
	}
}

// permissions edits the starting mode and the allow, ask and deny rules, each
// list a card sharing the height, scrolling inside it.
func (v *View) permissions(c *ui.Context, e *settingsEditor) {
	ui.Row(c.Key("mode")).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
		ui.Text(c, "Starts in").FontSize(theme.TextUI).TextColor(theme.TextSoft)
		cur := slices.IndexFunc(claudeModes, func(m struct{ id, name string }) bool { return m.id == e.perm.DefaultMode })
		names := make([]string, len(claudeModes))
		for i, m := range claudeModes {
			names[i] = m.name
		}
		if i := widget.Segmented(c, names, cur, nil); i >= 0 {
			e.perm.DefaultMode, e.dirty = claudeModes[i].id, true
		}
	})
	lists := []struct {
		name, about string
		rules       *[]string
	}{
		{"Allow", "run without asking", &e.perm.Allow},
		{"Ask", "always ask first", &e.perm.Ask},
		{"Deny", "never", &e.perm.Deny},
	}
	ui.Row(c.Key("rule-cards")).Grow(1).Basis(0).FillWidth().MinHeight(0).Gap(theme.S3).AlignItems(ui.Stretch).Children(func() {
		for i, l := range lists {
			widget.Card(c.Key(l.name)).Grow(1).Basis(0).MinHeight(0).Clip().Children(func() {
				ui.Row(c).FillWidth().Shrink(0).Children(func() {
					widget.Running(c, l.name, widget.RunStyle{Size: theme.TextRead, Weight: 600})
				})
				ui.Text(c, l.about).FontSize(theme.TextLabel).TextColor(theme.TextSoft).Shrink(0)
				widget.Scroll(c).Shrink(1).FillWidth().MinHeight(0).Children(func() {
					ui.Column(c).FillWidth().Children(func() {
						for j, r := range *l.rules {
							ruleLine(c.Key(strconv.Itoa(j)), j == 0, r, func() {
								*l.rules = slices.Delete(slices.Clone(*l.rules), j, j+1)
								e.dirty = true
							})
						}
					})
				})
				ui.Row(c).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
					in := widget.Input(c, &e.newRule[i], "Bash(git status)").Grow(1).Basis(0).MinWidth(0).Font(theme.MonoFont).
						Tooltip("Rules are a tool and what it works on: Bash(npm run test:*), Read(./.env), Edit(src/**), WebFetch(domain:docs.rs), mcp__github.")
					add := in.Submitted()
					if widget.Button(c, widget.Secondary, nil, "Add the rule", "").Clicked() || add {
						if r := strings.TrimSpace(e.newRule[i]); r != "" && !slices.Contains(*l.rules, r) {
							*l.rules = append(slices.Clone(*l.rules), r)
							e.newRule[i], e.dirty = "", true
						}
					}
				})
			})
		}
	})
}

// ruleLine is a rule of a list, divided from the one above by a hairline.
func ruleLine(c *ui.Context, first bool, rule string, remove func()) {
	r := ui.Row(c).FillWidth().Shrink(0).MinHeight(theme.ControlHeight).Gap(theme.S2)
	if !first {
		r.BorderWidth(0.5, 0, 0, 0).BorderColor(theme.Border)
	}
	r.Children(func() {
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
			widget.Running(c, rule, widget.RunStyle{Mono: true})
		})
		if widget.IconButton(c, icon.Close, "Remove", false).Clicked() {
			remove()
		}
	})
}

// hookLabel says when a hook runs.
func hookLabel(h envsync.ClaudeHook) string {
	label := h.Event
	if h.Matcher != "" {
		label += " · " + h.Matcher
	}
	if h.Timeout > 0 {
		label += " · " + strconv.Itoa(h.Timeout) + " s"
	}
	return label
}

// hookList edits the hooks: what runs on which event. The list takes the
// height the way to add one leaves.
func (v *View) hookList(c *ui.Context, e *settingsEditor) {
	widget.Card(c.Key("hooks")).Grow(1).Basis(0).FillWidth().MinHeight(72).Clip().Children(func() {
		if len(e.hooks) == 0 {
			ui.Text(c, "No hooks yet.").FontSize(theme.TextUI).TextColor(theme.TextSoft)
			return
		}
		widget.Scroll(c).Grow(1).Basis(0).FillWidth().MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				for i, h := range e.hooks {
					r := ui.Row(c.Key(strconv.Itoa(i))).FillWidth().Shrink(0).MinHeight(theme.ControlHeight).Gap(theme.S3)
					if i > 0 {
						r.BorderWidth(0.5, 0, 0, 0).BorderColor(theme.Border)
					}
					r.Children(func() {
						ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() { widget.Running(c, hookLabel(h), widget.RunStyle{}) })
						cmd := h.Command
						if cmd == "" {
							cmd, _ = h.Hook["type"].(string)
						}
						ui.Row(c).Grow(1).Basis(0).MinWidth(0).Justify(ui.End).Children(func() {
							widget.Running(c.Key("cmd"), cmd, widget.RunStyle{Mono: true})
						})
						if widget.IconButton(c, icon.Close, "Remove the hook", false).Clicked() {
							e.hooks = slices.Delete(slices.Clone(e.hooks), i, i+1)
							e.dirty = true
						}
					})
				}
			})
		})
	})
	widget.Card(c.Key("add-hook")).FillWidth().Shrink(0).Children(func() {
		ui.Row(c).FillWidth().Shrink(0).Children(func() {
			widget.Running(c, "Add a hook", widget.RunStyle{Size: theme.TextRead, Weight: 600})
		})
		ui.Row(c).FillWidth().Gap(theme.S3).Children(func() {
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
				widget.Field(c, "Event", func() {
					pickOne(c, "Event", 280, envsync.ClaudeHookEvents, func(ev string) string { return ev }, &e.hook.Event)
				})
			})
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
				widget.Field(c, "Matcher", func() {
					widget.Input(c, &e.hook.Matcher, "Bash").FillWidth().Tooltip("On tool events, the tools it names: Bash, Edit|Write; empty for all.")
				})
			})
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
				widget.Field(c, "Timeout (s)", func() { widget.Input(c, &e.timeout, "30").FillWidth() })
			})
		})
		widget.Field(c, "Command", func() {
			widget.Input(c, &e.hook.Command, "prettier --write \"$FILE\"").FillWidth().Font(theme.MonoFont)
		})
		ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
			if widget.Button(c, widget.Secondary, nil, "Add the hook", "").Clicked() && strings.TrimSpace(e.hook.Command) != "" {
				h := e.hook
				h.Command = strings.TrimSpace(h.Command)
				h.Timeout, _ = strconv.Atoi(strings.TrimSpace(e.timeout))
				e.hooks = append(slices.Clone(e.hooks), h)
				e.hook, e.timeout, e.dirty = envsync.ClaudeHook{Event: h.Event}, "", true
			}
		})
	})
}

// createClaudeSettings makes an empty settings.json for Claude Code, so
// its permissions and hooks can be set.
func (v *View) createClaudeSettings(o Options) {
	p := proto.EnvPayload{
		Item:  proto.EnvItem{ID: "claude/settings/settings.json", Agent: "claude", Kind: proto.EnvSettings, Name: "settings.json"},
		Home:  v.scan.Home,
		Files: []proto.EnvFile{{Path: ".claude/settings.json", Mode: 0o644, Data: []byte("{}\n")}},
	}
	v.kind = proto.EnvSettings
	v.save(o, p, "Made Claude Code's settings.json")
}
