package terminal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/platformagent"
	"github.com/enowx/enowx/core/platformagent/record"
)

func TestMessageTypedDuringATurnStartsAfterIt(t *testing.T) {
	release := make(chan struct{})
	s := start(t, nil)
	s.r.setTurn(func(ctx context.Context, text string) error {
		if text == "first" {
			<-release
		}
		return nil
	})
	s.press("first\r")
	s.waitOut("» first")
	s.press("second\r")
	s.waitOut("Queued: second")
	if got := s.r.turnsSeen(); !reflect.DeepEqual(got, []string{"first"}) {
		t.Fatalf("turns while the first runs: %v", got)
	}
	close(release)
	waitFor(t, "the queued turn", func() bool { return len(s.r.turnsSeen()) == 2 }, s.out.String)
	if got := s.r.turnsSeen(); got[1] != "second" {
		t.Fatalf("turns: %v", got)
	}
}

func TestPasteIsOneMessage(t *testing.T) {
	s := start(t, nil)
	s.press("\x1b[200~line one\nline two\x1b[201~")
	s.settle()
	s.press("\r")
	waitFor(t, "a turn", func() bool { return len(s.r.turnsSeen()) == 1 }, s.out.String)
	if got := s.r.turnsSeen()[0]; got != "line one\nline two" {
		t.Fatalf("message %q", got)
	}
}

func TestPermissionKeys(t *testing.T) {
	for _, c := range []struct {
		keys string
		want platformagent.Decision
	}{{"1", platformagent.AllowOnce}, {"2", platformagent.AllowAlways}, {"3", platformagent.Deny}, {"\x1b", platformagent.Deny}} {
		s := start(t, nil)
		got := make(chan platformagent.Decision, 1)
		s.r.setTurn(func(ctx context.Context, text string) error {
			d, err := s.term.Permission(ctx, platformagent.PermissionRequest{Tool: "Bash", Detail: "git status", Rule: "Bash(git *)"})
			if err != nil {
				t.Error(err)
			}
			got <- d
			return nil
		})
		s.press("go\r")
		s.waitOut("1 allow  2 always allow  3 deny")
		s.press(c.keys)
		select {
		case d := <-got:
			if d != c.want {
				t.Errorf("%q: decision %v, want %v", c.keys, d, c.want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%q: no answer; output:\n%s", c.keys, s.out.String())
		}
		for _, want := range []string{"Bash", "git status", "Bash(git *)"} {
			if !strings.Contains(s.out.String(), want) {
				t.Errorf("the prompt doesn't show %q:\n%s", want, s.out.String())
			}
		}
	}
}

// A message on its way doesn't answer for the user, and a key answers once the line is
// clear.
func TestTextTypedDoesNotAnswerAPermission(t *testing.T) {
	s := start(t, nil)
	got := make(chan platformagent.Decision, 1)
	s.r.setTurn(func(ctx context.Context, text string) error {
		if text != "go" {
			return nil
		}
		d, _ := s.term.Permission(ctx, platformagent.PermissionRequest{Tool: "Bash"})
		got <- d
		return nil
	})
	s.press("go\r")
	s.waitOut("1 allow")
	s.press("1. look at this")
	s.settle()
	s.press("\r")
	s.waitOut("Queued: 1. look at this")
	select {
	case d := <-got:
		t.Fatalf("answered %v by a message", d)
	case <-time.After(30 * time.Millisecond):
	}
	s.press("x")
	s.settle()
	s.press("1") // typing goes on, the line isn't clear
	s.settle()
	select {
	case d := <-got:
		t.Fatalf("answered %v while typing", d)
	case <-time.After(30 * time.Millisecond):
	}
	s.press("\x1b") // clears the line
	s.settle()
	s.press("1")
	select {
	case d := <-got:
		if d != platformagent.AllowOnce {
			t.Fatalf("decision %v", d)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("no answer:\n%s", s.out.String())
	}
}

func TestEndOfTheContextEndsAPermission(t *testing.T) {
	s := start(t, nil)
	got := make(chan error, 1)
	s.r.setTurn(func(ctx context.Context, text string) error {
		_, err := s.term.Permission(ctx, platformagent.PermissionRequest{Tool: "Bash"})
		got <- err
		return err
	})
	s.press("go\r")
	s.waitOut("1 allow")
	s.press("\x03") // Ctrl+C stops the turn, whose context the ask waits on
	select {
	case err := <-got:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("no end:\n%s", s.out.String())
	}
	s.waitOut("Interrupted")
}

// The keys here are what ui/chatview/ask.go presses: an option's number alone, or the
// number after the last then the words and CR, in one write.
func TestQuestionKeys(t *testing.T) {
	three := record.Question{Question: "Which?", Options: []string{"Red", "Green", "Blue"}}
	var twelve record.Question
	twelve.Question = "Which of many?"
	for i := range 12 {
		twelve.Options = append(twelve.Options, "o"+string(rune('a'+i)))
	}
	for _, c := range []struct {
		name  string
		q     record.Question
		keys  []string
		want  string
		fails error
	}{
		{name: "option", q: three, keys: []string{"2"}, want: "Green"},
		{name: "own words, as the chat sends", q: three, keys: []string{"4my own\r"}, want: "my own"},
		{name: "own words, typed", q: three, keys: []string{"4", "m", "y", "\r"}, want: "my"},
		{name: "own words without the number", q: three, keys: []string{"pur", "ple\r"}, want: "purple"},
		{name: "text starting with an option's number", q: three, keys: []string{"1 of them\r"}, want: "1 of them"},
		{name: "no options, as the chat sends", q: record.Question{Question: "Name?"}, keys: []string{"1Ada\r"}, want: "Ada"},
		{name: "two digits in one write", q: twelve, keys: []string{"10"}, want: "oj"},
		{name: "one digit of many", q: twelve, keys: []string{"1"}, want: "oa"},
		{name: "own words after many", q: twelve, keys: []string{"13words\r"}, want: "words"},
		{name: "several", q: record.Question{Question: "Which?", Options: three.Options, Multi: true}, keys: []string{"1, 3\r"}, want: "Red, Blue"},
		{name: "several with spaces", q: record.Question{Question: "Which?", Options: three.Options, Multi: true}, keys: []string{"2 3", "\r"}, want: "Green, Blue"},
		{name: "several in words", q: record.Question{Question: "Which?", Options: three.Options, Multi: true}, keys: []string{"none\r"}, want: "none"},
		{name: "declined", q: three, keys: []string{"\x1b"}, fails: ErrDeclined},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := start(t, nil)
			type answer struct {
				a   []string
				err error
			}
			got := make(chan answer, 1)
			s.r.setTurn(func(ctx context.Context, text string) error {
				a, err := s.term.Questions(ctx, []record.Question{c.q})
				got <- answer{a, err}
				return nil
			})
			s.press("go\r")
			s.waitOut(c.q.Question)
			for _, k := range c.keys {
				s.press(k)
				s.settle()
			}
			select {
			case a := <-got:
				if c.fails != nil {
					if !errors.Is(a.err, c.fails) {
						t.Fatalf("error %v", a.err)
					}
					return
				}
				if a.err != nil || !reflect.DeepEqual(a.a, []string{c.want}) {
					t.Fatalf("answers %q, %v, want %q", a.a, a.err, c.want)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("no answer:\n%s", s.out.String())
			}
		})
	}
}

func TestQuestionsGoOneAfterAnother(t *testing.T) {
	s := start(t, nil)
	got := make(chan []string, 1)
	s.r.setTurn(func(ctx context.Context, text string) error {
		a, _ := s.term.Questions(ctx, []record.Question{
			{Question: "First?", Options: []string{"a", "b"}},
			{Question: "Second?", Options: []string{"c", "d"}},
		})
		got <- a
		return nil
	})
	s.press("draft")
	s.settle()
	s.press("\x1b") // clear: nothing queued yet
	s.settle()
	s.press("go\r")
	s.waitOut("First? (1 of 2)")
	s.press("2")
	s.waitOut("Second? (2 of 2)")
	s.press("1")
	select {
	case a := <-got:
		if !reflect.DeepEqual(a, []string{"b", "c"}) {
			t.Fatalf("answers %v", a)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("no answer:\n%s", s.out.String())
	}
}

func TestOverBudget(t *testing.T) {
	for keys, want := range map[string]bool{"y": true, "n": false, "\x1b": false} {
		s := start(t, nil)
		got := make(chan bool, 1)
		s.r.setTurn(func(ctx context.Context, text string) error {
			ok, _ := s.term.OverBudget(ctx, 12.5, 10)
			got <- ok
			return nil
		})
		s.press("go\r")
		s.waitOut("Spent $12.50 of a $10.00 budget")
		s.press(keys)
		select {
		case ok := <-got:
			if ok != want {
				t.Errorf("%q: %v", keys, ok)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%q: no answer", keys)
		}
	}
}

func TestEscInterruptsATurnOrClearsTheLine(t *testing.T) {
	s := start(t, nil)
	s.r.setTurn(func(ctx context.Context, text string) error {
		<-ctx.Done()
		return ctx.Err()
	})
	s.press("kept for nothing")
	s.settle()
	s.press("\x1b")
	s.settle()
	s.press("run\r")
	s.waitOut("» run") // not "kept for nothing run"
	s.press("\x1b")
	s.waitOut("Interrupted")
	waitFor(t, "ready", func() bool { return strings.Contains(s.out.String(), "─ ask first mode on") }, s.out.String)
}

func TestLeaving(t *testing.T) {
	now := time.Unix(1000, 0)
	for name, keys := range map[string][]string{
		"ctrl+d on an empty line": {"\x04"},
		"ctrl+c twice":            {"\x03", "\x03"},
		"quit":                    {"/quit\r"},
	} {
		t.Run(name, func(t *testing.T) {
			s := start(t, func(c *Config, _ *fakeRunner) { c.Now = func() time.Time { return now } })
			for _, k := range keys {
				s.press(k)
				s.settle()
			}
			s.waitEnded()
			if s.r.closed != 1 {
				t.Fatalf("closed %d times", s.r.closed)
			}
		})
	}
}

func TestCtrlCOnceWithALineOnlyClearsIt(t *testing.T) {
	now := time.Unix(1000, 0)
	s := start(t, func(c *Config, _ *fakeRunner) { c.Now = func() time.Time { return now } })
	s.press("typed")
	s.settle()
	s.press("\x03")
	s.settle()
	s.press("\x03") // the first press of an empty line
	s.waitOut("Press Ctrl+C again")
	select {
	case <-s.ended:
		t.Fatal("left")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestCtrlCStopsTheTurnAndForgetsTheQueue(t *testing.T) {
	s := start(t, nil)
	s.r.setTurn(func(ctx context.Context, text string) error {
		<-ctx.Done()
		return ctx.Err()
	})
	s.press("first\r")
	s.waitOut("» first")
	s.press("second\r")
	s.waitOut("Queued")
	s.press("\x03")
	s.waitOut("Interrupted")
	s.waitOut("─ ask first mode on")
	if got := s.r.turnsSeen(); len(got) != 1 {
		t.Fatalf("turns %v", got)
	}
}

func TestShiftTabCyclesModes(t *testing.T) {
	cycle := func(allow bool, want []string) {
		t.Helper()
		s := start(t, func(c *Config, _ *fakeRunner) { c.AllowBypass = allow })
		for _, shown := range want {
			at := len(s.out.String())
			s.press("\x1b[Z")
			waitFor(t, shown, func() bool { return strings.Contains(s.out.String()[at:], shown) }, s.out.String)
		}
	}
	cycle(false, []string{"accept edits on", "plan mode on", "ask first mode on", "accept edits on"})
	cycle(true, []string{"accept edits on", "plan mode on", "bypass permissions on", "ask first mode on"})
}

func TestSlashCommands(t *testing.T) {
	s := start(t, func(_ *Config, r *fakeRunner) { r.cost = 1.5 })
	for _, c := range []struct{ line, says string }{
		{"/help", "/compact"},
		{"/model claude-other", "Model: claude-other"},
		{"/effort max", "Effort: max"},
		{"/mode plan", "plan mode on"},
		{"/mode bypassPermissions", "/mode: bypassPermissions needs"},
		{"/mode nothing", `unknown mode "nothing"`},
		{"/cost", "Spent $1.50 of a $10.00 budget"},
		{"/budget 25", "Budget: $25.00"},
		{"/budget lots", `"lots" isn't a number`},
		{"/context", "50000 of 200000 tokens (25%)"},
		{"/nonesuch now", "Unknown command /nonesuch"},
		{"/system-prompt /some/file", "can't change it while it runs"},
	} {
		at := len(s.out.String())
		s.press(c.line + "\r")
		waitFor(t, c.line+" saying "+c.says, func() bool { return strings.Contains(s.out.String()[at:], c.says) }, s.out.String)
	}
	if got := s.r.turnsSeen(); len(got) != 0 {
		t.Fatalf("commands reached the model: %v", got)
	}
	if m, e, _, b := s.r.Settings(); m != "claude-other" || e != "max" || b != 25 {
		t.Fatalf("settings %s %s %v", m, e, b)
	}

	s.r.mu.Lock()
	s.r.failModel = errors.New("no such model")
	s.r.mu.Unlock()
	s.press("/model nope\r")
	s.waitOut("/model: no such model")
}

// A path at a line's start, as the composer sends an attached file, is a message.
func TestPathIsAMessage(t *testing.T) {
	if _, _, ok := parseSlash("/home/me/shot.txt what's this"); ok {
		t.Fatal("read as a command")
	}
	if name, arg, ok := parseSlash("/model  claude-x"); !ok || name != "model" || arg != "claude-x" {
		t.Fatalf("%q %q %v", name, arg, ok)
	}
}

func TestClearAsksFirst(t *testing.T) {
	s := start(t, nil)
	s.press("/clear\r")
	s.waitOut("Do you want to continue? (y/n)")
	s.press("n")
	s.waitOut("Kept the conversation")
	if s.r.cleared != 0 {
		t.Fatal("cleared on no")
	}
	s.press("/clear\r")
	waitFor(t, "a second ask", func() bool { return strings.Count(s.out.String(), "(y/n)") == 2 }, s.out.String)
	s.press("y")
	s.waitOut("New conversation")
	s.r.mu.Lock()
	defer s.r.mu.Unlock()
	if s.r.cleared != 1 {
		t.Fatalf("cleared %d", s.r.cleared)
	}
}

func TestClearRefusedDuringATurn(t *testing.T) {
	s := start(t, nil)
	s.r.setTurn(func(ctx context.Context, text string) error {
		<-ctx.Done()
		return nil
	})
	s.press("go\r")
	s.waitOut("» go")
	s.press("/clear\r")
	s.waitOut("/clear: wait for the turn")
}

func TestCompactRunsAsAJob(t *testing.T) {
	release := make(chan struct{})
	s := start(t, nil)
	s.r.onCompact = func(ctx context.Context) error { <-release; return errors.New("too short") }
	s.press("/compact\r")
	s.waitOut("Compacting")
	s.press("after\r")
	s.waitOut("Queued: after")
	close(release)
	s.waitOut("compaction failed: too short") // what enowX reads (agent.CompactFailed)
	waitFor(t, "the queued turn", func() bool { return len(s.r.turnsSeen()) == 1 }, s.out.String)
}

func TestSystemPromptCommand(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prompt.md")
	if err := os.WriteFile(path, []byte("be brief"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &settingPrompt{fakeRunner: &fakeRunner{mode: record.ModeDefault}}
	pr, pw := pipe(t)
	out := &lockedBuffer{}
	term := New(Config{In: pr, Out: out, EscWait: time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- term.Run(ctx, r) }()
	if _, err := pw.Write([]byte("/system-prompt " + path + "\r")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the prompt set", func() bool { return r.prompt() == "be brief" }, out.String)
	cancel()
	<-done
}

type settingPrompt struct {
	*fakeRunner
	text string
}

func (s *settingPrompt) SetSystemPrompt(text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.text = text
	return nil
}

func (s *settingPrompt) prompt() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.text
}

func TestPipedInputFinishesItsWorkThenLeaves(t *testing.T) {
	var got []string
	s := start(t, func(c *Config, r *fakeRunner) { c.FirstPrompt = "from the flag" })
	s.waitOut("» from the flag")
	s.press("from the pipe")
	s.settle()
	s.in.Close()
	s.waitEnded()
	got = s.r.turnsSeen()
	if !reflect.DeepEqual(got, []string{"from the flag", "from the pipe"}) {
		t.Fatalf("turns %v", got)
	}
}

func TestAPermissionWithNoInputLeftIsDenied(t *testing.T) {
	got := make(chan platformagent.Decision, 1)
	s := start(t, func(c *Config, r *fakeRunner) {
		r.onTurn = func(ctx context.Context, text string) error {
			return nil
		}
	})
	s.r.setTurn(func(ctx context.Context, text string) error {
		d, err := s.term.Permission(ctx, platformagent.PermissionRequest{Tool: "Bash"})
		if err != nil {
			t.Error(err)
		}
		got <- d
		return nil
	})
	s.press("go\r")
	s.waitOut("1 allow")
	s.in.Close()
	select {
	case d := <-got:
		if d != platformagent.Deny {
			t.Fatalf("decision %v", d)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no answer")
	}
	s.waitEnded()
}

func TestATurnsPanicReachesRun(t *testing.T) {
	pr, pw := pipe(t)
	defer pw.Close()
	term := New(Config{In: pr, Out: &lockedBuffer{}, FirstPrompt: "go"})
	r := &fakeRunner{mode: record.ModeDefault, onTurn: func(context.Context, string) error { panic("boom") }}
	got := make(chan any, 1)
	go func() {
		defer func() { got <- recover() }()
		_ = term.Run(context.Background(), r)
	}()
	select {
	case v := <-got:
		if v != "boom" {
			t.Fatalf("recovered %v", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no panic")
	}
}

func TestAttachmentsAreTakenFromTheMessagesEnd(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "a.png")
	doc := filepath.Join(dir, "b.txt")
	for _, p := range []string{img, doc} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		in, text string
		files    []string
	}{
		{"look at " + img, "look at", []string{img}},
		{img, img, []string{img}},
		{"see " + doc, "see " + doc, nil},
		{"see " + filepath.Join(dir, "gone.png"), "see " + filepath.Join(dir, "gone.png"), nil},
		{img + " is it", img + " is it", nil},
	} {
		text, files := takeAttachments(c.in)
		if text != c.text || !reflect.DeepEqual(files, c.files) {
			t.Errorf("%q: %q, %v", c.in, text, files)
		}
	}
}

// Events draw as the user reads them, and what would move the terminal never reaches it.
func TestEventsDraw(t *testing.T) {
	s := start(t, nil)
	s.r.setTurn(func(ctx context.Context, text string) error {
		f := s.term
		f.Event(record.Record{Type: record.BlockStart, Msg: "m", Kind: "thinking"})
		f.Event(record.Record{Type: record.Delta, Msg: "m", Kind: "thinking", Text: "long\nthought"})
		f.Event(record.Record{Type: record.Delta, Msg: "m", Kind: "text", Text: "Hello \x1b[2Jworld\n"})
		f.Event(record.Record{Type: record.Assistant, Msg: "m"})
		f.Event(record.Record{Type: record.ToolStart, Tool: &record.Tool{ID: "1", Name: "Bash", Detail: "git\nstatus"}})
		f.Event(record.Record{Type: record.ToolEnd, Tool: &record.Tool{ID: "1", Name: "Bash", Result: "ok\nmore", DurationMs: 7}})
		f.Event(record.Record{Type: record.Notice, Level: "error", Text: "bad thing"})
		f.Event(record.Record{Type: record.Assistant, Msg: "whole", Message: []byte(`{"content":[{"type":"text","text":"not streamed"}]}`)})
		return nil
	})
	s.press("go\r")
	s.waitOut("─ ask first mode on · claude-test · high · ctx 25% · $0.00")
	out := s.out.String()
	for _, want := range []string{"✻ thinking: long thought", "Hello [2Jworld", "● Bash git status", "✓ Bash 7ms: ok", "! bad thing", "not streamed"} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\x1b") {
		t.Errorf("an escape sequence in piped output: %q", out)
	}
}

// interactive runs a terminal with a live region at a width, what it wrote read as a
// screen.
func interactive(t *testing.T, cols int, r func(*fakeRunner)) (*session, func() string) {
	t.Helper()
	s := start(t, func(c *Config, f *fakeRunner) {
		c.Interactive = true
		c.Cols = func() int { return cols }
		if r != nil {
			r(f)
		}
	})
	return s, func() string {
		v := newVT(cols)
		v.feed(s.out.String())
		return v.text()
	}
}

var modes = agent.Settings{Modes: []agent.Choice{
	{ID: "default", Shown: "ask first mode on"},
	{ID: "acceptEdits", Shown: "accept edits on"},
	{ID: "plan", Shown: "plan mode on"},
	{ID: "bypassPermissions", Shown: "bypass permissions on"},
}}

// The screen is what enowX reads a session's state from: ready, working, asking, and
// the mode.
func TestEnowXReadsTheScreen(t *testing.T) {
	release := make(chan struct{})
	s, screen := interactive(t, 100, nil)
	waitFor(t, "the prompt", func() bool { return strings.HasSuffix(screen(), "❯") }, screen)
	if !agent.TurnOverShown(screen()) {
		t.Errorf("a ready screen doesn't read as ready:\n%s", screen())
	}
	if got := modes.ModeShown(screen()); got != "default" {
		t.Errorf("mode %q:\n%s", got, screen())
	}

	s.r.setTurn(func(ctx context.Context, text string) error {
		// A model's words can't say what mode is on: the status is under them.
		s.term.Event(record.Record{Type: record.Delta, Msg: "m", Kind: "text", Text: "turning plan mode on\nand on"})
		<-release
		_, _ = s.term.Permission(ctx, platformagent.PermissionRequest{Tool: "Bash", Detail: "ls"})
		return nil
	})
	s.press("go\r")
	waitFor(t, "the turn", func() bool { return strings.Contains(screen(), "and on") }, screen)
	if agent.TurnOverShown(screen()) {
		t.Errorf("a working screen reads as ready:\n%s", screen())
	}
	if got := modes.ModeShown(screen()); got != "default" {
		t.Errorf("mode %q:\n%s", got, screen())
	}
	if !strings.Contains(screen(), "esc to interrupt") {
		t.Errorf("no sign of work:\n%s", screen())
	}
	s.press("\x1b[Z")
	waitFor(t, "the mode", func() bool { return modes.ModeShown(screen()) == "acceptEdits" }, screen)

	close(release)
	waitFor(t, "the ask", func() bool { return strings.Contains(screen(), "1 allow") }, screen)
	p, ok := agent.PromptShown(screen())
	if !ok || !agent.PromptAsksPermission(p.Text) {
		t.Errorf("the prompt isn't read: %+v\n%s", p, screen())
	}
	s.press("3")
	waitFor(t, "ready again", func() bool { return strings.HasSuffix(screen(), "❯") }, screen)
}

// At any width the live region is redrawn in place: nothing it drew is left behind.
func TestTheLiveRegionLeavesNothingBehind(t *testing.T) {
	for _, cols := range []int{12, 23, 80} {
		s, screen := interactive(t, cols, nil)
		release := make(chan struct{})
		s.r.setTurn(func(ctx context.Context, text string) error {
			for _, w := range strings.Fields(strings.Repeat("words go up a row at a time ", 8)) {
				s.term.Event(record.Record{Type: record.Delta, Msg: "m", Kind: "text", Text: w + " "})
			}
			<-release
			return nil
		})
		s.press("go\r")
		waitFor(t, "the text", func() bool { return strings.Contains(screen(), "words") }, screen)
		s.press(strings.Repeat("typing ", 12))
		s.settle()
		s.press("\x1b[D\x1b[D\x1b[D\x7f")
		s.settle()
		close(release)
		waitFor(t, "ready", func() bool { return strings.Count(s.out.String(), "─ ") > 0 }, screen)
		s.settle()
		got := screen()
		if strings.Contains(got, "esc to") {
			t.Errorf("%d columns: a working status left behind:\n%s", cols, got)
		}
		// One live status: the turn's end line and the footer are the same words at most twice.
		if n := strings.Count(strings.ReplaceAll(got, "\n", ""), "ask first"); n > 2 {
			t.Errorf("%d columns: %d statuses:\n%s", cols, n, got)
		}
		if !strings.Contains(strings.Join(strings.Fields(got), ""), strings.Repeat("wordsgoupa"+"rowatatime", 8)) {
			t.Errorf("%d columns: the text is gone:\n%s", cols, got)
		}
	}
}

func TestNarrowLineKeepsTheCursorOnIt(t *testing.T) {
	s, screen := interactive(t, 20, nil)
	waitFor(t, "the prompt", func() bool { return strings.HasSuffix(screen(), "❯") }, screen)
	long := strings.Repeat("x", 18) + "y" // the marker takes the row's other columns
	s.press(long)
	waitFor(t, "the line", func() bool { return strings.Contains(screen(), "y") }, screen)
	s.settle()
	s.press("\x7f")
	waitFor(t, "the line shorter", func() bool { return !strings.Contains(screen(), "y") }, screen)
	s.press("\x1b[H")
	s.settle()
	s.press("a")
	waitFor(t, "text at the start", func() bool { return strings.Contains(screen(), "❯ ax") }, screen)
}
