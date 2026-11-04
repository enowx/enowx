// Package libraryview is the Settings page's library of what the agents
// on a host use: skills, MCP servers, subagents, commands, instructions
// and settings (core/envsync), for each agent that keeps them, to look
// at, edit, delete and add to. Everything goes through the host's daemon,
// so it is the same for this device and a server.
package libraryview

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

// Options connect the view to the app.
type Options struct {
	Client *client.Client
	Host   string // the host's name
	// Update runs a function on the main thread.
	Update func(func())
	// OpenInEditor opens a file of the host, by its absolute path, in the
	// user's editor; nil hides the button.
	OpenInEditor func(path string)
	// Sync copies items to another host; nil hides the button.
	Sync func()
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

	asked    string            // the item whose content was asked for last
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

// Draw heads the frame with the library, then shows its kinds with their
// counts, the items of the one picked and the item picked. The window's least
// width leaves this frame over 820, so the list and the item always sit side
// by side.
func (v *View) Draw(c *ui.Context, o Options) {
	if !v.loaded && !v.loading {
		v.Reload(o)
	}
	if !v.plugins.loaded { // its figure shows from the start
		v.plugins.loaded = true // asked once; a reply sets it again
		v.loadPlugins(o)
	}
	ui.Column(c.Key("library")).Grow(1).FillWidth().MinHeight(0).Children(func() {
		widget.PageHeader(c, "Agents' library", "On "+o.Host, func() {
			if o.Sync != nil && widget.Button(c, widget.Secondary, nil, "Sync…", "").Tooltip("Copy items to another host").Clicked() {
				o.Sync()
			}
			if widget.Button(c, widget.Secondary, nil, "Add a skill", "").Clicked() {
				v.skill = &skillForm{open: true, agent: "claude"}
			}
			if widget.PrimaryButton(c, icon.Plus, "Add an MCP server", "").Clicked() {
				v.mcp = newMCPForm()
			}
		})
		v.nav(c)
		widget.PageFill(c, func() { v.body(c, o) })
	})
	v.mcpDialog(c, o)
	v.skillDialog(c, o)
}

// nav shows the kinds, with how many items each has; while they aren't known
// yet, no figure claims there are none.
func (v *View) nav(c *ui.Context) {
	figs := make([]widget.Fig, 0, len(kinds)+1)
	cur := len(kinds)
	for i, k := range kinds {
		n := "–"
		if v.loaded && v.err == "" {
			n = strconv.Itoa(len(v.items(k.id, "")))
		}
		figs = append(figs, widget.Fig{Label: k.name, Value: n})
		if v.kind == k.id {
			cur = i
		}
	}
	plugins := "–"
	if v.plugins.read {
		plugins = strconv.Itoa(len(v.plugins.list.Plugins))
	}
	figs = append(figs, widget.Fig{Label: "Plugins", Value: plugins})
	ui.Column(c.Key("library-kinds")).FillWidth().Shrink(0).Padding(theme.S4, theme.S5, 0, theme.S5).Children(func() {
		i := widget.FigsNav(c, figs, cur)
		switch {
		case i < 0 || i == cur:
		case i == len(kinds):
			v.kind = kindPlugins
			v.clearPick()
			v.plugins.loaded = false // read again
		default:
			v.kind = kinds[i].id
			v.clearPick()
		}
	})
}

// clearPick leaves no item picked.
func (v *View) clearPick() {
	v.selected, v.asked, v.item, v.itemErr, v.editing, v.deleting = "", "", nil, "", false, false
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

// body is what shows under the kinds.
func (v *View) body(c *ui.Context, o Options) {
	switch {
	case v.kind == kindPlugins:
		v.pluginsPanel(c, o)
	case v.err != "":
		widget.Notice(c, icon.Alert, theme.Error, "Can't read "+o.Host, v.err, nil)
	case !v.loaded:
		ui.Column(c).Grow(1).FillWidth().Center().Gap(theme.S2).Children(func() {
			widget.Spinner(c, 20)
			ui.Text(c, "Reading "+o.Host+"…").FontSize(theme.TextUI).TextColor(theme.TextSoft)
		})
	case v.kind == proto.EnvSettings:
		v.settingsKind(c, o)
	default:
		v.listed(c, o)
	}
}

// kindAbout is what the kind picked holds.
func (v *View) kindAbout() string {
	for _, k := range kinds {
		if k.id == v.kind {
			return k.about
		}
	}
	return ""
}

// noticeLine says how the last change went.
func (v *View) noticeLine(c *ui.Context) {
	if v.notice != "" {
		ui.Row(c.Key("notice")).FillWidth().Shrink(0).Children(func() {
			widget.Running(c, v.notice, widget.RunStyle{Color: theme.Done})
		})
	}
}

// aboutLine is a quiet line that takes the room it's given.
func aboutLine(c *ui.Context, text string) {
	ui.Row(c.Key("about")).Grow(1).FillWidth().Basis(0).MinWidth(0).Children(func() {
		widget.Running(c, text, widget.RunStyle{Color: theme.TextSoft})
	})
}

// listed shows the items of the kind picked by agent, beside the one picked.
func (v *View) listed(c *ui.Context, o Options) {
	ui.Row(c.Key("tools")).FillWidth().Shrink(0).Gap(theme.S3).Children(func() {
		aboutLine(c, "Under the home folder of "+o.Host)
		widget.Input(c, &v.filter, "Filter").Width(240).Shrink(0)
		if widget.Button(c, widget.Secondary, icon.Sync, "Refresh", "").Clicked() {
			v.Reload(o)
		}
	})
	ui.Text(c.Key("kind-about"), v.kindAbout()).FontSize(theme.TextUI).TextColor(theme.TextSoft).FillWidth().Shrink(0)
	v.noticeLine(c)
	if len(v.items(v.kind, "")) == 0 {
		noneHere(c, o)
		return
	}
	items := v.items(v.kind, v.filter)
	v.autoPick(items)
	widget.MasterDetail(c, 320, func() { v.list(c, o, items) }, func() { v.detail(c, o) })
}

// noneHere fills the frame when the host has none of the kind.
func noneHere(c *ui.Context, o Options) {
	ui.Column(c.Key("none")).Grow(1).FillWidth().Center().Children(func() {
		ui.Text(c, "None on "+o.Host+".").FontSize(theme.TextUI).TextColor(theme.TextSoft)
	})
}

// autoPick picks the first of the kind when none is, so the frame shows an
// item from the start. Not while reading: what was just deleted is still
// listed.
func (v *View) autoPick(items []proto.EnvItem) {
	if v.selected == "" && !v.loading && len(items) > 0 {
		v.selected = items[0].ID
	}
}

// list shows items by agent, each group folding so a long list reads; a
// filter being typed opens them all.
func (v *View) list(c *ui.Context, o Options, items []proto.EnvItem) {
	if len(items) == 0 {
		ui.Text(c.Key("no-match"), "None on "+o.Host+".").FontSize(theme.TextUI).TextColor(theme.TextSoft).Padding(theme.S1, 0)
		return
	}
	var agents []string
	byAgent := map[string][]proto.EnvItem{}
	for _, it := range items {
		if _, ok := byAgent[it.Agent]; !ok {
			agents = append(agents, it.Agent)
		}
		byAgent[it.Agent] = append(byAgent[it.Agent], it)
	}
	if v.folded == nil {
		v.folded = map[string]bool{}
	}
	for _, agent := range agents {
		group := v.kind + "/" + agent
		open := !v.folded[group] || v.filter != ""
		if groupHead(c.Key("agent:"+agent), agentName(agent), len(byAgent[agent]), open) && v.filter == "" {
			v.folded[group] = open
			open = !open
		}
		if open {
			for _, it := range byAgent[agent] {
				v.row(c.Key(it.ID), it)
			}
		}
	}
}

// groupHead names an agent's group, over its items, and folds it when clicked.
func groupHead(c *ui.Context, name string, n int, open bool) bool {
	title := fmt.Sprintf("%s · %d", name, n)
	h := ui.ButtonBase(c).FillWidth().Shrink(0).MinHeight(20).Gap(theme.S1).Justify(ui.Start).Cursor(ui.CursorPointer).Label(title)
	hovered := h.Hovered()
	h.Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Children(func() {
			widget.Running(c, title, widget.RunStyle{Weight: 500, Color: theme.TextSoft})
		})
		// The way to unfold shows always, the way to fold under the pointer.
		if !open {
			ui.Icon(c, icon.ChevronRight).FontSize(12).TextColor(theme.TextSoft)
		} else if hovered {
			ui.Icon(c, icon.ChevronDown).FontSize(12).TextColor(theme.TextSoft)
		}
	})
	return h.Clicked()
}

func (v *View) row(c *ui.Context, it proto.EnvItem) {
	var lock func()
	if len(it.Secrets) > 0 {
		lock = func() { ui.Icon(c, icon.Lock).FontSize(12).TextColor(theme.Warning) }
	}
	on := v.selected == it.ID
	if widget.Pick(c, nil, shownName(it), it.Detail, on, lock).Clicked() && !on {
		v.selected = it.ID
	}
}

// pickOne is a drop-down of values, shown by their names, holding its choice
// in current.
func pickOne(c *ui.Context, label string, width float32, values []string, name func(string) string, current *string) {
	sel := ui.SelectBase(c, current)
	widget.Trigger(sel.Trigger, sel.Open()).Label(label).Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, name(*current), widget.RunStyle{}) })
		ui.Icon(c, icon.ChevronDown).FontSize(12).TextColor(theme.TextSoft)
	})
	sel.Popup(func(panel ui.Element) {
		widget.Popup(panel.Margin(theme.S2, 0, 0, 0).Width(width))
		widget.Scroll(c).FillWidth().MaxHeight(widget.PopupRoom(c, sel.Trigger, 300)).Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				for _, val := range values {
					item := sel.Item(val)
					widget.PopupItem(item, item.Highlighted()).Children(func() { ui.Text(c, name(val)).FontSize(theme.TextUI) })
				}
			})
		})
	})
}

func agentName(id string) string {
	if n, ok := agentNames[id]; ok {
		return n
	}
	return id
}
