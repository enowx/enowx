package trafficview

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

// bestFrame is what a frame of tt costs, the best of a few runs: what a machine busy with
// other tests adds comes and goes, and what the view costs stays.
func bestFrame(tt *ui.Tester) time.Duration {
	const builds, tries = 16, 3
	best := time.Hour
	for range tries {
		began := time.Now()
		tt.Advance(builds * time.Millisecond)
		tt.Image()
		best = min(best, time.Since(began)/builds)
	}
	return best
}

// A long body doesn't cost a frame more than a short one: ten megabytes of JSON read as
// far as Show more goes, and one line of a megabyte. Its messages neither.
func TestLongBodiesStayQuick(t *testing.T) {
	bodies := map[string]proto.TrafficBody{
		"16 MB of JSON": {ContentType: "application/json", Total: 16 << 20, Data: []byte(strings.Repeat(`{"id":1,"name":"a row"},`, (16<<20)/24))},
		"a 1 MB line":   {ContentType: "text/html", Total: 1 << 20, Data: []byte(strings.Repeat("y", 1<<20))},
	}
	for name, b := range bodies {
		queue := make(chan func(), 16)
		o := Options{Update: func(fn func()) { queue <- fn }}
		v := &View{details: map[int64]*detail{}}
		d := &detail{id: 1, images: map[string]*ui.Bitmap{}, shown: bodyShown, tab: tabResponse, res: b}
		frame := func(c *ui.Context) {
			c.After(time.Millisecond)
			ui.Column(c).Width(1000).Height(700).Children(func() {
				v.bodyPane(c, o, d, "response", d.res, &d.resText, func() {}, nil)
			})
		}
		tt := ui.NewTester(frame, 1000, 700)
		tt.Image()
		select {
		case fn := <-queue:
			fn()
		case <-time.After(10 * time.Second):
			t.Fatalf("%s: never prepared", name)
		}
		took := bestFrame(tt)
		t.Logf("%s: %v a frame", name, took)
		if took > 16*time.Millisecond {
			t.Errorf("%s: %v a frame", name, took)
		}
	}

	msgs := make([]proto.TrafficMessage, 2000)
	for i := range msgs {
		msgs[i] = proto.TrafficMessage{Kind: "text", Data: []byte(strings.Repeat("z", 16<<10))}
	}
	d := &detail{id: 1, msgs: msgs}
	tt := ui.NewTester(func(c *ui.Context) {
		c.After(time.Millisecond)
		ui.Column(c).Width(1000).Height(700).Children(func() { messagesView(c, d) })
	}, 1000, 700)
	tt.Image()
	took := bestFrame(tt)
	t.Logf("2000 messages of 16 KB: %v a frame", took)
	if took > 16*time.Millisecond {
		t.Errorf("messages: %v a frame", took)
	}
}
