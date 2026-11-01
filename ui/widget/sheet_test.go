package widget

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
)

// sheet draws every control on a frame, in the palette applied.
func sheet(c *ui.Context) {
	var on, off = true, false
	text := "Some text"
	check, seg := true, 1
	c.SetTheme(theme.Window())
	ui.Column(c).Fill().Background(theme.Ground).Padding(theme.S3).TextColor(theme.Text).Children(func() {
		BrightFrame(ui.Column(c).Grow(1).FillWidth().MinHeight(0)).Children(func() {
			PageHeader(c, "Controls", "Every one, in this palette", func() {
				GhostButton(c.Key("add"), "Add a skill")
				PrimaryButton(c.Key("mcp"), icon.Plus, "Add an MCP server", "")
			})
			PageBody(c, true, func() {
				Figs(c, []Fig{{"Memory", "14.2 of 24 GB"}, {"CPU", "3.0 of 8 cores"}, {"Sessions here", "8"}})
				ui.Row(c).FillWidth().Gap(theme.S3).Wrap().Children(func() {
					PrimaryButton(c.Key("p"), nil, "Primary", "")
					GhostButton(c.Key("s"), "Secondary")
					DangerButton(c.Key("d"), "Delete")
					IconButton(c.Key("i"), icon.Settings, "Settings", false)
					IconButton(c.Key("i2"), icon.Git, "Git", true)
					Switch(c.Key("on"), &on, "On")
					Switch(c.Key("off"), &off, "Off")
					Checkbox(c.Key("c"), &check, "Checked")
					Segmented(c.Key("seg"), []string{"Selected", "Not selected"}, seg-1, nil)
				})
				ui.Row(c).FillWidth().Gap(theme.S3).Children(func() {
					Field(c.Key("f"), "Name", func() { Input(c, &text, "Placeholder") })
					Field(c.Key("g"), "Agent", func() {
						Trigger(ui.Row(c.Key("t")), false).Children(func() {
							ui.Text(c, "Claude Code").FontSize(theme.TextUI).Grow(1)
							ui.Icon(c, icon.ChevronDown).FontSize(12)
						})
					})
				})
				ui.Row(c).FillWidth().Gap(theme.S3).AlignItems(ui.Start).Children(func() {
					ui.Box(c.Key("left")).Grow(1).Basis(0).MinWidth(0).Children(func() {
						Card(c).FillWidth().Children(func() {
							ui.Text(c, "A card").FontSize(theme.TextRead).FontWeight(500)
							KV(c, "Agent", "Claude Code", false)
							KV(c, "File", "~/.claude/skills/x/SKILL.md", true)
							Meter(c, Segment{Fraction: 0.4, Color: theme.Working}, Segment{Fraction: 0.2, Color: theme.Warning})
							ui.Row(c).Gap(theme.S3).Children(func() {
								Legend(c, theme.Working, "Sessions")
								Badge(c, Status{Label: "Working", Color: theme.Working})
								Badge(c, Status{Label: "Waiting on you", Color: theme.Warning})
								Badge(c, Status{Label: "Failed", Color: theme.Error})
								Badge(c, Status{Label: "Done", Color: theme.Done})
								Tag(c, "New", ui.Color{})
								Chip(c, "1h", ui.Color{})
							})
						})
					})
					ui.Column(c.Key("right")).Grow(1).Basis(0).MinWidth(0).Gap(theme.S2).Children(func() {
						Notice(c, icon.Alert, ui.Color{}, "A plate", "What it says about the form it sits in", nil)
						Notice(c, icon.Alert, theme.Del, "A failure", "Why it failed", nil)
						Banner(c, theme.Warning, "Skips permission prompts", nil)
						CodeBlockText(c, "$ go test ./ui\nok  github.com/enowx/enowx/ui  6.1s")
						ListItem(c, theme.Working, "docs/goa.md", "A list's row", true, nil)
						ListItem(c, theme.Done, "ui/goa.go", "Another", false, nil)
					})
				})
			})
		})
	})
}

// The sheet draws in every palette, in both modes, nothing out of its frame
// at the narrowest window. With SHOTS set it keeps a picture of each.
func TestControlsSheetInEveryPalette(t *testing.T) {
	old := theme.Current()
	defer theme.Apply(old)
	dir := os.Getenv("SHOTS")
	for _, p := range theme.Palettes {
		for _, light := range []bool{false, true} {
			theme.Apply(theme.LookupAppearance(p.ID, ui.Color{}, light))
			tt := ui.NewTester(sheet, 1200, 720)
			tt.Frame()
			tt.Frame()
			if !tt.HasText("Every one, in this palette") {
				t.Fatalf("%s: %q", p.ID, tt.Texts())
			}
			if dir != "" {
				name := p.ID + "-dark.png"
				if light {
					name = p.ID + "-light.png"
				}
				_ = os.MkdirAll(dir, 0o755)
				f, err := os.Create(filepath.Join(dir, name))
				if err != nil {
					t.Fatal(err)
				}
				_ = png.Encode(f, tt.Image())
				f.Close()
			}
		}
	}
}

// A dialog the toolkit draws wears the palette: the confirms the app asks.
func TestAlertWearsThePalette(t *testing.T) {
	dir := os.Getenv("SHOTS")
	if dir == "" {
		t.Skip("to look at")
	}
	open := true
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).Fill().Background(theme.Ground).Children(func() {
			b := GhostButton(c, "With a tip").Tooltip("What a tooltip looks like")
			_ = b
		})
		AlertDialog(c, &open, "Delete “GOA session tabs”?", "Its worktree and branch stay on disk.", "Cancel", "Delete")
	}, 900, 500)
	tt.Frame()
	tt.Advance(1000000000)
	tt.Frame()
	f, _ := os.Create(filepath.Join(dir, "alert.png"))
	defer f.Close()
	_ = png.Encode(f, tt.Image())
}
