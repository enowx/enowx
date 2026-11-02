package ui

import (
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// A large image is kept scaled down, its shape as it was.
func TestReadBackgroundScalesALargeImage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 5120, 1280))); err != nil {
		t.Fatal(err)
	}
	f.Close()
	img, err := readBackground(path)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != backgroundMaxSide || b.Dy() != backgroundMaxSide/4 {
		t.Fatalf("kept at %v", b)
	}
}

// A missing or unreadable file falls back to the theme's own background, with
// a message naming it; going back to the default clears that.
func TestBackgroundFallsBackWithAMessage(t *testing.T) {
	dir := t.TempDir()
	broken := filepath.Join(dir, "broken.png")
	if err := os.WriteFile(broken, []byte("not an image"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(dir, "gone.png"), broken} {
		if _, err := readBackground(path); err == nil || !strings.Contains(err.Error(), filepath.Base(path)) {
			t.Fatalf("%s: %v", path, err)
		}
	}
	app := New(Options{})
	app.onMain = func(fn func()) { fn() }
	app.state.BackgroundImage = broken
	app.background = backgroundState{path: broken}
	_, err := readBackground(broken)
	app.backgroundRead(broken, nil, err)
	if app.background.bmp != nil || app.background.err == "" || app.status == "" {
		t.Fatalf("%+v, status %q", app.background, app.status)
	}
	app.state.BackgroundImage = ""
	app.keepBackground()
	if app.background.err != "" || app.background.bmp != nil {
		t.Fatalf("back to the default: %+v", app.background)
	}
	// With no image the window draws as ever.
	tt := ui.NewTester(func(c *ui.Context) { app.wallpaper(c) }, 400, 300)
	tt.Frame()
}

// The settings stay over a restart, and asking for opacity only touches the
// panels while an image is set.
func TestBackgroundSettingsPersist(t *testing.T) {
	blur := 0.0
	data, err := json.Marshal(appState{BackgroundImage: "/pictures/sea.png", BackgroundOpacity: 0.95, BackgroundBlur: &blur})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	st := loadState(path)
	if st.BackgroundImage != "/pictures/sea.png" || st.backgroundOpacity() != 0.95 || st.backgroundBlur() != 0 {
		t.Fatalf("%+v", st)
	}
	if got := st.palette(false).Panel.A; got != uint8(0.95*255+0.5) {
		t.Fatalf("panel alpha %d", got)
	}
	if plain, want := (appState{}).palette(false), theme.Default; plain.Panel != want.Panel || plain.Frame != want.Frame {
		t.Fatal("no image changed the panels")
	}
	if none := (appState{}); none.backgroundOpacity() != backgroundDefaultOpacity || none.backgroundBlur() != backgroundDefaultBlur {
		t.Fatalf("defaults: %v %v", none.backgroundOpacity(), none.backgroundBlur())
	}
}
