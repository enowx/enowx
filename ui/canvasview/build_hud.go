package canvasview

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/designcompare"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/unslopview"
	"github.com/enowx/enowx/ui/widget"
)

// A build under way (Options.BuildRun), over the canvas whatever its view:
// its stages, the directions' designs and agents, how distinct each pair
// is, and the independent review.

var buildStages = []struct{ id, label string }{
	{proto.BuildBrief, "Brief"}, {proto.BuildDirections, "Direction"}, {proto.BuildBuilding, "Building"},
	{proto.BuildCompare, "Compare"}, {proto.BuildRefining, "Refining"}, {proto.BuildReviewing, "Review"}, {proto.BuildDone, "Done"},
}

// buildHUD draws the build's panel at the bottom left of the canvas's
// views.
func (v *View) buildHUD(c *ui.Context, o Options, cv proto.Canvas) {
	switch {
	case o.Building:
		ui.Row(c.Key("build-hud")).Absolute().Left(12).Bottom(12).Padding(10, 12).Gap(8).Radius(theme.RadiusSmall).AlignItems(ui.Center).
			Background(theme.Dialog).Border(1, theme.BorderStrong).Children(func() {
			widget.Spinner(c, 12)
			ui.Text(c, "Starting the build…").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		})
		return
	case o.BuildErr != "":
		ui.Row(c.Key("build-hud")).Absolute().Left(12).Bottom(12).Padding(10, 12).Gap(8).Radius(theme.RadiusSmall).AlignItems(ui.Center).
			Background(theme.Dialog).Border(1, theme.Fade(theme.Error, 0.5)).Children(func() {
			ui.Icon(c, icon.Alert).FontSize(12).TextColor(theme.Error)
			ui.Text(c, "The build didn't start: "+o.BuildErr).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
			if o.BuildDismiss != nil && widget.IconButton(c, icon.Close, "Dismiss", false).Clicked() {
				o.BuildDismiss()
			}
		})
		return
	}
	run := o.BuildRun
	if run == nil || run.ID != cv.ID || v.buildClosed == run.What+run.Stage {
		return
	}
	ui.Column(c.Key("build-hud")).Absolute().Left(12).Bottom(12).Width(280).Padding(12).Gap(10).Radius(theme.RadiusSmall).
		Background(theme.Dialog).Border(1, theme.BorderStrong).Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon.Palette).FontSize(14).TextColor(theme.Accent)
			ui.Text(c, "Build").FontSize(theme.TextUI).FontWeight(600)
			ui.Text(c, run.What).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Grow(1).Shrink(1).MinWidth(0)
			if run.Done && widget.IconButton(c, icon.Close, "Close", false).Clicked() {
				v.buildClosed = run.What + run.Stage
			}
		})
		buildStepper(c, run.Stage)
		if len(run.Directions) > 0 {
			v.buildDirections(c, o, cv, *run)
		}
		if run.Review != nil {
			unslopview.DrawReview(c, *run.Review, o.OpenReviewer)
		}
	})
}

// buildStepper is the stages, those past checked, the current lit.
func buildStepper(c *ui.Context, stage string) {
	at := slices.IndexFunc(buildStages, func(s struct{ id, label string }) bool { return s.id == stage })
	ui.Row(c).FillWidth().Gap(4).Wrap().Children(func() {
		for i, s := range buildStages {
			col, bg := theme.Muted, theme.Sunken
			switch {
			case i < at || stage == proto.BuildDone:
				col = theme.Done
			case i == at:
				col, bg = theme.Accent, theme.Fade(theme.Accent, 0.14)
			}
			ui.Row(c.Key(s.id)).Height(20).Padding(0, 7).Gap(4).Radius(theme.RadiusSmall).AlignItems(ui.Center).Background(bg).Children(func() {
				if i < at || stage == proto.BuildDone {
					ui.Icon(c, icon.Check).FontSize(9).TextColor(col)
				}
				ui.Text(c, s.label).FontSize(theme.TextLabel).TextColor(col)
			})
		}
	})
}

// buildDirections are the directions: each design, its source, its agent;
// then how distinct each pair is.
func (v *View) buildDirections(c *ui.Context, o Options, cv proto.Canvas, run proto.CanvasBuildRun) {
	name := func(id string) string {
		if i := slices.IndexFunc(cv.Frames, func(f proto.CanvasFrame) bool { return f.ID == id }); i >= 0 {
			return cv.Frames[i].Name
		}
		return "a design"
	}
	ui.Column(c).FillWidth().Gap(2).Padding(8, 0, 0, 0).BorderWidth(1, 0, 0, 0).BorderColor(theme.Border).Children(func() {
		for i, d := range run.Directions {
			f := cv.Frames[max(0, slices.IndexFunc(cv.Frames, func(f proto.CanvasFrame) bool { return f.ID == d.Frame }))]
			row := ui.ButtonBase(c.Key(d.Frame)).FillWidth().Gap(8).Padding(4, 6).Radius(theme.RadiusSmall).AlignItems(ui.Center).
				Label("Show " + f.Name).Cursor(ui.CursorPointer)
			if row.Hovered() {
				row.Background(theme.Hover)
			}
			row.Children(func() {
				ui.Box(c).Size(7, 7).Radius(theme.RadiusTail).Background(ui.Hex(unslopview.RegionColors[i%len(unslopview.RegionColors)]))
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).Children(func() {
					ui.Text(c, f.Name).FontSize(theme.TextLabel).FontWeight(600).SingleLine()
					if f.Direction != nil {
						ui.Text(c, f.Direction.Source).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
					}
				})
				switch {
				case d.Done:
					ui.Icon(c, icon.Check).FontSize(11).TextColor(theme.Done)
				case d.Session != "" && o.AgentState != nil && o.AgentState(d.Session) == unslopview.AgentWaiting:
					widget.Tag(c, "needs you", theme.Warning)
				default:
					widget.Spinner(c, 10)
				}
			})
			if row.Clicked() {
				v.frame, v.view = d.Frame, viewDesign
				if d.Session != "" && o.OpenSession != nil {
					o.OpenSession(d.Session)
				}
			}
		}
		for _, p := range run.Pairs {
			col, text := theme.TextSoft, fmt.Sprintf("%s ↔ %s · %d distinct", short(name(p.A)), short(name(p.B)), p.Distinct)
			if designcompare.TooClose(p) {
				col = theme.Warning
				if len(p.Shared) > 0 {
					text += " · shares " + strings.Join(p.Shared, ", ")
				} else {
					text += " · first screens alike"
				}
			}
			ui.Text(c.Key(p.A+p.B), text).FontSize(theme.TextLabel).TextColor(col).Padding(0, 6)
		}
	})
	c.After(time.Second) // agents' states change outside the run
}

// short is a direction's name without its "Direction " prefix.
func short(name string) string { return strings.TrimPrefix(name, "Direction ") }
