package unslopview

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/framecheck"
)

// A report whose brief, review and error run to megabytes, with thousands of findings, stays quick.
func TestLongReportStaysQuick(t *testing.T) {
	line, lines := framecheck.Line(), framecheck.Lines()
	run := &proto.CanvasUnslopRun{ID: "c", Frame: "f", From: 40, Score: 52, Scans: 2, Passes: []string{"01 Truth"},
		Brief:  &proto.DesignBrief{Product: line, Users: line, Tone: "Calm"},
		Review: &proto.DesignReview{Round: 1, Done: true, Score: 7, Issues: []string{line}, Improvements: []string{lines}, Keep: line}}
	for range 10000 {
		run.Findings = append(run.Findings, proto.UnslopFinding{Pass: "01 Truth"})
	}
	p := Panel{brief: true}
	tt := framecheck.New(t, "unslop", func(c *ui.Context) {
		c.After(time.Millisecond)
		ui.Column(c).Width(900).Height(700).Children(func() {
			p.Draw(c, run, PanelOptions{Reading: "Reading…"})
			DrawReview(c, *run.Review, nil)
			DrawError(c, lines, nil)
		})
	}, 900, 700)
	framecheck.Quick(t, "unslop", tt)
}
