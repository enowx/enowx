package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/onboardview"
)

// A failure in onboarding gives way to the app: what follows it in the frame still draws,
// and the frames after it have no onboarding.
func TestOnboardingFailureGivesWayToTheApp(t *testing.T) {
	a := New(Options{})
	a.onboard.view = onboardview.New()
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			if a.onboardingShows() {
				a.onboardingGuarded(c, func() {
					ui.Column(c).Children(func() {
						ui.Row(c).Children(func() { panic("a step went wrong") })
					})
				})
			}
			ui.Text(c, "The app")
		})
	}, 400, 300)
	tt.Advance(16 * time.Millisecond)
	if a.onboardingShows() {
		t.Fatal("onboarding stayed")
	}
	if !a.state.OnboardedDevice {
		t.Fatal("it would run again at the next start")
	}
	if !tt.HasText("The app") {
		t.Fatalf("no app after it: %v", tt.Texts())
	}
}

// Onboarding that keeps starting and never finishing, as a crash in it would, stops starting.
func TestOnboardingStopsStartingAfterTries(t *testing.T) {
	a := New(Options{})
	a.state.OnboardingTries = onboardingTriesMost
	a.onboardingDecide(proto.OnboardingState{}, nil, false)
	if a.onboardingShows() {
		t.Fatal("it started again")
	}
	a.state.OnboardingTries = 0
	a.onboardingDecide(proto.OnboardingState{}, nil, false)
	if !a.onboardingShows() {
		t.Fatal("a new user got none")
	}
}
