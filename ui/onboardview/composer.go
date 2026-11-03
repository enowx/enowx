package onboardview

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
)

// The first task: a real message to a real session, typed in a composer that opens from
// its centre, and rising from it to the conversation once it is sent.

// taskState is the first task's: what's typed, and how it went.
type taskState struct {
	text    string
	prompt  string // what was sent
	sent    bool
	sending bool
	err     string
}

// taskRunning says the session is at work on the first task, as far as this moment shows.
func (v *View) taskRunning(s scene) bool {
	return v.task.sent && !s.returning && s.t >= 41900
}

// taskConversation is the conversation so far: empty, then the message, then its session
// at work. What it answers shows in its own session, where the user goes next.
func (v *View) taskConversation(c *ui.Context, o Options, s scene) {
	t := s.t
	on := seg(t, 40050, 40350, easeInOut)
	if s.returning {
		on = 1
		if v.detour != nil && v.detour.id == "style" {
			on = 0
		}
	}
	if on <= .004 {
		return
	}
	vd := s.doc.v
	lift := float32(0)
	if v.task.sent && !s.returning {
		lift = seg(t, 41250, 41880, easeInOut)
	}
	_, name := firstAgent(o)
	group(c, "g-task", s, on, 12, func() {
		// It keeps its room once the conversation starts, so nothing jumps.
		a := seg(t, 40100, 40400, easeInOut)
		if s.returning {
			a = 1
		}
		if lift > 0 {
			a = 0
		}
		ui.Box(c.Key("empty")).Height(20).Opacity(a).Children(func() {
			ui.Text(c, "No messages yet.").FontSize(12).LineHeight(20.0 / 12).TextColor(vd.ink2).SingleLine()
		})
		if lift <= 0 {
			return
		}
		// It leaves the composer's field and lands as a message: the field's place is
		// worked out from the region's, as the composer's own is.
		_, _, cy := composerPlace(s)
		fx := -40 * (1 - lift)
		fy := (cy - vd.hd - vd.tabh - 16 - 32) * (1 - lift)
		fg := vd.ink
		if lift >= .4 {
			fg = vd.ona
		}
		bubble(c.Key("bubble"), v.task.prompt, vd, fadeColor(vd.accent, lift), fg).Translate(fx, fy)
		if a, rise := show(t, 42350); a > .004 {
			ui.Column(c.Key("work")).FillWidth().Opacity(a).Translate(0, rise).Children(func() {
				step(c, func() { dot(c, 8, dotWork) }, func() {
					code(c, nonEmpty(name, "The agent")+" is working in its session", vd.focus)
				}, "Working", vd.focus)
			})
		}
	})
}

// composerPlace is the composer's left, width and top in the region's inside.
func composerPlace(s scene) (left, width, top float32) {
	vd := s.doc.v
	inner := s.doc.r.w - 2*vd.lw
	width = min(lerp(inner, 640, vd.cf), inner-lerp(0, 32, vd.cf))
	left = (inner - width) / 2
	top = s.doc.r.h - 2*vd.lw - lerp(0, 16, vd.cf) - composerHeight
	return left, width, top
}

const composerHeight = 84

func (v *View) composer(c *ui.Context, o Options, s scene) {
	t := s.t
	vd := s.doc.v
	open, in, base := seg(t, 40250, 40900, easeInOut), seg(t, 40250, 40420, easeInOut), float32(40680)
	if s.returning {
		open, in, base = seg(t, 8600, 9300, easeInOut), seg(t, 8600, 8800, easeInOut), 8900
	}
	if in <= .004 {
		return
	}
	left, width, top := composerPlace(s)
	cut := (1 - open) * width / 2
	ui.Box(c.Key("in-clip")).Absolute().Left(left+cut).Top(top+(1-open)*10).Size(width-2*cut, composerHeight).Radius(vd.cr).Opacity(in).Clip().Children(func() {
		box := ui.Column(c.Key("in")).Absolute().Left(-cut).Top(0).Size(width, composerHeight).Radius(vd.cr).Background(vd.comp).Padding(8, 12).Gap(4).
			InsetShadow(0, 1, 0, 0, vd.gl).InsetShadow(0, 0, 0, vd.lw, vd.rim)
		box.Children(func() { v.composerInside(c, o, s, base) })
	})
}

func (v *View) composerInside(c *ui.Context, o Options, s scene, base float32) {
	t := s.t
	vd := s.doc.v
	id, name := firstAgent(o)
	live := v.current().id == "task" && id != "" && !v.task.sent && !s.returning && len(o.Projects) > 0
	place := "Message " + nonEmpty(name, "your agent") + ", / for commands, @ for files"
	ui.Box(c.Key("msg")).Grow(1).FillWidth().MinHeight(0).Opacity(seg(t, base, base+300, easeOut)).Children(func() {
		if live {
			// Enter sends, Shift+Enter breaks the line, as in a chat.
			ui.TextAreaBase(c.Key("field"), &v.task.text).Fill().FontSize(vd.fs).LineHeight(lead13).TextColor(vd.ink).Placeholder(place).Label("Message").AutoFocus().
				OnTextKey(func(k ui.TextKey) bool {
					if k.Mods != 0 || k.Key != ui.KeyEnter {
						return false
					}
					if strings.TrimSpace(k.Text()) != "" && !v.task.sending {
						v.sendTask(o, id)
					}
					return true
				})
			return
		}
		line(c.Key("hint"), place, vd.fs, vd.ink)
	})
	ui.Row(c.Key("controls")).FillWidth().Height(28).Gap(2).AlignItems(ui.Center).Shrink(0).Children(func() {
		chip := func(i int, fn func()) {
			a := seg(t, base+40+float32(i)*60, base+300+float32(i)*60, easeOut)
			ui.Row(c.Key(i)).Height(24).PaddingX(8).Gap(4).AlignItems(ui.Center).Opacity(a).Shrink(0).Children(fn)
		}
		chip(0, func() { ui.Icon(c, icon.Paperclip).FontSize(14).TextColor(vd.ink2) })
		chip(1, func() {
			line(c, "Its default model", 12, vd.ink2)
			ui.Icon(c, icon.ChevronDown).FontSize(14).TextColor(vd.ink2)
		})
		chip(2, func() {
			line(c, "Ask first", 12, vd.ink2)
			ui.Icon(c, icon.ChevronDown).FontSize(14).TextColor(vd.ink2)
		})
		ui.Spacer(c)
		sx := clamp((t-(base+200))/320, 0, 1)
		ready := live && strings.TrimSpace(v.task.text) != "" && !v.task.sending
		bg, fg := vd.fill, vd.ink2
		if ready {
			bg, fg = vd.accent, vd.ona
		}
		scale := .72 + .28*easeBack(sx)
		if sx <= 0 {
			return
		}
		if !ready {
			ui.Box(c.Key("send")).Size(28, 28).Radius(vd.rc).Background(bg).Center().Scale(scale, scale).Shrink(0).Children(func() {
				ui.Icon(c, icon.ArrowUp).FontSize(14).TextColor(fg)
			})
			return
		}
		b := ui.ButtonBase(c.Key("send")).Size(28, 28).Radius(vd.rc).Background(bg).Center().Scale(scale, scale).Label("Send").Cursor(ui.CursorPointer).Shrink(0)
		b.Children(func() { ui.Icon(c, icon.ArrowUp).FontSize(14).TextColor(fg) })
		if b.Clicked() {
			v.sendTask(o, id)
		}
	})
}

// sendTask starts the first task in a session of its own.
func (v *View) sendTask(o Options, agentID string) {
	prompt := strings.TrimSpace(v.task.text)
	p, ok := v.project(o)
	if !ok {
		return
	}
	v.task.sending, v.task.err = true, ""
	o.StartTask(agentID, p.Path, prompt, func(err error) {
		v.task.sending = false
		if err != nil {
			v.task.err = err.Error()
			return
		}
		v.task.sent, v.task.prompt, v.task.text = true, prompt, ""
		v.next(o) // on to the session at work
	})
}
