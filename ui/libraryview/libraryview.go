// Package libraryview is the Settings page's library of what the agents
// on a host use: skills, MCP servers, subagents, commands, instructions
// and settings (core/envsync), for each agent that keeps them, to look
// at, edit, delete and add to. Everything goes through the host's daemon,
// so it is the same for this device and a server.
package libraryview

import (
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

// Options connect the view to the app.
type Options struct {
	Client *client.Client
	Host   string // the host's name
	// Update runs a function on the main thread.
	Update func(func())
	// OpenInEditor opens a file of the host, by its absolute path, in the
	// user's editor; nil hides the button.
	OpenInEditor func(path string)
}

// kinds are the kinds of item the page shows, in its order, with their
// names.
var kinds = []struct {
	id, name, about string
	svg             *ui.SVG
}{
	{proto.EnvSkill, "Skills", "Folders of instructions and scripts an agent loads when a task calls for them (SKILL.md).", icon.Brain},
	{proto.EnvMCP, "MCP servers", "Tools and data an agent reaches through the Model Context Protocol.", icon.Server},
	{proto.EnvSubagent, "Subagents", "Agents an agent hands a part of its work to.", icon.Agent("claude")},
	{proto.EnvCommand, "Commands", "Prompts an agent runs as /commands.", icon.Slash},
	{proto.EnvMemory, "Instructions", "What an agent reads in every conversation: CLAUDE.md, AGENTS.md, GEMINI.md.", icon.File},
	{proto.EnvSettings, "Agent settings", "An agent's own settings file.", icon.Gauge},
}

// agentNames name the places items live.
var agentNames = map[string]string{
	"claude": "Claude Code", "codex": "Codex", "opencode": "OpenCode", "gemini": "Gemini CLI",
	"shared": "Codex & OpenCode (~/.agents)",
}

// View is the library. It belongs to the main thread; what it asks the
// daemon answers through Options.Update.
type View struct {
	scan    proto.EnvScan
	loaded  bool
	loading bool
	err     string
	notice  string

	folded   map[string]bool // "<kind>/<agent>" groups folded
	kind     string
	filter   string
	selected string

	item     *proto.EnvPayload // the selected item's content
	itemFor  string
	itemErr  string
	editing  bool
	edited   string
	deleting bool

	settings settingsEditor // Claude Code's settings.json, as lists
	test     mcpTesting     // the MCP server tested last
	menu     widget.ContextMenu
	plugins  pluginState

	mcp   *mcpForm
	skill *skillForm
}

// New makes a view showing skills first.
func New() *View { return &View{kind: proto.EnvSkill} }

// Reload asks the daemon again for what its home holds.
func (v *View) Reload(o Options) {
	if o.Client == nil || v.loading {
		return
	}
	v.loading = true
	cl := o.Client
	go func() {
		scan, err := v.fetchScan(cl)
		o.Update(func() {
			v.loading, v.loaded = false, true
			if err != nil {
				v.err = client.Explain(err)
				return
			}
			v.scan, v.err = scan, ""
			if v.selected != "" && !slices.ContainsFunc(scan.Items, func(it proto.EnvItem) bool { return it.ID == v.selected }) {
				v.selected, v.item = "", nil
			}
		})
	}()
}

// Draw shows the library: its kinds, the items of the one picked, and the
// item picked.
func (v *View) Draw(c *ui.Context, o Options) {
	if !v.loaded && !v.loading {
		v.Reload(o)
	}
	row := ui.Row(c).Grow(1).FillWidth().MinHeight(0).AlignItems(ui.Stretch)
	// Narrow, the list and the item picked take turns, rather than the
	// item squeezed beside the list.
	narrow := row.Bounds().W > 0 && row.Bounds().W < 820
	row.Children(func() {
		v.nav(c, o)
		if v.kind == kindPlugins {
			ui.Column(c).Grow(1).MinWidth(0).BorderWidth(0, 0, 0, 1).BorderColor(theme.Border).Children(func() { v.pluginsPanel(c, o) })
			return
		}
		if narrow {
			ui.Column(c).Grow(1).MinWidth(0).BorderWidth(0, 0, 0, 1).BorderColor(theme.Border).Children(func() {
				if v.selected == "" {
					v.list(c, o)
					return
				}
				ui.Row(c).FillWidth().Padding(8, 12, 0, 12).Children(func() {
					if widget.Button(c, widget.Secondary, icon.ArrowLeft, "All "+kindName(v.kind), "").Height(28).Clicked() {
						v.selected, v.item, v.editing, v.deleting = "", nil, false, false
					}
				})
				v.detail(c, o)
			})
			return
		}
		ui.Column(c).Width(300).Shrink(0).BorderWidth(0, 1, 0, 1).BorderColor(theme.Border).Children(func() {
			v.list(c, o)
		})
		ui.Column(c).Grow(1).MinWidth(0).Children(func() { v.detail(c, o) })
	})
	v.mcpDialog(c, o)
	v.skillDialog(c, o)
}

// nav lists the kinds, with how many items each has.
func (v *View) nav(c *ui.Context, o Options) {
	ui.Column(c).Width(190).Shrink(0).Padding(12, 8).Gap(2).Children(func() {
		for _, k := range kinds {
			n := len(v.items(k.id, ""))
			on := v.kind == k.id
			b := ui.ButtonBase(c.Key(k.id)).FillWidth().Height(32).PaddingX(10).Gap(9).Radius(theme.RadiusControl).
				Justify(ui.Start).Cursor(ui.CursorPointer).Label(k.name)
			switch {
			case on:
				b.Background(theme.Selected).TextColor(theme.Accent)
			case b.Hovered():
				b.Background(theme.Hover)
			}
			b.Children(func() {
				ui.Icon(c, k.svg).FontSize(13)
				ui.Text(c, k.name).FontSize(theme.TextUI).SingleLine().Grow(1)
				if n > 0 {
					ui.Text(c, strconv.Itoa(n)).FontSize(theme.TextLabel).TextColor(theme.Muted)
				}
			})
			if b.Clicked() && !on {
				v.kind, v.selected, v.item, v.editing, v.deleting = k.id, "", nil, false, false
			}
		}
		// Claude Code's plugins.
		on := v.kind == kindPlugins
		b := ui.ButtonBase(c.Key(kindPlugins)).FillWidth().Height(32).PaddingX(10).Gap(9).Radius(theme.RadiusControl).
			Justify(ui.Start).Cursor(ui.CursorPointer).Label("Plugins")
		switch {
		case on:
			b.Background(theme.Selected).TextColor(theme.Accent)
		case b.Hovered():
			b.Background(theme.Hover)
		}
		b.Children(func() {
			ui.Icon(c, icon.Agent("claude")).FontSize(13)
			ui.Text(c, "Plugins").FontSize(theme.TextUI).SingleLine().Grow(1)
			if n := len(v.plugins.list.Plugins); n > 0 {
				ui.Text(c, strconv.Itoa(n)).FontSize(theme.TextLabel).TextColor(theme.Muted)
			}
		})
		if b.Clicked() && !on {
			v.kind, v.selected, v.item = kindPlugins, "", nil
			v.plugins.loaded = false
		}
	})
}

// items are the items of kind, matching filter.
func (v *View) items(kind, filter string) []proto.EnvItem {
	var out []proto.EnvItem
	for _, it := range v.scan.Items {
		if it.Kind != kind {
			continue
		}
		if filter != "" && !strings.Contains(strings.ToLower(it.Name+" "+it.Detail), strings.ToLower(filter)) {
			continue
		}
		out = append(out, it)
	}
	return out
}

// list shows the items of the kind picked, by agent, and the way to add
// one.
func (v *View) list(c *ui.Context, o Options) {
	about := ""
	for _, k := range kinds {
		if k.id == v.kind {
			about = k.about
		}
	}
	ui.Column(c).FillWidth().Padding(12, 12, 8, 12).Gap(8).BorderWidth(0, 0, 1, 0).BorderColor(theme.Border).Children(func() {
		ui.Text(c, about).FontSize(theme.TextLabel).TextColor(theme.Muted)
		ui.Row(c).FillWidth().Gap(6).Children(func() {
			widget.SearchField(c, &v.filter, "Filter", icon.Search)
			switch v.kind {
			case proto.EnvMCP:
				if widget.IconButton(c, icon.Plus, "Add an MCP server", false).Clicked() {
					v.mcp = newMCPForm()
				}
			case proto.EnvSkill:
				if widget.IconButton(c, icon.Plus, "Add a skill from a folder of this device", false).Clicked() {
					v.skill = &skillForm{open: true, agent: "claude"}
				}
			}
			if widget.IconButton(c, icon.Sync, "Read again", false).Clicked() {
				v.Reload(o)
			}
		})
		if v.notice != "" {
			ui.Text(c, v.notice).FontSize(theme.TextLabel).TextColor(theme.Done)
		}
	})
	ui.Scroll(c).Grow(1).FillWidth().Children(func() {
		ui.Column(c).FillWidth().Padding(8).Gap(2).Children(func() {
			switch {
			case v.err != "":
				ui.Text(c, v.err).FontSize(theme.TextLabel).TextColor(theme.Error).Padding(6)
				return
			case !v.loaded:
				ui.Row(c).Gap(8).Padding(6).Children(func() {
					widget.Spinner(c, 14)
					ui.Text(c, "Reading "+o.Host+"…").FontSize(theme.TextLabel).TextColor(theme.Muted)
				})
				return
			}
			items := v.items(v.kind, v.filter)
			if v.kind == proto.EnvSettings && !slices.ContainsFunc(items, isClaudeSettings) && v.filter == "" {
				if widget.GhostButton(c, "Make Claude Code's settings.json").Tooltip("To set its permissions and hooks").Clicked() {
					v.createClaudeSettings(o)
				}
			}
			if len(items) == 0 {
				ui.Text(c, "None on "+o.Host+".").FontSize(theme.TextLabel).TextColor(theme.Muted).Padding(6)
				return
			}
			// By agent, each group folding, so a long list reads.
			var agents []string
			byAgent := map[string][]proto.EnvItem{}
			for _, it := range items {
				if _, ok := byAgent[it.Agent]; !ok {
					agents = append(agents, it.Agent)
				}
				byAgent[it.Agent] = append(byAgent[it.Agent], it)
			}
			for _, agent := range agents {
				open := !v.folded[v.kind+"/"+agent] || v.filter != ""
				ui.Column(c.Key("agent:" + agent)).FillWidth().Children(func() {
					widget.Group(c, agentName(agent), len(byAgent[agent]), &open, func() {
						for _, it := range byAgent[agent] {
							v.row(c.Key(it.ID), o, it)
						}
					})
				})
				if v.filter == "" {
					if v.folded == nil {
						v.folded = map[string]bool{}
					}
					v.folded[v.kind+"/"+agent] = !open
				}
			}
		})
	})
}

func (v *View) row(c *ui.Context, o Options, it proto.EnvItem) {
	on := v.selected == it.ID
	b := ui.ButtonBase(c).FillWidth().Padding(6, 10).Radius(theme.RadiusControl).
		Justify(ui.Start).Cursor(ui.CursorPointer).Label(it.Name)
	switch {
	case on:
		b.Background(theme.Selected)
	case b.Hovered():
		b.Background(theme.Hover)
	}
	b.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Row(c).FillWidth().Gap(6).Children(func() {
				ui.Text(c, it.Name).FontSize(theme.TextUI).SingleLine().Shrink(1)
				if len(it.Secrets) > 0 {
					ui.Icon(c, icon.Lock).FontSize(11).TextColor(theme.Warning)
				}
			})
			// A path that only repeats the name (~/.claude/skills/<name>)
			// says nothing the group doesn't: the detail has it.
			if it.Detail != "" && !strings.HasSuffix(it.Detail, "/"+it.Name) {
				ui.Text(c, it.Detail).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Shrink(1)
			}
		})
	})
	if b.Clicked() && !on {
		v.selected, v.item, v.itemErr, v.editing, v.deleting = it.ID, nil, "", false, false
		v.load(o, it.ID)
	}
}

func agentName(id string) string {
	if n, ok := agentNames[id]; ok {
		return n
	}
	return id
}

// kindName is kind id's name, in the plural.
func kindName(id string) string {
	for _, k := range kinds {
		if k.id == id {
			return strings.ToLower(k.name)
		}
	}
	return "items"
}
