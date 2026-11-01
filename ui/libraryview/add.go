package libraryview

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/envsync"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// maxSkill bounds a skill folder read to add: it goes in one message.
const maxSkill = 12 << 20

// mcpForm is the Add an MCP server dialog.
type mcpForm struct {
	open    bool
	agent   string
	name    string
	byURL   bool
	command string // the command line, as typed in a shell
	url     string
	env     string // KEY=VALUE lines
	err     string
}

func newMCPForm() *mcpForm { return &mcpForm{open: true, agent: "claude"} }

// agentPicker picks one of agents.
func agentPicker(c *ui.Context, agents []string, current *string) {
	labels := make([]string, len(agents))
	for i, a := range agents {
		labels[i] = agentName(a)
	}
	if i := widget.Segmented(c, labels, slices.Index(agents, *current), nil); i >= 0 {
		*current = agents[i]
	}
}

func (v *View) mcpDialog(c *ui.Context, o Options) {
	f := v.mcp
	if f == nil {
		return
	}
	if !f.open {
		v.mcp = nil
		return
	}
	submit := false
	widget.Dialog(c, &f.open, 540, 14, func() {
		widget.DialogHeader(c, "Add an MCP server on "+o.Host, "Written into the agent's own config, as it keeps its servers; the file is backed up first.", false)
		widget.Field(c, "For", func() { agentPicker(c, envsync.AgentsIn(proto.EnvMCP, envsync.User), &f.agent) })
		widget.Field(c, "Name", func() { submit = widget.Input(c, &f.name, "docs").FillWidth().Submitted() || submit })
		widget.Field(c, "It", func() {
			cur := 0
			if f.byURL {
				cur = 1
			}
			if i := widget.Segmented(c, []string{"Runs a command", "Serves at a URL"}, cur, nil); i >= 0 {
				f.byURL = i == 1
			}
		})
		if f.byURL {
			widget.Field(c, "URL", func() {
				submit = widget.Input(c, &f.url, "https://mcp.example.com/mcp").FillWidth().Font(theme.MonoFont).Submitted() || submit
			})
		} else {
			widget.Field(c, "Command", func() {
				submit = widget.Input(c, &f.command, "npx -y @modelcontextprotocol/server-filesystem ~/code").FillWidth().Font(theme.MonoFont).Submitted() || submit
			})
		}
		widget.Field(c, "Environment (optional)", func() {
			ui.TextAreaBase(c, &f.env).FillWidth().MinHeight(56).Padding(8, 10).Radius(theme.RadiusControl).
				Background(theme.Sunken).Font(theme.MonoFont).FontSize(theme.TextLabel).Placeholder("API_KEY=…  one per line").Label("Environment")
		})
		if f.err != "" {
			ui.Text(c, f.err).FontSize(theme.TextLabel).TextColor(theme.Error)
		}
		widget.DialogFooter(c, "", func() {
			if widget.GhostButton(c, "Cancel").Clicked() {
				f.open = false
			}
			if widget.PrimaryButton(c, icon.Plus, "Add", "").Clicked() {
				submit = true
			}
		})
	})
	if !submit {
		return
	}
	srv := envsync.MCPServer{Env: map[string]string{}}
	if f.byURL {
		srv.URL = strings.TrimSpace(f.url)
	} else if args := splitCommand(f.command); len(args) > 0 {
		srv.Command, srv.Args = args[0], args[1:]
	}
	for _, line := range strings.Split(f.env, "\n") {
		if k, val, ok := strings.Cut(strings.TrimSpace(line), "="); ok && k != "" {
			srv.Env[strings.TrimSpace(k)] = strings.TrimSpace(val)
		}
	}
	p, err := envsync.NewMCPIn(f.agent, strings.TrimSpace(f.name), srv, envsync.User)
	if err != nil {
		f.err = strings.TrimPrefix(err.Error(), "envsync: ")
		return
	}
	p.Home = v.scan.Home
	f.open = false
	v.kind = proto.EnvMCP
	v.save(o, p, "Added "+p.Item.Name+" to "+agentName(f.agent))
}

// splitCommand splits a command line as a shell would its words, quotes
// kept together.
func splitCommand(s string) []string {
	var out []string
	var cur strings.Builder
	quote, in := rune(0), false
	for _, r := range s {
		switch {
		case quote != 0 && r == quote:
			quote = 0
		case quote == 0 && (r == '"' || r == '\''):
			quote, in = r, true
		case quote == 0 && (r == ' ' || r == '\t'):
			if in {
				out, in = append(out, cur.String()), false
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
			in = true
		}
	}
	if in {
		out = append(out, cur.String())
	}
	return out
}

// skillForm is the Add a skill dialog: from a folder of this device, or
// from a git repository the host clones.
type skillForm struct {
	open     bool
	agent    string
	err      string
	fromRepo bool
	url      string
	fetching bool
	fetch    *proto.SkillFetch
	picked   map[string]bool // found skills, by path
}

func (v *View) skillDialog(c *ui.Context, o Options) {
	f := v.skill
	if f == nil {
		return
	}
	if !f.open {
		v.skill = nil
		return
	}
	widget.Dialog(c, &f.open, 560, 14, func() {
		widget.DialogHeader(c, "Add a skill on "+o.Host, "A skill is a folder holding a SKILL.md.", false)
		widget.Field(c, "For", func() { agentPicker(c, envsync.AgentsIn(proto.EnvSkill, envsync.User), &f.agent) })
		widget.Field(c, "From", func() {
			cur := 0
			if f.fromRepo {
				cur = 1
			}
			if i := widget.Segmented(c, []string{"A folder of this device", "A git repository"}, cur, nil); i >= 0 {
				f.fromRepo = i == 1
			}
		})
		if f.fromRepo {
			v.repoSkills(c, o, f)
		}
		if f.err != "" {
			ui.Text(c, f.err).FontSize(theme.TextLabel).TextColor(theme.Error)
		}
		widget.DialogFooter(c, "", func() {
			if widget.GhostButton(c, "Cancel").Clicked() {
				f.open = false
			}
			switch {
			case !f.fromRepo:
				if widget.PrimaryButton(c, icon.FolderPlus, "Pick the folder…", "").Clicked() {
					go v.pickSkill(o, f)
				}
			case f.fetch == nil:
				b := widget.PrimaryButton(c, icon.Search, "Look", "").Disabled(f.fetching || strings.TrimSpace(f.url) == "")
				if b.Clicked() && !f.fetching {
					v.fetchSkills(o, f)
				}
			default:
				n := 0
				for _, on := range f.picked {
					if on {
						n++
					}
				}
				if widget.PrimaryButton(c, icon.Plus, fmt.Sprintf("Install %d", n), "").Disabled(n == 0).Clicked() && n > 0 {
					v.installSkills(o, f)
				}
			}
		})
	})
}

// pickSkill asks for a skill's folder, reads it and adds it; it runs off
// the main thread, as the dialog waits.
func (v *View) pickSkill(o Options, f *skillForm) {
	dirs, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Title: "A skill's folder", Directory: true})
	if err != nil || len(dirs) == 0 {
		return
	}
	files, err := readSkill(dirs[0])
	var p proto.EnvPayload
	if err == nil {
		p, err = envsync.NewSkillIn(f.agent, filepath.Base(dirs[0]), files, envsync.User)
	}
	o.Update(func() {
		if err != nil {
			f.err = strings.TrimPrefix(err.Error(), "envsync: ")
			return
		}
		p.Home = v.scan.Home
		f.open = false
		v.kind = proto.EnvSkill
		v.save(o, p, "Added the skill "+p.Item.Name+" to "+agentName(f.agent))
	})
}

// readSkill reads a skill's folder on this device, without git's folder.
func readSkill(dir string) ([]proto.EnvFile, error) {
	var files []proto.EnvFile
	total := 0
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if total += len(data); total > maxSkill {
			return fmt.Errorf("the folder holds more than %d MB", maxSkill>>20)
		}
		rel, _ := filepath.Rel(dir, p)
		info, _ := d.Info()
		files = append(files, proto.EnvFile{Path: filepath.ToSlash(rel), Mode: uint32(info.Mode().Perm()), Data: data})
		return nil
	})
	if err == nil && len(files) == 0 {
		err = errors.New("the folder is empty")
	}
	return files, err
}

// repoSkills asks for a repository, and offers the skills found in it.
func (v *View) repoSkills(c *ui.Context, o Options, f *skillForm) {
	widget.Field(c, "Repository", func() {
		if widget.Input(c, &f.url, "https://github.com/anthropics/skills  (or …/tree/main/<folder>)").FillWidth().Font(theme.MonoFont).Changed() {
			f.fetch = nil
		}
	})
	switch {
	case f.fetching:
		ui.Row(c).Gap(8).Children(func() {
			widget.Spinner(c, 14)
			ui.Text(c, "Cloning it on "+o.Host+"…").FontSize(theme.TextLabel).TextColor(theme.Muted)
		})
	case f.fetch != nil:
		ui.Column(c).FillWidth().Gap(4).MaxHeight(260).Children(func() {
			ui.Scroll(c).FillWidth().Children(func() {
				ui.Column(c).FillWidth().Gap(4).AlignItems(ui.Start).Children(func() {
					for _, sk := range f.fetch.Skills {
						on := f.picked[sk.Path]
						label := sk.Name
						if sk.Description != "" {
							label += " · " + sk.Description
						}
						if widget.Checkbox(c.Key(sk.Path), &on, label) {
							f.picked[sk.Path] = on
						}
					}
				})
			})
		})
	}
}

// fetchSkills clones the repository on the host and lists its skills.
func (v *View) fetchSkills(o Options, f *skillForm) {
	cl := o.Client
	if cl == nil {
		return
	}
	f.fetching, f.err = true, ""
	url := strings.TrimSpace(f.url)
	go func() {
		fetch, err := cl.SkillFetch(url)
		o.Update(func() {
			f.fetching = false
			if err != nil {
				f.err = strings.TrimPrefix(client.Explain(err), "envsync: ")
				return
			}
			f.fetch, f.picked = &fetch, map[string]bool{}
			if len(fetch.Skills) == 1 {
				f.picked[fetch.Skills[0].Path] = true
			}
		})
	}()
}

// installSkills adds the skills picked.
func (v *View) installSkills(o Options, f *skillForm) {
	cl := o.Client
	if cl == nil || f.fetch == nil {
		return
	}
	var paths []string
	for _, sk := range f.fetch.Skills {
		if f.picked[sk.Path] {
			paths = append(paths, sk.Path)
		}
	}
	p := proto.SkillInstallParams{ID: f.fetch.ID, Paths: paths, Agent: f.agent}
	f.fetching = true
	go func() {
		names, err := cl.SkillInstall(p)
		o.Update(func() {
			f.fetching = false
			if err != nil {
				f.err = client.Explain(err)
				f.fetch = nil
				return
			}
			f.open = false
			v.kind, v.notice = proto.EnvSkill, "Added "+strings.Join(names, ", ")+" to "+agentName(p.Agent)
			v.Reload(o)
		})
	}()
}
