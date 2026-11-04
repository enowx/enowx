package canvasview

import (
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

// A long page's layers draw part by part: a row's drawing state is what took the memory.
func TestLayersOfALongPageShowOnAsking(t *testing.T) {
	var html strings.Builder
	for i := range layersAtOnce + 50 {
		fmt.Fprintf(&html, `<div id="row%d">x</div>`, i)
	}
	v := &View{}
	v.canvas = &proto.Canvas{ID: "c1", Frames: []proto.CanvasFrame{{ID: "f1", Name: "Home", HTML: html.String()}}}
	v.frame = "f1"
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(320).Children(func() { v.layers(c, Options{}) })
	}, 400, 12000)
	tt.Frame()
	if _, ok := tt.Find("Show 50 more"); !ok {
		t.Fatalf("no offer: %d texts", len(tt.Texts()))
	}
	if err := tt.Click("Show 50 more"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, ok := tt.Find("Show 50 more"); ok || !v.layersAll {
		t.Fatal("the rest didn't come")
	}
}
