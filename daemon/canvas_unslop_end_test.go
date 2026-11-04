package daemon

import (
	"testing"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
)

// However an unslop ends, nothing of it stays: its regions' agents go with the run.
func TestUnslopRegionsEndWithTheRun(t *testing.T) {
	type rig struct {
		srv    *Server
		c      *client.Client
		canvas string
		frame  string
		lead   string
	}
	start := func(t *testing.T) rig {
		t.Helper()
		srv := startServer(t)
		c := connect(t, srv, nil)
		cv, err := c.CreateCanvas(proto.CanvasCreateParams{Title: "App", Kind: proto.CanvasMockup})
		if err != nil {
			t.Fatal(err)
		}
		page := `<aside id="side"><a id="nav1">Home</a></aside><main id="main"><h1 id="title">Numbers</h1></main>`
		if _, err := c.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpHTMLSet, Frame: cv.Frames[0].ID, HTML: page}); err != nil {
			t.Fatal(err)
		}
		started, err := c.CanvasUnslop(proto.CanvasUnslopParams{ID: cv.ID, Start: true})
		if err != nil {
			t.Fatal(err)
		}
		lead, err := c.CreateSession(proto.SessionCreateParams{Agent: "shell", Name: "lead", Cols: 80, Rows: 24})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.CanvasUnslop(proto.CanvasUnslopParams{ID: cv.ID, Frame: started.Frame, Regions: []string{"#side", "#main"}, Session: lead.ID}); err != nil {
			t.Fatal(err)
		}
		if n := len(srv.unslopRegionSessions(cv.ID)); n != 2 {
			t.Fatalf("%d regions' agents running", n)
		}
		return rig{srv, c, cv.ID, started.Frame, lead.ID}
	}
	gone := func(t *testing.T, r rig) {
		t.Helper()
		for deadline := time.Now().Add(15 * time.Second); len(r.srv.unslopRegionSessions(r.canvas)) > 0; time.Sleep(50 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("still running: %+v", r.srv.unslopRegionSessions(r.canvas))
			}
		}
		if run, ok := r.srv.unslopRuns.status(r.canvas); ok && len(run.Regions) != 0 {
			t.Fatalf("the run still has them: %+v", run.Regions)
		}
	}
	t.Run("stopped", func(t *testing.T) {
		r := start(t)
		if err := r.c.Call(proto.MethodCanvasControl, proto.CanvasControl{ID: r.canvas, Action: proto.CanvasStop}, nil); err != nil {
			t.Fatal(err)
		}
		gone(t, r)
		if _, err := r.srv.sessions.Get(r.lead); err != nil {
			t.Fatal("the redesign's agent went too: it is only interrupted")
		}
	})
	t.Run("its agent closed", func(t *testing.T) {
		r := start(t)
		if err := r.c.KillSession(r.lead); err != nil {
			t.Fatal(err)
		}
		gone(t, r)
	})
	t.Run("its agent stopped by hand", func(t *testing.T) {
		r := start(t)
		sess, err := r.srv.sessions.Get(r.lead)
		if err != nil {
			t.Fatal(err)
		}
		if err := r.srv.hibernateSession(sess, true); err != nil {
			t.Fatal(err)
		}
		gone(t, r)
	})
	t.Run("finished", func(t *testing.T) {
		r := start(t)
		if _, err := r.c.CanvasUnslop(proto.CanvasUnslopParams{ID: r.canvas, Frame: r.frame, Pass: "done", Session: r.lead}); err != nil {
			t.Fatal(err)
		}
		gone(t, r)
	})
	t.Run("a new run", func(t *testing.T) {
		r := start(t)
		if _, err := r.c.CanvasUnslop(proto.CanvasUnslopParams{ID: r.canvas, Start: true}); err != nil {
			t.Fatal(err)
		}
		gone(t, r)
	})
	t.Run("hung past its bound", func(t *testing.T) {
		r := start(t)
		r.srv.helpersPass(time.Now().Add(regionMax + time.Minute))
		gone(t, r)
	})
	t.Run("reported done but never settling", func(t *testing.T) {
		r := start(t)
		regions := r.srv.unslopRegionSessions(r.canvas)
		if sess, err := r.srv.sessions.Get(regions[0].ID); err != nil {
			t.Fatal(err)
		} else if err := r.srv.sessions.SetState(sess, proto.StateWorking); err != nil {
			t.Fatal(err)
		}
		r.srv.helperFinished(regions[0].ID)
		r.srv.helpersPass(time.Now().Add(regionFinishedMax + time.Minute))
		if _, err := r.srv.sessions.Get(regions[0].ID); err == nil {
			t.Fatal("still here after reporting done")
		}
		if _, err := r.srv.sessions.Get(regions[1].ID); err != nil {
			t.Fatal("the other, still at work, went too")
		}
	})
}
