package ui

import (
	"fmt"
	"os"
	"path"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/core/setup"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/transport"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The Setup page brings the connected host to what this device has
// (core/setup): this device's scan makes the manifest (or an enowx.toml
// does), the host's scan says what it lacks, and the steps picked run as
// one script in a session of the host, where the user sees them and
// answers what they ask (a sudo password, a sign-in code).

const pageSetup = "setup"

// setupStatusPollEvery is how often the page reads how the steps go.
const setupStatusPollEvery = 2 * time.Second

// setupPage is the Setup page's state, for one host. It belongs to the
// main thread.
type setupPage struct {
	host     hosts.Host
	manifest setup.Manifest
	from     string // where the manifest comes from: this device, a file
	target   proto.SetupScan
	scanned  bool
	scanning bool
	err      string
	steps    []setup.Step
	picked   map[string]bool
	commands bool // the dry run: each step's commands shown

	session string            // the session running the setup
	status  map[string]string // each step's state, from its status file
}

// openSetup shows the Setup page for the connected host, scanning both
// ends.
func (a *App) openSetup() {
	if a.setup == nil || a.setup.host.ID != a.host.ID {
		a.setup = &setupPage{host: a.host}
	}
	a.page = pageSetup
	if !a.setup.scanned && !a.setup.scanning {
		a.scanSetup()
	}
}

// scanSetup scans this device (for the manifest, unless one was loaded)
// and the connected host, then plans.
func (a *App) scanSetup() {
	p, cl := a.setup, a.client
	if cl == nil {
		return
	}
	p.scanning, p.err = true, ""
	keep := p.from != "" && p.from != setupFromDevice
	go func() {
		local := setup.Scan()
		target, err := cl.ScanSetup()
		a.onMain(func() {
			p.scanning = false
			if err != nil {
				p.err = client.Explain(err)
				return
			}
			if !keep {
				p.manifest, p.from = setup.FromScan(local), setupFromDevice
			}
			p.target, p.scanned = target, true
			a.planSetup()
		})
	}()
}

const setupFromDevice = "this device"

// planSetup plans the steps, picking those the host needs.
func (a *App) planSetup() {
	p := a.setup
	p.steps = setup.Plan(p.manifest, p.target)
	p.picked = map[string]bool{}
	for _, s := range p.steps {
		p.picked[s.ID] = !s.Done
	}
	a.setDrift(p.host.ID, len(setup.Pending(p.steps)))
}

// setDrift records how many steps host id lacks to be what this device is.
func (a *App) setDrift(id string, n int) {
	if a.drift == nil {
		a.drift = map[string]int{}
	}
	a.drift[id] = n
}

// checkDrift scans this device and the connected server off the main
// thread, and records what the server lacks, which its card says; it runs
// as the app connects to a server.
func (a *App) checkDrift(cl *client.Client, h hosts.Host) {
	if h.ID == hosts.LocalID {
		return
	}
	local := setup.Scan()
	target, err := cl.ScanSetup()
	if err != nil {
		return // an older daemon: nothing to say
	}
	n := len(setup.Pending(setup.Plan(setup.FromScan(local), target)))
	a.onMain(func() { a.setDrift(h.ID, n) })
}

func (a *App) setupPage(c *ui.Context) {
	p := a.setup
	a.viewHeader(c, func() {
		ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Breadcrumb(c, "Hosts", p.host.Name, "Set up") })
		if widget.GhostButton(c, "Load enowx.toml…").Clicked() {
			go a.loadManifest()
		}
		if p.scanned && widget.GhostButton(c, "Save enowx.toml…").Clicked() {
			go a.saveManifest()
		}
		if widget.IconButton(c, icon.Sync, "Scan again", false).Clicked() && !p.scanning {
			a.scanSetup()
		}
		if widget.IconButton(c, icon.Close, "Back to sessions", false).Clicked() {
			a.page = ""
		}
	})
	ui.Scroll(c).Grow(1).FillWidth().Children(func() {
		ui.Column(c).FillWidth().MaxWidth(760).AlignSelf(ui.Center).Padding(20).Gap(14).Children(func() {
			if a.host.ID == hosts.LocalID {
				ui.Text(c, "Setup brings a server to what this device has: connect to the server, then open Set up there.").FontSize(theme.TextUI).TextColor(theme.Muted)
				return
			}
			ui.Text(c, "Brings "+p.host.Name+" to what "+p.from+" has: toolchains at the same versions (through mise), git's user, GitHub's CLI signed in, the agents. Each step checks first, so running it again changes nothing. Skills, MCP servers and settings go with Sync environment.").
				FontSize(12.5).TextColor(theme.TextSoft)
			switch {
			case p.err != "":
				widget.Notice(c, icon.Alert, theme.Error, p.err, "", nil)
			case !p.scanned:
				ui.Row(c).Gap(8).Children(func() {
					widget.Spinner(c, 14)
					ui.Text(c, "Scanning this device and "+p.host.Name+"…").FontSize(theme.TextUI).TextColor(theme.Muted)
				})
				return
			}
			a.setupSteps(c, p)
			a.setupActions(c, p)
		})
	})
	if p.session != "" {
		a.pollSetupStatus(c, p)
	}
}

// setupSteps lists the steps: picked or not, what each brings, what the
// host has, and how it went.
func (a *App) setupSteps(c *ui.Context, p *setupPage) {
	if len(p.steps) == 0 {
		ui.Text(c, "Nothing to bring: the manifest asks for nothing "+p.host.Name+" could get.").FontSize(theme.TextUI).TextColor(theme.Muted)
		return
	}
	widget.Card(c).Children(func() {
		for _, s := range p.steps {
			ui.Column(c.Key(s.ID)).FillWidth().Gap(4).Padding(6, 0).Children(func() {
				ui.Row(c).FillWidth().Gap(10).Children(func() {
					on := p.picked[s.ID]
					if p.session == "" {
						if widget.Checkbox(c, &on, s.Title) {
							p.picked[s.ID] = on
						}
					} else {
						setupStateMark(c, p.status[s.ID], p.picked[s.ID])
						ui.Text(c, s.Title).FontSize(theme.TextUI)
					}
					ui.Spacer(c)
					col := theme.Muted
					if s.Done {
						col = theme.Done
					}
					detail := s.Detail
					if s.Asks {
						detail += " · asks you"
					}
					ui.Text(c, detail).FontSize(theme.TextLabel).TextColor(col).SingleLine().Shrink(1)
				})
				if p.commands && p.picked[s.ID] {
					ui.Text(c, s.Script).Font(theme.MonoFont).FontSize(theme.TextLabel).Padding(8, 10).Radius(theme.RadiusSmall).Background(theme.Sunken).FillWidth()
				}
			})
		}
	})
}

// setupStateMark is a step's state while the setup runs.
func setupStateMark(c *ui.Context, state string, picked bool) {
	switch {
	case !picked:
		widget.Dot(c, theme.Idle, true, 7)
	case state == setup.StateRunning:
		widget.Spinner(c, 13)
	case state == setup.StateOK:
		ui.Icon(c, icon.Check).FontSize(13).TextColor(theme.Done)
	case state == setup.StateFailed:
		ui.Icon(c, icon.Alert).FontSize(13).TextColor(theme.Error)
	default:
		widget.Dot(c, theme.Idle, false, 7)
	}
}

// setupActions are the dry run, the run, and the ways to the rest.
func (a *App) setupActions(c *ui.Context, p *setupPage) {
	picked := a.pickedSteps()
	ui.Row(c).FillWidth().Gap(8).Children(func() {
		label := "Show commands"
		if p.commands {
			label = "Hide commands"
		}
		if widget.GhostButton(c, label).Clicked() {
			p.commands = !p.commands
		}
		if widget.GhostButton(c, "Sync environment").Tooltip("Skills, subagents, commands, MCP servers and settings").Clicked() {
			a.page = pageSync
		}
		ui.Spacer(c)
		if p.session != "" {
			if widget.GhostButton(c, "Show the terminal").Clicked() {
				if s, ok := a.sessionByID(p.session); ok {
					a.selectSession(s)
					a.page = ""
				}
			}
			if widget.Button(c, widget.Secondary, icon.Sync, "Done: scan again", "").Clicked() {
				p.session, p.status = "", nil
				a.scanSetup()
			}
			return
		}
		label = "Set up"
		if n := len(picked); n > 0 {
			label = fmt.Sprintf("Set up %d %s", n, textfmt.Plural(n, "step", "steps"))
		}
		b := widget.PrimaryButton(c, icon.Play, label, "").Disabled(len(picked) == 0)
		if b.Clicked() && len(picked) > 0 {
			a.runSetup(picked)
		}
	})
}

// pickedSteps are the steps to run.
func (a *App) pickedSteps() []setup.Step {
	var out []setup.Step
	for _, s := range a.setup.steps {
		if a.setup.picked[s.ID] {
			out = append(out, s)
		}
	}
	return out
}

// runSetup runs steps in a session of the host, whose terminal shows them.
func (a *App) runSetup(steps []setup.Step) {
	p, cl := a.setup, a.client
	if cl == nil || a.daemonInfo.Home == "" {
		return
	}
	status := path.Join(a.daemonInfo.Home, transport.HomeName, "setup", "status")
	params := proto.SessionCreateParams{
		Agent: agent.Custom, Name: "Set up from " + p.from, Dir: a.daemonInfo.Home,
		Command: []string{"/bin/sh", "-c", setup.Script(steps, status)},
	}
	go func() {
		info, err := cl.CreateSession(params)
		a.onMain(func() {
			if err != nil {
				p.err = client.Explain(err)
				return
			}
			a.upsert(info)
			p.session, p.status = info.ID, map[string]string{}
		})
	}()
}

// pollSetupStatus reads the setup's status file now and then, while the
// page shows.
func (a *App) pollSetupStatus(c *ui.Context, p *setupPage) {
	c.After(setupStatusPollEvery)
	if time.Since(a.setupPolled) < setupStatusPollEvery || a.client == nil {
		return
	}
	a.setupPolled = time.Now()
	cl, id := a.client, p.session
	file := path.Join(a.daemonInfo.Home, transport.HomeName, "setup", "status")
	go func() {
		f, err := cl.ReadFile(id, file, time.Time{})
		if err != nil {
			return
		}
		a.onMain(func() {
			if p.session == id {
				p.status = setup.Status([]byte(f.Text))
			}
		})
	}()
}

// loadManifest takes the manifest from an enowx.toml the user picks.
func (a *App) loadManifest() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: "Load a manifest",
		Filters: []mygo.FileFilter{{Name: "enowX manifest", Extensions: []string{"toml"}}}})
	if err != nil || len(paths) == 0 {
		return
	}
	data, err := os.ReadFile(paths[0])
	var m setup.Manifest
	if err == nil {
		m, err = setup.Decode(data)
	}
	a.onMain(func() {
		if err != nil {
			a.setup.err = err.Error()
			return
		}
		a.setup.manifest, a.setup.from = m, path.Base(paths[0])
		if a.setup.scanned {
			a.planSetup()
		}
	})
}

// saveManifest writes the manifest as an enowx.toml the user places.
func (a *App) saveManifest() {
	data, err := a.setup.manifest.Encode()
	if err != nil {
		return
	}
	file, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: a.win, Title: "Save the manifest", DefaultPath: "enowx.toml"})
	if err != nil || file == "" {
		return
	}
	if err := os.WriteFile(file, data, 0o644); err != nil {
		a.onMain(func() { a.setup.err = err.Error() })
		return
	}
	a.onMain(func() { a.notify("Manifest saved to " + file) })
}
