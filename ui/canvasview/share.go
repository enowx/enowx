package canvasview

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
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
	widget.Dialog(c, &open, 520, 14, func() {
		widget.DialogHeader(c, "Share a link", "A read-only page of this canvas, as it is when opened; it reloads as the canvas changes.", false)
		switch {
		case st.busy:
			ui.Text(c, "Making the link…").FontSize(theme.TextLabel).TextColor(theme.Muted)
		case st.err != "":
			ui.Text(c, st.err).FontSize(theme.TextLabel).TextColor(theme.Error)
		case st.link != nil:
			for _, u := range st.link.URLs {
				ui.Row(c.Key(u)).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, u).FontSize(theme.TextLabel).TextColor(theme.Text).Grow(1).Shrink(1).SingleLine().Selectable()
					label := "Copy"
					if st.saved == u {
						label = "Copied"
					}
					if widget.Button(c, widget.Secondary, icon.Copy, label, "").Height(26).PaddingX(10).Clicked() {
						c.WriteClipboard(u)
						st.saved = u
					}
				})
			}
			ui.Text(c, "Whoever has the link sees it, on this device or your tailnet; nothing goes to the internet. It works while this host runs, until you stop sharing.").
				FontSize(11).TextColor(theme.Muted)
		}
		widget.DialogFooter(c, "", func() {
			if st.link != nil && widget.Button(c, widget.Danger, nil, "Stop sharing", "").Clicked() {
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
