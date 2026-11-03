package onboardview

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
)

// The session list: slots waiting as ghost rows, filled one by one as each agent lands in
// its place. Places are worked out here because what flies needs them too.

// listRows are the agents the panel lists and the list has slots for: those found, then
// others up to the design's six, never more than fit.
func listRows(o Options) []proto.AgentInfo {
	const six, most = 6, 9
	var found, not []proto.AgentInfo
	for _, a := range o.Agents {
		switch {
		case a.Command == "":
		case a.Installed:
			found = append(found, a)
		default:
			not = append(not, a)
		}
	}
	out := found[:min(len(found), most)]
	return append(out, not[:min(len(not), max(six-len(out), 0))]...)
}

func foundOf(rows []proto.AgentInfo) int {
	n := 0
	for _, a := range rows {
		if a.Installed {
			n++
		}
	}
	return n
}

// listPush is how far GOD of Agents' node pushes the list down and in.
func listPush(s scene) (push, indent float32) {
	if s.returning {
		return 0, 0
	}
	w := clamp(seg(s.t, 37350, 37800, easeBack), 0, 1.2) * (1 - seg(s.t, 39980, 40380, easeInOut))
	return w * 42, w * 22
}

// rowY is the top of list row n, in the region's inside.
func rowY(s scene, n float32) float32 {
	vs := s.sb.v
	push, _ := listPush(s)
	return vs.ctl + 16 + push + 24 + n*vs.rowh
}

// slotIcon is the middle of the icon in list row n, in the window.
func slotIcon(s scene, n float32) (float32, float32) {
	vs := s.sb.v
	_, ind := listPush(s)
	return s.sb.r.x + vs.lw + 8 + ind + 8 + 8, s.sb.r.y + vs.lw + rowY(s, n) + vs.rowh/2
}

// projectY is the top of the project's row, in the region's inside, once the list has
// shrunk from its six slots to what was found.
func projectY(s scene, found int) float32 {
	count := lerp(6, float32(found), seg(s.t, 12950, 13450, easeInOut))
	if s.returning {
		count = float32(found)
	}
	return rowY(s, count) + 12 + 20
}

// landing says how far agent n has landed: its icon, then its words.
func landing(s scene, n int) (at, icons, words, kick float32) {
	at = 11900 + float32(n)*80 + 800
	if s.returning {
		return at, 1, 1, 0
	}
	return at, seg(s.t, at-30, at+60, easeInOut), seg(s.t, at+20, at+280, easeOut), max(0, 1-abs(s.t-(at+70))/150)
}

func (v *View) sideRegion(c *ui.Context, o Options, s scene) {
	regionBox(c, "ob-side", s.sb, s.sb.v.side, func() {
		vs := s.sb.v
		if a := s.matS; a > .004 {
			ui.Row(c.Key("head")).Absolute().Left(8).Right(8).Top(8).Height(vs.ctl).Gap(4).Opacity(a).Children(func() {
				ui.Row(c).Grow(1).MinWidth(0).Children(func() {
					segmentedShown(c, vs, []string{"Sessions only", "By project"}, true)
				})
				ui.Box(c.Key("close")).Size(vs.ctl, vs.ctl).Center().Shrink(0).Children(func() {
					ui.Icon(c, icon.ChevronLeft).FontSize(14).TextColor(vs.ink2)
				})
			})
		}
		sbA := float32(1)
		if !s.returning {
			sbA = seg(s.t, 8600, 9000, easeOut)
		}
		if sbA <= .004 {
			return
		}
		push, ind := listPush(s)
		head := vs.ctl + 16
		ui.Box(c.Key("h-s")).Absolute().Left(12 + ind).Top(head + push + 2).Opacity(sbA).Children(func() {
			ui.Text(c, "Sessions").FontSize(12).LineHeight(lead12).FontWeight(500).TextColor(vs.ink2).SingleLine()
		})
		rows := listRows(o)
		found := foundOf(rows)
		for pos := range max(len(rows), 6) {
			v.listSlot(c, o, s, rows, found, pos, ind)
		}
		v.projectSlot(c, o, s, found)
	})
}

// listSlot is row pos of the list: a ghost, then what landed there.
func (v *View) listSlot(c *ui.Context, o Options, s scene, rows []proto.AgentInfo, found, pos int, ind float32) {
	vs := s.sb.v
	t := s.t
	ghost := float32(0)
	if !s.returning {
		ghost = seg(t, 8700+float32(pos)*50, 9000+float32(pos)*50, easeOut)
	}
	box := ui.Box(c.Key(pos)).Absolute().Left(8+ind).Top(rowY(s, float32(pos))).Size(230-ind, vs.rowh).Radius(vs.rr).Clip()
	if pos >= found {
		ghost *= (1 - seg(t, 12900, 13250, easeIn)) * .9
		box.Children(func() { ghostRow(c, ghost) })
		return
	}
	at, icons, words, kick := landing(s, pos)
	if !s.returning {
		ghost *= 1 - seg(t, at-220, at-20, easeInOut)
	}
	ag := rows[pos]
	work := pos == 0 && v.taskRunning(s)
	selected := float32(0)
	if pos == 0 {
		switch {
		case s.returning:
			selected = 1
		case t >= 18300:
			selected = seg(t, 18300, 18600, easeInOut)
		}
	}
	ink, ink2 := vs.ink, vs.ink2
	if selected > .5 {
		ink, ink2 = vs.onsel, vs.onsel
	}
	if selected > .004 {
		box.Background(fadeColor(vs.sel, selected))
	}
	box.Children(func() {
		ghostRow(c, ghost)
		ui.Row(c.Key("row")).Fill().Gap(8).PaddingX(8).AlignItems(ui.Center).Children(func() {
			scale := 1 + .18*kick
			ui.Icon(c.Key("icon"), icon.Agent(agent.Base(ag.ID))).FontSize(16).TextColor(ink).Shrink(0).Opacity(icons).Scale(scale, scale)
			ui.Column(c.Key("words")).Grow(1).MinWidth(0).Opacity(words).Translate((1-words)*6, 0).Children(func() {
				title, sub := ag.Name, "Ready"
				if work {
					title, sub = v.task.prompt, v.projectName(o)
				}
				ui.Row(c.Key("title")).FillWidth().MinWidth(0).Clip().Children(func() { line(c, title, vs.fs, ink) })
				ui.Row(c.Key("sub")).FillWidth().MinWidth(0).Clip().Children(func() { line(c, sub, 12, ink2) })
			})
			col := vs.idle
			if work {
				col = dotWork
			}
			dot(c.Key("dot"), 8, col).Opacity(words)
		})
	})
}

// ghostRow is a slot waiting: a square and two bars.
func ghostRow(c *ui.Context, a float32) {
	if a <= .004 {
		return
	}
	ui.Row(c.Key("ghost")).Absolute().Left(8).Right(8).Top(0).Bottom(0).Gap(8).AlignItems(ui.Center).Opacity(a).Children(func() {
		ui.Box(c.Key("square")).Size(16, 16).Radius(4).Background(frameLine).Shrink(0)
		ui.Column(c.Key("bars")).Grow(1).MinWidth(0).Gap(4).Children(func() {
			ui.Box(c.Key("a")).Width(118).Height(6).Radius(4).Background(frameLine)
			ui.Box(c.Key("b")).Width(68).Height(6).Radius(4).Background(frameLine).Opacity(.6)
		})
	})
}

// projectSlot is the Projects head and its row, where the folder lands.
func (v *View) projectSlot(c *ui.Context, o Options, s scene, found int) {
	vs := s.sb.v
	t := s.t
	y := projectY(s, found)
	sbA := float32(1)
	if !s.returning {
		sbA = seg(t, 8600, 9000, easeOut)
	}
	ui.Box(c.Key("h-p")).Absolute().Left(12).Top(y - 20).Opacity(sbA).Children(func() {
		ui.Text(c, "Projects").FontSize(12).LineHeight(lead12).FontWeight(500).TextColor(vs.ink2).SingleLine()
	})
	const land = 14750
	name := v.projectName(o)
	ghost, icons, words, kick := float32(0), float32(1), float32(1), float32(0)
	if !s.returning {
		ghost = seg(t, 8900, 9200, easeOut)
		if name != "" {
			ghost *= 1 - seg(t, land-220, land-20, easeInOut)
			icons, words = seg(t, land-30, land+60, easeInOut), seg(t, land+20, land+280, easeOut)
			kick = max(0, 1-abs(t-(land+70))/150)
		}
	}
	ui.Box(c.Key("project")).Absolute().Left(8).Top(y).Size(230, vs.rowh).Radius(vs.rr).Clip().Children(func() {
		ghostRow(c, ghost)
		if name == "" {
			return
		}
		ui.Row(c.Key("row")).Fill().Gap(8).PaddingX(8).AlignItems(ui.Center).Children(func() {
			scale := 1 + .18*kick
			ui.Icon(c.Key("icon"), icon.Folder).FontSize(14).TextColor(vs.ink).Shrink(0).Opacity(icons).Scale(scale, scale)
			ui.Column(c.Key("words")).Grow(1).MinWidth(0).Opacity(words).Children(func() {
				ui.Row(c.Key("title")).FillWidth().MinWidth(0).Clip().Children(func() { line(c, name, vs.fs, vs.ink) })
				line(c.Key("sub"), o.HostName, 12, vs.ink2)
			})
		})
	})
}
