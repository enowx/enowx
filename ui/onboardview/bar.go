package onboardview

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
)

// The title bar's four pieces and the tools dock, with the real controls' words and icons.

func (v *View) barRegions(c *ui.Context, s scene, o Options) {
	row := func(gap, left, right float32, fn func()) {
		ui.Row(c).Fill().Gap(gap).Padding(0, right, 0, left).AlignItems(ui.Center).Clip().Children(fn)
	}
	regionBox(c, "ob-ctx", s.ctx, s.ctx.v.side, func() {
		vc := s.ctx.v
		row(8, 12, 12, func() {
			ui.Row(c).Shrink(1).MinWidth(0).Children(func() { line(c, o.HostName, vc.fs, vc.ink) })
			ui.Icon(c, icon.ChevronsUD).FontSize(14).TextColor(vc.ink2).Shrink(0)
			if name := v.projectName(o); name != "" && (s.returning || s.t >= 14700) {
				ui.Row(c).Shrink(1).MinWidth(0).Clip().Children(func() { line(c, name, vc.fs, vc.ink2) })
			}
		})
	})
	regionBox(c, "ob-pg", s.pg, s.pg.v.side, func() {
		vp := s.pg.v
		row(2, 4, 4, func() {
			for i, name := range []string{"Sessions", "Notes", "Canvas", "GOD of Agents"} {
				bg, fg := rgba(0, 0, 0, 0), vp.ink2
				if i == 0 {
					bg, fg = vp.sel, vp.onsel
				}
				b := ui.Row(c.Key(name)).Height(vp.ctl).PaddingX(8).Radius(vp.rc).Background(bg).AlignItems(ui.Center).Shrink(0).Children(func() { line(c, name, vp.fs, fg) })
				if i == 3 {
					v.goaChip = b.Bounds()
				}
			}
			ui.Row(c.Key("more")).Height(vp.ctl).PaddingX(8).Gap(4).AlignItems(ui.Center).Shrink(0).Children(func() {
				line(c, "More", vp.fs, vp.ink2)
				ui.Icon(c, icon.ChevronDown).FontSize(14).TextColor(vp.ink2)
			})
		})
	})
	regionBox(c, "ob-fd", s.fd, s.fd.v.side, func() {
		vf := s.fd.v
		row(8, 12, 12, func() {
			ui.Icon(c, icon.Search).FontSize(14).TextColor(vf.ink2).Shrink(0)
			ui.Row(c).Grow(1).MinWidth(0).Clip().Children(func() { line(c, "Search sessions, hosts and actions", vf.fs, vf.ink2) })
			line(c, "⌘K", vf.fs, vf.ink2).Shrink(0)
		})
	})
	regionBox(c, "ob-act", s.act, s.act.v.side, func() {
		va := s.act.v
		row(2, 4, 4, func() {
			ui.Row(c.Key("new")).Height(va.ctl).PaddingX(8).Gap(4).AlignItems(ui.Center).Shrink(0).Children(func() {
				ui.Icon(c, icon.Plus).FontSize(14).TextColor(va.ink)
				line(c, "New session", va.fs, va.ink)
			})
			ui.Spacer(c)
			for i, svg := range []*ui.SVG{icon.Usage, icon.Settings, icon.Account} {
				ui.Box(c.Key(i)).Size(va.ctl, va.ctl).Center().Shrink(0).Children(func() {
					ui.Icon(c, svg).FontSize(14).TextColor(va.ink)
				})
			}
		})
	})
}

// railRegion is the tools dock: a capsule floating at the side in the styles that float.
func (v *View) railRegion(c *ui.Context, s scene) {
	regionBox(c, "ob-rail", s.rail, s.rail.v.side, func() {
		vr := s.rail.v
		ui.Column(c).Fill().Gap(4).Center().Children(func() {
			for i, svg := range []*ui.SVG{icon.Git, icon.Folder, icon.ListChecks, icon.Globe, icon.Traffic, icon.More} {
				ui.Box(c.Key(i)).Size(vr.rail-12, vr.rail-12).Center().Shrink(0).Children(func() {
					ui.Icon(c, svg).FontSize(14).TextColor(vr.ink2)
				})
			}
		})
	})
}
