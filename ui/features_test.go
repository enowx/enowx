package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// Settings' Features turns one off, which leaves the guide agents get,
// and an agent asking to turn one on gets the user's answer.
func TestFeaturesSettingsAndAsking(t *testing.T) {
	h := newNotesHarness(t)
	h.app.page, h.app.settingsSection = pageSettings, sectionFeatures
	h.tt = ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(1000).Height(2400).Background(theme.Background).Children(func() {
			h.app.settingsBody(c)
			h.app.asksView(c)
		})
	}, 1000, 2400)
	h.settle("the features", func() bool { return h.tt.HasText("Traffic (HTTP inspector)") })
	shot(t, h.tt, "features.png")
	if err := h.tt.Click("Feature Browser"); err != nil {
		t.Fatal(err)
	}
	h.settle("browser off", func() bool {
		g, err := h.cl.FeaturesGuide(proto.FeaturesParams{})
		return err == nil && !g.On["browser"]
	})

	answered := make(chan proto.FeatureAnswer, 1)
	go func() {
		a, err := h.cl.RequestFeature(proto.FeatureRequest{ID: "traffic", Reason: "to mock the payment API"})
		if err == nil {
			answered <- a
		}
	}()
	h.settle("the question", func() bool { return h.tt.HasText("Turn on Traffic (HTTP inspector)?") })
	shot(t, h.tt, "features-ask.png")
	if err := h.tt.Click("Turn on everywhere"); err != nil {
		t.Fatal(err)
	}
	select {
	case a := <-answered:
		if !a.Allowed {
			t.Fatalf("answer %+v", a)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no answer")
	}
	if g, _ := h.cl.FeaturesGuide(proto.FeaturesParams{}); !g.On["traffic"] {
		t.Error("traffic not on after the user said yes")
	}
}
