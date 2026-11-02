package widget

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// What the picker keeps as #RRGGBB comes back the color it was, through its
// hue, saturation and brightness.
func TestColorHSVRoundTrip(t *testing.T) {
	for _, c := range []ui.Color{
		ui.Hex("#FF0000"), ui.Hex("#00FF00"), ui.Hex("#0000FF"), ui.Hex("#FFFFFF"), ui.Hex("#000000"),
		ui.Hex("#808080"), ui.Hex("#A0D9DA"), ui.Hex("#0A6FE0"), ui.Hex("#D49A6A"), ui.Hex("#C2410C"),
	} {
		h, s, v := colorToHSV(c)
		if got := colorFromHSV(h, s, v); got != c {
			t.Errorf("%+v through %.1f %.2f %.2f: %+v", c, h, s, v, got)
		}
	}
	if got := colorFromHSV(360, 1, 1); got != ui.Hex("#FF0000") {
		t.Errorf("the hue slider's end: %+v", got)
	}
}

// The swatch opens the square and the slider, inside the pop-over, and shows
// nothing to type in.
func TestColorPickOpens(t *testing.T) {
	var got []ui.Color
	tt := ui.NewTester(func(c *ui.Context) {
		ColorPick(c, "Custom accent", ui.Hex("#0A6FE0"), func(col ui.Color, done bool) { got = append(got, col) })
	}, 500, 400)
	tt.Frame()
	if _, ok := tt.Find("Hue"); ok {
		t.Fatal("open before it was clicked")
	}
	if err := tt.Click("Custom accent"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, ok := tt.Find("Hue"); !ok {
		t.Fatalf("no slider: %q", tt.Texts())
	}
	if _, ok := tt.Find("Saturation and brightness"); !ok {
		t.Fatalf("no square: %q", tt.Texts())
	}
	if len(got) != 0 {
		t.Fatalf("opening changed the color: %+v", got)
	}
}
