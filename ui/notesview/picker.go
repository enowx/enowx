package notesview

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/iconlib"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The icon picker: SVG icons of the libraries kept (core/iconlib), found
// by name and keyword, the favorites and recent ones first, in the text's
// color or one picked; or the user's own SVG, pasted or from a file,
// cleaned before it is kept.

// iconPicker is the picker's state, one per View: what it shows.
type iconPicker struct {
	custom    bool // the Custom tab
	query     string
	color     string // "#rrggbb" for the icons picked, "" the text's
	shown     int    // icons listed, more on demand
	svg       string // the SVG code pasted
	ownColors bool   // the user's SVG keeps its colors
	err       string
	prefs     proto.NoteIconPrefs
	loaded    bool
	// cache is the last search, by its query and libraries.
	cacheKey string
	cache    []iconlib.Icon
	// customKey is the SVG last cleaned, as customRef or customErr.
	customKey string
	customRef string
	customErr error
}

// pickerStep is how many icons the picker lists at a time.
const pickerStep = 160

var pickerColors = []string{"#e5484d", "#f76b15", "#f5a524", "#30a46c", "#12a594", "#0090ff", "#6e56cf", "#d6409f", "#8b8d98"}

// openPicker readies the picker for a new choice; its preferences come
// from the host.
func (v *View) openPicker(o Options) {
	p := &v.picker
	p.query, p.shown, p.err, p.custom, p.svg = "", pickerStep, "", false, ""
	if o.IconPrefs != nil {
		o.IconPrefs(func(prefs proto.NoteIconPrefs, err error) {
			if err == nil {
				p.prefs, p.loaded = prefs, true
			}
		})
	}
}

// libraries are the libraries the picker shows.
func (p *iconPicker) libraries() []string {
	if len(p.prefs.Libraries) > 0 {
		return p.prefs.Libraries
	}
	var out []string
	for _, l := range iconlib.Libraries {
		out = append(out, l.ID)
	}
	return out
}

// results are the icons the query names in the libraries shown.
func (p *iconPicker) results() []iconlib.Icon {
	libs := p.libraries()
	key := p.query + "\x00" + strings.Join(libs, ",")
	if key != p.cacheKey {
		p.cacheKey, p.cache = key, nil
		if strings.TrimSpace(p.query) == "" {
			for _, l := range libs {
				p.cache = append(p.cache, iconlib.All(l)...) // by name
			}
		} else {
			p.cache = iconlib.Search(p.query, libs, 0)
		}
	}
	return p.cache
}

// iconPopover is the picker in a pop-over below anchor while *open.
func (v *View) iconPopover(c *ui.Context, o Options, anchor ui.Element, open *bool, current string, pick func(ref string)) {
	ui.PopoverBase(c, anchor, open, func(panel ui.Element) {
		widget.Popup(panel).Padding(0)
		v.iconPickerBody(c, o, current, pick)
	})
}

// iconPickerBody is the picker, current the icon now; pick gets the one
// chosen ("" to remove it). Its calls to the host go through o.
func (v *View) iconPickerBody(c *ui.Context, o Options, current string, pick func(ref string)) {
	p := &v.picker
	ui.Column(c).Width(348).MaxWidth(392).Shrink(1).Padding(theme.S3).Gap(theme.S2).Children(func() {
		ui.Row(c).FillWidth().Gap(theme.S2).AlignItems(ui.Center).Children(func() {
			tab := 0
			if p.custom {
				tab = 1
			}
			if i := widget.Segmented(c, []string{"Library", "Custom"}, tab, nil); i >= 0 {
				p.custom, p.err = i == 1, ""
			}
			ui.Spacer(c)
			if current != "" && widget.GhostButton(c, "Remove").Clicked() {
				pick("")
			}
		})
		if p.custom {
			v.customIcon(c, o, pick)
			return
		}
		v.libraryIcons(c, o, current, pick)
	})
}

// libraryIcons is the Library tab.
func (v *View) libraryIcons(c *ui.Context, o Options, current string, pick func(ref string)) {
	p := &v.picker
	before := p.query
	widget.SearchField(c, &p.query, "Search icons: rocket, bug, calendar…", icon.Search)
	if p.query != before {
		p.shown = pickerStep
	}
	ui.Row(c).FillWidth().Gap(theme.S1).Wrap().AlignItems(ui.Center).Children(func() {
		shown := p.libraries()
		for _, l := range iconlib.Libraries {
			on := slices.Contains(shown, l.ID)
			b := ui.ButtonBase(c.Key(l.ID)).Height(24).PaddingX(theme.S2).Radius(theme.RadiusFull).Background(theme.Sunken).Cursor(ui.CursorPointer).
				Label("Library " + l.Name)
			if on {
				b.Background(theme.Selected)
			}
			b.Children(func() {
				ui.Text(c, l.Name).FontSize(theme.TextLabel).TextColor(map[bool]ui.Color{true: theme.TextStrong, false: theme.TextSoft}[on])
			})
			if b.Clicked() {
				v.toggleLibrary(o, l.ID, on)
			}
		}
	})
	ui.Row(c).FillWidth().Gap(theme.S2).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Color").FontSize(theme.TextLabel).TextColor(theme.TextSoft).Width(40).Shrink(0)
		for _, col := range append([]string{""}, pickerColors...) {
			sw := ui.ButtonBase(c.Key("c"+col)).Size(16, 16).Radius(theme.RadiusFull).Cursor(ui.CursorPointer).Label("Icon color " + col)
			if col == "" {
				sw.Border(1, theme.BorderStrong).Background(theme.TextSoft).Tooltip("The text's color, as the theme")
			} else {
				sw.Background(ui.Hex(col))
			}
			if p.color == col {
				sw.Border(2, theme.TextStrong)
			}
			if sw.Clicked() {
				p.color = col
				if r, err := iconlib.Parse(current); err == nil && r.Lib != "" {
					r.Color = col
					pick(r.String())
				}
			}
		}
	})
	if r, err := iconlib.Parse(current); err == nil && current != "" {
		// How large it shows above its page's title.
		ui.Row(c).FillWidth().Gap(theme.S2).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Size").FontSize(theme.TextLabel).TextColor(theme.TextSoft).Width(40).Shrink(0)
			for _, sz := range []struct {
				label string
				pts   int
			}{{"Usual", 0}, {"Small", 24}, {"Large", 56}, {"Huge", 80}} {
				b := widget.Button(c.Key(sz.label), widget.Secondary, nil, sz.label, "").Height(24).PaddingX(theme.S2)
				if r.Size == sz.pts {
					b.Background(theme.Selected)
				}
				if b.Clicked() && r.Size != sz.pts {
					r.Size = sz.pts
					pick(r.String())
				}
			}
		})
	}
	ui.Scroll(c).FillWidth().MaxHeight(300).Children(func() {
		ui.Column(c).FillWidth().Gap(theme.S2).Children(func() {
			if strings.TrimSpace(p.query) == "" {
				v.iconSection(c, "Favorites", p.prefs.Favorites, current, pick)
				v.iconSection(c, "Recent", p.prefs.Recent, current, pick)
			}
			found := p.results()
			var refs []string
			for _, i := range found[:min(len(found), p.shown)] {
				refs = append(refs, i.Ref())
			}
			label := "All"
			if strings.TrimSpace(p.query) != "" {
				label = "Found"
			}
			if len(refs) == 0 {
				ui.Text(c, "No icon for that: try a simpler word.").FontSize(theme.TextUI).TextColor(theme.TextSoft).Padding(theme.S2, 2)
			}
			v.iconSection(c, label, refs, current, pick)
			if len(found) > p.shown && widget.GhostButton(c, "Show more").Clicked() {
				p.shown += pickerStep
			}
		})
	})
	v.favoriteRow(c, o, current)
}

// iconSection is a labeled grid of icons, the current one marked.
func (v *View) iconSection(c *ui.Context, label string, refs []string, current string, pick func(ref string)) {
	if len(refs) == 0 {
		return
	}
	ui.Column(c.Key(label)).FillWidth().Gap(theme.S1).Children(func() {
		ui.Text(c, label).FontSize(theme.TextLabel).FontWeight(500).TextColor(theme.TextSoft)
		ui.Row(c).FillWidth().Gap(2).Wrap().Children(func() {
			for _, ref := range refs {
				b := ui.ButtonBase(c.Key(ref)).Size(36, 32).Radius(theme.RadiusFull).Center().Cursor(ui.CursorPointer).Label("Icon " + ref).Tooltip(ref)
				if sameIcon(ref, current) {
					b.Background(theme.Selected)
				} else if b.Hovered() {
					b.Background(theme.Hover)
				}
				b.Children(func() {
					widget.NoteIcon(c, ref, 18, theme.Text, nil)
				})
				if b.Clicked() {
					v.pickIcon(ref, pick)
				}
			}
		})
	})
}

// pickIcon picks ref in the picker's color, if it has none of its own.
func (v *View) pickIcon(ref string, pick func(ref string)) {
	r, err := iconlib.Parse(ref)
	if err == nil && r.Lib != "" && r.Color == "" && v.picker.color != "" {
		r.Color = v.picker.color
		ref = r.String()
	}
	pick(ref)
}

// sameIcon says a and b are one icon, whatever their colors.
func sameIcon(a, b string) bool {
	ra, err := iconlib.Parse(a)
	rb, err2 := iconlib.Parse(b)
	if err != nil || err2 != nil {
		return a == b
	}
	return ra.Lib == rb.Lib && ra.Name == rb.Name && ra.Custom == rb.Custom
}

// favoriteRow names the current icon and makes it a favorite, or not.
func (v *View) favoriteRow(c *ui.Context, o Options, current string) {
	r, err := iconlib.Parse(current)
	if err != nil || o.SaveIconPrefs == nil {
		return
	}
	base := withoutColor(r)
	p := &v.picker
	fav := slices.Contains(p.prefs.Favorites, base)
	ui.Row(c).FillWidth().Gap(theme.S2).AlignItems(ui.Center).BorderWidth(0.5, 0, 0, 0).BorderColor(theme.Border).Padding(theme.S2, 2, 0, 2).Children(func() {
		widget.NoteIcon(c, current, 16, theme.Text, nil)
		name := r.Lib + ":" + r.Name
		if r.Custom != "" {
			name = "Your own SVG"
		}
		ui.Row(c).Grow(1).MinWidth(0).Children(func() {
			widget.Running(c, name, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
		})
		label := "Add to favorites"
		if fav {
			label = "In favorites"
		}
		star, _, _ := icon.Note("lucide:star")
		b := widget.Button(c, widget.Secondary, star, label, "").Height(24).PaddingX(theme.S2)
		if fav {
			b.TextColor(theme.Warning)
		}
		if b.Clicked() {
			prefs := p.prefs
			if fav {
				prefs.Favorites = slices.DeleteFunc(slices.Clone(prefs.Favorites), func(f string) bool { return f == base })
			} else {
				prefs.Favorites = append(slices.Clone(prefs.Favorites), base)
			}
			v.savePrefs(o, prefs)
		}
	})
}

// withoutColor is r without its color: the icon a favorite keeps.
func withoutColor(r iconlib.Ref) string {
	r.Color = ""
	return r.String()
}

// toggleLibrary shows library id in the picker, or not; one stays.
func (v *View) toggleLibrary(o Options, id string, on bool) {
	libs := slices.Clone(v.picker.libraries())
	if on {
		if len(libs) == 1 {
			return
		}
		libs = slices.DeleteFunc(libs, func(l string) bool { return l == id })
	} else {
		libs = append(libs, id)
	}
	prefs := v.picker.prefs
	prefs.Libraries = libs
	v.savePrefs(o, prefs)
}

// savePrefs keeps prefs on the host, shown at once.
func (v *View) savePrefs(o Options, prefs proto.NoteIconPrefs) {
	p := &v.picker
	p.prefs = prefs
	if o.SaveIconPrefs == nil {
		return
	}
	o.SaveIconPrefs(prefs, func(saved proto.NoteIconPrefs, err error) {
		if err != nil {
			p.err = err.Error()
			return
		}
		p.prefs = saved
	})
}

// customIcon is the Custom tab: the user's SVG, pasted or from a file,
// seen as it will be kept.
func (v *View) customIcon(c *ui.Context, o Options, pick func(ref string)) {
	p := &v.picker
	ui.Text(c, "Paste SVG code, or choose a file. It is cleaned first: scripts, links out and embedded content go.").
		FontSize(theme.TextLabel).TextColor(theme.TextSoft)
	widget.Well(ui.TextAreaBase(c, &p.svg).FillWidth().Lines(4, 8).Font(theme.MonoFont).FontSize(theme.TextLabel).Padding(theme.S2, theme.S3).Radius(theme.RadiusCard), false).Placeholder("<svg viewBox=\"0 0 24 24\">…</svg>").Label("SVG code")
	ui.Row(c).FillWidth().Gap(theme.S2).AlignItems(ui.Center).Children(func() {
		if o.OpenSVG != nil && widget.Button(c, widget.Secondary, icon.File, "Choose a file…", "").Clicked() {
			o.OpenSVG(func(data []byte, err error) {
				if err != nil {
					p.err = err.Error()
					return
				}
				p.svg, p.err = string(data), ""
			})
		}
		widget.Checkbox(c, &p.ownColors, "Keep its colors")
	})
	key := p.svg + map[bool]string{true: "\x00own", false: ""}[p.ownColors]
	if key != p.customKey {
		p.customKey, p.customRef, p.customErr = key, "", nil
		if strings.TrimSpace(p.svg) != "" {
			p.customRef, p.customErr = iconlib.Custom([]byte(p.svg), !p.ownColors)
		}
	}
	ref, err := p.customRef, p.customErr
	ui.Row(c).FillWidth().Gap(theme.S2).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Size(48, 48).Radius(theme.RadiusSmall).Center().Shrink(0).Background(theme.Sunken).Children(func() {
			if ref != "" {
				widget.NoteIcon(c, ref, 32, theme.Text, nil)
			}
		})
		switch {
		case err != nil:
			ui.Text(c, err.Error()).FontSize(theme.TextLabel).TextColor(theme.Error).Grow(1).MinWidth(0)
		case p.err != "":
			ui.Text(c, p.err).FontSize(theme.TextLabel).TextColor(theme.Error).Grow(1).MinWidth(0)
		default:
			ui.Spacer(c)
		}
		if widget.PrimaryButton(c, nil, "Use this icon", "").Clicked() && ref != "" {
			pick(ref)
		}
	})
}
