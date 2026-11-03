package onboardview

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
)

// The main region: a header and a tab like a session's, the setup log while things are
// found, a sample conversation while a style is chosen, what GOD of Agents does in its own
// words, then the first task, in a real composer.

// firstAgent is who the first task goes to.
func firstAgent(o Options) (id, name string) {
	for _, a := range o.Agents {
		if a.Installed && a.Command != "" {
			return a.ID, a.Name
		}
	}
	return "", ""
}

// show is the design's way in: a fade and a small rise.
func show(t, at float32) (alpha, rise float32) {
	e := seg(t, at, at+300, easeOut)
	return e, (1 - e) * 6
}

func (v *View) docRegion(c *ui.Context, o Options, s scene) {
	regionBox(c, "ob-doc", s.doc, s.doc.v.main, func() {
		vd := s.doc.v
		t := s.t
		r := s.returning
		// The titles change on a short dip, never a pop.
		dip := float32(1)
		for _, at := range []float32{18000, 37000, 39750, 41900} {
			dip = min(dip, clamp(abs(t-at)/140, 0, 1))
		}
		chrome := seg(t, 17950, 18400, easeInOut)
		if r {
			chrome = seg(t, 8700, 9200, easeInOut)
		}
		title, sub := v.docTitle(o, s)
		titleIn := float32(1)
		if !r {
			titleIn = seg(t, 8700, 9000, easeOut)
		}
		ui.Row(c.Key("hd")).Absolute().Left(0).Right(0).Top(0).Height(vd.hd).Gap(12).Padding(0, 8, 2, 16).AlignItems(ui.Center).InsetShadow(0, -1, 0, 0, vd.rim2).Children(func() {
			ui.Row(c.Key("title")).Shrink(1).MinWidth(0).Opacity(titleIn * dip).Clip().Children(func() {
				line(c, title, vd.fs, vd.ink).FontWeight(600)
			})
			ui.Row(c.Key("sub")).Grow(1).Basis(0).MinWidth(0).Opacity(dip).Clip().Children(func() { line(c, sub, 12, vd.ink2) })
			ui.Row(c.Key("sw")).Opacity(chrome).Shrink(0).Children(func() {
				segmentedShown(c, vd, []string{"Chat", "Terminal"}, false)
			})
			ui.Box(c.Key("more")).Size(vd.ctl, vd.ctl).Center().Opacity(chrome).Shrink(0).Children(func() {
				ui.Icon(c, icon.More).FontSize(14).TextColor(vd.ink2)
			})
		})
		v.docTabs(c, o, s, chrome)
		ui.Box(c.Key("scroll")).Absolute().Left(0).Right(0).Top(vd.hd + vd.tabh).Bottom(0).Clip().Children(func() {
			v.setupLog(c, o, s)
			v.sampleChat(c, s)
			v.sampleGOA(c, o, s)
			v.taskConversation(c, o, s)
		})
		v.composer(c, o, s)
	})
}

// docTitle is the header's words, for what the region shows now.
func (v *View) docTitle(o Options, s scene) (title, sub string) {
	t := s.t
	_, name := firstAgent(o)
	project := v.projectName(o)
	switch {
	case !s.returning && t < 18000:
		return "Setup", ""
	case !s.returning && t >= 37000 && t < 39750:
		return "GOD of Agents", nonEmpty(v.goaName(o), "Claude Code") + " · " + o.HostName
	case v.taskRunning(s):
		return v.task.prompt, strings.Join(nonEmptyOf(name, project, o.HostName), " · ")
	}
	return "New session", strings.Join(nonEmptyOf(name, project, o.HostName), " · ")
}

func nonEmptyOf(words ...string) []string {
	var out []string
	for _, w := range words {
		if w != "" {
			out = append(out, w)
		}
	}
	return out
}

func (v *View) docTabs(c *ui.Context, o Options, s scene, chrome float32) {
	vd := s.doc.v
	t := s.t
	r := s.returning
	tabA := seg(t, 8700, 9000, easeOut)
	tabB := seg(t, 17900, 18200, easeInOut)
	if r {
		tabA, tabB = 0, 1
	}
	in := tabA
	if r {
		in = seg(t, 8600, 9000, easeInOut)
	}
	id, name := firstAgent(o)
	ui.Row(c.Key("tabs")).Absolute().Left(0).Right(0).Top(vd.hd).Height(vd.tabh).Gap(4).Padding(0, 4, 1, 8).AlignItems(ui.Center).InsetShadow(0, -1, 0, 0, vd.rim2).Children(func() {
		ui.Box(c.Key("tab")).Width(160).Height(vd.tabh - 8).Radius(vd.rt).Background(vd.sel).Opacity(in).Shrink(0).Children(func() {
			col := vd.idle
			if v.taskRunning(s) {
				col = dotWork
			}
			dotAt := (vd.tabh - 8 - 7) / 2
			ui.Box(c.Key("dot")).Absolute().Left(8).Top(dotAt).Size(7, 7).Radius(3.5).Background(col)
			label := func(key string, a float32, svg *ui.SVG, word string) {
				if a <= .004 {
					return
				}
				ui.Row(c.Key(key)).Absolute().Left(30).Right(6).Top(0).Bottom(1).Gap(4).AlignItems(ui.Center).Opacity(a).Children(func() {
					ui.Icon(c, svg).FontSize(16).TextColor(vd.onsel).Shrink(0)
					ui.Row(c).Grow(1).MinWidth(0).Clip().Children(func() { line(c, word, 12, vd.onsel) })
				})
			}
			label("log", tabA*(1-tabB), icon.Terminal, "Setup log")
			label("agent", tabB, icon.Agent(agent.Base(id)), nonEmpty(name, "New session"))
		})
		ui.Spacer(c)
		for i, svg := range []*ui.SVG{icon.Plus, icon.Play} {
			ui.Box(c.Key(i)).Size(24, 24).Center().Opacity(chrome).Shrink(0).Children(func() {
				ui.Icon(c, svg).FontSize(13).TextColor(vd.ink2)
			})
		}
	})
}

// setupLog is what setup finds, a line for each thing, what it found in the third column.
func (v *View) setupLog(c *ui.Context, o Options, s scene) {
	if s.returning {
		return
	}
	t := s.t
	a := seg(t, 8900, 9200, easeOut) * (1 - seg(t, 17500, 17900, easeIn))
	if a <= .004 {
		return
	}
	vd := s.doc.v
	mono := func(col ui.Color, text string) ui.Element {
		return ui.Text(c, text).Font(theme.MonoFont).FontSize(12).LineHeight(20.0 / 12).TextColor(col).SingleLine()
	}
	row := func(key string, at float32, first, second, third string, thirdIn float32, thirdCol ui.Color) {
		al, rise := show(t, at)
		if al <= .004 {
			return
		}
		ui.Box(c.Key(key)).FillWidth().Height(20).Opacity(al).Translate(0, rise).Children(func() {
			ui.Box(c.Key("a")).Absolute().Left(0).Top(0).Children(func() { mono(vd.ink, first) })
			ui.Box(c.Key("b")).Absolute().Left(96).Top(0).Children(func() { mono(vd.ink2, second) })
			if third != "" {
				ui.Box(c.Key("c")).Absolute().Left(344).Top(0).Opacity(thirdIn).Children(func() { mono(thirdCol, third) })
			}
		})
	}
	ui.Column(c.Key("g-log")).Absolute().Left(16).Right(16).Top(12).Opacity(a).Children(func() {
		ui.Box(c.Key("head")).FillWidth().Height(20).Opacity(seg(t, 8950, 9200, easeOut)).Children(func() {
			mono(vd.ink2, "Looking for agents on "+o.Device)
		})
		rows := listRows(o)
		for i, ag := range rows {
			at := 9100 + float32(i)*420
			second, third, col := "not found", "", vd.ink2
			if ag.Installed {
				second, third, col = ag.Command, "found", addInk
			}
			row(ag.ID, at, ag.Command, second, third, seg(t, at+260, at+420, easeInOut), col)
		}
		if p, ok := v.project(o); ok {
			third := ""
			if p.Branch != "" {
				third = "branch " + p.Branch
			}
			row("project", 14650, "project", p.Path, third, 1, vd.ink2)
		}
	})
}

// group is where a conversation's rows lie: down the middle, the design's width.
func group(c *ui.Context, key string, s scene, a float32, gap float32, fn func()) {
	vd := s.doc.v
	width := min(640, s.doc.r.w-2*vd.lw-32)
	left := (s.doc.r.w - 2*vd.lw - width) / 2
	ui.Column(c.Key(key)).Absolute().Left(left).Width(width).Top(16).Gap(gap).Opacity(a).TextColor(vd.ink).Children(fn)
}

// para is a paragraph of a conversation.
func para(c *ui.Context, text string, v look) ui.Element {
	return ui.Text(c, text).FontSize(v.fs).LineHeight(20 / v.fs).TextColor(v.ink).FillWidth()
}

// bubble is the user's message.
func bubble(c *ui.Context, text string, v look, bg, fg ui.Color) ui.Element {
	return ui.Text(c, text).FontSize(v.fs).LineHeight(20/v.fs).TextColor(fg).MaxWidth(520).AlignSelf(ui.End).Padding(4, 12).Radius(v.rb).Background(bg)
}

// step is a line of what an agent did: what it did, and how that went at its right.
func step(c *ui.Context, lead func(), words func(), right string, rightCol ui.Color) {
	ui.Row(c).FillWidth().Height(20).Gap(8).AlignItems(ui.Center).Children(func() {
		lead()
		ui.Row(c.Key("words")).Grow(1).MinWidth(0).Gap(4).AlignItems(ui.Center).Clip().Children(words)
		if right != "" {
			line(c.Key("right"), right, 12, rightCol).Shrink(0)
		}
	})
}

func code(c *ui.Context, text string, col ui.Color) ui.Element {
	return ui.Text(c, text).Font(theme.MonoFont).FontSize(12).LineHeight(20.0 / 12).TextColor(col).SingleLine()
}

func tagLabel(c *ui.Context, text string, v look, a float32) {
	ui.Row(c.Key("tag")).Height(20).PaddingX(8).Radius(v.rt).Background(v.fill).AlignItems(ui.Center).AlignSelf(ui.Start).Opacity(a).Children(func() {
		line(c, text, 12, v.ink2)
	})
}

// sampleChat shows the style on a conversation, marked a sample.
func (v *View) sampleChat(c *ui.Context, s scene) {
	t := s.t
	on := seg(t, 18700, 18800, easeInOut) * (1 - seg(t, 36860, 37160, easeInOut))
	if s.returning {
		on = 0
		if v.detour != nil && v.detour.id == "style" {
			on = 1
		}
	}
	if on <= .004 {
		return
	}
	vd := s.doc.v
	kid := func(i int, fn func()) {
		a, rise := show(t, 18700+float32(i)*120)
		if s.returning {
			a, rise = 1, 0
		}
		ui.Column(c.Key(i)).FillWidth().Opacity(a).Translate(0, rise).Children(fn)
	}
	icon14 := func(svg *ui.SVG) func() {
		return func() { ui.Icon(c, svg).FontSize(14).TextColor(vd.ink2).Shrink(0) }
	}
	group(c, "g-prev", s, on, 12, func() {
		kid(0, func() { tagLabel(c, "Sample conversation, to show the style", vd, 1) })
		kid(1, func() { bubble(c, "Does it hold for a session on a server?", vd, vd.accent, vd.ona) })
		kid(2, func() {
			para(c, "It should: the tab attaches through that host's client, the same path as this Mac's, and a stopped session is continued first.", vd)
		})
		kid(3, func() {
			step(c, icon14(icon.File), func() { code(c, "Edited ui/goa_tabs.go", vd.ink2) }, "+211", addInk)
		})
		kid(4, func() {
			step(c, icon14(icon.Terminal), func() { code(c, "Ran go test ./ui", vd.ink2) }, "passed", addInk)
		})
		kid(5, func() {
			step(c, func() { dot(c, 8, dotWork) }, func() { code(c, "Editing ui/goa_test.go", vd.focus) }, "Working", vd.focus)
		})
	})
}

// sampleGOA is what GOD of Agents does, in its own words.
func (v *View) sampleGOA(c *ui.Context, o Options, s scene) {
	t := s.t
	on := seg(t, 37800, 37900, easeInOut) * (1 - seg(t, 39440, 39740, easeInOut))
	if s.returning || on <= .004 {
		return
	}
	vd := s.doc.v
	kid := func(key string, at float32, fn func()) {
		a, rise := show(t, at)
		ui.Column(c.Key(key)).FillWidth().Opacity(a).Translate(0, rise).Children(fn)
	}
	icon14 := func(svg *ui.SVG) func() {
		return func() { ui.Icon(c, svg).FontSize(14).TextColor(vd.ink2).Shrink(0) }
	}
	delegated := func(name string) func() {
		return func() {
			line(c, "Delegated to", 12, vd.ink2)
			line(c, name, 12, vd.ink).FontWeight(500)
		}
	}
	group(c, "g-goa", s, on, 12, func() {
		tagLabel(c, "Sample", vd, seg(t, 37800, 38050, easeOut))
		kid("ga0", 37900, func() { bubble(c, "Fix the failing check, then update the README.", vd, vd.accent, vd.ona) })
		kid("ga1", 38250, func() {
			para(c, "I gave the check to Codex and the README to Claude Code. I'll report when both are back.", vd)
		})
		kid("ga2", 38600, func() {
			step(c, icon14(icon.Sparkle), delegated("Codex"), "fix the failing check", vd.ink2)
		})
		kid("ga3", 38850, func() {
			step(c, icon14(icon.Sparkle), delegated("Claude Code"), "update the README", vd.ink2)
		})
		kid("ga4", 39100, func() {
			step(c, icon14(icon.Lock), func() {
				line(c, "Codex", 12, vd.ink).FontWeight(500)
				line(c, "asks to run", 12, vd.ink2)
				code(c, "git push", vd.ink2)
			}, "yours to answer", dotWait)
		})
	})
}
