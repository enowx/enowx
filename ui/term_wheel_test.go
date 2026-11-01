package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

// A full-screen program that takes the mouse, as OpenCode does, gets the wheel
// through a session: attached as it starts, and attached again later, as when
// the app comes back to it.
func TestWheelReachesAFullScreenProgram(t *testing.T) {
	h := newCanvasHarness(t)
	dir := t.TempDir()
	got := filepath.Join(dir, "got")
	script := `printf '\033[?1049h\033[?1000h\033[?1002h\033[?1003h\033[?1006hREADY'; stty raw -echo; cat > ` + got
	info, err := h.cl.CreateSession(proto.SessionCreateParams{Command: []string{"sh", "-c", script}, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	try := func(when string) {
		t.Helper()
		st, err := h.cl.Attach(info.ID)
		if err != nil {
			t.Fatal(err)
		}
		term, err := terminal.New(terminal.Options{Conn: st, ResizeByConn: true})
		if err != nil {
			t.Fatal(err)
		}
		defer term.Close()
		tt := ui.NewTester(func(c *ui.Context) { terminal.View(c, term).Fill() }, 800, 500)
		for range 40 {
			tt.Frame()
			time.Sleep(20 * time.Millisecond)
		}
		os.Truncate(got, 0)
		for range 5 {
			tt.Scroll(300, 200, 0, -40)
			tt.Frame()
			time.Sleep(30 * time.Millisecond)
		}
		for range 25 {
			if data, _ := os.ReadFile(got); strings.Contains(string(data), "[<6") {
				return
			}
			tt.Frame()
			time.Sleep(40 * time.Millisecond)
		}
		data, _ := os.ReadFile(got)
		t.Errorf("%s: the wheel didn't reach the program: %q", when, data)
	}
	try("attached as it starts")
	time.Sleep(200 * time.Millisecond)
	try("attached again")
}
