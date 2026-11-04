package platformagent

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/core/platformagent/record"
	"github.com/enowx/enowx/internal/standinapi"
)

func TestRateLimitHonorsRetryAfter(t *testing.T) {
	r := newRig(t, nil, standinapi.RateLimited("1"), standinapi.Text("after the wait"))
	began := time.Now()
	r.mustTurn("hi")
	if took := time.Since(began); took < 900*time.Millisecond {
		t.Errorf("retried after %v", took)
	}
	if !strings.Contains(r.front.notices(), "rate limited, retrying in 1s (2 of 6)") {
		t.Errorf("notices: %s", r.front.notices())
	}
	if len(r.srv.Requests()) != 2 || r.front.turnStop(t) != "done" {
		t.Errorf("requests %d, stop %s", len(r.srv.Requests()), r.front.turnStop(t))
	}
}

func TestOverloadedAndServerErrorsAreRetried(t *testing.T) {
	r := newRig(t, nil, standinapi.Err(529), standinapi.Err(500), standinapi.Err(408), standinapi.Text("through"))
	r.mustTurn("hi")
	notices := r.front.notices()
	for _, want := range []string{"overloaded", "the API failed (500)", "timed out", "(4 of 6)"} {
		if !strings.Contains(notices, want) {
			t.Errorf("no %q in %s", want, notices)
		}
	}
	if r.front.turnStop(t) != "done" {
		t.Errorf("stop: %s", r.front.turnStop(t))
	}
}

func TestDropMidStreamRetriesWholeWithoutThePartial(t *testing.T) {
	partial := strings.Repeat("partial text ", 10)
	r := newRig(t, nil,
		standinapi.Reply{Blocks: []standinapi.Block{standinapi.TextBlock(partial)}, Drop: true},
		standinapi.Reply{Blocks: []standinapi.Block{standinapi.TextBlock(partial)}, StreamError: "overloaded_error"},
		standinapi.Text("whole answer"),
		standinapi.Text("second answer"))
	r.mustTurn("hi")
	notices := r.front.notices()
	if !strings.Contains(notices, "connection lost") || !strings.Contains(notices, "overloaded") {
		t.Errorf("notices: %s", notices)
	}
	answers := r.front.of(record.Assistant)
	if len(answers) != 1 {
		t.Fatalf("answers: %d", len(answers))
	}
	r.mustTurn("again")
	reqs := r.srv.Requests()
	if len(reqs) != 4 {
		t.Fatalf("requests: %d", len(reqs))
	}
	assertValidHistory(t, reqs[3])
	raw := string(reqs[3].Raw("messages"))
	if strings.Contains(raw, "partial") || !strings.Contains(raw, "whole answer") {
		t.Errorf("history: %s", raw)
	}
	// The same request went out each time.
	if string(reqs[0].Body) != string(reqs[2].Body) {
		t.Error("the retry's request differs")
	}
}

func TestGivesUpAfterSixTries(t *testing.T) {
	r := newRig(t, nil, standinapi.Err(500), standinapi.Err(500), standinapi.Err(500), standinapi.Err(500), standinapi.Err(500), standinapi.Err(500))
	err := r.turn("hi")
	if err == nil || !strings.Contains(err.Error(), "gave up after 6 tries") {
		t.Fatalf("err: %v", err)
	}
	if len(r.srv.Requests()) != 6 || r.front.turnStop(t) != "error" {
		t.Errorf("requests %d, stop %s", len(r.srv.Requests()), r.front.turnStop(t))
	}
	if !strings.Contains(r.front.notices(), "gave up") {
		t.Errorf("notices: %s", r.front.notices())
	}
}

func TestRequestErrorsExplainPlainlyWithoutRetrying(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   string
	}{
		{401, "key is wrong"},
		{403, "isn't allowed"},
		{400, "refused the request: bad field"},
		{404, "doesn't know this model"},
	} {
		r := newRig(t, nil, standinapi.Reply{Status: tc.status, Message: "bad field"})
		err := r.turn("hi")
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%d: %v", tc.status, err)
		}
		if len(r.srv.Requests()) != 1 {
			t.Errorf("%d: %d requests", tc.status, len(r.srv.Requests()))
		}
		if r.front.turnStop(t) != "error" {
			t.Errorf("%d: stop %s", tc.status, r.front.turnStop(t))
		}
	}
}

func TestEditedHistoryIsRecoveredOnceWithoutThinking(t *testing.T) {
	r := newRig(t, nil,
		standinapi.Reply{Blocks: []standinapi.Block{standinapi.Thinking("reasoning", "sig-old"), standinapi.TextBlock("one")}},
		standinapi.Reply{Status: 400, Message: "messages.1.content.0: Invalid `signature` in `thinking` block. The block is bound to a different conversation."},
		standinapi.Text("two"))
	r.mustTurn("first")
	r.mustTurn("second")
	reqs := r.srv.Requests()
	if len(reqs) != 3 {
		t.Fatalf("requests: %d", len(reqs))
	}
	if !strings.Contains(string(reqs[1].Raw("messages")), "sig-old") || strings.Contains(string(reqs[2].Raw("messages")), "sig-old") {
		t.Errorf("recovery: %s", reqs[2].Raw("messages"))
	}
	if !strings.Contains(string(reqs[2].Raw("messages")), `"one"`) {
		t.Errorf("the text went too: %s", reqs[2].Raw("messages"))
	}
	assertValidHistory(t, reqs[2])
}

func TestKeyNeverInFileOrErrors(t *testing.T) {
	echo := "bad key " + fakeKey + " was sent"
	r := newRig(t, nil,
		standinapi.Reply{Status: 400, Message: echo},
		standinapi.Reply{Status: 500, Message: echo},
		standinapi.Err(500), standinapi.Err(500), standinapi.Err(500), standinapi.Err(500), standinapi.Err(500),
	)
	var errs []error
	errs = append(errs, r.turn("first"))
	errs = append(errs, r.turn("second"))
	for _, err := range errs {
		if err == nil {
			t.Fatal("no error")
		}
		if strings.Contains(err.Error(), fakeKey) {
			t.Errorf("error holds the key: redacted output only")
		}
	}
	data, err := os.ReadFile(r.path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), fakeKey) {
		t.Error("the file holds the key")
	}
	if !strings.Contains(string(data), "[key]") {
		t.Error("the echo didn't reach the file at all, which this test relies on")
	}
	if strings.Contains(r.front.notices(), fakeKey) {
		t.Error("a notice holds the key")
	}
	sent := r.srv.Requests()[0].Header
	if sent.Get("X-Api-Key") != fakeKey || sent.Get("Authorization") != "" {
		t.Error("an API key goes on x-api-key and nowhere else")
	}
}
