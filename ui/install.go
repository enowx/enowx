package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Agents missing on a host are installed in a session there, so the
// installer's output and questions stay in view, local or remote alike.

// needsInstall reports whether the chosen agent is missing on the host.
func needsInstall(ag proto.AgentInfo) bool { return ag.Command != "" && !ag.Installed }

// installCard tells what installing the agent runs.
func (a *App) installCard(c *ui.Context, ag proto.AgentInfo) {
	widget.Notice(c, icon.Alert, theme.Warning, ag.Name+" isn't installed on "+a.host.Name+".", "", func() {
		if ag.Install == "" {
			ui.Text(c, "Install "+ag.Command+" there, then pick it again.").FontSize(theme.TextLabel).TextColor(theme.Muted)
			if ag.Homepage != "" {
				ui.Link(c, "How to install "+ag.Name, ag.Homepage).FontSize(theme.TextLabel)
			}
			return
		}
		ui.Text(c, "Install runs its official installer in a session here, as the user enowX runs as:").FontSize(theme.TextLabel).TextColor(theme.Muted)
		ui.Text(c, ag.Install).Font(theme.MonoFont).FontSize(theme.TextLabel).Padding(8, 10).Radius(theme.RadiusSmall).Background(theme.Sunken).FillWidth()
	})
}

// installAgent starts the agent's installer as a session.
func (a *App) installAgent(ag proto.AgentInfo) {
	if ag.Install == "" {
		return
	}
	a.runInstall("Install "+ag.Name, ag.Install, func() {
		a.recheck(func(cl *client.Client) error {
			agents, err := cl.Agents()
			if err == nil {
				a.onMain(func() { a.agents = agents })
			}
			return err
		}, func() bool { return a.agentInfo(ag.ID).Installed }, ag.Name+" is installed. Start a session with it from New session.", ag.Name+" didn't install; its session shows why.")
	})
}

// installDisplay installs what the host needs for sessions with a display.
func (a *App) installDisplay() {
	command := a.daemonInfo.DisplayInstall
	if command == "" {
		return
	}
	a.runInstall("Install Xpra", command, func() {
		a.recheck(func(cl *client.Client) error {
			info, err := cl.Info()
			if err == nil {
				a.onMain(func() { a.daemonInfo = info })
			}
			return err
		}, func() bool { return a.daemonInfo.Display }, "Xpra is installed. New sessions can run with a display.", "Xpra didn't install; its session shows why.")
	})
}

// runInstall runs command as a session in the project's folder, and ended
// once it exits.
func (a *App) runInstall(name, command string, ended func()) {
	f := &a.newSession
	cl := a.client
	if cl == nil {
		return
	}
	params := proto.SessionCreateParams{
		Project:    f.project.Path,
		Name:       name,
		Agent:      agent.Custom,
		BranchMode: proto.BranchNone,
		Command:    []string{"sh", "-c", command},
	}
	f.busy, f.err = true, ""
	go func() {
		info, err := cl.CreateSession(params)
		a.onMain(func() {
			f.busy = false
			if err != nil {
				f.err = strings.TrimPrefix(err.Error(), proto.MethodSessionCreate+": ")
				return
			}
			f.open = false
			if a.installs == nil {
				a.installs = map[string]func(){}
			}
			a.installs[info.ID] = ended
			a.upsert(info)
			a.selectSession(info)
		})
	}()
}

// installEnded runs what an install session asked for once it exits.
func (a *App) installEnded(info proto.SessionInfo) {
	ended, ok := a.installs[info.ID]
	if !ok || !info.Exited {
		return
	}
	delete(a.installs, info.ID)
	ended()
}

// recheck asks the daemon again with fetch, which applies what it learns
// on the main thread, then tells whether ok holds.
func (a *App) recheck(fetch func(*client.Client) error, ok func() bool, yes, no string) {
	cl, gen := a.client, a.gen
	if cl == nil {
		return
	}
	go func() {
		err := fetch(cl)
		a.onMain(func() {
			if gen != a.gen || err != nil {
				return
			}
			if ok() {
				a.notify(yes)
			} else {
				a.status = no
			}
		})
	}()
}
