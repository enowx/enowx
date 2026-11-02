package libraryview

import (
	"bytes"
	"encoding/json"
	"path"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/envsync"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// maxShown bounds how much of a file the detail shows.
const maxShown = 200 << 10

// load asks the daemon for item id's content.
func (v *View) load(o Options, id string) {
	v.asked = id
	cl := o.Client
	if cl == nil {
		return
	}
	go func() {
		p, err := v.fetchItem(cl, id)
		o.Update(func() {
			if v.selected != id {
				return
			}
			if err != nil {
				v.itemErr = client.Explain(err)
				return
			}
			v.item, v.itemFor = &p, id
		})
	}()
}

// detail shows the item picked as a card filling the height: what it is, what
// it holds, and what to do with it.
func (v *View) detail(c *ui.Context, o Options) {
	if v.selected == "" {
		ui.Column(c.Key("nothing-picked")).Grow(1).FillWidth().Center().Children(func() {
			ui.Text(c, "Pick one to see it.").FontSize(theme.TextUI).TextColor(theme.TextSoft)
		})
		return
	}
	if v.asked != v.selected {
		v.item, v.itemErr, v.editing, v.deleting = nil, "", false, false
		v.load(o, v.selected)
	}
	var it proto.EnvItem
	for _, i := range v.scan.Items {
		if i.ID == v.selected {
			it = i
		}
	}
	loaded := v.item != nil && v.itemFor == v.selected
	if it.ID == "" { // just made: the scan doesn't list it yet
		if loaded {
			it = v.item.Item
		} else {
			it.ID, it.Name = v.selected, v.selected
		}
	}
	card := widget.Card(c.Key("item:" + it.ID)).Grow(1).Basis(0).FillWidth().MinHeight(0).Clip()
	card.Children(func() {
		if loaded && !v.editing && isClaudeSettings(it) {
			v.claudeSettings(c, o, *v.item)
			return
		}
		ui.Row(c.Key("item-head")).FillWidth().Shrink(0).MinHeight(theme.ControlHeight).Gap(theme.S2).Children(func() {
			ui.Row(c).Grow(1).MinWidth(0).Children(func() {
				widget.Running(c, shownName(it), widget.RunStyle{Size: theme.TextRead, Weight: 600, Always: true})
			})
			v.actions(c, o, it, false)
		})
		v.facts(c, it)
		v.itemNotes(c, it)
		switch {
		case v.editing:
			v.editor(c, o)
		case loaded:
			v.content(c, o, *v.item)
		case v.itemErr == "":
			ui.Column(c.Key("item-loading")).Grow(1).FillWidth().Center().Children(func() { widget.Spinner(c, 20) })
		}
	})
	if v.deleting && widget.AlertDialog(c, &v.deleting, "Delete "+it.Name+"?", "It goes from "+o.Host+", for every agent reading it."+deleteNote(it), "Cancel", "Delete") == 1 {
		v.delete(o, it)
	}
}

// facts says whose the item is and where it lives, or what it runs.
func (v *View) facts(c *ui.Context, it proto.EnvItem) {
	ui.Column(c.Key("item-facts")).FillWidth().Shrink(0).Children(func() {
		widget.KV(c, "Agent", agentName(it.Agent), false)
		if it.Detail == "" {
			return
		}
		label := "File"
		switch {
		case it.Kind == proto.EnvSkill:
			label = "Folder"
		case it.Kind == proto.EnvMCP && strings.Contains(it.Detail, "://"):
			label = "URL"
		case it.Kind == proto.EnvMCP:
			label = "Command"
		}
		widget.KV(c, label, it.Detail, true)
	})
}

// itemNotes says what stands out about the item, and what went wrong with it.
func (v *View) itemNotes(c *ui.Context, it proto.EnvItem) {
	if len(it.Secrets) > 0 {
		widget.Banner(c, theme.Warning, "Holds "+strings.Join(it.Secrets, ", ")+". Sync sends them only when asked.", nil)
	}
	for i, w := range it.Warnings {
		ui.Text(c.Key("warning:"+strconv.Itoa(i)), w).FontSize(theme.TextLabel).TextColor(theme.TextSoft).FillWidth().Shrink(0)
	}
	if v.itemErr != "" {
		ui.Text(c.Key("item-error"), v.itemErr).FontSize(theme.TextUI).TextColor(theme.Error).FillWidth().Shrink(0)
	}
}

// deleteNote says what deleting leaves behind.
func deleteNote(it proto.EnvItem) string {
	switch it.Kind {
	case proto.EnvMCP, proto.EnvMemory, proto.EnvSettings:
		return " Its file is kept as .enowx.bak."
	}
	return " A deleted file comes back only from a backup."
}

// editable: one file, which the page edits as text.
func editable(p *proto.EnvPayload) bool {
	return p != nil && len(p.Files) == 1 && isText(p.Files[0].Data)
}

// actions edit, open, copy and delete the item. Compact, beside what Claude
// Code's settings.json shows first, editing the file waits for its File tab
// and opening it is an icon.
func (v *View) actions(c *ui.Context, o Options, it proto.EnvItem, compact bool) {
	if v.editing {
		return
	}
	loaded := v.item != nil && v.itemFor == it.ID
	if loaded && editable(v.item) && (!compact || v.settings.tab == tabFile) && widget.Button(c, widget.Secondary, icon.Pencil, "Edit", "").Clicked() {
		v.editing, v.edited = true, string(v.item.Files[0].Data)
	}
	if o.OpenInEditor != nil && loaded && len(v.item.Files) > 0 {
		var open ui.Element
		if compact {
			open = widget.IconButton(c, icon.External, "Open in editor", false)
		} else {
			open = widget.Button(c, widget.Secondary, icon.External, "Open in editor", "")
		}
		if open.Clicked() {
			o.OpenInEditor(path.Join(v.scan.Home, v.item.Files[0].Path))
		}
	}
	if loaded {
		if targets := envsync.CopyTargets(*v.item, envsync.User); len(targets) > 0 {
			b := widget.IconButton(c.Key("copy"), icon.More, "Copy to another agent", false)
			v.menu.OpenBelow(b, "copy:"+it.ID, theme.ControlHeight)
			v.menu.Show(c, b, "copy:"+it.ID, func(done func()) {
				for _, agent := range targets {
					if widget.MenuItem(c.Key(agent), icon.Copy, "To "+agentName(agent), false) {
						done()
						out, err := envsync.CopyTo(*v.item, agent, envsync.User)
						if err != nil {
							v.itemErr = err.Error()
							return
						}
						out.Home = v.scan.Home
						v.save(o, out, "Copied "+it.Name+" to "+agentName(agent))
					}
				}
			})
		}
	}
	if it.Kind != proto.EnvGit && widget.IconButton(c.Key("delete"), icon.Trash, "Delete "+it.Name, false).Clicked() {
		v.deleting = true
	}
}

// codeFill is a file's text on the code ground, taking the height that is
// left and scrolling inside it.
func codeFill(c *ui.Context, key string, fn func()) {
	ui.Scroll(c.Key(key)).Grow(1).Basis(0).FillWidth().MinHeight(96).Radius(theme.RadiusCard).Clip().
		Background(theme.CodeGround).InsetShadow(0, 0, 0, 0.5, theme.Border).Children(func() {
		ui.Column(c).FillWidth().Padding(theme.S3, theme.S4).Gap(theme.S3).Children(fn)
	})
}

// codeLine is text in codeFill.
func codeLine(c *ui.Context, text string, col ui.Color) {
	ui.Text(c, text).Font(theme.MonoFont).FontSize(theme.TextUI).TextColor(col).FillWidth().MinWidth(0)
}

// content shows what the item holds: its files, or an MCP server's entry with
// the way to test it.
func (v *View) content(c *ui.Context, o Options, p proto.EnvPayload) {
	if len(p.Value) > 0 && p.Item.Kind == proto.EnvMCP {
		v.mcpTestPanel(c, o, p.Item)
	}
	codeFill(c, "code:"+p.Item.ID, func() {
		if len(p.Value) > 0 {
			codeLine(c, pretty(p.Value), widget.CodeText)
			return
		}
		quiet := theme.Fade(widget.CodeText, 0.6)
		for _, f := range p.Files {
			ui.Column(c.Key(f.Path)).FillWidth().Gap(theme.S1).Children(func() {
				if len(p.Files) > 1 {
					ui.Text(c, strings.TrimPrefix(f.Path, commonDir(p.Files)+"/")+" · "+textfmt.Bytes(uint64(len(f.Data)))).FontSize(theme.TextLabel).TextColor(quiet)
				}
				if isText(f.Data) {
					codeLine(c, clip(string(f.Data)), widget.CodeText)
				} else {
					codeLine(c, "Not text.", quiet)
				}
			})
		}
	})
}

// editor edits the item's one file and saves it to the host.
func (v *View) editor(c *ui.Context, o Options) {
	area := ui.TextAreaBase(c.Key("editor"), &v.edited).Grow(1).Basis(0).FillWidth().MinHeight(96).Padding(theme.S3, theme.S4).Radius(theme.RadiusCard).
		Font(theme.MonoFont).FontSize(theme.TextUI).Label("Content").AutoFocus()
	widget.Well(area, area.Focused())
	ui.Row(c.Key("editor-buttons")).FillWidth().Shrink(0).Gap(theme.S2).Justify(ui.End).Children(func() {
		if widget.Button(c, widget.Secondary, nil, "Cancel", "").Clicked() {
			v.editing = false
		}
		if widget.PrimaryButton(c, nil, "Save", "").Clicked() {
			p := *v.item
			f := p.Files[0]
			f.Data = []byte(v.edited)
			p.Files = []proto.EnvFile{f}
			p.Home = v.scan.Home
			v.save(o, p, "Saved "+p.Item.Name)
		}
	})
}

// save writes p to the host, then reads the library again.
func (v *View) save(o Options, p proto.EnvPayload, done string) {
	cl := o.Client
	if cl == nil {
		return
	}
	go func() {
		res, err := v.writeItem(cl, p)
		o.Update(func() {
			if err != nil {
				v.itemErr = client.Explain(err)
				return
			}
			v.editing, v.itemErr, v.notice = false, "", done
			if len(res.Warnings) > 0 {
				v.notice += ": " + strings.Join(res.Warnings, " ")
			}
			v.selected, v.item = p.Item.ID, nil
			v.load(o, p.Item.ID)
			v.Reload(o)
		})
	}()
}

// delete deletes it from the host.
func (v *View) delete(o Options, it proto.EnvItem) {
	cl := o.Client
	if cl == nil {
		return
	}
	go func() {
		err := v.deleteItem(cl, it.ID)
		o.Update(func() {
			v.deleting = false
			if err != nil {
				v.itemErr = client.Explain(err)
				return
			}
			v.clearPick()
			v.notice = "Deleted " + it.Name
			v.Reload(o)
		})
	}()
}

// pretty indents a JSON value; TOML shows as it is.
func pretty(value []byte) string {
	var b bytes.Buffer
	if json.Indent(&b, value, "", "  ") == nil {
		return b.String()
	}
	return string(value)
}

func clip(s string) string {
	if len(s) > maxShown {
		return s[:maxShown] + "\n…"
	}
	return s
}

func isText(data []byte) bool { return !bytes.ContainsRune(data[:min(len(data), 8000)], 0) }

// commonDir is the folder the files share: a skill's, a subagent's.
func commonDir(files []proto.EnvFile) string {
	if len(files) == 0 {
		return ""
	}
	dir := path.Dir(files[0].Path)
	for _, f := range files[1:] {
		for dir != "." && !strings.HasPrefix(f.Path, dir+"/") {
			dir = path.Dir(dir)
		}
	}
	return dir
}

// shownName is an item's name to read: a command as it's typed in the agent.
func shownName(it proto.EnvItem) string {
	if it.Kind == proto.EnvCommand {
		return "/" + strings.TrimSuffix(strings.TrimSuffix(it.Name, ".md"), ".toml")
	}
	return it.Name
}
