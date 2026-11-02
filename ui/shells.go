package ui

import (
	"cmp"
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
	widget.PageHeader(c, "Terminal", "What terminals start with", nil)
	ui.Row(c.Key("terminal-page")).Grow(1).FillWidth().MinHeight(0).AlignItems(ui.Stretch).Gap(theme.S3).Padding(theme.S4, theme.S5, theme.S5, theme.S5).Children(func() {
		ui.Scroll(c.Key("terminal-picks")).Grow(1).Basis(0).MinWidth(0).FillHeight().Children(func() {
			ui.Column(c).FillWidth().Gap(theme.S3).Children(func() {
				ui.Column(c.Key("shell")).FillWidth().Gap(theme.S2).Children(func() {
					widget.Heading(c, "Shell")
					switch {
					case hs.err != "":
						widget.CardNote(c, hs.err, 0).TextColor(theme.Error)
					case len(hs.list.Shells) == 0:
						widget.CardNote(c, "Looking for "+a.host.Name+"'s shells…", 0)
					}
					if len(hs.list.Shells) > 0 {
						a.shellPicker(c, hs)
					}
					widget.CardNote(c, "The shell terminals on "+a.host.Name+" start in.", 0)
				})
				a.fontSection(c)
			})
		})
		widget.Card(c.Key("terminal-look")).Grow(1).Basis(0).FillHeight().Children(func() {
			widget.CardTitle(c, nil, "How terminals look", func() { a.termSizeControl(c) })
			ui.Scroll(c.Key("term-sample")).Grow(1).FillWidth().MinHeight(0).Radius(theme.RadiusCard).Background(widget.CodeGround).Children(func() {
				ui.Text(c, a.termSample(hs)).Font(cmp.Or(a.state.TermFont, theme.MonoFont)).FontSize(a.termSize()).TextColor(widget.CodeText).FillWidth().MinWidth(0).Padding(theme.S3, theme.S4)
			})
			widget.CardNote(c, "The font terminals draw with on this device. The size is for every terminal, and "+zoomKey()+"0 goes back to the default.", 0)
		})
	})
}

// termSample is what the look's preview shows.
func (a *App) termSample(hs *hostShells) string {
	shell := "shell"
	for _, s := range hs.list.Shells {
		if hs.list.Default == s.Path || hs.list.Default == "" && s.Login {
			shell = s.Name
		}
	}
	return shell + " on " + a.host.Name + "\n$ go version\ngo version go1.27.3\n$ go test ./ui\nok  ui  6.1s\n$ _"
}

// shellPicker lists the host's shells, the one terminals start with
// picked.
func (a *App) shellPicker(c *ui.Context, hs *hostShells) {
	for _, s := range hs.list.Shells {
		on := hs.list.Default == s.Path || hs.list.Default == "" && s.Login
		b := widget.PickFace(c, s.Path, s.Name, on, func() {
			ui.Icon(c, icon.Terminal).FontSize(14).Shrink(0)
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				ui.Row(c).FillWidth().Gap(theme.S1).Children(func() {
					ui.Text(c, s.Name).FontSize(theme.TextUI).FontWeight(500).SingleLine().Shrink(0)
					if s.Login {
						ui.Text(c, "login shell").FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(1).MinWidth(0)
					}
				})
				widget.Running(c.Key("path"), s.Path, widget.RunStyle{Size: theme.TextLabel, Mono: true, Color: theme.TextSoft})
			})
		})
		if b.Clicked() && !on {
			path := s.Path
			if s.Login {
				path = "" // follows the login shell as it changes
			}
			a.setShell(path)
		}
	}
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
