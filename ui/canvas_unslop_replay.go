package ui

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/unslop"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// A canvas redesign's steps, after: which passes to run before it starts,
// and its Replay, a dialog playing the steps back (core/unslop's story
// page in a web view), each step's changes as measured, two steps
// compared with a slider, a step left out in a copy, the play exported
// as a video (canvas_unslop_video.go).

// unslopChoice is the dialog picking the passes of an unslop. It belongs
// to the main thread.
type unslopChoice struct {
	open  bool
	frame string
	on    []bool
}

func (a *App) chooseUnslop(frame string) {
	a.unslopPick = unslopChoice{open: true, frame: frame, on: make([]bool, len(unslop.Passes))}
	for i := range a.unslopPick.on {
		a.unslopPick.on[i] = true
	}
}

func (a *App) unslopChoiceDialog(c *ui.Context) {
	p := &a.unslopPick
	if !p.open {
		return
	}
	widget.FormSheet(c, &p.open, 460, "Unslop with these passes", "The agent redesigns a copy of the design, these passes in order, the others skipped.", func() {
		for i, ps := range unslop.Passes {
			switchRowKey(c, ps.Name, &p.on[i], ps.Name, "")
		}
	}, func() {
		var only []string
		for i, on := range p.on {
			if on {
				only = append(only, unslop.Passes[i].Name)
			}
		}
		if widget.PrimaryButton(c, nil, "Unslop", "").Disabled(len(only) == 0).Clicked() {
			p.open = false
			if len(only) == len(unslop.Passes) {
				only = nil
			}
			a.unslopCanvasOnly(p.frame, only)
		}
	})
}

// unslopReplay is the Replay dialog's. It belongs to the main thread.
type unslopReplay struct {
	open    bool
	canvas  string
	frame   string
	story   proto.UnslopStory
	loaded  bool
	err     string
	wv      *mygo.WebView
	step    int  // the step shown, 0 the design started from
	compare int  // the step compared with step, -1 for none
	picking bool // the next step clicked is compared with step
	busy    string
	video   videoExport
}

// openReplay opens the Replay of the redesign of frame, at step k.
func (a *App) openReplay(frame string, k int) {
	cv, cl, win := a.canvases.Canvas(), a.client, a.win
	if cv == nil || cl == nil {
		return
	}
	a.closeReplay()
	r := &a.replay
	*r = unslopReplay{open: true, canvas: cv.ID, frame: frame, step: k, compare: -1}
	id := cv.ID
	go func() {
		st, err := cl.UnslopStory(id, frame)
		var wv *mygo.WebView
		if err == nil && win != nil && webViewAvailable() {
			wv, err = win.NewWebView(mygo.WebViewOptions{})
		}
		a.onMain(func() {
			if !r.open || r.frame != frame {
				if wv != nil {
					wv.Destroy()
				}
				return
			}
			if err != nil {
				r.err = client.Explain(err)
				return
			}
			r.story, r.wv, r.loaded = st, wv, true
			r.step = min(r.step, len(st.Steps))
			if wv != nil {
				wv.Page().LoadHTML(unslop.StoryHTML(st, ""), "")
				a.replayEval(fmt.Sprintf("setTimeout(function(){__story.load(%d)},50)", r.step))
			}
		})
	}()
}

func (a *App) closeReplay() {
	if a.replay.wv != nil {
		a.replay.wv.Destroy()
	}
	a.replay = unslopReplay{}
}

// replayEval runs code in the Replay's page.
func (a *App) replayEval(code string) {
	if wv := a.replay.wv; wv != nil {
		go func() { _, _ = wv.Page().Eval(code) }()
	}
}

func (a *App) replayDialog(c *ui.Context) {
	r := &a.replay
	if !r.open {
		return
	}
	open := true
	widget.Dialog(c, &open, 1040, theme.S3, func() {
		title := "Replay"
		if r.story.Name != "" {
			title = "Replay · " + r.story.Name
		}
		widget.DialogHeader(c, title, "Each step as the agent left the design, what it changed boxed and measured.", true)
		switch {
		case r.err != "":
			widget.Notice(c, icon.Alert, theme.Error, r.err, "", nil)
			return
		case !r.loaded:
			widget.Spinner(c, 16)
			return
		}
		ui.Row(c.Key("replay-body")).FillWidth().Gap(theme.S3).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c.Key("stage")).Grow(1).MinWidth(0).Gap(theme.S2).Children(func() {
				if r.wv != nil {
					ui.WebView(c, r.wv).FillWidth().Height(500).Radius(theme.RadiusCard)
				} else {
					panelNote(c, "This device has no web view to play it in.")
				}
				a.replayStrip(c, r)
			})
			a.replaySide(c, r)
		})
	})
	if !open {
		a.closeReplay()
	}
}

// replayStrip is the steps, a click showing one (or comparing it), and
// the play button.
func (a *App) replayStrip(c *ui.Context, r *unslopReplay) {
	ui.Row(c.Key("steps")).FillWidth().Gap(theme.S1).Wrap().AlignItems(ui.Center).Children(func() {
		if widget.Button(c.Key("play"), widget.Secondary, icon.Play, "Play", "").Clicked() {
			r.compare = -1
			a.replayEval("__story.play()")
		}
		label := "Compare"
		if r.picking {
			label = "Pick a step…"
		}
		if widget.Button(c.Key("compare"), widget.Secondary, icon.Columns, label, "").Tooltip("Compare the step shown with another, a slider between them").Clicked() {
			r.picking = !r.picking
		}
		names := []string{"Original"}
		for _, s := range r.story.Steps {
			names = append(names, unslop.StoryName(s))
		}
		for k, name := range names {
			on := k == r.step || k == r.compare
			label := fmt.Sprintf("%d %s", k, name)
			if k == 0 {
				label = name
			}
			b := widget.Button(c.Key(k), widget.Secondary, nil, label, "")
			if on {
				b.Background(theme.Selected)
			}
			if b.Clicked() {
				switch {
				case r.picking && k != r.step:
					r.compare, r.picking = k, false
					a.replayEval(fmt.Sprintf("__story.stop();__story.compare(%d,%d)", min(r.step, k), max(r.step, k)))
				default:
					r.step, r.compare = k, -1
					a.replayEval(fmt.Sprintf("__story.stop();__story.load(%d)", k))
				}
			}
		}
	})
}

// replaySide is the step shown's changes, leaving it out, and the
// export.
func (a *App) replaySide(c *ui.Context, r *unslopReplay) {
	ui.Column(c.Key("side")).Width(280).Shrink(0).Gap(theme.S2).Children(func() {
		if r.step == 0 {
			ui.Text(c.Key("original"), "The design the redesign started from.").FontSize(theme.TextUI).TextColor(theme.TextSoft)
		} else {
			s := r.story.Steps[r.step-1]
			widget.Running(c.Key("step"), fmt.Sprintf("Step %d · %s", r.step, unslop.StoryName(s)), widget.RunStyle{Weight: 500})
			ui.Scroll(c.Key("changes")).FillWidth().MaxHeight(330).Children(func() {
				ui.Column(c).FillWidth().Gap(theme.S2).Children(func() {
					if !s.Measured {
						ui.Text(c, "Not measured: the design is measured as rendered, by an app connected when the step ended.").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
					}
					for i, ch := range s.Changes {
						ui.Column(c.Key(i)).FillWidth().Gap(theme.S1).Padding(theme.S2, theme.S3).Radius(theme.RadiusCard).Background(theme.Sunken).Children(func() {
							widget.Running(c.Key("el"), "#"+ch.Element, widget.RunStyle{Size: theme.TextLabel})
							switch {
							case ch.Added:
								ui.Text(c, "added").FontSize(theme.TextLabel).TextColor(theme.Done)
							case ch.Removed:
								ui.Text(c, "removed").FontSize(theme.TextLabel).TextColor(theme.Error)
							}
							for _, p := range ch.Props {
								ui.Text(c, p.Name+"  "+p.From+" → "+p.To).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.TextSoft).MaxLines(2)
							}
						})
					}
				})
			})
			if widget.Button(c.Key("leave"), widget.Secondary, nil, "Leave this step out", "").Disabled(r.busy != "").
				Tooltip("A copy of the design with what this step changed put back, unless changed again since").Clicked() {
				a.leaveStepOut(r, r.step)
			}
		}
		if r.busy != "" {
			ui.Text(c.Key("busy"), r.busy).FontSize(theme.TextUI).TextColor(theme.TextSoft)
		}
		a.videoButtons(c, r)
	})
}

// leaveStepOut makes the copy without step k and shows it.
func (a *App) leaveStepOut(r *unslopReplay, k int) {
	cl := a.client
	if cl == nil {
		return
	}
	r.busy = "Making a copy without the step…"
	id, frame := r.canvas, r.frame
	go func() {
		out, err := cl.UnslopLeaveOut(id, frame, k)
		a.onMain(func() {
			r.busy = ""
			if err != nil {
				r.err = strings.TrimPrefix(client.Explain(err), "canvas: ")
				return
			}
			msg := fmt.Sprintf("Made a design without step %d: %d put back", k, len(out.Back))
			if len(out.Kept) > 0 {
				msg += fmt.Sprintf(", %d kept as changed again since (%s)", len(out.Kept), strings.Join(out.Kept, ", "))
			}
			a.notify(msg)
			a.closeReplay()
			a.canvases.Goto(a.canvasOptions(), id, out.Frame)
		})
	}()
}
