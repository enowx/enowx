package canvasview

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// shareState is the Share link dialog: the canvas's link once made.
type shareState struct {
	id    string
	link  *proto.CanvasShare
	busy  bool
	err   string
	saved string // the address copied last
}

// openShare opens the dialog, making the canvas's link (or reading the
// one it has).
func (v *View) openShare(o Options, id string) {
	st := &shareState{id: id, busy: true}
	v.sharing = st
	o.Share(id, false, func(x proto.CanvasShare, err error) {
		st.busy = false
		if err != nil {
			st.err = err.Error()
			return
		}
		st.link = &x
	})
}

func (v *View) shareDialog(c *ui.Context, o Options) {
	st := v.sharing
	if st == nil || o.Share == nil {
		return
	}
	open := true
	widget.Dialog(c, &open, 520, theme.S3, func() {
		widget.DialogHeader(c, "Share a link", "For this device and your tailnet.", false)
		switch {
		case st.busy:
			ui.Text(c.Key("busy"), "Making the link…").FontSize(theme.TextUI).TextColor(theme.TextSoft)
		case st.err != "":
			ui.Text(c.Key("err"), st.err).FontSize(theme.TextUI).TextColor(theme.Error)
		case st.link != nil:
			widget.Field(c, "Link", func() {
				ui.Column(c.Key("links")).FillWidth().Gap(theme.S2).Children(func() {
					for _, u := range st.link.URLs {
						ui.Row(c.Key(u)).FillWidth().Gap(theme.S2).AlignItems(ui.Center).Children(func() {
							widget.Well(ui.Row(c.Key("url")).Grow(1).MinWidth(0).Height(theme.ControlHeight).PaddingX(theme.S3).Radius(theme.RadiusControl), false).Children(func() {
								widget.Running(c, u, widget.RunStyle{})
							})
							label := "Copy"
							if st.saved == u {
								label = "Copied"
							}
							if widget.Button(c.Key("copy"), widget.Primary, nil, label, "").Clicked() {
								c.WriteClipboard(u)
								st.saved = u
							}
						})
					}
				})
			})
			ui.Text(c.Key("about"), "A read-only page of this canvas, which reloads as it changes. Whoever has the link sees it; nothing goes to the internet. It works while this host runs, until you stop sharing.").
				FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		}
		widget.DialogFooter(c, "", func() {
			if st.link != nil && widget.DangerButton(c, "Stop sharing").Clicked() {
				confirmed(o, "Stop sharing this canvas?", "Its link stops working for whoever has it. Sharing again makes a new link.", "Stop sharing", func() {
					o.Share(st.id, true, func(_ proto.CanvasShare, err error) {
						if err != nil {
							st.err = err.Error()
							return
						}
						v.sharing = nil
					})
				})
			}
			if widget.GhostButton(c, "Done").Clicked() {
				open = false
			}
		})
	})
	if !open {
		v.sharing = nil
	}
}
