package daemon

import (
	"encoding/json"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/unslop"
	"github.com/enowx/enowx/proto"
)

// A canvas's design is unslopped as rendered too: the app runs the lint
// in the page at its width, and the layout at a phone's; what they find
// joins what its HTML shows.
func TestCanvasUnslopAsRendered(t *testing.T) {
	srv := startServer(t)
	var app *client.Client
	var widths []int
	app = connect(t, srv, func(e proto.Event) {
		var req proto.CanvasRenderRequest
		if e.Name != proto.EventCanvasRender || json.Unmarshal(e.Data, &req) != nil {
			return
		}
		widths = append(widths, req.Width)
		text := `{"vw":1440,"stuck":[{"s":"#hero","d":"h1 “Ship”"}]}`
		if req.Script != unslop.LintScript {
			text = `{"width":390,"overflow":120,"off":["#hero"]}`
		}
		go func() { _ = app.CanvasRenderReply(proto.CanvasRenderReply{ID: req.ID, Text: text}) }()
	})
	if err := app.ServeBrowsers(); err != nil {
		t.Fatal(err)
	}
	cv, _ := app.CreateCanvas(proto.CanvasCreateParams{Title: "Ship", Kind: proto.CanvasMockup, Frames: []proto.CanvasFrame{{Name: "Desktop", Width: 1440, Height: 900}}})
	app.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpHTMLSet, Frame: cv.Frames[0].ID, HTML: `<main id="hero"><h1>Ship</h1></main>`})
	u, err := app.CanvasUnslop(proto.CanvasUnslopParams{ID: cv.ID})
	if err != nil {
		t.Fatal(err)
	}
	rules := []string{}
	for _, f := range u.Findings {
		rules = append(rules, f.Rule)
	}
	if !slices.Contains(rules, "stuck-hidden") || !slices.Contains(rules, "overflow") || u.Score == 100 {
		t.Fatalf("as rendered: %d, %v", u.Score, rules)
	}
	if !slices.Equal(widths, []int{1440, 390}) {
		t.Fatalf("rendered at %v", widths)
	}
}

// A design with hidden tabs and a dialog is walked by the app: what those states
// break is found, naming them, and scores it; a design with nothing to click
// isn't loaded again for it.
func TestCanvasUnslopWalksStates(t *testing.T) {
	srv := startServer(t)
	var app *client.Client
	var walked atomic.Int32
	app = connect(t, srv, func(e proto.Event) {
		var req proto.CanvasRenderRequest
		if e.Name != proto.EventCanvasRender || json.Unmarshal(e.Data, &req) != nil {
			return
		}
		text := `{"vw":1440}`
		switch req.Script {
		case unslop.WalkScript:
			walked.Add(1)
			lint := `{"vw":1440,"stuck":[{"s":"#tab2","d":"h2 “Hidden”"}]}`
			b, _ := json.Marshal(map[string]any{"states": []map[string]any{{"name": "Billing", "steps": []string{"Billing"}, "lint": lint}}, "tried": 2, "ms": 40})
			text = string(b)
		case unslop.LintScript:
		default:
			text = `{"width":0}`
		}
		go func() { _ = app.CanvasRenderReply(proto.CanvasRenderReply{ID: req.ID, Text: text}) }()
	})
	if err := app.ServeBrowsers(); err != nil {
		t.Fatal(err)
	}
	cv, _ := app.CreateCanvas(proto.CanvasCreateParams{Title: "Tabs", Kind: proto.CanvasMockup, Frames: []proto.CanvasFrame{{Name: "One", Width: 1440, Height: 900}}})
	plain := `<main><h1>Ship</h1><p>Words enough to read.</p></main>`
	app.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpHTMLSet, Frame: cv.Frames[0].ID, HTML: plain})
	u, err := app.CanvasUnslop(proto.CanvasUnslopParams{ID: cv.ID})
	if err != nil || walked.Load() != 0 || u.Walk != nil {
		t.Fatalf("plain: %v, walked %d, %+v", err, walked.Load(), u.Walk)
	}
	app.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpHTMLSet, Frame: cv.Frames[0].ID,
		HTML: plain + `<button onclick="document.getElementById('tab2').hidden=false">Billing</button><section id="tab2" hidden><h2>Hidden</h2></section>`})
	u, err = app.CanvasUnslop(proto.CanvasUnslopParams{ID: cv.ID})
	if err != nil || walked.Load() != 1 || u.Walk == nil || len(u.Walk.States) != 1 {
		t.Fatalf("walked %d: %v, %+v", walked.Load(), err, u.Walk)
	}
	found := slices.IndexFunc(u.Findings, func(f proto.UnslopFinding) bool {
		return f.State == "Billing" && f.Rule == "stuck-hidden" && slices.Equal(f.Steps, []string{"Billing"})
	})
	if found < 0 || u.Score == 100 {
		t.Fatalf("score %d: %+v", u.Score, u.Findings)
	}
}
