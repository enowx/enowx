package daemon

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/enowx/enowx/proto"
)

// judgeServer is a local model server judging as a person would: rm -rf of
// the home folder refused, tests allowed, a question on the screen waits,
// failing go test again and again loops, a long output trimmed.
func judgeServer(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "judge"}}})
			return
		}
		var in struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		q := in.Messages[len(in.Messages)-1].Content
		answer := `{"verdict":"allow","confidence":0.95,"why":"runs the tests"}`
		switch {
		case strings.Contains(q, "rm -rf ~"):
			answer = `{"verdict":"refuse","confidence":0.99,"why":"it wipes the home folder"}`
		case strings.Contains(q, "Proceed? (y/n)"):
			answer = `{"verdict":"waiting","confidence":0.9,"why":"it asks whether to proceed"}`
		case strings.Contains(q, "<steps>"):
			answer = `{"verdict":"looping","confidence":0.9,"why":"the same failing test five times"}`
		case strings.Contains(q, "<output>"):
			answer = `{"verdict":"trimmed","confidence":0.9,"why":"one test failed","text":"--- FAIL: TestTotal\nFAIL"}`
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": answer}}}})
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/v1"
}

// The judge, through the daemon: an agent's dangerous command refused, a
// test allowed without asking the user; a quiet session that asks marked
// waiting; a loop stopped; a long output trimmed; all in the log.
func TestDecisionsThroughTheDaemon(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv, sess := hookServer(t)
	local := judgeServer(t)
	on := proto.DecideUse{On: true}
	if err := srv.judge.Settings().Set(proto.DecideSettings{Policy: proto.DecideLocalOnly,
		Own:  []proto.Judge{{Kind: proto.JudgeLocal, URL: local, Model: "judge"}},
		Uses: map[string]proto.DecideUse{proto.DecideApprove: on, proto.DecideState: on, proto.DecideLoop: on, proto.DecideTrim: on}}); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.hooks.store.Save([]proto.Hook{{Name: "Judge", On: true, Event: proto.HookToolBefore, Tool: "Bash", Action: proto.HookActionJudge}}); err != nil {
		t.Fatal(err)
	}
	hook := func(event, input string) (string, string, int) {
		var out, errOut bytes.Buffer
		code := Hook([]string{"--event", event}, strings.NewReader(input), &out, &errOut)
		return out.String(), errOut.String(), code
	}
	_, reason, code := hook(proto.HookToolBefore, `{"tool_name":"Bash","tool_input":{"command":"rm -rf ~"}}`)
	if code == 0 || !strings.Contains(reason, "wipes the home folder") {
		t.Fatalf("rm -rf ~: %d %q", code, reason)
	}
	out, _, code := hook(proto.HookToolBefore, `{"tool_name":"Bash","tool_input":{"command":"go test ./..."}}`)
	if code != 0 || !strings.Contains(out, `"permissionDecision":"allow"`) {
		t.Fatalf("go test: %d %q", code, out)
	}

	// A quiet session asking something: waiting.
	srv.sessions.Input(sess, []byte("Delete the old builds? Proceed? (y/n)\r"))
	waitFor(t, "the screen", func() bool { return strings.Contains(sess.Terminal().Text(), "Proceed?") })
	srv.judgeState(sess.Info())
	if st := sess.Info().State; st != proto.StateWaiting {
		t.Fatalf("state %q", st)
	}

	// The same test failing again and again: stopped.
	for range 3 {
		hook(proto.HookToolAfter, `{"hook_event_name":"PostToolUseFailure","tool_name":"Bash","tool_input":{"command":"go test ./billing"},"error":"exit 1"}`)
	}
	waitFor(t, "the loop stopped", func() bool {
		return len(srv.judge.Log(proto.DecideLogParams{Use: proto.DecideLoop})) == 1 && srv.judge.Log(proto.DecideLogParams{Use: proto.DecideLoop})[0].Acted == proto.ActedPaused
	})

	// A long output: what the agent needs of it.
	var d proto.Decision
	long := strings.Repeat("=== RUN TestX\n--- PASS: TestX\n", 400) + "--- FAIL: TestTotal\nFAIL\n"
	c, err := dialDaemon()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Call(proto.MethodDecideAsk, proto.DecideAsk{Use: proto.DecideTrim, Session: sess.ID(), Command: "go test ./...", Output: long, Exit: 1}, &d); err != nil {
		t.Fatal(err)
	}
	if d.Text != "--- FAIL: TestTotal\nFAIL" || d.Acted != proto.ActedTrimmed || d.Saved < 10000 {
		t.Fatalf("trim: %+v", d)
	}
	acted := map[string]string{}
	for _, d := range srv.judge.Log(proto.DecideLogParams{}) {
		acted[d.Use+":"+d.Verdict] = d.Acted
	}
	if acted["approve:refuse"] != proto.ActedRefused || acted["approve:allow"] != proto.ActedAllowed || acted["state:waiting"] != proto.ActedWaiting {
		t.Fatalf("log %v", acted)
	}
}
