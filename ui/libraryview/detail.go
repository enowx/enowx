package libraryview

import (
	"bytes"
	"encoding/json"
	"path"
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

// detail shows the item picked: what it is, what it holds, and what to do
// with it.
func (v *View) detail(c *ui.Context, o Options) {
	if v.selected == "" {
		ui.Column(c).Grow(1).FillWidth().Center().Children(func() {
			ui.Text(c, "Pick one to see it.").FontSize(theme.TextUI).TextColor(theme.Muted)
		})
		return
	}
	var it proto.EnvItem
	for _, i := range v.scan.Items {
		if i.ID == v.selected {
			it = i
		}
	}
	ui.Column(c).FillWidth().Padding(14, 18).Gap(8).BorderWidth(0, 0, 1, 0).BorderColor(theme.Border).Children(func() {
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, it.Name).FontSize(theme.TextRead).SingleLine()
				ui.Text(c, agentName(it.Agent)+" · "+it.Detail).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Shrink(1)
			})
			v.actions(c, o, it)
		})
		if len(it.Secrets) > 0 {
			ui.Text(c, "Holds secrets: "+strings.Join(it.Secrets, ", ")+". Sync sends them only when asked.").FontSize(theme.TextLabel).TextColor(theme.Warning)
		}
		for _, w := range it.Warnings {
			ui.Text(c, w).FontSize(theme.TextLabel).TextColor(theme.Muted)
		}
		if v.deleting {
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				ui.Text(c, "Delete "+it.Name+" from "+o.Host+"?"+deleteNote(it)).FontSize(theme.TextLabel).Grow(1)
				if widget.GhostButton(c, "Cancel").Clicked() {
					v.deleting = false
				}
				if widget.DangerButton(c, "Delete").Clicked() {
					v.delete(o, it)
				}
			})
		}
	})
	ui.Scroll(c).Grow(1).FillWidth().Children(func() {
		ui.Column(c).FillWidth().MaxWidth(820).AlignSelf(ui.Center).Padding(16, 18).Gap(14).Children(func() {
			switch {
			case v.itemErr != "":
				ui.Text(c, v.itemErr).FontSize(theme.TextLabel).TextColor(theme.Error)
			case v.item == nil || v.itemFor != it.ID:
				widget.Spinner(c, 14)
			case v.editing:
				v.editor(c, o)
			case isClaudeSettings(it):
				v.claudeSettings(c, o, *v.item)
			default:
				v.content(c, o, *v.item)
			}
		})
	})
}

// deleteNote says what deleting leaves behind.
func deleteNote(it proto.EnvItem) string {
	switch it.Kind {
	case proto.EnvMCP, proto.EnvMemory, proto.EnvSettings:
		return " Its file is kept as .enowx.bak."
	}
	return ""
}

// editable: one file, which the page edits as text.
func editable(p *proto.EnvPayload) bool {
	return p != nil && len(p.Files) == 1 && isText(p.Files[0].Data)
}

// actions edit, open and delete the item.
func (v *View) actions(c *ui.Context, o Options, it proto.EnvItem) {
	if v.editing {
		return
	}
	if editable(v.item) && v.itemFor == it.ID && widget.GhostButton(c, "Edit").Clicked() {
		v.editing, v.edited = true, string(v.item.Files[0].Data)
	}
	if o.OpenInEditor != nil && v.item != nil && v.itemFor == it.ID && len(v.item.Files) > 0 {
		if widget.IconButton(c, icon.Pencil, "Open in the editor", false).Clicked() {
			o.OpenInEditor(path.Join(v.scan.Home, v.item.Files[0].Path))
		}
	}
	if v.item != nil && v.itemFor == it.ID {
		if targets := envsync.CopyTargets(*v.item, envsync.User); len(targets) > 0 {
			b := widget.GhostButton(c, "Copy to…").Tooltip("Give it to another agent, as that agent keeps it")
			v.menu.OpenBelow(b, "copy:"+it.ID, 30)
			v.menu.Show(c, b, "copy:"+it.ID, func(done func()) {
				for _, agent := range targets {
					if widget.MenuItem(c.Key(agent), icon.Copy, agentName(agent), false) {
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
	if it.Kind != proto.EnvGit && widget.IconButton(c, icon.Trash, "Delete", false).Clicked() {
		v.deleting = true
	}
}

// content shows what the item holds: its files, Markdown rendered, or an
// MCP server's entry.
func (v *View) content(c *ui.Context, o Options, p proto.EnvPayload) {
	if len(p.Value) > 0 && p.Item.Kind == proto.EnvMCP {
		v.mcpTestPanel(c, o, p.Item)
	}
	if len(p.Value) > 0 {
		ui.Text(c, pretty(p.Value)).Font(theme.MonoFont).FontSize(theme.TextLabel).Padding(10, 12).Radius(theme.RadiusSmall).Background(theme.Sunken).FillWidth()
		return
	}
	for _, f := range p.Files {
		ui.Column(c.Key(f.Path)).FillWidth().Gap(6).Children(func() {
			if len(p.Files) > 1 {
				ui.Text(c, strings.TrimPrefix(f.Path, commonDir(p.Files)+"/")+" · "+textfmt.Bytes(uint64(len(f.Data)))).FontSize(theme.TextLabel).TextColor(theme.Muted)
			}
			switch {
			case !isText(f.Data):
				ui.Text(c, "Not text.").FontSize(theme.TextLabel).TextColor(theme.Muted)
			case strings.HasSuffix(f.Path, ".md"):
				meta, body := frontMatter(string(f.Data))
				for _, kv := range meta {
					ui.Row(c.Key("meta:" + kv[0])).FillWidth().Gap(8).Children(func() {
						ui.Text(c, kv[0]).FontSize(theme.TextLabel).TextColor(theme.Muted).Width(90).Shrink(0)
						ui.Text(c, kv[1]).FontSize(theme.TextLabel).TextColor(theme.TextSoft).Grow(1).MinWidth(0)
					})
				}
				widget.Markdown(c, clip(body))
			default:
				ui.Text(c, clip(string(f.Data))).Font(theme.MonoFont).FontSize(theme.TextLabel).Padding(10, 12).Radius(theme.RadiusSmall).Background(theme.Sunken).FillWidth()
			}
		})
	}
}

// editor edits the item's one file and saves it to the host.
func (v *View) editor(c *ui.Context, o Options) {
	ui.TextAreaBase(c, &v.edited).FillWidth().MinHeight(360).Padding(10, 12).Radius(theme.RadiusSmall).Background(theme.Sunken).
		Font(theme.MonoFont).FontSize(theme.TextLabel).Label("Content").AutoFocus()
	ui.Row(c).FillWidth().Gap(8).Justify(ui.End).Children(func() {
		if widget.GhostButton(c, "Cancel").Clicked() {
			v.editing = false
		}
		if widget.PrimaryButton(c, icon.Check, "Save", "").Clicked() {
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
			v.editing, v.notice = false, done
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
			v.selected, v.item, v.notice = "", nil, "Deleted "+it.Name
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

// frontMatter splits a Markdown file's front matter (the --- block a skill
// or a subagent starts with) into its fields, and the rest.
func frontMatter(text string) (fields [][2]string, body string) {
	rest, ok := strings.CutPrefix(text, "---\n")
	if !ok {
		return nil, text
	}
	head, body, ok := strings.Cut(rest, "\n---")
	if !ok {
		return nil, text
	}
	lines := strings.Split(head, "\n")
	for i, line := range lines {
		k, val, ok := strings.Cut(line, ":")
		if !ok || strings.HasPrefix(line, " ") {
			continue
		}
		val = strings.TrimSpace(val)
		if strings.HasPrefix(val, ">") || strings.HasPrefix(val, "|") {
			// A block: the indented lines after it, as one.
			var parts []string
			for _, next := range lines[i+1:] {
				if next != "" && !strings.HasPrefix(next, " ") {
					break
				}
				if t := strings.TrimSpace(next); t != "" {
					parts = append(parts, t)
				}
			}
			val = strings.Join(parts, " ")
		}
		fields = append(fields, [2]string{strings.TrimSpace(k), strings.Trim(val, `"'`)})
	}
	return fields, strings.TrimLeft(strings.TrimPrefix(body, "\n"), "\n")
}
