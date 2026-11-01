package chatview

import (
	"path"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
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
	ui.Row(c.Key("pictures")).FillWidth().Gap(6).Wrap().Children(func() {
		for _, p := range pics {
			b := ui.ButtonBase(c.Key(p)).Height(26).Padding(0, 9).Gap(6).Radius(theme.RadiusSmall).AlignItems(ui.Center).
				Background(theme.Raised).Border(1, theme.Border).Cursor(ui.CursorPointer).Label("Show " + path.Base(p)).Tooltip(p)
			if b.Hovered() {
				b.Border(1, theme.AccentLine)
			}
			b.Children(func() {
				ui.Icon(c, icon.Image).FontSize(12).TextColor(theme.TextSoft)
				ui.Text(c, path.Base(p)).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().MaxWidth(220)
			})
			if b.Clicked() && ch.links != nil {
				ch.links(p)
			}
		}
	})
}
