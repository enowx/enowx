// Package localpicker is the dialog that picks files and folders of this
// device, to copy somewhere (a session's folder, wherever it runs). It
// reads this device's disk itself: what it picks is here by nature.
package localpicker

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Picker is the dialog. It belongs to the main thread.
type Picker struct {
	open     bool
	title    string
	about    string
	dir      string
	path     string // the path bar, as typed
	entries  []entry
	selected map[string]bool // absolute paths picked, across folders
	hidden   bool
	err      string
	pick     func(paths []string)
	last     string // the folder last shown, where the next opening starts
}

type entry struct {
	name   string
	dir    bool
	size   int64
	hidden bool
}

// Open shows the dialog, titled title and about, where it was left (else
// at the home folder); pick gets the paths picked.
func (p *Picker) Open(title, about string, pick func(paths []string)) {
	start := p.last
	if start == "" {
		start, _ = os.UserHomeDir()
	}
	*p = Picker{open: true, title: title, about: about, pick: pick, last: p.last, selected: map[string]bool{}}
	p.browse(start)
}

// IsOpen reports whether the dialog shows.
func (p *Picker) IsOpen() bool { return p.open }

func (p *Picker) browse(dir string) {
	if strings.HasPrefix(dir, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, strings.TrimPrefix(dir, "~"))
		}
	}
	list, err := os.ReadDir(dir)
	if err != nil {
		p.err = err.Error()
		return
	}
	p.dir, p.last, p.err = dir, dir, ""
	p.path = textfmt.ShortHome(dir)
	p.entries = p.entries[:0]
	for _, d := range list {
		e := entry{name: d.Name(), hidden: strings.HasPrefix(d.Name(), ".")}
		info, err := d.Info()
		if err != nil {
			continue
		}
		e.dir = info.IsDir()
		if info.Mode()&os.ModeSymlink != 0 {
			if t, err := os.Stat(filepath.Join(dir, d.Name())); err == nil {
				e.dir = t.IsDir()
			}
		}
		e.size = info.Size()
		p.entries = append(p.entries, e)
	}
	slices.SortFunc(p.entries, func(a, b entry) int {
		if a.dir != b.dir {
			if a.dir {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name))
	})
}

// View draws the dialog while it is open.
func (p *Picker) View(c *ui.Context) {
	if !p.open {
		return
	}
	widget.Dialog(c, &p.open, 640, 12, func() {
		if widget.DialogHeader(c, p.title, p.about, true) {
			p.open = false
		}
		p.toolbar(c)
		p.places(c)
		p.list(c)
		if p.err != "" {
			ui.Text(c, p.err).FontSize(theme.TextLabel).TextColor(theme.Error)
		}
		p.footer(c)
	})
}

func (p *Picker) toolbar(c *ui.Context) {
	ui.Row(c).FillWidth().Gap(6).Children(func() {
		if widget.IconButton(c, icon.ArrowUp, "Up one folder", true).Clicked() {
			if parent := filepath.Dir(p.dir); parent != p.dir {
				p.browse(parent)
			}
		}
		if widget.IconButton(c, icon.Home, "Home folder", true).Clicked() {
			p.browse("~")
		}
		if widget.Input(c, &p.path, "Folder path").Font(theme.MonoFont).Grow(1).Submitted() {
			p.browse(p.path)
		}
	})
}

// places are the folders people pick from most.
func (p *Picker) places(c *ui.Context) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	ui.Row(c).FillWidth().Gap(6).Wrap().Children(func() {
		for _, name := range []string{"Desktop", "Downloads", "Documents", "Pictures"} {
			dir := filepath.Join(home, name)
			if _, err := os.Stat(dir); err != nil {
				continue
			}
			b := ui.ButtonBase(c.Key(name)).Gap(6).Padding(4, 9).Radius(theme.RadiusControl).Cursor(ui.CursorPointer).
				Border(1, theme.Border).Label(name)
			if p.dir == dir {
				b.Border(1, theme.Accent).Background(theme.AccentSoft)
			} else if b.Hovered() {
				b.Background(theme.Hover)
			}
			b.Children(func() {
				ui.Icon(c, icon.Folder).FontSize(12).TextColor(theme.Muted)
				ui.Text(c, name).FontSize(theme.TextLabel)
			})
			if b.Clicked() {
				p.browse(dir)
			}
		}
	})
}

func (p *Picker) list(c *ui.Context) {
	ui.Scroll(c).FillWidth().Height(340).Radius(theme.RadiusCard).Background(theme.Sunken).Border(1, theme.BorderStrong).Children(func() {
		ui.Column(c).FillWidth().Padding(4).Gap(1).Children(func() {
			shown := 0
			for _, e := range p.entries {
				if e.hidden && !p.hidden {
					continue
				}
				shown++
				p.row(c.Key(e.name), e)
			}
			if shown == 0 {
				ui.Text(c, "Nothing here.").TextColor(theme.Muted).Padding(12)
			}
		})
	})
}

// row is an entry: its box picks it; a folder's name opens it.
func (p *Picker) row(c *ui.Context, e entry) {
	full := filepath.Join(p.dir, e.name)
	on := p.selected[full]
	ui.Row(c).FillWidth().Gap(8).Padding(2, 6).Radius(theme.RadiusControl).Children(func() {
		if widget.Checkbox(c.Key("pick"), &on, "") {
			if on {
				p.selected[full] = true
			} else {
				delete(p.selected, full)
			}
		}
		b := ui.ButtonBase(c.Key("name")).Grow(1).MinWidth(0).Height(28).PaddingX(6).Gap(8).Radius(theme.RadiusControl).
			Justify(ui.Start).Cursor(ui.CursorPointer).Label(e.name)
		if b.Hovered() {
			b.Background(theme.Hover)
		}
		b.Children(func() {
			svg, col := icon.File, theme.Muted
			if e.dir {
				svg, col = icon.Folder, theme.TextSoft
			}
			ui.Icon(c, svg).FontSize(13).TextColor(col)
			name := ui.Text(c, e.name).FontSize(theme.TextUI).SingleLine().Shrink(1)
			if e.hidden {
				name.TextColor(theme.Muted)
			}
			ui.Spacer(c)
			if e.dir {
				ui.Icon(c, icon.ChevronRight).FontSize(11).TextColor(theme.Muted)
			} else {
				ui.Text(c, textfmt.Bytes(uint64(max(e.size, 0)))).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
			}
		})
		if b.Clicked() {
			if e.dir {
				p.browse(full)
			} else if p.selected[full] {
				delete(p.selected, full)
			} else {
				p.selected[full] = true
			}
		}
	})
}

func (p *Picker) footer(c *ui.Context) {
	ui.Row(c).FillWidth().Gap(8).Children(func() {
		widget.Switch(c, &p.hidden, "Show hidden files")
		ui.Text(c, "Hidden files").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		ui.Box(c).Grow(1)
		n := len(p.selected)
		if n > 0 {
			ui.Text(c, fmt.Sprintf("%d picked", n)).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		}
		if widget.GhostButton(c, "Cancel").Clicked() {
			p.open = false
		}
		label := "Add"
		if n > 0 {
			label = fmt.Sprintf("Add %d", n)
		}
		b := widget.PrimaryButton(c, icon.Plus, label, "")
		if n == 0 {
			b.Disabled(true).Opacity(0.5)
		} else if b.Clicked() {
			paths := make([]string, 0, n)
			for path := range p.selected {
				paths = append(paths, path)
			}
			slices.Sort(paths)
			p.open = false
			p.pick(paths)
		}
	})
}
