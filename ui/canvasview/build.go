package canvasview

import (
	"os"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The Build dialog: what to build and from what (a new design, a change to
// the one shown, a project's pages, a screenshot), for which devices, one
// direction or several to compare. Its agent takes it from there
// (proto.CanvasBuildParams).

// buildForm is the Build dialog's state.
type buildForm struct {
	open     bool
	what     string
	from     int // buildFroms
	project  int
	work     int // buildWorks
	desktop  bool
	mobile   bool
	count    int // directions: 0 one, 1 two, 2 three, 3 four
	confirm  bool
	image    []byte
	imageErr string
	preview  *ui.Bitmap
}

var buildFroms = []struct{ id, label string }{
	{proto.BuildNew, "New"}, {proto.BuildChange, "Change this design"}, {proto.BuildProject, "A project"}, {proto.BuildScreenshot, "A screenshot"},
}

var buildWorks = []struct{ id, label, about string }{
	{proto.WorkRefresh, "UI refresh", "Tidy the noise, then upgrade the layout; brand kept"},
	{proto.WorkFlow, "Flow fix", "Count the steps and clicks, then cut them"},
	{proto.WorkFeature, "New feature", "Fit what is there"},
	{proto.WorkReview, "Review only", "What to change and why, on the screens; nothing changed"},
}

// maxBuildImage is the largest screenshot a build takes.
const maxBuildImage = 15 << 20

// buildButton opens the Build dialog.
func (v *View) buildButton(c *ui.Context, o Options) {
	tip := "Build a UI: from a brief to a finished design, one direction or several to compare"
	var b ui.Element
	if v.compact() {
		b = widget.IconButton(c.Key("build"), icon.Palette, tip, false).Label("Build a UI")
	} else {
		b = widget.GhostButton(c.Key("build"), "Build").Tooltip(tip).Label("Build a UI")
	}
	if o.Building {
		b.Disabled(true)
	} else if b.Clicked() {
		v.building = &buildForm{open: true, desktop: true, mobile: true, confirm: true}
	}
}

func (v *View) buildDialog(c *ui.Context, o Options) {
	f := v.building
	if f == nil {
		return
	}
	if !f.open {
		v.building = nil
		return
	}
	widget.FormSheet(c, &f.open, 560, "Build a UI", "The canvas's agent writes the brief, then designs; you follow it here and pick.", func() {
		col := ui.Column(c.Key("build-form")).FillWidth().Gap(theme.S3)
		if files := col.DroppedFiles(); len(files) > 0 {
			f.setImage(readImage(files[0]))
			f.from = 3
		}
		col.Children(func() {
			widget.Field(c, "What to build", func() {
				widget.Well(ui.TextArea(c, &f.what).FillWidth().TextColor(theme.Text).Height(80).Padding(theme.S2, theme.S3).Radius(theme.RadiusCard).FontSize(theme.TextUI).
					Label("What to build").Placeholder("A pricing page for a ledger app small shops use"), false)
			})
			widget.Field(c, "From", func() {
				f.from = chosen(widget.Segmented(c.Key("from"), labels(buildFroms), f.from, func(i int) bool {
					return i == 2 && len(o.Projects) == 0
				}), f.from)
			})
			switch buildFroms[f.from].id {
			case proto.BuildProject:
				v.buildProjectFields(c, o, f)
			case proto.BuildScreenshot:
				f.screenshotField(c)
			}
			widget.Field(c, "Devices", func() {
				ui.Row(c.Key("devices")).Gap(theme.S4).Children(func() {
					widget.Checkbox(c.Key("desktop"), &f.desktop, "Desktop")
					widget.Checkbox(c.Key("mobile"), &f.mobile, "Phone")
				})
			})
			if f.lookIsOpen() {
				widget.Field(c, "Directions", func() {
					v.countSelect(c, f)
				})
				if f.count == 0 {
					widget.Checkbox(c.Key("confirm"), &f.confirm, "Show me the direction before building it")
				}
			}
			if f.imageErr != "" {
				ui.Text(c.Key("image-err"), f.imageErr).FontSize(theme.TextUI).TextColor(theme.Error)
			}
		})
	}, func() {
		b := widget.PrimaryButton(c, nil, "Start", "").Label("Start the build")
		if !f.ready(o) {
			b.Disabled(true)
		} else if b.Clicked() {
			o.Build(f.params(o, v.frameOf().ID), f.image)
			f.open = false
		}
	})
}

// countSelect picks how many directions to design.
func (v *View) countSelect(c *ui.Context, f *buildForm) {
	names := []string{"One, recommended", "2 to compare", "3", "4"}
	t := widget.Trigger(ui.ButtonBase(c.Key("count")).Label("Directions"), false)
	t.Children(func() {
		ui.Text(c, names[f.count]).FontSize(theme.TextUI).TextColor(theme.Text).Grow(1).Shrink(1).MinWidth(0).SingleLine()
		ui.Icon(c, icon.ChevronDown).FontSize(12).TextColor(theme.TextSoft)
	})
	v.menu.OpenBelow(t, "build-count", theme.ControlHeight)
	v.menu.Show(c, t, "build-count", func(done func()) {
		for i, name := range names {
			if widget.MenuItem(c.Key(name), nil, name, false) {
				f.count = i
				done()
			}
		}
	})
}

func (v *View) buildProjectFields(c *ui.Context, o Options, f *buildForm) {
	widget.Field(c, "Project", func() {
		if len(o.Projects) == 0 {
			return
		}
		cur := min(f.project, len(o.Projects)-1)
		t := widget.Trigger(ui.ButtonBase(c.Key("project")).Label("Project"), false)
		t.Children(func() {
			ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, o.Projects[cur].Name, widget.RunStyle{}) })
			ui.Icon(c, icon.ChevronDown).FontSize(12).TextColor(theme.TextSoft)
		})
		v.menu.OpenBelow(t, "build-project", theme.ControlHeight)
		v.menu.Show(c, t, "build-project", func(done func()) {
			for i, p := range o.Projects {
				if widget.MenuItem(c.Key(p.Path), icon.Folder, p.Name, false) {
					f.project = i
					done()
				}
			}
		})
	})
	widget.Field(c, "The work", func() {
		ui.Column(c).FillWidth().Gap(theme.S1).Children(func() {
			f.work = chosen(widget.Segmented(c.Key("work"), labels3(buildWorks), f.work, nil), f.work)
			ui.Text(c.Key("about"), buildWorks[f.work].about).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		})
	})
}

func (f *buildForm) screenshotField(c *ui.Context) {
	widget.Field(c, "Screenshot", func() {
		box := widget.Well(ui.Row(c.Key("shot")).FillWidth().Height(96).Shrink(0).Gap(theme.S3).Padding(theme.S2, theme.S3).Radius(theme.RadiusCard).Clip().AlignItems(ui.Center), false)
		box.Children(func() {
			if f.preview != nil {
				w, h := f.preview.Size()
				ui.Image(c.Key("pv"), f.preview).Size(min(float32(76*w/max(h, 1)), 200), 76).Shrink(0).Fit(ui.Contain).Radius(theme.RadiusSmall)
			}
			ui.Column(c.Key("how")).Grow(1).Shrink(1).MinWidth(0).Gap(theme.S1).AlignItems(ui.Start).Children(func() {
				ui.Row(c.Key("say")).FillWidth().MinWidth(0).Children(func() {
					widget.Running(c, "Drop a picture here, or paste one", widget.RunStyle{Color: theme.TextSoft})
				})
				if widget.GhostButton(c.Key("paste"), "Paste from the clipboard").Clicked() {
					f.setImage(mygo.Clipboard.ReadImage(), nil)
				}
			})
		})
	})
}

func (f *buildForm) setImage(data []byte, err error) {
	f.imageErr = ""
	switch {
	case err != nil:
		f.imageErr = err.Error()
	case len(data) == 0:
		f.imageErr = "The clipboard holds no picture"
	case len(data) > maxBuildImage:
		f.imageErr = "The picture is over 15 MB"
	default:
		bmp, derr := ui.DecodeBitmap(data)
		if derr != nil {
			f.imageErr = "That isn't a picture this can read"
			return
		}
		f.image, f.preview = data, bmp
	}
}

func readImage(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > maxBuildImage {
		return nil, os.ErrInvalid
	}
	return os.ReadFile(path)
}

// lookIsOpen says the build may choose its look: not a recreation, nor
// work that keeps a project's look.
func (f *buildForm) lookIsOpen() bool {
	switch buildFroms[f.from].id {
	case proto.BuildScreenshot:
		return false
	case proto.BuildProject:
		return buildWorks[f.work].id == proto.WorkRefresh
	}
	return true
}

func (f *buildForm) ready(o Options) bool {
	if o.Build == nil || !f.desktop && !f.mobile {
		return false
	}
	switch buildFroms[f.from].id {
	case proto.BuildScreenshot:
		return len(f.image) > 0
	case proto.BuildProject:
		return len(o.Projects) > 0 && (strings.TrimSpace(f.what) != "" || buildWorks[f.work].id == proto.WorkReview)
	}
	return strings.TrimSpace(f.what) != ""
}

// params are the form as a build to start; frame the design shown.
func (f *buildForm) params(o Options, frame string) proto.CanvasBuildParams {
	p := proto.CanvasBuildParams{Start: true, What: strings.TrimSpace(f.what), From: buildFroms[f.from].id, Directions: 1, Confirm: f.confirm}
	if f.desktop {
		p.Devices = append(p.Devices, "desktop")
	}
	if f.mobile {
		p.Devices = append(p.Devices, "mobile")
	}
	if f.lookIsOpen() {
		p.Directions = f.count + 1
	}
	switch p.From {
	case proto.BuildChange:
		p.Frame = frame
	case proto.BuildProject:
		p.Project, p.Work = o.Projects[min(f.project, len(o.Projects)-1)].Path, buildWorks[f.work].id
		if p.What == "" {
			p.What = "Review the project's UI"
		}
	}
	return p
}

// chosen is the option a Segmented returned, else the one it had.
func chosen(clicked, current int) int {
	if clicked >= 0 {
		return clicked
	}
	return current
}

func labels(xs []struct{ id, label string }) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = x.label
	}
	return out
}

func labels3(xs []struct{ id, label, about string }) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = x.label
	}
	return out
}
