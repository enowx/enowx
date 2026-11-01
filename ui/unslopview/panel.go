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
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon.Wand).FontSize(14).TextColor(theme.Accent)
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
			ui.Text(c, label).FontSize(12.5).FontWeight(600).SingleLine().Grow(1).Shrink(1)
			if run.Done && o.Compare != nil {
				if widget.Button(c, widget.Secondary, icon.Columns, "Compare", "").Clicked() {
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
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.End).Children(func() {
			ui.Text(c, fmt.Sprint(int(math.Round(h.shown)))).FontSize(30).FontWeight(300).TextColor(col).Font(theme.MonoFont)
			ui.Text(c, "/100").FontSize(12).TextColor(theme.Muted).Margin(0, 0, 6, 0)
			ui.Spacer(c)
			if run.Score != run.From || run.Done {
				ui.Text(c, fmt.Sprintf("from %d", run.From)).FontSize(11.5).TextColor(theme.Muted).Margin(0, 0, 6, 0)
			}
		})
	}, func() {
		if h.folded {
			return
		}
		if run.Branch != "" {
			// Where a project's code is redesigned: going back undoes it.
			ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Tooltip("Switch back to " + run.Base + " to undo it all; each pass is a commit of its own").Children(func() {
				ui.Icon(c, icon.Git).FontSize(11).TextColor(theme.Muted)
				ui.Text(c, run.Branch+" · from "+run.Base).FontSize(11).TextColor(theme.Muted).Font(theme.MonoFont).SingleLine().Shrink(1)
			})
		}
		if run.Brief != nil {
			p.drawBrief(c, *run.Brief)
		}
		// How far through the passes.
		ui.Box(c).FillWidth().Height(4).Radius(2).Background(theme.Sunken).Children(func() {
			done := float32(len(run.Finished)) / float32(max(len(run.Passes), 1))
			if run.Done {
				done = 1
			}
			ui.Box(c).Width(244 * done).Height(4).Radius(2).Background(col)
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
	ui.Column(c).FillWidth().Gap(2).Padding(8, 0, 0, 0).BorderWidth(1, 0, 0, 0).BorderColor(theme.Border).Children(func() {
		ui.Text(c, fmt.Sprintf("%d agents at once", len(run.Regions))).FontSize(10.5).FontWeight(600).TextColor(theme.Muted).Margin(0, 6, 4, 6)
		for i, r := range run.Regions {
			state := AgentWorking
			if o.AgentState != nil {
				state = o.AgentState(r.Session)
			}
			row := ui.ButtonBase(c.Key("region:" + r.ID)).FillWidth().Height(24).PaddingX(6).Gap(8).Radius(6).AlignItems(ui.Center).
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
				ui.Box(c).Size(8, 8).Radius(4).Background(ui.Hex(RegionColors[i%len(RegionColors)]))
				name := "#" + r.ID
				if o.AgentName != nil {
					if n := o.AgentName(r.Session); n != "" {
						name = n + " · #" + r.ID
					}
				}
				ui.Text(c, name).FontSize(11.5).TextColor(theme.TextSoft).Grow(1).Shrink(1).SingleLine()
				switch {
				case r.Done || run.Done:
					ui.Icon(c, icon.Check).FontSize(11).TextColor(theme.Done)
				case state == AgentStopped:
					ui.Text(c, "stopped").FontSize(10.5).TextColor(theme.Error)
				case state == AgentWaiting:
					ui.Text(c, "needs you").FontSize(10.5).FontWeight(600).TextColor(theme.Warning)
				case state == AgentIdle:
					ui.Text(c, "idle").FontSize(10.5).TextColor(theme.Muted).Tooltip("Not working: open it to see why")
				default:
					if r.Pass != "" {
						num, _, _ := strings.Cut(r.Pass, " ")
						ui.Text(c, num).FontSize(10.5).TextColor(theme.Accent).Font(theme.MonoFont)
					}
					widget.Spinner(c, 10)
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
	row := ui.Row(c.Key(pass)).FillWidth().Height(22).PaddingX(6).Gap(8).Radius(6).AlignItems(ui.Center)
	if active {
		// A slow pulse while the agent works on it.
		a := 0.12 + 0.08*math.Sin(float64(now.UnixMilli())/300)
		row.Background(theme.Fade(theme.Accent, a))
		c.After(33 * time.Millisecond)
	}
	row.Children(func() {
		switch {
		case done:
			ui.Icon(c, icon.Check).FontSize(11).TextColor(theme.Done)
		case active:
			widget.Spinner(c, 10)
		default:
			ui.Box(c).Size(6, 6).Radius(3).Background(theme.Border).Margin(0, 2)
		}
		num, name, _ := strings.Cut(pass, " ")
		ui.Text(c, num).FontSize(10.5).TextColor(theme.Idle).Font(theme.MonoFont)
		tc := theme.TextSoft
		if !done && !active && found == 0 {
			tc = theme.Muted
		}
		ui.Text(c, name).FontSize(11.5).TextColor(tc).Grow(1).SingleLine()
		if found > 0 {
			ui.Text(c, fmt.Sprint(found)).FontSize(10.5).FontWeight(600).TextColor(theme.Warning)
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

// hudMargin is what a card leaves around it in the element it is over.
const hudMargin = 12

// hudCard is a panel's card at the top right of the element it is in: head
// always in view, body scrolling inside it once they'd pass the element's
// height. The pointer's scrolling and zooming over it stay with it, not the
// board below.
func hudCard(c *ui.Context, key string, head, body func()) {
	room := ui.Box(c.Key(key + ":room")).Absolute().Top(0).Bottom(0).Right(0).Width(0).Bounds().H
	card := ui.Column(c.Key(key)).Absolute().Right(hudMargin).Top(hudMargin).Width(264).Padding(12, 0).Gap(10).Radius(12).Clip().
		Background(theme.Dialog).Border(1, theme.BorderStrong)
	if room > 0 {
		card.MaxHeight(max(room-2*hudMargin, 80))
	}
	// The nearest element handling input gets a scroll first, so without this
	// the board around would take it. Not taken here, it scrolls the body.
	card.HandleInput(func(ev ui.InputEvent) bool { return ev.Kind == ui.InputZoom })
	card.Children(func() {
		ui.Column(c).FillWidth().PaddingX(12).Gap(10).Shrink(0).Children(head)
		ui.Scroll(c.Key("body")).FillWidth().Shrink(1).MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Padding(0, 12, 2, 12).Gap(10).Children(body)
		})
	})
}

// DrawStarting is the panel while an unslop starts, before its first read
// is in: what it is doing, a spinner.
func DrawStarting(c *ui.Context, doing string) {
	ui.Row(c.Key("unslop-starting")).Absolute().Right(12).Top(12).Width(264).Padding(12).Gap(10).Radius(12).AlignItems(ui.Center).
		Background(theme.Dialog).Border(1, theme.BorderStrong).Children(func() {
		ui.Icon(c, icon.Wand).FontSize(14).TextColor(theme.Accent)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, "Unslop starting…").FontSize(12.5).FontWeight(600).SingleLine()
			ui.Text(c, doing).FontSize(11).TextColor(theme.Muted).SingleLine()
		})
		widget.Spinner(c, 14)
	})
}

// DrawError says why an unslop didn't start, over what it was for, until
// dismissed.
func DrawError(c *ui.Context, msg string, dismiss func()) {
	ui.Row(c.Key("unslop-error")).Absolute().Right(12).Top(12).Width(300).Padding(10, 12).Gap(8).Radius(12).AlignItems(ui.Start).
		Background(theme.Dialog).Border(1, theme.Fade(theme.Error, 0.6)).Children(func() {
		ui.Icon(c, icon.Alert).FontSize(14).TextColor(theme.Error).Margin(1, 0, 0, 0)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, "Unslop didn't start").FontSize(12.5).FontWeight(600)
			ui.Scroll(c.Key("msg")).FillWidth().MaxHeight(200).Children(func() {
				ui.Text(c, msg).FontSize(11.5).TextColor(theme.TextSoft)
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
	ui.Column(c.Key("unslop-steps")).Absolute().Right(12).Top(12).Width(264).Padding(12).Gap(10).Radius(12).
		Background(theme.Dialog).Border(1, theme.BorderStrong).Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon.Wand).FontSize(14).TextColor(theme.Accent)
			ui.Text(c, "Unslop starting…").FontSize(12.5).FontWeight(600).Grow(1).SingleLine()
			ui.Text(c, fmt.Sprintf("%d/%d", min(run.Step+1, len(run.Steps)), len(run.Steps))).FontSize(11).TextColor(theme.Muted).Font(theme.MonoFont)
		})
		ui.Box(c).FillWidth().Height(4).Radius(2).Background(theme.Sunken).Children(func() {
			ui.Box(c).Width(240 * float32(run.Step) / float32(len(run.Steps))).Height(4).Radius(2).Background(theme.Accent)
		})
		ui.Column(c).FillWidth().Gap(2).Children(func() {
			for i, step := range run.Steps {
				row := ui.Row(c.Key(step)).FillWidth().Height(24).PaddingX(6).Gap(8).Radius(6).AlignItems(ui.Center)
				active := i == run.Step
				if active {
					a := 0.12 + 0.08*math.Sin(float64(now.UnixMilli())/300)
					row.Background(theme.Fade(theme.Accent, a))
					c.After(33 * time.Millisecond)
				}
				row.Children(func() {
					tc := theme.Muted
					switch {
					case i < run.Step:
						ui.Icon(c, icon.Check).FontSize(11).TextColor(theme.Done)
						tc = theme.TextSoft
					case active:
						widget.Spinner(c, 10)
						tc = theme.Text
					default:
						ui.Box(c).Size(6, 6).Radius(3).Background(theme.Border).Margin(0, 2)
					}
					ui.Text(c, step).FontSize(11.5).TextColor(tc).Grow(1).Shrink(1).SingleLine()
				})
			}
		})
	})
}

// drawBrief is the run's brief in a line, all of it once clicked.
func (p *Panel) drawBrief(c *ui.Context, b proto.DesignBrief) {
	row := ui.ButtonBase(c.Key("brief")).FillWidth().Gap(6).AlignItems(ui.Start).Padding(4, 6).Radius(6).
		Label("The brief: what it is for").Cursor(ui.CursorPointer)
	if row.Hovered() {
		row.Background(theme.Hover)
	}
	row.Children(func() {
		ui.Icon(c, icon.Target).FontSize(11).TextColor(theme.Accent).Margin(2, 0, 0, 0)
		if !p.brief {
			ui.Text(c, designbrief.Summary(b)).FontSize(11).TextColor(theme.TextSoft).SingleLine().Shrink(1)
			return
		}
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(5).Children(func() {
			for _, kv := range designbrief.Fields(b) {
				if strings.TrimSpace(kv[1]) == "" {
					continue
				}
				ui.Column(c.Key(kv[0])).FillWidth().Gap(1).Children(func() {
					ui.Text(c, kv[0]).FontSize(10).FontWeight(600).TextColor(theme.Muted)
					ui.Text(c, kv[1]).FontSize(11).TextColor(theme.TextSoft)
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
	ui.Column(c.Key("review")).FillWidth().Gap(4).Padding(8, 0, 0, 0).BorderWidth(1, 0, 0, 0).BorderColor(theme.Border).Children(func() {
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon.Eye).FontSize(11).TextColor(theme.Muted)
			label := fmt.Sprintf("Independent review · round %d", rv.Round)
			ui.Text(c, label).FontSize(10.5).FontWeight(600).TextColor(theme.Muted).Grow(1).Shrink(1).SingleLine()
			switch {
			case !rv.Done:
				widget.Spinner(c, 10)
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
			row := ui.ButtonBase(c.Key("reviewer")).FillWidth().Padding(2, 6).Radius(6).Label("Show the reviewer").Cursor(ui.CursorPointer)
			if rv.Session == "" || open == nil {
				row.Disabled(true)
			} else if row.Hovered() {
				row.Background(theme.Hover)
			}
			row.Children(func() { ui.Text(c, text).FontSize(11).TextColor(theme.TextSoft) })
			if row.Clicked() && open != nil {
				open(rv.Session)
			}
			return
		}
		for i, s := range append(append([]string{}, rv.Issues...), rv.Improvements...) {
			ui.Text(c.Key(fmt.Sprint("r", i)), "· "+s).FontSize(11).TextColor(theme.TextSoft).Padding(0, 6)
		}
		if rv.Keep != "" {
			ui.Text(c, "Keep: "+rv.Keep).FontSize(11).TextColor(theme.Muted).Padding(0, 6)
		}
	})
}

// hudSteps are the run's steps so far, each what it changed, a click
// playing it back.
func hudSteps(c *ui.Context, o PanelOptions, run *proto.CanvasUnslopRun) {
	ui.Column(c.Key("steps")).FillWidth().Gap(4).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Steps").FontSize(11).FontWeight(600).TextColor(theme.Muted).Grow(1)
			if widget.GhostButton(c.Key("replay"), "Replay").Height(22).Clicked() {
				o.Replay(0)
			}
		})
		for i, s := range run.Passed {
			label := unslopStepName(s)
			b := ui.ButtonBase(c.Key(i)).FillWidth().Height(24).PaddingX(6).Gap(6).Radius(6).AlignItems(ui.Center).
				Cursor(ui.CursorPointer).Label("Play step " + label)
			if b.Hovered() {
				b.Background(theme.Hover)
			}
			b.Children(func() {
				ui.Text(c, fmt.Sprint(i+1)).FontSize(10.5).Font(theme.MonoFont).TextColor(theme.Muted).Width(14)
				ui.Text(c, label).FontSize(11.5).SingleLine().Grow(1).Shrink(1)
				what := "not measured yet"
				if s.Measured {
					what = textfmt.Plural(len(s.Changes), "1 change", fmt.Sprintf("%d changes", len(s.Changes)))
				}
				ui.Text(c, what).FontSize(10.5).TextColor(theme.Muted).SingleLine()
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
