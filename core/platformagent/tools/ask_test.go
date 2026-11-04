package tools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/platformagent/record"
)

type fakeAsker struct {
	got     []record.Question
	answers []string
	err     error
}

func (f *fakeAsker) Questions(_ context.Context, qs []record.Question) ([]string, error) {
	f.got = qs
	return f.answers, f.err
}

func TestAskUserQuestion(t *testing.T) {
	asker := &fakeAsker{answers: []string{"Postgres"}}
	tool := NewAskUserQuestion(asker)
	if !tool.ReadOnly() || tool.Edits() {
		t.Fatal("it changes nothing")
	}
	input := map[string]any{"questions": []map[string]any{{
		"header": "Database", "question": "Which one?", "multiSelect": true,
		"options": []map[string]any{{"label": "Postgres", "description": "What it runs"}, {"label": "SQLite", "description": "No server"}},
	}, {"question": "And then?"}}}
	r := call(t, tool, t.TempDir(), input)
	if r.Failed || !strings.Contains(r.Text, "Which one?\n→ Postgres") || !strings.Contains(r.Text, "And then?\n→ (no answer)") {
		t.Fatalf("%+v", r)
	}
	q := asker.got[0]
	if q.Header != "Database" || !q.Multi || q.Options[1] != "SQLite" || q.About[1] != "No server" {
		t.Fatalf("asked %+v", q)
	}
	asker.err = errors.New("no answer came")
	if r = call(t, tool, t.TempDir(), input); !r.Failed || !strings.Contains(r.Text, "no answer came") {
		t.Fatalf("%+v", r)
	}
	if r = call(t, NewAskUserQuestion(nil), t.TempDir(), input); !r.Failed {
		t.Fatalf("no asker: %+v", r)
	}
}

func TestBuiltinOrderAndKinds(t *testing.T) {
	want := []struct {
		name            string
		readOnly, edits bool
	}{{"Read", true, false}, {"Write", false, true}, {"Edit", false, true}, {"Glob", true, false}, {"Grep", true, false}, {"Bash", false, false}}
	got := Builtin()
	if len(got) != len(want) {
		t.Fatalf("%d tools", len(got))
	}
	for i, w := range want {
		if got[i].Name() != w.name || got[i].ReadOnly() != w.readOnly || got[i].Edits() != w.edits {
			t.Errorf("tool %d: %s read-only %v edits %v", i, got[i].Name(), got[i].ReadOnly(), got[i].Edits())
		}
	}
}
