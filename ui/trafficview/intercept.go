package trafficview

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// interceptState is the Intercept page: what is being attached, and the
// CA when asked for.
type interceptState struct {
	busy     string // the target being attached
	electron string
	// electronOpen and caOpen show their dialogs.
	electronOpen, caOpen bool
	agent                bool
	ca                   *proto.TrafficCA
	err                  string
}

// attach attaches a target, then shows a tab made for it.
func (v *View) attach(o Options, p proto.TrafficAttachParams) {
	s := &v.inter
	if s.busy != "" || o.Host == nil {
		return
	}
	s.busy, s.err = p.Kind+p.Program+p.Tab, ""
	h := o.Host
	go func() {
		t, err := h.TrafficAttach(p)
		targets, _ := h.TrafficAttached()
		o.Update(func() {
			s.busy = ""
			if err != nil {
				s.err = client.Explain(err)
				return
			}
			v.targets = targets
			if t.Note != "" {
				v.notice = t.Note
			}
			if t.Tab != "" && o.OpenTab != nil && p.Kind == proto.TargetTerminal {
				o.OpenTab(t.Tab)
			}
		})
	}()
}

func (v *View) detach(o Options, source string) {
	h := o.Host
	go func() {
		err := h.TrafficDetach(source)
		targets, _ := h.TrafficAttached()
		o.Update(func() {
			if err != nil {
				v.inter.err = client.Explain(err)
			}
			v.targets = targets
		})
	}()
}

func (v *View) interceptPage(c *ui.Context, o Options) {
	s := &v.inter
	session := o.Session.ID
	if o.Session.Parent != "" {
		session = o.Session.Parent
	}
	widget.Scroll(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
		ui.Column(c).FillWidth().Gap(theme.S1).Children(func() {
			ui.Text(c, "What runs on "+o.HostName+" can send its HTTP through enowX. Nothing is caught until you attach it.").FontSize(theme.TextUI).TextColor(theme.TextSoft).Padding(0, theme.S3)
			if s.err != "" {
				ui.Text(c, s.err).FontSize(theme.TextUI).TextColor(theme.Error).Padding(0, theme.S3)
			}
			if len(v.targets) > 0 {
				section(c, "Attached", func() {
					for _, t := range v.targets {
						widget.ItemRow(c.Key(t.Source), widget.Item{Dot: theme.Done, Title: t.Label + " · " + t.Kind, Sub: t.Proxy}, func() {
							if widget.GhostButton(c, "Detach").Clicked() {
								v.detach(o, t.Source)
							}
						})
					}
				})
			}
			if session == "" {
				ui.Text(c, "Select a session to attach to it.").FontSize(theme.TextUI).TextColor(theme.TextSoft).Padding(theme.S2, theme.S3)
				return
			}
			section(c, "Terminals", func() {
				targetRow(c, "New terminal", "A tab of this session: what you run there is caught.", "Open", s.busy != "", func() {
					v.attach(o, proto.TrafficAttachParams{Kind: proto.TargetTerminal, Session: session})
				})
				attached := v.attachedTo(session, proto.TargetSession)
				title, body := "This session's new tabs", "Tabs opened from now on, and what the Run menu starts."
				if attached != "" {
					targetRow(c, title, "Caught.", "Stop catching", false, func() { v.detach(o, attached) })
				} else {
					targetRow(c, title, body, "Attach", s.busy != "", func() {
						v.attach(o, proto.TrafficAttachParams{Kind: proto.TargetSession, Session: session, Agent: s.agent})
					})
					ui.Column(c).FillWidth().PaddingX(theme.S3).Children(func() {
						widget.ToggleRow(c, "The agent's own traffic too", "Once it starts again; its API keys show, masked", &s.agent)
					})
				}
				for _, tab := range o.Tabs {
					if tab.Exited || (tab.Agent != "shell" && tab.Agent != "") {
						continue
					}
					name := tab.Name
					if name == "" {
						name = "Terminal"
					}
					if v.attachedTo(tab.ID, proto.TargetShell) != "" || v.attachedTab(tab.ID) {
						continue
					}
					targetRow(c.Key("tab:"+tab.ID), "“"+name+"”", "This open tab: what it runs next is caught.", "Attach", s.busy != "", func() {
						v.attach(o, proto.TrafficAttachParams{Kind: proto.TargetShell, Tab: tab.ID})
					})
				}
			})
			section(c, "Browsers", func() {
				if o.SetBrowserCapture != nil {
					if o.BrowserCaptured {
						targetRow(c, "The Browser tool: caught", "What this session's previews load from its dev servers, HMR included. Other sites a page calls aren't, yet.", "Stop", false, func() {
							o.SetBrowserCapture(false)
						})
					} else {
						targetRow(c, "The Browser tool", "What this session's previews load from its dev servers, HMR included. Previews open from now on.", "Attach", false, func() {
							o.SetBrowserCapture(true)
						})
					}
				}
				if len(v.status.Browsers) == 0 {
					ui.Text(c, "No Chrome, Chromium, Edge, Brave or Firefox found on "+o.HostName+".").FontSize(theme.TextUI).TextColor(theme.TextSoft).Padding(theme.S1, theme.S3)
				}
				where := "a window of its own"
				if v.status.Display {
					where = "the session's display, shown here"
				}
				for _, b := range v.status.Browsers {
					targetRow(c.Key("b:"+b.ID), b.Name, "A fresh profile, in "+where+". Your own profile isn't touched.", "Start", s.busy != "", func() {
						v.attach(o, proto.TrafficAttachParams{Kind: proto.TargetBrowser, Session: session, Program: b.ID})
					})
				}
				targetRow(c, "An Electron app", "Its executable started with the proxy and the CA trusted.", "Start…", s.busy != "", func() {
					s.electronOpen = true
				})
			})
			section(c, "Devices and containers", func() {
				if len(o.Android) == 0 || o.CatchAndroid == nil {
					targetRow(c, "Android", "No device online: start an emulator or connect a phone in the Android tool.", "", true, nil)
				}
				for _, d := range o.Android {
					if d.Caught {
						targetRow(c.Key("a:"+d.Key), d.Label+": caught", "Its HTTP goes through enowX, and it trusts enowX's CA.", "Stop", false, func() {
							o.CatchAndroid(d.Key, false)
						})
						continue
					}
					targetRow(c.Key("a:"+d.Key), d.Label, "Its proxy set to enowX, and enowX's CA trusted: among the system's with root, else installed from Settings.", "Catch", false, func() {
						o.CatchAndroid(d.Key, true)
					})
				}
				if o.AndroidNote != "" {
					ui.Text(c, o.AndroidNote).FontSize(theme.TextLabel).TextColor(theme.TextSoft).Padding(theme.S1, theme.S3)
				}
				targetRow(c, "Docker containers", "Containers this session starts afterwards get the proxy and the CA.", "Attach", s.busy != "", func() {
					v.attach(o, proto.TrafficAttachParams{Kind: proto.TargetDocker, Session: session})
				})
			})
			section(c, "Agents", func() {
				targetRow(c, "enowX's tools for the agents", "Adds enowX's MCP server to the agents on "+o.HostName+" (Claude Code, Codex, OpenCode, Gemini CLI): they read and mock this traffic, and drive the Browser, as tools of their own. Agents started afterwards have them.", "Add", s.busy != "", func() {
					h := o.Host
					s.busy = "mcp"
					go func() {
						res, err := h.TrafficMCP()
						o.Update(func() {
							s.busy = ""
							switch {
							case err != nil:
								s.err = client.Explain(err)
							case len(res.Added) == 0:
								v.notice = "No agents set up on " + o.HostName + " to add it to."
							default:
								v.notice = "Added to " + strings.Join(res.Added, ", ") + ". " + strings.Join(res.Notes, " ")
							}
						})
					}()
				})
			})
			section(c, "Anything else", func() {
				targetRow(c, "An address of its own", "For a program set up by hand: an HTTP or SOCKS5 proxy on "+o.HostName+", and the CA to trust.", "Make one", s.busy != "", func() {
					v.attach(o, proto.TrafficAttachParams{Kind: proto.TargetManual, Session: session, Label: "Manual"})
				})
				targetRow(c, "The CA", "What a program must trust for enowX to read its HTTPS.", "Show", false, func() {
					s.caOpen = true
					v.loadCA(o)
				})
			})
		})
	})
}

func (v *View) loadCA(o Options) {
	s := &v.inter
	if s.ca != nil {
		return
	}
	h := o.Host
	go func() {
		ca, err := h.TrafficCA()
		o.Update(func() {
			if err != nil {
				s.err = client.Explain(err)
				return
			}
			s.ca = &ca
		})
	}()
}

// interceptSheets are the Intercept page's dialogs: an Electron app to
// start, and the CA.
func (v *View) interceptSheets(c *ui.Context, o Options) {
	s := &v.inter
	session := o.Session.ID
	if o.Session.Parent != "" {
		session = o.Session.Parent
	}
	if s.electronOpen {
		sheet(c, &s.electronOpen, 520, "Start an Electron app", "Its traffic is caught: it starts with the proxy, and Chromium told to trust enowX's CA.", func() {
			widget.Field(c, "The app on "+o.HostName, func() {
				ui.Row(c).FillWidth().Gap(theme.S2).Children(func() {
					widget.Input(c, &s.electron, "/Applications/Slack.app").Grow(1).Basis(0).MinWidth(0).Font(theme.MonoFont)
					if o.PickHostFile != nil && widget.Button(c, widget.Secondary, nil, "Choose…", "").Clicked() {
						s.electronOpen = false // one dialog at a time: this one comes back with the pick
						o.PickHostFile("Choose an Electron app", "Its executable, or on macOS its .app.", func(path string) { s.electron, s.electronOpen = path, true })
					}
				})
			})
			errorText(c, s.err)
		}, func() {
			if widget.PrimaryButton(c, nil, "Start", "").Disabled(strings.TrimSpace(s.electron) == "" || s.busy != "").Clicked() {
				v.attach(o, proto.TrafficAttachParams{Kind: proto.TargetElectron, Session: session, Program: strings.TrimSpace(s.electron)})
				s.electronOpen = false
			}
		})
	}
	if s.caOpen {
		sheet(c, &s.caOpen, 560, "enowX's certificate authority", "Programs that trust it let enowX read their HTTPS. Its key never leaves "+o.HostName+".", func() {
			if s.ca == nil {
				ui.Text(c, "Reading…").FontSize(theme.TextUI).TextColor(theme.TextSoft)
				return
			}
			widget.Field(c, "SHA-256", func() {
				ui.Text(c, s.ca.Fingerprint).Font(theme.MonoFont).FontSize(theme.TextLabel).Selectable()
			})
			widget.Field(c, "On "+o.HostName, func() {
				ui.Column(c).Gap(2).Children(func() {
					ui.Text(c, s.ca.File).Font(theme.MonoFont).FontSize(theme.TextLabel).Selectable()
					ui.Text(c, s.ca.Bundle+" (with the system's roots)").Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.TextSoft).Selectable()
				})
			})
		}, func() {
			if s.ca != nil && o.CopyText != nil && widget.PrimaryButton(c, nil, "Copy the CA (PEM)", "").Clicked() {
				o.CopyText(s.ca.PEM)
				v.notice = "The CA's certificate is on the clipboard."
				s.caOpen = false
			}
		})
	}
}

func (v *View) attachedTo(id, kind string) string {
	for _, t := range v.targets {
		if t.Kind == kind && (t.Session == id || t.Tab == id) {
			return t.Source
		}
	}
	return ""
}

func (v *View) attachedTab(tab string) bool {
	for _, t := range v.targets {
		if t.Tab == tab {
			return true
		}
	}
	return false
}

func section(c *ui.Context, title string, body func()) {
	ui.Column(c.Key(title)).FillWidth().Children(func() {
		widget.SectionLabel(c, title, theme.TextSoft).Padding(theme.S3, theme.S3, theme.S1, theme.S3)
		body()
	})
}

// targetRow is a target to attach: what it is, in a line that runs, and its
// button (none when action is nil).
func targetRow(c *ui.Context, title, body, button string, busy bool, action func()) {
	widget.ItemRow(c.Key(title), widget.Item{Title: title, Sub: body}, func() {
		if action != nil && widget.Button(c, widget.Secondary, nil, button, "").Disabled(busy).Clicked() {
			action()
		}
	})
}

// pausedEdit is a paused exchange being changed before it goes on.
type pausedEdit struct {
	paused  proto.TrafficPaused
	status  string
	headers string
	body    string
}

// pausedList shows the exchanges waiting at breakpoints, each to resume
// as it is, changed, or to drop.
func (v *View) pausedList(c *ui.Context, o Options) {
	ui.Scroll(c.Key("paused")).FillWidth().Shrink(0).MaxHeight(320).Radius(theme.RadiusCard).Background(theme.Sunken).Children(func() {
		ui.Column(c).FillWidth().Padding(theme.S2, theme.S3).Gap(theme.S3).Children(func() {
			for _, p := range v.paused {
				v.pausedRow(c.Key(fmt.Sprint(p.ID)), o, p)
			}
		})
	})
}

func (v *View) pausedRow(c *ui.Context, o Options, p proto.TrafficPaused) {
	ui.Column(c).FillWidth().Gap(theme.S2).Children(func() {
		ui.Row(c).FillWidth().Gap(theme.S2).Children(func() {
			widget.Tag(c, "paused "+p.Stage, theme.Warning)
			ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
				widget.Running(c, p.Method+" "+p.URL, widget.RunStyle{Size: theme.TextLabel, Mono: true})
			})
		})
		ui.Row(c).FillWidth().Gap(theme.S2).Children(func() {
			if widget.GhostButton(c, "Drop").Clicked() {
				v.resume(o, proto.TrafficResumeParams{ID: p.ID, Drop: true})
			}
			if widget.Button(c, widget.Secondary, nil, "Change", "").Tooltip("Change it before it goes on").Clicked() {
				e := &pausedEdit{paused: p, headers: headerLines(p.Headers), body: string(p.Body)}
				if p.Status != 0 {
					e.status = fmt.Sprint(p.Status)
				}
				v.pausedEdit = e
			}
			if widget.PrimaryButton(c, nil, "Resume", "").Clicked() {
				v.resume(o, proto.TrafficResumeParams{ID: p.ID})
			}
		})
	})
}

// pausedSheet changes a paused exchange before it goes on.
func (v *View) pausedSheet(c *ui.Context, o Options) {
	e := v.pausedEdit
	if e == nil {
		return
	}
	p := e.paused
	open := true
	sheet(c, &open, 600, "Change, then resume", p.Method+" "+p.URL+" · paused at its "+p.Stage, func() {
		ui.Column(c).FillWidth().Gap(theme.S3).Children(func() {
			if p.Stage == "response" {
				ui.Column(c).Width(120).Children(func() {
					widget.Field(c, "Status", func() { widget.Input(c, &e.status, "200").FillWidth() })
				})
			}
			codeArea(c, "Headers (Name: value per line)", &e.headers, 80)
			codeArea(c, "Body", &e.body, 140)
		})
	}, func() {
		if widget.PrimaryButton(c, nil, "Resume changed", "").Clicked() {
			edit := &proto.TrafficEdit{Headers: parseHeaderLines(e.headers)}
			if n, err := parseNum(e.status, "Status"); err == nil {
				edit.Status = n
			}
			body := []byte(e.body)
			edit.Body = &body
			v.resume(o, proto.TrafficResumeParams{ID: p.ID, Edit: edit})
			open = false
		}
	})
	if !open {
		v.pausedEdit = nil
	}
}

func (v *View) resume(o Options, p proto.TrafficResumeParams) {
	h := o.Host
	go func() {
		err := h.TrafficResume(p)
		o.Update(func() {
			if err != nil {
				v.notice = client.Explain(err)
			}
		})
	}()
}
