package ui

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The connected host's shells: Settings' Terminal section picks the one
// its terminals start with, and a session's + opens a tab of any.

// hostShells is the connected host's shells, as last read.
type hostShells struct {
	host    string
	list    proto.ShellList
	loading bool
	err     string
}

// shellsOf is the connected host's shells, read anew when the host
// changed; empty until they come.
func (a *App) shellsOf() *hostShells {
	if a.shells == nil || a.shells.host != a.host.ID {
		a.shells = &hostShells{host: a.host.ID}
		a.loadShells()
	}
	return a.shells
}

func (a *App) loadShells() {
	hs, cl := a.shells, a.client
	if cl == nil || hs.loading {
		return
	}
	hs.loading = true
	go func() {
		list, err := cl.Shells()
		a.onMain(func() {
			hs.loading = false
			if err != nil {
				hs.err = client.Explain(err)
				return
			}
			hs.list, hs.err = list, ""
		})
	}()
}

func (a *App) setShell(path string) {
	hs, cl := a.shells, a.client
	if cl == nil || hs == nil {
		return
	}
	hs.list.Default = path // shown picked now
	go func() {
		list, err := cl.SetShell(path)
		a.onMain(func() {
			if err != nil {
				hs.err = client.Explain(err)
				return
			}
			hs.list, hs.err = list, ""
		})
	}()
}

// terminalSection picks the shell the host's terminals start with, and
// the font they draw with.
func (a *App) terminalSection(c *ui.Context) {
	hs := a.shellsOf()
	sectionIntro(c, "Terminal", "The shell terminals on "+a.host.Name+" start with: new terminal tabs, and agents' sessions once the agent exits. A session's + opens a tab of any other.")
	if hs.err != "" {
		ui.Text(c, hs.err).FontSize(theme.TextLabel).TextColor(theme.Error)
	}
	switch {
	case len(hs.list.Shells) == 0 && hs.err == "":
		ui.Text(c, "Looking for "+a.host.Name+"'s shells…").FontSize(theme.TextLabel).TextColor(theme.Muted)
	default:
		a.shellPicker(c, hs)
	}
	a.termSizeSection(c)
	a.fontSection(c)
}

// shellPicker lists the host's shells, the one terminals start with
// picked.
func (a *App) shellPicker(c *ui.Context, hs *hostShells) {
	widget.Field(c, "Shell", func() {
		ui.Column(c).FillWidth().Gap(4).Children(func() {
			for _, s := range hs.list.Shells {
				on := hs.list.Default == s.Path || hs.list.Default == "" && s.Login
				b := ui.ButtonBase(c.Key(s.Path)).FillWidth().Height(40).PaddingX(12).Gap(10).Radius(theme.RadiusControl).
					Border(1, theme.Border).Cursor(ui.CursorPointer).Label(s.Name)
				if on {
					b.Border(1, theme.AccentLine).Background(theme.Selected)
				} else if b.Hovered() {
					b.Background(theme.Hover)
				}
				b.Children(func() {
					ui.Icon(c, icon.Terminal).FontSize(13).TextColor(theme.TextSoft)
					ui.Text(c, s.Name).FontSize(theme.TextUI).TextColor(theme.Text)
					if s.Login {
						ui.Text(c, "login shell").FontSize(theme.TextLabel).TextColor(theme.Muted)
					}
					ui.Spacer(c)
					ui.Text(c, s.Path).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Shrink(1)
					if on {
						ui.Icon(c, icon.Check).FontSize(12).TextColor(theme.Accent)
					}
				})
				if b.Clicked() && !on {
					path := s.Path
					if s.Login {
						path = "" // follows the login shell as it changes
					}
					a.setShell(path)
				}
			}
		})
	})
}

// shellTabItems are the + menu's terminals: the default, then the host's
// other shells.
func (a *App) shellTabItems(c *ui.Context, s proto.SessionInfo, done func()) {
	hs := a.shellsOf()
	if widget.MenuItem(c, icon.Terminal, "Terminal", false) {
		a.openTab(s, agent.Shell, "")
		done()
	}
	if len(hs.list.Shells) < 2 {
		return
	}
	for _, sh := range hs.list.Shells {
		if hs.list.Default == sh.Path || hs.list.Default == "" && sh.Login {
			continue // the Terminal above
		}
		if widget.MenuItem(c.Key(sh.Path), icon.Terminal, "Terminal: "+sh.Name, false) {
			a.openTab(s, agent.Shell, sh.Path)
			done()
		}
	}
}
