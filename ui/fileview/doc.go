package fileview

import (
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Doc is a file of a session's folder, shown in a tab of its own: its
// lines, numbered. It belongs to the main thread; its poller reports
// through Options.Update.
type Doc struct {
	path     string          // in the session's folder
	spans    [][]widget.Span // its lines colored, nil for plain text
	content  *proto.FileContent
	lines    []string
	image    ui.ImageSource // an image file's picture, decoded
	imgW     int            // its size in pixels, 0 for an SVG
	imgH     int
	err      string
	polling  bool
	list     ui.ListState
	copyText widget.CopyFeedback
	copyPath widget.CopyFeedback
	// markdown: the file is Markdown, which shows rendered unless raw.
	markdown bool
	raw      bool
	preview  ui.ListState
	// html: the file is a web page, which shows as one when page is set.
	html bool
	page bool
	// pdf: the file is a PDF, which shows in a web view where there is one.
	pdf bool
	// from and to are lines to show and mark (1-based), as a search found
	// them; scrolled to once the file is read.
	from, to int
	goTo     bool
}

// GoTo shows lines from to to (1-based) of the file, marked: its text
// rather than a Markdown file rendered.
func (d *Doc) GoTo(from, to int) {
	d.from, d.to, d.goTo, d.raw = from, max(to, from), true, true
}

// NewDoc returns the doc of the file at p, not read yet.
func NewDoc(p string) *Doc {
	return &Doc{path: p, markdown: isMarkdown(p), html: isHTML(p),
		pdf: strings.EqualFold(path.Ext(p), ".pdf")}
}

// isHTML reports whether the file at p is a web page, by its name.
func isHTML(p string) bool {
	switch strings.ToLower(path.Ext(p)) {
	case ".html", ".htm", ".xhtml":
		return true
	}
	return false
}

// isMarkdown reports whether the file at p is Markdown, by its name.
func isMarkdown(p string) bool {
	switch strings.ToLower(path.Ext(p)) {
	case ".md", ".markdown", ".mdx":
		return true
	}
	return false
}

// Path is the file's path in the session's folder.
func (d *Doc) Path() string { return d.path }

// follow reads the file while the doc shows, unless it already does.
func (d *Doc) follow(o Options) {
	if d.polling || o.Client == nil {
		return
	}
	d.polling = true
	cl, id, p := o.Client, o.Session.ID, d.path
	var seen time.Time // when the file last changed, as last read
	go func() {
		for {
			f, err := cl.ReadFile(id, p, seen)
			// Decoding a picture takes a while: not on the main thread.
			var pic ui.ImageSource
			var w, h int
			var picErr error
			if err == nil && !f.Unchanged && f.Image != "" {
				pic, w, h, picErr = decodeImage(f.Image, f.Data)
			}
			// Nor does lexing a whole file.
			var spans [][]widget.Span
			if err == nil && !f.Unchanged && f.Image == "" {
				spans = widget.HighlightLines(path.Base(p), "", f.Text)
			}
			keep := make(chan bool, 1)
			o.Update(func() {
				if !o.Wanted() {
					d.polling = false
					keep <- false
					return
				}
				keep <- true
				switch {
				case err != nil:
					d.err = client.Explain(err)
					return
				case f.Unchanged:
				case picErr != nil:
					d.content, d.image, d.err = &f, nil, "Can't show this image: "+picErr.Error()
					return
				default:
					d.content, d.lines, d.spans, d.image, d.imgW, d.imgH = &f, splitLines(f.Text), spans, pic, w, h
				}
				d.err = ""
			})
			if !<-keep {
				return
			}
			if err == nil {
				seen = f.Modified
			}
			time.Sleep(refreshEvery)
		}
	}()
}

// decodeImage makes a picture of an image file's bytes.
func decodeImage(format string, data []byte) (ui.ImageSource, int, int, error) {
	if format == "svg" {
		svg, err := ui.ParseSVG(data)
		return svg, 0, 0, err
	}
	bmp, err := ui.DecodeBitmap(data)
	if err != nil {
		return nil, 0, 0, err
	}
	w, h := bmp.Size()
	return bmp, w, h, nil
}

// Draw shows the file: a bar with its path and what to do with it, then
// its lines.
func (d *Doc) Draw(c *ui.Context, o Options) {
	d.follow(o)
	ui.Column(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
		d.bar(c, o)
		if d.err != "" {
			ui.Row(c).FillWidth().Shrink(0).Padding(theme.S3, theme.S4, 0, theme.S4).Children(func() {
				widget.Notice(c, nil, theme.Del, d.err, "", nil)
			})
		}
		if (d.page || d.pdf) && o.Page != nil && d.content != nil {
			o.Page(c, d.path, d.content.Modified)
			return
		}
		ui.Column(c).Grow(1).FillWidth().MinHeight(0).Background(codeGround(d)).Children(func() { d.text(c) })
	})
}

// codeGround is what a file shows on: code on its dark ground in every
// palette, a rendered page on the frame itself.
func codeGround(d *Doc) ui.Color {
	if d.markdown && !d.raw {
		return ui.Transparent
	}
	return theme.CodeGround
}

// bar is the file's path, its size, and copying it or opening it in the
// editor.
func (d *Doc) bar(c *ui.Context, o Options) {
	ui.Row(c).Height(44).Shrink(0).FillWidth().Padding(0, theme.S2, 0, theme.S4).Gap(theme.S2).Clip().
		BorderWidth(0, 0, theme.Rule, 0).BorderColor(theme.Border).Children(func() {
		ui.Icon(c, icon.File).FontSize(14).TextColor(theme.TextSoft)
		ui.Row(c).MinWidth(64).Shrink(1).Children(func() {
			widget.Running(c, d.path, widget.RunStyle{Size: theme.TextLabel, Mono: true, Always: true})
		})
		changed := false
		if o.Status != nil {
			if letter, col := statusLetter(o.Status(d.path)); letter != "" {
				changed = true
				ui.Text(c, letter).FontSize(theme.TextLabel).FontWeight(500).TextColor(col).SingleLine().Shrink(0)
			}
		}
		ui.Spacer(c)
		if changed && o.ShowChanges != nil && widget.Button(c.Key("show-changes"), widget.Secondary, icon.Git, "Show changes", "").Height(28).Clicked() {
			o.ShowChanges(d.path)
		}
		if d.markdown && d.content != nil && !d.content.Binary {
			cur := 0
			if d.raw {
				cur = 1
			}
			if i := widget.Segmented(c.Key("view"), []string{"Preview", "Raw"}, cur, nil); i >= 0 {
				d.raw = i == 1
			}
		}
		if d.html && o.Page != nil && d.content != nil && !d.content.Binary {
			cur := 0
			if d.page {
				cur = 1
			}
			if i := widget.Segmented(c.Key("page"), []string{"Code", "Page"}, cur, nil); i >= 0 {
				d.page = i == 1
			}
		}
		if d.content != nil {
			size := textfmt.Bytes(uint64(d.content.Size))
			if d.imgW > 0 {
				size = strconv.Itoa(d.imgW) + "×" + strconv.Itoa(d.imgH) + " · " + size
			}
			ui.Text(c, size).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(0)
		}
		abs := ""
		if o.Session.Dir != "" {
			abs = path.Join(o.Session.Dir, d.path) // the host's path, slashes and all
		}
		if f := d.content; f != nil && d.err == "" && !f.Binary && f.Image == "" && !d.pdf {
			label, tip := "Copy text", "Copy the file's source text"
			if f.Truncated {
				label, tip = "Copy displayed text", "Only the displayed part of this file will be copied"
			}
			widget.CopyIcon(c.Key("copy-text"), &d.copyText, icon.Copy, label, tip, f.Text)
		}
		if abs != "" {
			widget.CopyIcon(c.Key("copy-path"), &d.copyPath, icon.Link, "Copy the path", "Copy the path", abs)
		}
		if abs != "" && o.OpenInEditor != nil && widget.IconButton(c, icon.External, "Open in "+o.EditorName, false).Clicked() {
			o.OpenInEditor(abs, 0)
		}
	})
}

// text is the file's lines, numbered, or why it shows none.
func (d *Doc) text(c *ui.Context) {
	f := d.content
	switch {
	case f == nil && d.err == "":
		centered(c, "Reading…")
		return
	case f == nil:
		return
	case d.image != nil:
		d.picture(c)
		return
	case d.markdown && !d.raw && !f.Binary && f.Text != "":
		widget.MarkdownList(c.Key("preview"), &d.preview, f.Text, 720).Grow(1).FillWidth().MinHeight(0).Padding(theme.S4, theme.S5)
		return
	case f.Binary:
		centered(c, "A binary file: nothing to show as text.")
		return
	case len(d.lines) == 0:
		centered(c, "This file is empty.")
		return
	}
	lines := d.lines
	n := len(lines)
	if f.Truncated {
		n++
	}
	width := max(float32(len(strconv.Itoa(len(lines))))*8+theme.S4, 44)
	if d.goTo {
		d.goTo = false
		d.list.ScrollTo(min(d.from, len(lines))-1, ui.Center)
	}
	ui.List(c.Key("doc"), &d.list, n, func(i int) {
		if i == len(lines) {
			ui.Row(c).FillWidth().Height(lineHeight + 6).PaddingX(theme.S4).Children(func() {
				ui.Text(c, "…the rest of this file is too long to show here").FontSize(theme.TextLabel).TextColor(ui.Hex("#8A8FA3")).SingleLine()
			})
			return
		}
		row := ui.Row(c).FillWidth().Height(lineHeight).Clip()
		if i+1 >= d.from && i+1 <= d.to {
			row.Background(theme.Selected)
		}
		row.Children(func() {
			ui.Text(c, strconv.Itoa(i+1)).Font(theme.MonoFont).FontSize(theme.TextUI).TextColor(ui.Hex("#6E7386")).Width(width).PaddingX(theme.S2).
				TextAlign(ui.End).SingleLine().Shrink(0)
			var spans []widget.Span
			if i < len(d.spans) {
				spans = d.spans[i]
				if spans == nil {
					spans = []widget.Span{} // an empty line
				}
			}
			widget.CodeSpans(c, spans, lines[i]).Font(theme.MonoFont).FontSize(theme.TextUI).TextColor(ui.Hex("#E4E6EE")).SingleLine().Shrink(1)
		})
	}).Grow(1).FillWidth().MinHeight(0).Padding(theme.S2, widget.ScrollGutter, theme.S2, 0)
}

// picture shows an image: at its size when it fits, else scaled down to
// the panel's width, on a ground that shows its transparent parts.
func (d *Doc) picture(c *ui.Context) {
	ui.Scroll(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
		ui.Column(c).FillWidth().Padding(theme.S4).AlignItems(ui.Center).Children(func() {
			img := ui.Image(c, d.image).MaxWidth(float32(max(d.imgW, 1))).FillWidth().Background(theme.Sunken).Radius(theme.RadiusSmall)
			if d.imgW == 0 { // an SVG: as wide as the panel
				img.MaxWidth(10000)
			}
		})
	})
}
