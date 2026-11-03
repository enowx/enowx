package chatview

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// With SHOTS set, keeps pictures of the composer in every style, at a reading column and
// at GOD of Agents' whole width.
func TestComposerLook(t *testing.T) {
	if os.Getenv("SHOTS") == "" {
		t.Skip("SHOTS isn't set")
	}
	defer theme.UseStyle(theme.LiquidGlass)
	for _, s := range theme.Styles {
		theme.UseStyle(s)
		theme.Apply(theme.LookupAppearance("graphite", ui.Color{}, false))
		for name, column := range map[string]float32{"normal": 0, "goa": 2000} {
			ch := New()
			ch.model, ch.mode, ch.effort = "opus", "auto", "high"
			ch.contextUsed, ch.contextWindow = 790000, 1000000
			o := Options{Agent: "claude", AgentName: "Claude Code", ColumnWidth: column, Spent: "3.5M tokens · ≈$114.95 API-equivalent", AttachFiles: func() {}}
			frame := ui.Rect{X: 10, Y: 10, W: 1284, H: 300}
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Box(c).Fill().Background(theme.Ground).Padding(10).Children(func() {
					widget.BrightFrame(ui.Box(c).Width(frame.W).Height(frame.H)).Children(func() { ch.composer(c, o) })
				})
			}, 1304, 320)
			tt.Advance(16 * time.Millisecond)
			for _, label := range []string{"Message", "Send (Enter)", "Model", "Attach files or images (or drop or paste them)"} {
				r, ok := tt.Find(label)
				if !ok || r.X < frame.X || r.Y < frame.Y || r.X+r.W > frame.X+frame.W || r.Y+r.H > frame.Y+frame.H {
					t.Errorf("%s %s: %s at %+v, out of the frame %+v", s.ID, name, label, r, frame)
				}
			}
			timelineShot(t, tt, fmt.Sprintf("composer-%s-%s.png", s.ID, name))
		}
	}
}
