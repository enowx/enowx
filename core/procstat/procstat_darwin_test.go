package procstat

import "testing"

// A start's five words and a path with spaces are read apart.
func TestParsePS(t *testing.T) {
	list := parsePS("  201   200   1024  0:01.50 Tue Nov  3 04:08:00 2026 /Applications/My App.app/Contents/MacOS/my app\nshort line\n")
	if len(list) != 1 {
		t.Fatalf("%+v", list)
	}
	p := list[0]
	if p.PID != 201 || p.PPID != 200 || p.RSS != 1024<<10 || p.Start != "Tue Nov 3 04:08:00 2026" || p.Name != "my app" {
		t.Fatalf("%+v", p)
	}
}
