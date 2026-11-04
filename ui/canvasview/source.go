package canvasview

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/designshare"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// A canvas copied from the community library says whose it is, and tells of a
// newer version there. The copy never changes by itself.

// sourceState is what's known of the open canvas's original.
type sourceState struct {
	canvas string
	newer  proto.LibraryNewer
}

// sourceNote is the line above the stage of a copied canvas.
func (v *View) sourceNote(c *ui.Context, o Options, cv proto.Canvas) {
	s := cv.Source
	if s == nil {
		return
	}
	if v.source.canvas != cv.ID {
		v.source = sourceState{canvas: cv.ID}
		if o.LibraryNewer != nil {
			id := cv.ID
			o.LibraryNewer(id, func(n proto.LibraryNewer, err error) {
				if err == nil && v.source.canvas == id {
					v.source.newer = n
				}
			})
		}
	}
	n := v.source.newer
	ui.Row(c.Key("source")).FillWidth().MinWidth(0).Shrink(0).Gap(theme.S2).AlignItems(ui.Center).Padding(theme.S1, theme.S3).Children(func() {
		text := fmt.Sprintf("A copy of %s, version %d, %s", designshare.Credit(s.Title, s.Creator), s.Version, designshare.LicenseName(s.License))
		switch {
		case n.Gone:
			text += ". The original was taken back; your copy stays."
		case n.Latest > s.Version:
			text += fmt.Sprintf(". Version %d is out; your copy stays as it is.", n.Latest)
		}
		ui.Row(c.Key("words")).Grow(1).Basis(0).MinWidth(0).Children(func() {
			widget.Running(c, text, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
		})
		if s.Address != "" && o.OpenURL != nil && !n.Gone {
			word := "See the original"
			if n.Latest > s.Version {
				word = "See the new version"
			}
			if widget.GhostButton(c.Key("see"), word).Shrink(0).Clicked() {
				o.OpenURL(s.Address)
			}
		}
	})
}

// briefReferences lists what a canvas keeps of other people's designs, in its
// brief's dialog.
func briefReferences(c *ui.Context, cv *proto.Canvas) {
	if cv == nil || len(cv.References) == 0 {
		return
	}
	widget.Field(c.Key("references"), "References", func() {
		ui.Column(c).FillWidth().MinWidth(0).Gap(theme.S1).Children(func() {
			for i, line := range cv.References {
				widget.Running(c.Key(fmt.Sprint("ref", i)), line, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			}
		})
	})
}
