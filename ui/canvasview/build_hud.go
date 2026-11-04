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

// hudBottom is how far over the views' foot the build's panel floats: above
// Compare's pick buttons and the designs' line and strip, which it would cover.
func (v *View) hudBottom(cv proto.Canvas) float32 {
	switch v.view {
	case viewCompare:
		return theme.ControlHeight + theme.S2 + theme.S3
	case viewDesign:
		bottom := float32(theme.S3)
		if len(cv.Frames) > 1 || v.frameOf().Description != "" {
			bottom += theme.ControlHeight + theme.S1 // the line
		}
		if len(cv.Frames) > 1 && v.stageRoom() >= stripRoom {
			bottom += thumbH + theme.S6 // the strip
		}
		return bottom
	}
	return theme.S3
}

// buildHUD draws the build's panel at the bottom left of the canvas's
// views.
func (v *View) buildHUD(c *ui.Context, o Options, cv proto.Canvas) {
	switch {
	case o.Building:
		raised(ui.Row(c.Key("build-hud")).Absolute().Left(theme.S3).Bottom(v.hudBottom(cv)).Width(hudWidth).MinHeight(40).Padding(theme.S2, theme.S4).Gap(theme.S2).Radius(theme.RadiusCard).AlignItems(ui.Center)).Children(func() {
			widget.Spinner(c, 14)
			ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, "Starting the build…", widget.RunStyle{Color: theme.TextSoft}) })
		})
		return
	case o.BuildErr != "":
		raised(ui.Row(c.Key("build-hud")).Absolute().Left(theme.S3).Bottom(v.hudBottom(cv)).Width(hudWidth).Padding(theme.S2, theme.S2, theme.S2, theme.S4).Gap(theme.S2).Radius(theme.RadiusCard).AlignItems(ui.Center)).Children(func() {
			ui.Icon(c, icon.Alert).FontSize(14).TextColor(theme.Error)
			ui.Text(c, "The build didn't start: "+o.BuildErr).FontSize(theme.TextUI).TextColor(theme.TextSoft).Grow(1).Basis(0).MinWidth(0)
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
	// However long what it says: it fits above where it floats, its body scrolling.
	bottom := v.hudBottom(cv)
	room := ui.Box(c.Key("build-hud:room")).Absolute().Top(0).Bottom(bottom).Left(0).Width(0).Bounds().H
	card := raised(ui.Column(c.Key("build-hud")).Absolute().Left(theme.S3).Bottom(bottom).Width(hudWidth).Padding(theme.S3, theme.S4).Gap(theme.S2).Radius(theme.RadiusCard))
	if room > 0 {
		card.MaxHeight(max(room-theme.S3, 80))
	}
	// The nearest element handling input gets a scroll first: without this the board takes it.
	card.HandleInput(func(ev ui.InputEvent) bool { return ev.Kind == ui.InputZoom })
	if v.sel == "" && v.editing == nil && c.Shortcut(0, ui.KeyEscape) {
		v.buildClosed = run.What + run.Stage
	}
	card.Children(func() {
		ui.Row(c.Key("head")).FillWidth().Gap(theme.S2).MinHeight(theme.ControlHeight).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Build").FontSize(theme.TextUI).FontWeight(600).TextColor(theme.TextStrong).SingleLine().Shrink(0)
			ui.Row(c).Grow(1).MinWidth(0).Children(func() {
				widget.Running(c, run.What, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			})
			// Closed at any stage, not only once it is done: the next stage shows it again.
			if widget.IconButton(c, icon.Close, "Close", false).Clicked() {
				v.buildClosed = run.What + run.Stage
			}
		})
		buildStepper(c, run.Stage)
		widget.Scroll(c.Key("body")).FillWidth().Shrink(1).MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Gap(theme.S2).Children(func() {
				if len(run.Directions) > 0 {
					v.buildDirections(c, o, cv, *run)
				}
				if run.Review != nil {
					unslopview.DrawReview(c, *run.Review, o.OpenReviewer)
				}
			})
		})
	})
}

// hudWidth is the build panel's: it floats over the page's views.
const hudWidth = 288

// buildStepper is the stages, those past checked, the current lit.
func buildStepper(c *ui.Context, stage string) {
	at := slices.IndexFunc(buildStages, func(s struct{ id, label string }) bool { return s.id == stage })
	ui.Row(c.Key("stages")).FillWidth().Gap(theme.S1).Wrap().Children(func() {
		for i, s := range buildStages {
			col, bg := theme.TextSoft, theme.Sunken
			switch {
			case i < at || stage == proto.BuildDone:
				col = theme.Done
			case i == at:
				col, bg = theme.Accent, theme.Fade(theme.Accent, 0.14)
			}
			ui.Row(c.Key(s.id)).Height(24).PaddingX(theme.S2).Gap(theme.S1).Radius(theme.RadiusFull).AlignItems(ui.Center).Background(bg).Children(func() {
				if i < at || stage == proto.BuildDone {
					ui.Icon(c, icon.Check).FontSize(12).TextColor(col)
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
	ui.Column(c.Key("directions")).FillWidth().Gap(theme.S1).Padding(theme.S2, 0, 0, 0).BorderWidth(theme.Rule, 0, 0, 0).BorderColor(theme.Border).Children(func() {
		for i, d := range run.Directions {
			f := cv.Frames[max(0, slices.IndexFunc(cv.Frames, func(f proto.CanvasFrame) bool { return f.ID == d.Frame }))]
			row := ui.ButtonBase(c.Key(d.Frame)).FillWidth().Gap(theme.S2).Padding(theme.S1, theme.S2).Radius(theme.RadiusSmall).AlignItems(ui.Center).Justify(ui.Start).
				Label("Show " + f.Name).Cursor(ui.CursorPointer)
			if row.Hovered() {
				row.Background(theme.Hover)
			}
			row.Children(func() {
				ui.Box(c).Size(8, 8).Shrink(0).Radius(theme.RadiusFull).Background(ui.Hex(unslopview.RegionColors[i%len(unslopview.RegionColors)]))
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
					widget.Running(c, f.Name, widget.RunStyle{Size: theme.TextLabel, Weight: 500})
					if f.Direction != nil {
						widget.Running(c.Key("src"), f.Direction.Source, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
					}
				})
				switch {
				case d.Done:
					ui.Icon(c, icon.Check).FontSize(12).TextColor(theme.Done)
				case d.Session != "" && o.AgentState != nil && o.AgentState(d.Session) == unslopview.AgentWaiting:
					widget.Tag(c, "needs you", theme.Warning)
				default:
					widget.Spinner(c, 12)
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
			ui.Text(c.Key(p.A+p.B), text).FontSize(theme.TextLabel).TextColor(col).PaddingX(theme.S2)
		}
	})
	c.After(time.Second) // agents' states change outside the run
}

// short is a direction's name without its "Direction " prefix.
func short(name string) string { return strings.TrimPrefix(name, "Direction ") }
