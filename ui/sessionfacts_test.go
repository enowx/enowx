package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/textfmt"
)

// A row says what the agent said, never a guess, marked as the last when it
// isn't running; what isn't known says so; a program that isn't an agent says
// nothing of models.
func TestModelLine(t *testing.T) {
	for _, tc := range []struct {
		s    proto.SessionInfo
		want string
	}{
		{proto.SessionInfo{Agent: "claude", Model: "claude-opus-5-5", Effort: "medium"}, "opus-5-5 · medium"},
		{proto.SessionInfo{Agent: "codex", Model: "gpt-6", Effort: "high", ModelLast: true}, "gpt-6 · high (last)"},
		{proto.SessionInfo{Agent: "opencode"}, "unknown model"},
		{proto.SessionInfo{Agent: "shell"}, ""},
	} {
		if got, _ := modelLine(tc.s); got != tc.want {
			t.Errorf("%+v: %q", tc.s, got)
		}
	}
}

// A task keeps what it ran on over what it asked for, its last change winning.
func TestTaskRanOn(t *testing.T) {
	task := proto.GOATask{Model: "asked-model", Effort: "low"}
	if got := taskRanOn(task, proto.SessionInfo{}, false); got != "asked-model · low (asked)" {
		t.Fatal(got)
	}
	if got := taskRanOn(proto.GOATask{}, proto.SessionInfo{}, false); got != "unknown" {
		t.Fatal(got)
	}
	task.Ran = []string{"claude-a · low", "claude-b · high"}
	if got := taskRanOn(task, proto.SessionInfo{}, false); got != "b · high" {
		t.Fatal(got)
	}
}

// Rows name the project by name and path: a long path cut in its middle, a
// session or task with none says so, and an old task can't say.
func TestProjectLines(t *testing.T) {
	if got := textfmt.ShortPath("/Users/me/code/a/very/long/way/down/to/the/project", 24); got != "~/code…own/to/the/project" && len([]rune(got)) > 26 {
		t.Fatalf("%q", got)
	}
	if got := textfmt.ShortPath("/home/me/app", 34); got != "~/app" {
		t.Fatalf("%q", got)
	}
	shows := func(project, dir, branch, host, want string) {
		t.Helper()
		tt := ui.NewTester(func(c *ui.Context) { projectLine(c, project, dir, branch, host) }, 400, 40)
		tt.Frame()
		if !tt.HasText(want) {
			t.Fatalf("wanted %q: %q", want, tt.Texts())
		}
	}
	shows("", "", "", "", "no project")
	shows("unknown", "", "", "", "project unknown")
	shows("/Volumes/SSD/app", "/Volumes/SSD/app", "", "vps", "app · /Volumes/SSD/app on vps")
	shows("/Users/me/app", "/tmp/wt", "fix", "", "app · /tmp/wt · fix")
}

// Calls a day: empty or "unlimited" is no limit, a number a limit.
func TestParseDailyCalls(t *testing.T) {
	for in, want := range map[string]int{"": 0, " unlimited ": 0, "1000000": 1000000, "5": 5} {
		if n, ok := parseDailyCalls(in); !ok || n != want {
			t.Errorf("%q: %d %v", in, n, ok)
		}
	}
	if _, ok := parseDailyCalls("-1"); ok {
		t.Error("a negative limit taken")
	}
}

// A session of a host not connected says why it can't open, offers the way to
// that host, and its dialog's button says Close, not Cancel.
func TestGOADetailsOfAnUnreachableHost(t *testing.T) {
	a := New(Options{})
	a.watches = map[string]*hostWatch{"h": {host: hosts.Host{ID: "h", Name: "vps"}}}
	a.goa.detail = &goaDetail{open: true, host: "vps", session: proto.SessionInfo{ID: "s1", Agent: "claude"}}
	tt := ui.NewTester(func(c *ui.Context) { a.goaDetailDialog(c) }, 800, 600)
	tt.Frame()
	tt.Frame()
	for _, want := range []string{"Go to vps", "Close"} {
		if !tt.HasText(want) {
			t.Fatalf("no %q: %q", want, tt.Texts())
		}
	}
	if tt.HasText("Cancel") || tt.HasText("Open in a tab") {
		t.Fatalf("%q", tt.Texts())
	}
}
