package ui

import (
	"fmt"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
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
	commands map[string]bool // the dry run: each card's commands shown, by its title

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
	picked := a.pickedSteps()
	widget.PageHeader(c, "Set up "+p.host.Name, "Brings it to what "+p.fromName()+" has", func() {
		if widget.GhostButton(c, "Back to hosts").Clicked() {
			a.page = pageHosts
		}
		if widget.GhostButton(c, "Load enowx.toml…").Clicked() {
			go a.loadManifest()
		}
		if p.scanned && widget.GhostButton(c, "Save enowx.toml…").Clicked() {
			go a.saveManifest()
		}
		if a.host.ID != hosts.LocalID && p.scanned {
			a.setupRun(c, p, picked)
		}
	})
	if a.host.ID == hosts.LocalID {
		widget.PageBody(c, false, func() {
			ui.Text(c, "Setup brings a server to what this device has: connect to the server, then open Set up there.").FontSize(theme.TextUI).TextColor(theme.TextSoft)
		})
		return
	}
	toBring, here := 0, 0
	for _, s := range p.steps {
		if s.Done {
			here++
		} else {
			toBring++
		}
	}
	widget.FigsBand(c, []widget.Fig{
		{Label: "To bring", Value: strconv.Itoa(toBring)},
		{Label: "Already here", Value: strconv.Itoa(here)},
		{Label: "Comes from", Value: p.fromName()},
	})
	ui.Column(c.Key("setup-body")).Grow(1).FillWidth().MinHeight(0).Padding(theme.S4, theme.S5, theme.S5, theme.S5).Gap(theme.S3).Children(func() {
		if p.err != "" {
			widget.Notice(c, icon.Alert, theme.Error, p.err, "", nil)
		}
		ui.Row(c.Key("setup-parts")).Grow(1).FillWidth().MinHeight(0).Gap(theme.S4).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c.Key("setup-steps")).Grow(7).Basis(0).MinWidth(0).Children(func() { a.setupSteps(c, p) })
			ui.Column(c.Key("setup-side")).Grow(5).Basis(0).MinWidth(0).Gap(theme.S3).Children(func() { a.setupSide(c, p) })
		})
	})
	if p.session != "" {
		a.pollSetupStatus(c, p)
	}
}

// fromName is where the manifest comes from, this device by its name.
func (p *setupPage) fromName() string {
	if p.from == "" || p.from == setupFromDevice {
		return LocalHost().Name
	}
	return p.from
}

// setupGroup is a card of the page: steps of a kind.
type setupGroup struct {
	title string
	steps []setup.Step
}

// setupGroups sorts the plan's steps into the page's cards, in the plan's
// order, leaving out those with no step.
func setupGroups(steps []setup.Step) []setupGroup {
	groups := []setupGroup{{title: "Toolchains"}, {title: "git and gh"}, {title: "Agents"}, {title: "Desktop and Android"}}
	for _, s := range steps {
		i := 3
		switch {
		case s.ID == "packages" || s.ID == "mise" || strings.HasPrefix(s.ID, "tool:"):
			i = 0
		case s.ID == "git" || s.ID == "gh" || s.ID == "gh-auth":
			i = 1
		case strings.HasPrefix(s.ID, "agent:") || strings.HasPrefix(s.ID, "signin:"):
			i = 2
		}
		groups[i].steps = append(groups[i].steps, s)
	}
	return slices.DeleteFunc(groups, func(g setupGroup) bool { return len(g.steps) == 0 })
}

// setupSteps are the steps in their cards: picked or not, what each brings,
// what the host has, and how it went.
func (a *App) setupSteps(c *ui.Context, p *setupPage) {
	if !p.scanned {
		if p.err != "" {
			return
		}
		ui.Column(c).Grow(1).FillWidth().Center().Gap(theme.S2).Children(func() {
			widget.Spinner(c, 18)
			ui.Text(c, "Scanning this device and "+p.host.Name+"…").FontSize(theme.TextUI).TextColor(theme.TextSoft)
		})
		return
	}
	groups := setupGroups(p.steps)
	if len(groups) == 0 {
		ui.Column(c).Grow(1).FillWidth().Center().Children(func() {
			ui.Text(c, "Nothing to bring: the manifest asks for nothing "+p.host.Name+" could get.").FontSize(theme.TextUI).TextColor(theme.TextSoft).TextAlign(ui.Center)
		})
		return
	}
	rows := make([]ui.Track, (len(groups)+1)/2)
	for i := range rows {
		rows[i] = ui.Fr(1)
	}
	ui.Grid(c).Grow(1).FillWidth().MinHeight(0).Columns(2).RowTracks(rows...).Gap(theme.S3).Children(func() {
		for _, g := range groups {
			a.setupCard(c.Key(g.title), p, g)
		}
	})
}

// setupCard is a kind of steps: whether the host has them all, each step, and
// its commands when asked for. The steps scroll inside the card.
func (a *App) setupCard(c *ui.Context, p *setupPage, g setupGroup) {
	widget.GridCard(c).Children(func() {
		widget.CardTitle(c, nil, g.title, nil)
		col, state := theme.Done, "Here"
		if slices.ContainsFunc(g.steps, func(s setup.Step) bool { return !s.Done }) {
			col, state = theme.Working, "To bring"
		}
		ui.Row(c.Key("status")).FillWidth().Shrink(0).Children(func() { widget.StatusLine(c, col, false, state) })
		ui.Scroll(c.Key("steps")).Grow(1).FillWidth().MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Gap(theme.S1).Children(func() {
				for _, s := range g.steps {
					a.setupStep(c.Key(s.ID), p, g, s)
				}
			})
		})
		widget.CardFoot(c, func() {
			label := "Show commands"
			if p.commands[g.title] {
				label = "Hide commands"
			}
			if widget.GhostButton(c, label).Clicked() {
				if p.commands == nil {
					p.commands = map[string]bool{}
				}
				p.commands[g.title] = !p.commands[g.title]
			}
		})
	})
}

func (a *App) setupStep(c *ui.Context, p *setupPage, g setupGroup, s setup.Step) {
	ui.Column(c).FillWidth().Gap(theme.S1).Children(func() {
		ui.Row(c).FillWidth().MinHeight(24).Gap(theme.S2).Children(func() {
			on := p.picked[s.ID]
			if p.session == "" {
				if widget.Switch(c, &on, s.Title) {
					p.picked[s.ID] = on
				}
			} else {
				setupStateMark(c, p.status[s.ID], p.picked[s.ID])
			}
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				widget.Running(c, s.Title, widget.RunStyle{Size: theme.TextLabel})
				col := theme.TextSoft
				if s.Done {
					col = theme.Done
				}
				detail := s.Detail
				if s.Asks {
					detail += " · asks you"
				}
				widget.Running(c.Key("detail"), detail, widget.RunStyle{Size: theme.TextLabel, Color: col})
			})
		})
		if p.commands[g.title] && p.picked[s.ID] {
			ui.Text(c, s.Script).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(widget.CodeText).Padding(theme.S2, theme.S3).
				Radius(theme.RadiusSmall).Background(widget.CodeGround).FillWidth().MinWidth(0)
		}
	})
}

// setupSide is the way to Sync environment and the terminal the steps run in.
func (a *App) setupSide(c *ui.Context, p *setupPage) {
	widget.Card(c.Key("sync")).FillWidth().Shrink(0).Children(func() {
		widget.CardTitle(c, nil, "Sync environment", nil)
		widget.CardNote(c.Key("note"), "Skills, subagents, commands, MCP servers and settings.", 0)
		ui.Row(c.Key("open")).FillWidth().Shrink(0).Children(func() {
			if widget.GhostButton(c, "Open").Clicked() {
				a.openSync()
			}
		})
	})
	widget.GridCard(c.Key("terminal")).FillWidth().Grow(1).Basis(0).Children(func() {
		widget.CardTitle(c, nil, "Its terminal", func() {
			if p.session != "" {
				if widget.GhostButton(c, "Show the terminal").Clicked() {
					if s, ok := a.sessionByID(p.session); ok {
						a.selectSession(s)
						a.page = ""
					}
				}
				return
			}
			if widget.Button(c, widget.Secondary, icon.Sync, "Scan again", "").Clicked() && !p.scanning {
				a.scanSetup()
			}
		})
		ui.Column(c.Key("ground")).Grow(1).FillWidth().MinHeight(0).Radius(theme.RadiusCard).Clip().Background(widget.CodeGround).Children(func() {
			if pane := a.panes[p.session]; p.session != "" && pane != nil {
				terminal.View(c.Key("term"), pane.term).Grow(1).FillWidth().Background(theme.Background).Padding(theme.S2, theme.S3)
				return
			}
			ui.Scroll(c.Key("what")).Grow(1).FillWidth().MinHeight(0).Children(func() {
				ui.Text(c, a.setupIntro(p)).Font(theme.MonoFont).FontSize(theme.TextUI).TextColor(widget.CodeText).Padding(theme.S3, theme.S4).FillWidth().MinWidth(0)
			})
		})
	})
}

// setupIntro is what the terminal's place says before anything runs.
func (a *App) setupIntro(p *setupPage) string {
	return "The steps run here, in a session of " + p.host.Name + ": toolchains at the same versions (through mise), git's user, GitHub's CLI signed in, the agents. Answer here what they ask. Each step checks first, so running it again changes nothing."
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

// setupRun is the header's run, or while it runs the way on after it.
func (a *App) setupRun(c *ui.Context, p *setupPage, picked []setup.Step) {
	if p.session != "" {
		if widget.Button(c, widget.Primary, icon.Sync, "Done: scan again", "").Clicked() {
			p.session, p.status = "", nil
			a.scanSetup()
		}
		return
	}
	label := "Set up"
	if n := len(picked); n > 0 {
		label = fmt.Sprintf("Set up %d %s", n, textfmt.Plural(n, "item", "items"))
	}
	b := widget.PrimaryButton(c, icon.Play, label, "").Disabled(len(picked) == 0)
	if b.Clicked() && len(picked) > 0 {
		a.runSetup(picked)
	}
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
