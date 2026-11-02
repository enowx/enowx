package chatview

import (
	"path"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Pictures a message names (the user's attached screenshots, written into
// it as paths): a chip each under it, opening the picture in the side
// panel.

var pictureExts = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".svg": true}

// picturePaths are the paths of pictures in text, as attachments write
// them: absolute, shell-escaped, separated by spaces.
func picturePaths(text string) []string {
	var out []string
	var b strings.Builder
	escaped := false
	flush := func() {
		p := b.String()
		b.Reset()
		if strings.HasPrefix(p, "/") && pictureExts[strings.ToLower(path.Ext(p))] {
			out = append(out, p)
		}
	}
	for _, r := range text {
		switch {
		case escaped:
			b.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case r == ' ' || r == '\n' || r == '\t':
			flush()
		default:
			b.WriteRune(r)
		}
	}
	flush()
	return out
}

// pictureChips are a message's pictures, each a click from showing.
func (ch *Chat) pictureChips(c *ui.Context, text string) {
	pics := picturePaths(text)
	if len(pics) == 0 {
		return
	}
	ui.Row(c.Key("pictures")).FillWidth().Justify(ui.End).Gap(theme.S1).Wrap().Children(func() {
		for _, p := range pics {
			b := ui.ButtonBase(c.Key(p)).Height(24).PaddingX(theme.S2).Gap(theme.S1).Radius(theme.RadiusFull).AlignItems(ui.Center).
				MaxWidth(240).Shrink(1).MinWidth(0).Cursor(ui.CursorPointer).Label("Show " + path.Base(p)).Tooltip(p)
			bg := theme.Raised
			if b.Hovered() {
				bg = theme.Selected
			}
			b.Background(bg).Children(func() {
				ui.Icon(c, icon.Image).FontSize(13).TextColor(theme.TextSoft).Shrink(0)
				widget.Running(c, path.Base(p), widget.RunStyle{Size: theme.TextLabel, Color: theme.Text})
			})
			if b.Clicked() && ch.links != nil {
				ch.links(p)
			}
		}
	})
}
