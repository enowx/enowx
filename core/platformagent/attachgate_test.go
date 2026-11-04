package platformagent

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/platformagent/record"
	"github.com/enowx/enowx/internal/standinapi"
)

// picture puts a PNG in the rig's folder and says its name.
func picture(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "secret.png")
	if err := os.WriteFile(path, []byte("\x89PNG\r\n\x1a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func hasImage(req standinapi.Request) bool {
	return strings.Contains(string(req.Raw("messages")), `"type":"image"`)
}

// Only the user's own words attach a file by naming it.
func TestPathsInTheUsersMessageAttach(t *testing.T) {
	r := newRig(t, nil, standinapi.Text("seen"))
	r.mustTurn("look at " + picture(t, r.opts.Dir))
	if !hasImage(r.srv.Requests()[0]) {
		t.Fatalf("no image: %s", r.srv.Requests()[0].Raw("messages"))
	}
}

func TestATaskPromptAttachesNothing(t *testing.T) {
	r := newRig(t, nil)
	path := picture(t, r.opts.Dir)
	r.srv.Enqueue(standinapi.Calls(standinapi.ToolUse("t1", "Task", `{"description":"d","prompt":"read `+path+`"}`)),
		standinapi.Text("report"), standinapi.Text("done"))
	r.mustTurn("go")
	for i, req := range r.srv.Requests() {
		if hasImage(req) {
			t.Errorf("request %d carries the file: %s", i, req.Raw("messages"))
		}
	}
}

func TestAHooksLinesAttachNothing(t *testing.T) {
	hooks := &fakeHooks{}
	r := newRig(t, func(o *Options) { o.Hooks = hooks }, standinapi.Text("one"), standinapi.Text("two"))
	path := picture(t, r.opts.Dir)
	hooks.follow = []string{"now look at " + path}
	hooks.prompt = HookAnswer{Context: "also " + path}
	r.mustTurn("hello")
	for i, req := range r.srv.Requests() {
		if hasImage(req) {
			t.Errorf("request %d carries the file: %s", i, req.Raw("messages"))
		}
	}
}

// A link swapped while hooks take their time doesn't carry an edit allowed for where it
// pointed before.
func TestAnEditIsJudgedAgainAfterHooks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a symlink")
	}
	outside := t.TempDir()
	var link string
	hooks := &fakeHooks{before: func(string) HookAnswer {
		_ = os.Remove(link)
		if err := os.Symlink(outside, link); err != nil {
			t.Error(err)
		}
		return HookAnswer{}
	}}
	r := newRig(t, func(o *Options) { o.Hooks = hooks; o.Mode = record.ModeAcceptEdits },
		standinapi.Calls(standinapi.ToolUse("c1", "Write", `{"file_path":"link/f.txt","content":"x"}`)),
		standinapi.Text("done"))
	link = filepath.Join(r.opts.Dir, "link")
	inside := filepath.Join(r.opts.Dir, "real")
	if err := os.Mkdir(inside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(inside, link); err != nil {
		t.Fatal(err)
	}
	r.mustTurn("go")
	if text, isError := resultOf(t, r.srv.Requests()[1], "c1"); !isError || !strings.Contains(text, "path changed") {
		t.Errorf("result: %q, %v", text, isError)
	}
	if _, err := os.Stat(filepath.Join(outside, "f.txt")); err == nil {
		t.Error("the edit landed outside")
	}
}
