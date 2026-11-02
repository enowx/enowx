package icon

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// Every icon the app ships parses and draws, in one sheet to look at: one
// stroke style, the agents' marks and the logo among them.
func TestEveryIconDrawsInOneSheet(t *testing.T) {
	entries, err := files.ReadDir("svg")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 100 {
		t.Fatalf("%d icons", len(entries))
	}
	svgs := make([]*ui.SVG, 0, len(entries))
	for _, e := range entries {
		svgs = append(svgs, load(strings.TrimSuffix(e.Name(), ".svg")))
	}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Width(1000).Height(640).Wrap().Gap(12).Padding(12).Background(ui.Hex("#20222c")).Children(func() {
			for i, svg := range svgs {
				ui.Icon(c.Key(i), svg).FontSize(24).TextColor(ui.Hex("#f4f4f8"))
			}
		})
	}, 1000, 640)
	tt.Frame()
	if dir := os.Getenv("SHOTS"); dir != "" {
		f, err := os.Create(filepath.Join(dir, "ds-icons.png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		_ = png.Encode(f, tt.Image())
	}
}
