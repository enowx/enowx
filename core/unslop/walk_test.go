package unslop

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/enowx/enowx/proto"
)

func walkRaw(t *testing.T, lint string) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"states": []map[string]any{
			{"name": "Settings", "steps": []string{"Settings"}, "lint": lint},
			{"name": "Delete", "steps": []string{"Settings", "Delete"}, "lint": lint},
		},
		"tried": 9, "left": 3, "limit": "time", "ms": 6001,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// What only a hidden tab or a dialog shows is found, naming the state and its
// steps; what the first screen has already isn't told twice; the report says
// what was reached and the limit.
func TestParseWalk(t *testing.T) {
	const lint = `{"vw":1440,"stuck":[{"s":"#tab","d":"h1"}]}`
	walk, found := ParseWalk(walkRaw(t, lint), 1440, nil)
	if walk == nil || len(walk.States) != 2 || walk.Limit != "time" || walk.Left != 3 || walk.Millis != 6001 {
		t.Fatalf("walk %+v", walk)
	}
	if len(found) != 1 || found[0].State != "Settings" || strings.Join(found[0].Steps, ">") != "Settings" {
		t.Fatalf("found %+v", found)
	}
	if _, again := ParseWalk(walkRaw(t, lint), 1440, found); len(again) != 0 {
		t.Fatalf("told twice: %+v", again)
	}
	if r := WalkReport(walk); !strings.Contains(r, "Settings > Delete") || !strings.Contains(r, "limit of time") || !strings.Contains(r, "3 controls") {
		t.Fatalf("report: %s", r)
	}
	if w, f := ParseWalk("not json", 1440, nil); w != nil || f != nil {
		t.Fatalf("garbage: %+v %+v", w, f)
	}
}

// The score is the worst state's, not an average and not the first screen's.
func TestScoreStatesIsTheWorst(t *testing.T) {
	base := proto.UnslopFinding{Rule: "a", Severity: proto.UnslopLow}
	bad := proto.UnslopFinding{Rule: "b", Severity: proto.UnslopHigh, State: "Dialog"}
	mild := proto.UnslopFinding{Rule: "c", Severity: proto.UnslopLow, State: "Tab"}
	got := ScoreStates([]proto.UnslopFinding{base, bad, mild})
	if want := Score([]proto.UnslopFinding{base, bad}); got != want {
		t.Fatalf("score %d, the worst state's %d", got, want)
	}
	if got := ScoreStates([]proto.UnslopFinding{base}); got != Score([]proto.UnslopFinding{base}) {
		t.Fatalf("no states: %d", got)
	}
}

// Only a walk clicks: the lint a Browser page gets doesn't.
func TestOnlyTheWalkClicks(t *testing.T) {
	if strings.Contains(LintScript, ".click(") {
		t.Fatal("the lint clicks")
	}
	if !strings.Contains(WalkScript, ".click(") || !strings.Contains(WalkScript, LintScript) {
		t.Fatal("the walk doesn't click, or doesn't measure")
	}
	if Walkable("<main><h1>Plain</h1></main>") || !Walkable(`<button onclick="x()">Go</button>`) {
		t.Fatal("Walkable")
	}
}
