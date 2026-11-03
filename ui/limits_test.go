package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

func TestWorkTime(t *testing.T) {
	for secs, want := range map[int64]string{45: "45s", 720: "12m", 3600: "1h", 5400: "1h 30m"} {
		if got := workTime(secs); got != want {
			t.Errorf("%d: %s, want %s", secs, got, want)
		}
	}
}

func TestBudgetAndQueueShow(t *testing.T) {
	s := proto.SessionInfo{Budget: 1800, Worked: 720}
	if text, _ := budgetFact(s); text != "worked 12m of 30m" {
		t.Fatalf("budget fact %q", text)
	}
	s.OverBudget = true
	if text, _ := budgetFact(s); text != "worked 12m of 30m · budget reached" {
		t.Fatalf("over budget %q", text)
	}
	if st := statusOf(proto.SessionInfo{Queued: true}); st.label != "queued" {
		t.Fatalf("queued status %q", st.label)
	}
	a := &App{}
	if body := a.alertBody(proto.SessionInfo{}, proto.SessionInfo{OverBudget: true}); body == "" {
		t.Fatal("no alert for a budget reached")
	}
}

func TestBudgetDialogOffersChoices(t *testing.T) {
	a := &App{}
	a.askBudget(proto.SessionInfo{ID: "s", Name: "search", Worked: 300, Budget: 3600})
	tt := ui.NewTester(func(c *ui.Context) { a.budgetDialog(c) }, 700, 500)
	tt.Frame()
	for _, want := range []string{"Time budget of this session", "“search”", "It has worked 5m so far.", "Stop after", "1h", "Save"} {
		if !tt.HasText(want) {
			t.Errorf("no %q in %q", want, tt.Texts())
		}
	}
	if a.budget.minutes != "60" {
		t.Fatalf("the current budget isn't filled in: %q", a.budget.minutes)
	}
	if err := tt.Click("1h"); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		tt.Frame()
	}
	if err := tt.Click("30m"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.budget.minutes != "30" {
		t.Fatalf("after picking 30m: %q", a.budget.minutes)
	}
}
