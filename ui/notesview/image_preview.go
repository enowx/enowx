package notesview

import (
	"github.com/egoist/mygo/ui"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

type imagePreview struct {
	open      bool
	name, ref string
}

func (v *View) imageDialog(c *ui.Context, o Options) {
	p := v.preview
	if p == nil || !p.open {
		return
	}
	w, h := c.Size()
	widget.Dialog(c, &p.open, min(960, w-40), theme.S3, func() {
		ui.Row(c).FillWidth().Gap(theme.S2).Children(func() {
			ui.Row(c).Grow(1).MinWidth(0).Children(func() {
				widget.Running(c, p.name, widget.RunStyle{Size: theme.TextTitle, Weight: 600, Always: true})
			})
			if widget.IconButton(c, icon.Close, "Close image", false).Clicked() {
				p.open = false
			}
		})
		r := fetch(p.ref, true, o.Asset)
		if r.img == nil {
			preview := editor{env: pageEnv{width: min(900, w-80), asset: o.Asset}}
			preview.embedView(c, p.ref)
			return
		}
		iw, ih := r.size()
		scale := min(float32(1), max(1, w-80)/iw, max(1, h-140)/ih)
		ui.Image(c, r.img).Size(iw*scale, ih*scale).AlignSelf(ui.Center).Radius(theme.RadiusCard)
	})
}
