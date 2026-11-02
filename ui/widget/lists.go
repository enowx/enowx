package widget

import (
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
)

// Pieces long pages are made of, so they stay short to look at: groups
// that fold, rows of one line with their actions behind a menu, and a log
// in a box of its own height.

// Group is a titled group of rows that folds, its count beside the title;
// body draws the rows while *open. It reports a click on the title.
func Group(c *ui.Context, title string, count int, open *bool, body func()) {
	ui.Column(c).FillWidth().Gap(2).Children(func() {
		h := ui.ButtonBase(c.Key("head")).FillWidth().Height(theme.ControlHeight).PaddingX(theme.S2).Gap(theme.S1).Radius(theme.RadiusFull).
			Justify(ui.Start).Cursor(ui.CursorPointer).Label(title)
		if h.Hovered() {
			h.Background(theme.Hover)
		}
		h.Children(func() {
			chev := icon.ChevronRight
			if *open {
				chev = icon.ChevronDown
			}
			ui.Icon(c, chev).FontSize(12).TextColor(theme.TextSoft)
			ui.Text(c, title).FontSize(theme.TextLabel).FontWeight(500).TextColor(theme.TextSoft).SingleLine().Shrink(1)
			if count > 0 {
				ui.Text(c, strconv.Itoa(count)).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
			}
		})
		if h.Clicked() {
			*open = !*open
		}
		if *open {
			body()
		}
	})
}

// Row is a row of one or two lines: a title, a quieter detail beside it
// and a note under it (each "" for none), with trailing controls at its
// right (a switch, a menu button).
func Row(c *ui.Context, title, detail, note string, trailing func()) ui.Element {
	r := ui.Row(c).FillWidth().Gap(theme.S2).MinHeight(36).Padding(theme.S1, theme.S3).Radius(theme.RadiusCard)
	if r.Hovered() {
		r.Background(theme.Hover)
	}
	r.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			ui.Row(c).FillWidth().Gap(theme.S2).Children(func() {
				ui.Text(c, title).FontSize(theme.TextUI).SingleLine().Shrink(0)
				if detail != "" {
					ui.Text(c, detail).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(1)
				}
			})
			if note != "" {
				ui.Text(c, note).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(1)
			}
		})
		if trailing != nil {
			ui.Row(c).Gap(theme.S1).Shrink(0).Children(trailing)
		}
	})
	return r
}

// MoreMenu is a ⋯ button whose menu items draws (MenuItem, calling done
// to close it), keyed by key in m.
func MoreMenu(c *ui.Context, m *ContextMenu, key string, items func(done func())) {
	b := IconButton(c.Key("more:"+key), icon.More, "More", false)
	m.OpenBelow(b, key, theme.ControlHeight)
	m.Show(c, b, key, items)
}

// LogView is a log's lines in a box of its own height, the newest first,
// each line one line, colored by its level (warn, error).
func LogView(c *ui.Context, text string, height float32) {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	slices.Reverse(lines)
	ui.Scroll(c).FillWidth().Height(height).Radius(theme.RadiusCard).Background(theme.Surface(theme.Background)).Children(func() {
		ui.Column(c).FillWidth().Padding(theme.S3, theme.S4).Children(func() {
			for i, line := range lines {
				col := theme.TextSoft
				switch lower := strings.ToLower(line); {
				case strings.Contains(lower, "level=error") || strings.Contains(lower, "error:"):
					col = theme.Error
				case strings.Contains(lower, "level=warn"):
					col = theme.Warning
				}
				ui.Text(c.Key(strconv.Itoa(i)), line).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(col).SingleLine().Tooltip(line)
			}
		})
	})
}
