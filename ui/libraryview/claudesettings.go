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

// claudeSettings shows the settings.json picked as lists to edit.
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
	if i := widget.Segmented(c.Key("tab"), []string{"Permissions", "Hooks", "File"}, e.tab, nil); i >= 0 {
		e.tab = i
	}
	if e.err != "" {
		ui.Text(c, e.err).FontSize(theme.TextLabel).TextColor(theme.Error)
	}
	switch e.tab {
	case tabPermissions:
		v.permissions(c, e)
	case tabHooks:
		v.hookList(c, e)
	default:
		v.content(c, o, p)
		return
	}
	if e.dirty {
		ui.Row(c).FillWidth().Gap(8).Justify(ui.End).Children(func() {
			if widget.GhostButton(c, "Discard").Clicked() {
				e.forID = "" // read again
			}
			if widget.PrimaryButton(c, icon.Check, "Save", "").Clicked() {
				data, err := envsync.WriteClaudeSettings(p.Files[0].Data, e.perm, e.hooks)
				if err != nil {
					e.err = err.Error()
					return
				}
				out := p
				f := p.Files[0]
				f.Data = data
				out.Files, out.Home = []proto.EnvFile{f}, v.scan.Home
				e.forID = ""
				v.save(o, out, "Saved "+p.Item.Name)
			}
		})
	}
}

// permissions edits the allow, ask and deny rules and the starting mode.
func (v *View) permissions(c *ui.Context, e *settingsEditor) {
	ui.Text(c, "Rules are a tool and what it works on: Bash(npm run test:*), Read(./.env), Edit(src/**), WebFetch(domain:docs.rs), mcp__github.").
		FontSize(11.5).TextColor(theme.Muted)
	widget.Field(c, "Starts in", func() {
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
		col         ui.Color
	}{
		{"Allow", "run without asking", &e.perm.Allow, theme.Done},
		{"Ask", "always ask first", &e.perm.Ask, theme.Warning},
		{"Deny", "never", &e.perm.Deny, theme.Error},
	}
	for i, l := range lists {
		widget.Field(c.Key(l.name), l.name+" · "+l.about, func() {
			ui.Column(c).FillWidth().Gap(4).Children(func() {
				for j, r := range *l.rules {
					ui.Row(c.Key(strconv.Itoa(j))).FillWidth().Gap(8).Padding(4, 8).Radius(theme.RadiusSmall).Background(theme.Sunken).Children(func() {
						widget.Dot(c, l.col, false, 6)
						ui.Text(c, r).Font(theme.MonoFont).FontSize(theme.TextLabel).Grow(1).MinWidth(0)
						if widget.IconButton(c, icon.Close, "Remove", false).Clicked() {
							*l.rules = slices.Delete(slices.Clone(*l.rules), j, j+1)
							e.dirty = true
						}
					})
				}
				ui.Row(c).FillWidth().Gap(6).Children(func() {
					add := widget.Input(c, &e.newRule[i], "Bash(git status)").FillWidth().Font(theme.MonoFont).Submitted()
					if widget.IconButton(c, icon.Plus, "Add the rule", false).Clicked() || add {
						if r := strings.TrimSpace(e.newRule[i]); r != "" && !slices.Contains(*l.rules, r) {
							*l.rules = append(slices.Clone(*l.rules), r)
							e.newRule[i], e.dirty = "", true
						}
					}
				})
			})
		})
	}
}

// hookList edits the hooks: what runs on which event.
func (v *View) hookList(c *ui.Context, e *settingsEditor) {
	ui.Text(c, "Each runs a shell command on an event; on tool events, for the tools its matcher names (Bash, Edit|Write; empty for all).").
		FontSize(11.5).TextColor(theme.Muted)
	if len(e.hooks) == 0 {
		ui.Text(c, "No hooks yet.").FontSize(theme.TextLabel).TextColor(theme.Muted)
	}
	for i, h := range e.hooks {
		ui.Row(c.Key(strconv.Itoa(i))).FillWidth().Gap(10).Padding(6, 10).Radius(theme.RadiusSmall).Background(theme.Sunken).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				label := h.Event
				if h.Matcher != "" {
					label += " · " + h.Matcher
				}
				if h.Timeout > 0 {
					label += " · " + strconv.Itoa(h.Timeout) + "s"
				}
				ui.Text(c, label).FontSize(theme.TextLabel).TextColor(theme.Muted)
				cmd := h.Command
				if cmd == "" {
					cmd, _ = h.Hook["type"].(string)
				}
				ui.Text(c, cmd).Font(theme.MonoFont).FontSize(theme.TextLabel)
			})
			if widget.IconButton(c, icon.Close, "Remove the hook", false).Clicked() {
				e.hooks = slices.Delete(slices.Clone(e.hooks), i, i+1)
				e.dirty = true
			}
		})
	}
	widget.Card(c).Children(func() {
		ui.Text(c, "Add a hook").FontSize(theme.TextUI)
		widget.Field(c, "On", func() {
			ui.Row(c).FillWidth().Gap(4).Wrap().Children(func() {
				for _, ev := range envsync.ClaudeHookEvents {
					on := e.hook.Event == ev
					b := ui.ButtonBase(c.Key(ev)).Height(24).PaddingX(8).Radius(theme.RadiusControl).Border(1, theme.Border).
						Cursor(ui.CursorPointer).Label(ev)
					if on {
						b.Border(1, theme.AccentLine).Background(theme.Selected)
					}
					b.Children(func() { ui.Text(c, ev).FontSize(theme.TextLabel) })
					if b.Clicked() {
						e.hook.Event = ev
					}
				}
			})
		})
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			ui.Column(c).Width(160).Children(func() {
				widget.Field(c, "Matcher", func() { widget.Input(c, &e.hook.Matcher, "Bash").FillWidth().Font(theme.MonoFont) })
			})
			ui.Column(c).Grow(1).Children(func() {
				widget.Field(c, "Command", func() {
					widget.Input(c, &e.hook.Command, "npx prettier --write \"$FILE\"").FillWidth().Font(theme.MonoFont)
				})
			})
			ui.Column(c).Width(90).Children(func() {
				widget.Field(c, "Timeout (s)", func() { widget.Input(c, &e.timeout, "60").FillWidth() })
			})
		})
		ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
			if widget.Button(c, widget.Secondary, icon.Plus, "Add", "").Clicked() && strings.TrimSpace(e.hook.Command) != "" {
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
