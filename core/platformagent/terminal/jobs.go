package terminal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// job is one thing at a time the agent does: a turn, or compacting.
type job struct {
	text    string // a turn's message, as typed
	compact bool
	cancel  context.CancelFunc
	started time.Time
}

// submit takes a line sent: a command of the terminal's, or a message.
func (t *Terminal) submit(text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	if name, arg, ok := parseSlash(text); ok {
		t.slash(name, arg)
		return
	}
	t.enqueue(job{text: text})
}

// enqueue starts j, or keeps it for after what runs.
func (t *Terminal) enqueue(j job) {
	if t.job == nil {
		t.start(j)
		return
	}
	t.queue = append(t.queue, j)
	what := "/compact"
	if !j.compact {
		what = oneLine(j.text)
	}
	t.sc.Line(t.sc.paint(styleDim, clipCols("Queued: "+what, t.sc.width()-1)))
}

// start runs j on a goroutine of its own, Run's goroutine going on reading keys.
func (t *Terminal) start(j job) {
	ctx, cancel := context.WithCancel(t.ctx)
	j.cancel = cancel
	j.started = t.cfg.Now()
	t.job = &j
	t.stopped, t.saidStop = false, false
	r := t.r
	if j.compact {
		t.sc.Line(t.sc.paint(styleDim, "Compacting"))
	} else {
		t.sc.Line(t.sc.paint(styleBold, "» ") + cleanText(j.text))
	}
	go func() {
		defer func() {
			if v := recover(); v != nil {
				t.post(func() { panic(v) })
			}
		}()
		var err error
		if j.compact {
			err = r.Compact(ctx)
		} else {
			text, files := takeAttachments(j.text)
			err = r.Turn(ctx, text, files)
		}
		cancel()
		t.post(func() { t.jobEnded(err) })
	}()
}

// stopJob tells what runs to stop.
func (t *Terminal) stopJob() {
	if t.job != nil {
		t.stopped = true
		t.job.cancel()
	}
}

// jobEnded goes on after a job, with the next queued.
func (t *Terminal) jobEnded(err error) {
	j := t.job
	t.job = nil
	t.endThinking()
	t.sc.EndStream()
	switch {
	case t.stopped || errors.Is(err, context.Canceled):
		if !t.saidStop {
			t.sc.Line(t.sc.paint(styleYellow, "Interrupted"))
		}
	case err != nil && j.compact:
		t.sc.Line(t.sc.paint(styleRed, "! compaction failed: "+oneLine(err.Error())))
	case err != nil:
		t.sc.Line(t.sc.paint(styleRed, "! "+oneLine(err.Error())))
	}
	t.refresh()
	if !j.compact {
		t.sc.Line(t.sc.paint(styleDim, "─ "+t.statusLine()))
	}
	if t.exiting {
		t.finished = true
		return
	}
	if len(t.queue) > 0 {
		next := t.queue[0]
		t.queue = t.queue[1:]
		t.start(next)
		return
	}
	if t.eof {
		t.quit()
	}
}

// attachable are the files an agent sends along as they are, rather than reading.
var attachable = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".pdf": true}

// takeAttachments splits off a message's end the paths of existing images and PDFs, as
// the app's composer puts files there. Another file stays in the text, for a tool to
// read; a message of nothing else keeps its text as well, for the model to have words.
func takeAttachments(text string) (string, []string) {
	fields := strings.Fields(text)
	var files []string
	end := len(text)
	for i := len(fields) - 1; i >= 0; i-- {
		f := fields[i]
		if !filepath.IsAbs(f) || !attachable[strings.ToLower(filepath.Ext(f))] {
			break
		}
		if st, err := os.Stat(f); err != nil || !st.Mode().IsRegular() {
			break
		}
		at := strings.LastIndex(text[:end], f)
		if at < 0 {
			break
		}
		end = at
		files = append([]string{f}, files...)
	}
	left := strings.TrimRight(text[:end], " \t\n")
	if left == "" {
		return text, files
	}
	return left, files
}

// settingsView is what the status says, read from the Runner.
type settingsView struct {
	model, effort, mode string
	budget, cost        float64
	tokens, window      int64
}

// refresh reads the settings and counts anew.
func (t *Terminal) refresh() {
	v := &t.view
	v.model, v.effort, v.mode, v.budget = t.r.Settings()
	v.cost = t.r.Cost()
	v.tokens, v.window = t.r.Context()
}

// statusLine says the settings: the mode first, narrow screens clipping the rest. The
// mode reads exactly as the agent's Settings say (agent.Choice.Shown), which is how
// enowX knows and switches it.
func (t *Terminal) statusLine() string {
	v := t.view
	parts := []string{modeShown(v.mode)}
	if v.model != "" {
		parts = append(parts, v.model)
	}
	if v.effort != "" {
		parts = append(parts, v.effort)
	}
	if v.window > 0 {
		parts = append(parts, fmt.Sprintf("ctx %d%%", v.tokens*100/v.window))
	}
	parts = append(parts, fmt.Sprintf("$%.2f", v.cost))
	return strings.Join(parts, " · ")
}

// footerText is the status line that stays under the conversation. "esc to interrupt"
// is what enowX reads as an agent at work (agent.PromptShown), so it goes away while
// the agent waits on an answer. The seconds counting are what keeps a session's screen
// moving through a long command or a slow answer: a screen still for long reads as
// stopped (session.staleWorking).
func (t *Terminal) footerText() string {
	if !t.cfg.Interactive {
		return ""
	}
	s := t.statusLine()
	if t.job != nil && t.currentAsk() == nil {
		elapsed := int(t.cfg.Now().Sub(t.job.started).Seconds())
		s = fmt.Sprintf("%s · esc to interrupt · %ds%s", modeShown(t.view.mode), elapsed, strings.TrimPrefix(s, modeShown(t.view.mode)))
	}
	if n := len(t.queue); n > 0 {
		s += fmt.Sprintf(" · %d queued", n)
	}
	return s
}
