package unslopview

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/designbrief"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// An unslop under way (proto.CanvasUnslopRun), over what it redesigns (a
// canvas's design, a page in the Browser): the score counting to its
// latest read, from where it started; the passes, those done checked, the
// one under way pulsing, each with what the latest read still finds of it;
// a fanned-out run's agents; and, when it is done, the score it went from
// and to.

// Panel is what the panel animates. It belongs to the main thread.
type Panel struct {
	folded bool    // the card shows its head alone
	shown  float64 // the score as it shows, counting to the run's
	at     time.Time
	frame  string // the run's design or page, its own count
	closed string // the run the user closed when done
	brief  bool   // the brief shown in full
}

// PanelOptions are what the panel shows besides the run.
type PanelOptions struct {
	// Reading is what it says before the first read is in.
	Reading string
	// AgentName names a session's agent; AgentState says what it is at
	// (AgentWorking, …); OpenSession shows one; Viewing is the one shown.
	AgentName   func(session string) string
	AgentState  func(session string) string
	OpenSession func(session string)
	Viewing     string
	// Compare, when not nil, shows the run's before and after once done.
	Compare func()
	// OpenReviewer shows the session of the run's independent reviewer.
	OpenReviewer func(session string)
	// Replay, when not nil, plays the run's steps back from step k (0 the
	// design it started from).
	Replay func(k int)
}

// Draw draws run's panel at the top right of the element it is in.
func (p *Panel) Draw(c *ui.Context, run *proto.CanvasUnslopRun, o PanelOptions) {
	if run == nil || p.closed == run.Frame+fmt.Sprint(run.Done) {
		return
	}
	if run.Scans == 0 && len(run.Steps) > 0 {
		drawSteps(c, run)
		return
	}
	now := time.Now()
	h := p
	if h.frame != run.Frame {
		h.frame, h.shown, h.at = run.Frame, float64(run.From), now
	}
	// Counts toward the latest score, a tenth of the way each frame.
	if d := float64(run.Score) - h.shown; math.Abs(d) > 0.4 {
		h.shown += d * min(1, now.Sub(h.at).Seconds()*8)
		c.After(16 * time.Millisecond)
	} else {
		h.shown = float64(run.Score)
	}
	h.at = now
	counts := map[string]int{}
	for _, fd := range run.Findings {
		counts[fd.Pass]++
	}
	col := scoreColor(int(math.Round(h.shown)))
	hudCard(c, "unslop-hud", func() {
		ui.Row(c.Key("bar")).FillWidth().Gap(theme.S1).MinHeight(theme.ControlHeight).AlignItems(ui.Center).Children(func() {
			label := "Unslop"
			switch {
			case run.Done:
				label = "Unslopped"
			case run.Pass != "":
				label = run.Pass
			case regionsAtWork(run) > 0:
				n := regionsAtWork(run)
				label = textfmt.Plural(n, "1 agent at work", fmt.Sprintf("%d agents at work", n))
			case run.Scans <= 1:
				label = o.Reading
			}
			ui.Row(c.Key("title")).Grow(1).MinWidth(0).Children(func() {
				widget.Running(c, label, widget.RunStyle{Weight: 600, Color: theme.TextStrong})
			})
			if run.Done && o.Compare != nil {
				if widget.GhostButton(c, "Compare").Clicked() {
					o.Compare()
				}
			}
			fold, tip := icon.ChevronDown, "Fold this card to its head"
			if h.folded {
				fold, tip = icon.ChevronRight, "Unfold this card"
			}
			if widget.IconButton(c, fold, tip, false).Clicked() {
				h.folded = !h.folded
			}
			if run.Done && widget.IconButton(c, icon.Close, "Close", false).Clicked() {
				h.closed = run.Frame + "true"
			}
		})
		ui.Row(c.Key("score")).FillWidth().Gap(theme.S1).AlignItems(ui.Baseline).Children(func() {
			ui.Text(c, fmt.Sprint(int(math.Round(h.shown)))).FontSize(24).FontWeight(500).TextColor(col)
			ui.Text(c, "/100").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
			ui.Spacer(c)
			if run.Score != run.From || run.Done {
				ui.Text(c, fmt.Sprintf("from %d", run.From)).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
			}
		})
	}, func() {
		if h.folded {
			return
		}
		if run.Branch != "" {
			// Where a project's code is redesigned: going back undoes it.
			ui.Row(c.Key("branch")).FillWidth().Gap(theme.S1).AlignItems(ui.Center).Tooltip("Switch back to " + run.Base + " to undo it all; each pass is a commit of its own").Children(func() {
				ui.Icon(c, icon.Git).FontSize(12).TextColor(theme.TextSoft)
				ui.Row(c).Grow(1).MinWidth(0).Children(func() {
					widget.Running(c, run.Branch+" · from "+run.Base, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
				})
			})
		}
		if run.Brief != nil {
			p.drawBrief(c, *run.Brief)
		}
		// How far through the passes.
		ui.Box(c.Key("progress")).FillWidth().Height(4).Shrink(0).Radius(theme.RadiusFull).Clip().Background(theme.Sunken).Children(func() {
			done := min(float32(len(run.Finished))/float32(max(len(run.Passes), 1)), 1)
			if run.Done {
				done = 1
			}
			ui.Box(c).Width(hudInner * done).Height(4).Radius(theme.RadiusFull).Background(col)
		})
		ui.Column(c).FillWidth().Gap(2).Children(func() {
			for _, p := range run.Passes {
				hudPass(c, run, p, counts[p], now)
			}
		})
		if len(run.Passed) > 0 && o.Replay != nil {
			hudSteps(c, o, run)
		}
		if len(run.Regions) > 0 {
			hudRegions(c, o, run)
		}
		if run.Review != nil {
			DrawReview(c, *run.Review, o.OpenReviewer)
		}
	})
}

func hudRegions(c *ui.Context, o PanelOptions, run *proto.CanvasUnslopRun) {
	ui.Column(c.Key("regions")).FillWidth().Gap(theme.S1).Padding(theme.S2, 0, 0, 0).BorderWidth(0.5, 0, 0, 0).BorderColor(theme.Border).Children(func() {
		ui.Text(c, fmt.Sprintf("%d agents at once", len(run.Regions))).FontSize(theme.TextLabel).FontWeight(500).TextColor(theme.TextSoft)
		for i, r := range run.Regions {
			state := AgentWorking
			if o.AgentState != nil {
				state = o.AgentState(r.Session)
			}
			row := ui.ButtonBase(c.Key("region:" + r.ID)).FillWidth().Height(28).PaddingX(theme.S2).Gap(theme.S2).Radius(theme.RadiusSmall).AlignItems(ui.Center).Justify(ui.Start).
				Label("Show the agent of #" + r.ID).Tooltip("Show what this agent does")
			if o.OpenSession != nil {
				row.Cursor(ui.CursorPointer)
				switch {
				case o.Viewing == r.Session:
					row.Background(theme.Fade(theme.Accent, 0.16))
				case row.Hovered():
					row.Background(theme.Hover)
				}
				if row.Clicked() {
					o.OpenSession(r.Session)
				}
			}
			row.Children(func() {
				ui.Box(c).Size(8, 8).Shrink(0).Radius(theme.RadiusFull).Background(ui.Hex(RegionColors[i%len(RegionColors)]))
				name := "#" + r.ID
				if o.AgentName != nil {
					if n := o.AgentName(r.Session); n != "" {
						name = n + " · #" + r.ID
					}
				}
				ui.Row(c.Key("who")).Grow(1).MinWidth(0).Children(func() {
					widget.Running(c, name, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
				})
				switch {
				case r.Done || run.Done:
					ui.Icon(c, icon.Check).FontSize(12).TextColor(theme.Done)
				case state == AgentStopped:
					ui.Text(c, "stopped").FontSize(theme.TextLabel).TextColor(theme.Error).SingleLine().Shrink(0)
				case state == AgentWaiting:
					ui.Text(c, "needs you").FontSize(theme.TextLabel).FontWeight(500).TextColor(theme.Warning).SingleLine().Shrink(0)
				case state == AgentIdle:
					ui.Text(c, "idle").FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(0).Tooltip("Not working: open it to see why")
				default:
					if r.Pass != "" {
						num, _, _ := strings.Cut(r.Pass, " ")
						ui.Text(c, num).FontSize(theme.TextLabel).TextColor(theme.Now)
					}
					widget.Spinner(c, 12)
				}
			})
		}
	})
}

// hudPass is a pass's line: checked when done, pulsing while under way,
// with what the latest read finds of it.
func hudPass(c *ui.Context, run *proto.CanvasUnslopRun, pass string, found int, now time.Time) {
	done := run.Done || contains(run.Finished, pass)
	active := pass == run.Pass && !run.Done
	for _, r := range run.Regions {
		active = active || (!r.Done && !run.Done && r.Pass == pass)
	}
	row := ui.Row(c.Key(pass)).FillWidth().Height(24).PaddingX(theme.S2).Gap(theme.S2).Radius(theme.RadiusSmall).AlignItems(ui.Center)
	if active {
		// A slow pulse while the agent works on it.
		a := 0.12 + 0.08*math.Sin(float64(now.UnixMilli())/300)
		row.Background(theme.Fade(theme.Accent, a))
		c.After(33 * time.Millisecond)
	}
	row.Children(func() {
		switch {
		case done:
			ui.Icon(c, icon.Check).FontSize(12).TextColor(theme.Done)
		case active:
			widget.Spinner(c, 12)
		default:
			ui.Row(c).Size(12, 12).Shrink(0).Center().Children(func() {
				ui.Box(c).Size(6, 6).Radius(theme.RadiusFull).Background(theme.Border)
			})
		}
		num, name, _ := strings.Cut(pass, " ")
		ui.Text(c, num).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(0)
		tc := theme.Text
		if !done && !active && found == 0 {
			tc = theme.TextSoft
		}
		ui.Row(c.Key("name")).Grow(1).MinWidth(0).Children(func() {
			widget.Running(c, name, widget.RunStyle{Size: theme.TextLabel, Color: tc})
		})
		if found > 0 {
			ui.Text(c, fmt.Sprint(found)).FontSize(theme.TextLabel).FontWeight(500).TextColor(theme.Warning).SingleLine().Shrink(0)
		}
	})
}

// What a region's agent is at, as PanelOptions.AgentState says.
const (
	AgentWorking = "working"
	AgentWaiting = "waiting" // asking the user something
	AgentIdle    = "idle"    // neither working nor done: stuck, maybe
	AgentStopped = "stopped"
)

// regionsAtWork are run's regions not done yet.
func regionsAtWork(run *proto.CanvasUnslopRun) int {
	n := 0
	for _, r := range run.Regions {
		if !r.Done {
			n++
		}
	}
	return n
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// scoreColor is a score's: red low, amber middling, green high.
func scoreColor(score int) ui.Color {
	switch {
	case score >= 85:
		return theme.Done
	case score >= 60:
		return theme.Warning
	}
	return theme.Error
}

// hudMargin is what a card leaves around it in the element it is over;
// hudWidth its width; hudInner the room inside its padding.
const (
	hudMargin = theme.S3
	hudWidth  = 288
	hudInner  = hudWidth - 2*theme.S4
)

// raised is a card's material, over the stage: as a menu's, with an overlay's edge.
func raised(e ui.Element) ui.Element {
	return widget.Rim(e.Radius(theme.RadiusCard).Background(theme.Popover))
}

// hudCard is a panel's card at the top right of the element it is in: head
// always in view, body scrolling inside it once they'd pass the element's
// height. The pointer's scrolling and zooming over it stay with it, not the
// board below.
func hudCard(c *ui.Context, key string, head, body func()) {
	room := ui.Box(c.Key(key + ":room")).Absolute().Top(0).Bottom(0).Right(0).Width(0).Bounds().H
	card := raised(ui.Column(c.Key(key)).Absolute().Right(hudMargin).Top(hudMargin).Width(hudWidth).Padding(theme.S3, 0).Gap(theme.S2).Clip())
	if room > 0 {
		card.MaxHeight(max(room-2*hudMargin, 80))
	}
	// The nearest element handling input gets a scroll first, so without this
	// the board around would take it. Not taken here, it scrolls the body.
	card.HandleInput(func(ev ui.InputEvent) bool { return ev.Kind == ui.InputZoom })
	card.Children(func() {
		ui.Column(c.Key("head")).FillWidth().PaddingX(theme.S4).Gap(theme.S2).Shrink(0).Children(head)
		ui.Scroll(c.Key("body")).FillWidth().Shrink(1).MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().PaddingX(theme.S4).Gap(theme.S2).Children(body)
		})
	})
}

// DrawStarting is the panel while an unslop starts, before its first read
// is in: what it is doing, a spinner.
func DrawStarting(c *ui.Context, doing string) {
	raised(ui.Row(c.Key("unslop-starting")).Absolute().Right(hudMargin).Top(hudMargin).Width(hudWidth).Padding(theme.S3, theme.S4).Gap(theme.S2).AlignItems(ui.Center)).Children(func() {
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
			widget.Running(c, "Unslop starting…", widget.RunStyle{Weight: 600, Color: theme.TextStrong})
			widget.Running(c.Key("doing"), doing, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
		})
		widget.Spinner(c, 14)
	})
}

// DrawError says why an unslop didn't start, over what it was for, until
// dismissed.
func DrawError(c *ui.Context, msg string, dismiss func()) {
	raised(ui.Row(c.Key("unslop-error")).Absolute().Right(hudMargin).Top(hudMargin).Width(hudWidth).Padding(theme.S2, theme.S2, theme.S2, theme.S4).Gap(theme.S2).AlignItems(ui.Start)).Children(func() {
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(theme.S1).Children(func() {
			ui.Text(c, "Unslop didn't start").FontSize(theme.TextUI).FontWeight(600).TextColor(theme.Error)
			ui.Scroll(c.Key("msg")).FillWidth().MaxHeight(200).Children(func() {
				ui.Text(c, msg).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
			})
		})
		if dismiss != nil && widget.IconButton(c, icon.Close, "Dismiss", false).Clicked() {
			dismiss()
		}
	})
}

// drawSteps is a run's panel before its first read: its steps one by one,
// those done checked, the one under way pulsing.
func drawSteps(c *ui.Context, run *proto.CanvasUnslopRun) {
	now := time.Now()
	raised(ui.Column(c.Key("unslop-steps")).Absolute().Right(hudMargin).Top(hudMargin).Width(hudWidth).Padding(theme.S3, theme.S4).Gap(theme.S2)).Children(func() {
		ui.Row(c.Key("head")).FillWidth().Gap(theme.S2).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Grow(1).MinWidth(0).Children(func() {
				widget.Running(c, "Unslop starting…", widget.RunStyle{Weight: 600, Color: theme.TextStrong})
			})
			ui.Text(c, fmt.Sprintf("%d/%d", min(run.Step+1, len(run.Steps)), len(run.Steps))).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(0)
		})
		ui.Box(c.Key("progress")).FillWidth().Height(4).Shrink(0).Radius(theme.RadiusFull).Clip().Background(theme.Sunken).Children(func() {
			ui.Box(c).Width(hudInner * min(float32(run.Step)/float32(len(run.Steps)), 1)).Height(4).Radius(theme.RadiusFull).Background(theme.Accent)
		})
		ui.Column(c).FillWidth().Gap(2).Children(func() {
			for i, step := range run.Steps {
				row := ui.Row(c.Key(step)).FillWidth().Height(24).PaddingX(theme.S2).Gap(theme.S2).Radius(theme.RadiusSmall).AlignItems(ui.Center)
				active := i == run.Step
				if active {
					a := 0.12 + 0.08*math.Sin(float64(now.UnixMilli())/300)
					row.Background(theme.Fade(theme.Accent, a))
					c.After(33 * time.Millisecond)
				}
				row.Children(func() {
					tc := theme.TextSoft
					switch {
					case i < run.Step:
						ui.Icon(c, icon.Check).FontSize(12).TextColor(theme.Done)
						tc = theme.TextSoft
					case active:
						widget.Spinner(c, 12)
						tc = theme.Text
					default:
						ui.Row(c).Size(12, 12).Shrink(0).Center().Children(func() {
							ui.Box(c).Size(6, 6).Radius(theme.RadiusFull).Background(theme.Border)
						})
					}
					ui.Row(c.Key("step")).Grow(1).MinWidth(0).Children(func() {
						widget.Running(c, step, widget.RunStyle{Size: theme.TextLabel, Color: tc})
					})
				})
			}
		})
	})
}

// drawBrief is the run's brief in a line, all of it once clicked.
func (p *Panel) drawBrief(c *ui.Context, b proto.DesignBrief) {
	row := ui.ButtonBase(c.Key("brief")).FillWidth().Gap(theme.S1).AlignItems(ui.Start).Justify(ui.Start).Padding(theme.S1, theme.S2).Radius(theme.RadiusSmall).
		Label("The brief: what it is for").Cursor(ui.CursorPointer)
	if row.Hovered() {
		row.Background(theme.Hover)
	}
	row.Children(func() {
		if !p.brief {
			ui.Row(c.Key("line")).Grow(1).MinWidth(0).Children(func() {
				widget.Running(c, designbrief.Summary(b), widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			})
			return
		}
		ui.Column(c.Key("whole")).Grow(1).Shrink(1).MinWidth(0).Gap(theme.S1).Children(func() {
			for _, kv := range designbrief.Fields(b) {
				if strings.TrimSpace(kv[1]) == "" {
					continue
				}
				ui.Column(c.Key(kv[0])).FillWidth().Gap(1).Children(func() {
					ui.Text(c, kv[0]).FontSize(theme.TextLabel).FontWeight(500).TextColor(theme.TextSoft)
					ui.Text(c, kv[1]).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
				})
			}
		})
	})
	if row.Clicked() {
		p.brief = !p.brief
	}
}

// DrawReview is an independent review (an unslop's, a build's): under
// way, its reviewer a click away (open), or its score and what it asks.
func DrawReview(c *ui.Context, rv proto.DesignReview, open func(session string)) {
	ui.Column(c.Key("review")).FillWidth().Gap(theme.S1).Padding(theme.S2, 0, 0, 0).BorderWidth(0.5, 0, 0, 0).BorderColor(theme.Border).Children(func() {
		ui.Row(c.Key("head")).FillWidth().Gap(theme.S2).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Grow(1).MinWidth(0).Children(func() {
				widget.Running(c, fmt.Sprintf("Independent review · round %d", rv.Round), widget.RunStyle{Size: theme.TextLabel, Weight: 500, Color: theme.TextSoft})
			})
			switch {
			case !rv.Done:
				widget.Spinner(c, 12)
			case rv.Passed:
				widget.Tag(c, fmt.Sprintf("%d/10 · passes", rv.Score), theme.Done)
			default:
				widget.Tag(c, fmt.Sprintf("%d/10", rv.Score), theme.Warning)
			}
		})
		if !rv.Done {
			text := "A fresh agent looks at the screenshots…"
			if rv.Session == "" {
				text = "Starting a fresh agent to look at it…"
			}
			row := ui.ButtonBase(c.Key("reviewer")).FillWidth().Padding(theme.S1, 0).Radius(theme.RadiusSmall).Justify(ui.Start).Label("Show the reviewer").Cursor(ui.CursorPointer)
			if rv.Session == "" || open == nil {
				row.Disabled(true)
			} else if row.Hovered() {
				row.Background(theme.Hover)
			}
			row.Children(func() { ui.Text(c, text).FontSize(theme.TextLabel).TextColor(theme.TextSoft) })
			if row.Clicked() && open != nil {
				open(rv.Session)
			}
			return
		}
		for i, s := range append(append([]string{}, rv.Issues...), rv.Improvements...) {
			ui.Text(c.Key(fmt.Sprint("r", i)), "· "+s).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		}
		if rv.Keep != "" {
			ui.Text(c, "Keep: "+rv.Keep).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		}
	})
}

// hudSteps are the run's steps so far, each what it changed, a click
// playing it back.
func hudSteps(c *ui.Context, o PanelOptions, run *proto.CanvasUnslopRun) {
	ui.Column(c.Key("steps")).FillWidth().Gap(theme.S1).Children(func() {
		ui.Row(c.Key("head")).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Steps").FontSize(theme.TextLabel).FontWeight(500).TextColor(theme.TextSoft).Grow(1)
			if widget.GhostButton(c.Key("replay"), "Replay").Clicked() {
				o.Replay(0)
			}
		})
		for i, s := range run.Passed {
			label := unslopStepName(s)
			b := ui.ButtonBase(c.Key(i)).FillWidth().Height(28).PaddingX(theme.S2).Gap(theme.S2).Radius(theme.RadiusSmall).AlignItems(ui.Center).Justify(ui.Start).
				Cursor(ui.CursorPointer).Label("Play step " + label)
			if b.Hovered() {
				b.Background(theme.Hover)
			}
			b.Children(func() {
				ui.Text(c, fmt.Sprint(i+1)).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(0)
				ui.Row(c.Key("name")).Grow(1).MinWidth(0).Children(func() {
					widget.Running(c, label, widget.RunStyle{Size: theme.TextLabel})
				})
				what := "not measured yet"
				if s.Measured {
					what = textfmt.Plural(len(s.Changes), "1 change", fmt.Sprintf("%d changes", len(s.Changes)))
				}
				ui.Text(c, what).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(0)
			})
			if b.Clicked() {
				o.Replay(i + 1)
			}
		}
	})
}

// unslopStepName names a step by its passes.
func unslopStepName(s proto.UnslopStep) string {
	switch len(s.Passes) {
	case 0:
		return "Step"
	case 1:
		return s.Passes[0]
	}
	return s.Passes[0] + " … " + s.Passes[len(s.Passes)-1]
}
