package vaultview

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/vault"
	"github.com/enowx/enowx/ui/theme"
)

// Every platform has its tile, a brand's mark or initials on a color, in one
// sheet to look at.
func TestEveryPlatformHasItsTile(t *testing.T) {
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Row(c).Width(1000).Height(640).Wrap().Gap(theme.S2).Padding(theme.S3).Background(theme.Panel).TextColor(theme.Text).Children(func() {
			for _, p := range vault.Platforms {
				ui.Row(c.Key(p.ID)).Width(150).Height(36).Gap(theme.S2).AlignItems(ui.Center).Children(func() {
					Logo(c, p, 24)
					ui.Text(c, p.Name).FontSize(theme.TextLabel).SingleLine().Shrink(1).MinWidth(0)
				})
			}
		})
	}, 1000, 640)
	tt.Frame()
	for _, p := range vault.Platforms {
		if !tt.HasText(p.Name) {
			t.Errorf("no %q", p.Name)
		}
	}
	if dir := os.Getenv("SHOTS"); dir != "" {
		f, err := os.Create(filepath.Join(dir, "ds-logos.png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		_ = png.Encode(f, tt.Image())
	}
}
