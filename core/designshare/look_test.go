package designshare

import (
	"slices"
	"testing"

	"github.com/enowx/enowx/proto"
)

func names(l Look) []string {
	out := []string{}
	for _, c := range l.Colors {
		out = append(out, c.Name)
	}
	return out
}

func TestLookOf(t *testing.T) {
	for _, tc := range []struct {
		name   string
		frame  Frame
		theme  string
		device string
		has    []string
		hasnt  []string
	}{
		{"a dark page with a blue button, by a rule", Frame{Width: 1200, HTML: `<style>body{background:#0b0d14;color:#fff}.b{background:#2c5fd4}</style><body><a class="b">Go</a></body>`},
			"dark", "desktop", []string{"black", "blue"}, []string{"paper"}},
		{"variables are followed", Frame{Width: 1200, HTML: `<style>:root{--ground:#f4f0e8;--accent:var(--brand);--brand:rgb(47,125,91)}body{background:var(--ground)}.x{background-color:var(--accent)}</style>`},
			"light", "desktop", []string{"paper", "green"}, []string{"black"}},
		{"the first block filling the page is its ground, faint tints aren't colors", Frame{Width: 390, HTML: `<div style="min-height:100vh;background:#c8481f;color:#fff"><p style="background:#2d5b4522">x</p></div>`},
			"dark", "phone", []string{"red"}, []string{"green"}},
		{"a script's colors aren't what shows", Frame{Width: 1200, HTML: `<body style="background:#fff"><script>const color="#2c5fd4"; var s="background:#2c5fd4"</script></body>`},
			"light", "desktop", []string{"paper"}, []string{"blue"}},
		{"utility classes count, a family's dark shade stays dark", Frame{Width: 1200, HTML: `<body class="bg-slate-900 text-white"><div class="bg-violet-600 from-violet-500"></div></body>`},
			"dark", "desktop", []string{"black", "violet"}, nil},
		{"the frame's own background and a diagram's shapes", Frame{Width: 800, Background: "#1f7a8c", Elements: []proto.CanvasElement{{Fill: "#e3b23c"}, {Fill: "#e3b23c"}, {Fill: "#e3b23c"}, {Fill: "#e3b23c"}}},
			"dark", "desktop", []string{"teal", "yellow"}, nil},
		{"what the page's body says goes over the frame's background", Frame{Width: 1200, Background: "#ffffff", HTML: `<style>body{background:#0b0d14}.b{background:#2c5fd4}</style><button class="b">Go</button>`},
			"dark", "desktop", []string{"black", "blue"}, []string{"paper"}},
		{"a page saying nothing is white, as a browser draws it", Frame{Width: 800, HTML: `<h1>Plain</h1>`},
			"light", "desktop", []string{"paper"}, nil},
		{"a picture inside a page isn't read for colors", Frame{Width: 800, HTML: `<img src="data:image/png;base64,QUJDI2ZmMDAwMA=="><style>body{background:#000}</style>`},
			"dark", "desktop", []string{"black"}, []string{"red"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := LookOf(tc.frame)
			if l.Theme != tc.theme || l.Device != tc.device {
				t.Fatalf("theme %s, device %s: %+v", l.Theme, l.Device, l)
			}
			got := names(l)
			for _, c := range tc.has {
				if !slices.Contains(got, c) {
					t.Fatalf("no %s in %+v", c, l.Colors)
				}
			}
			for _, c := range tc.hasnt {
				if slices.Contains(got, c) {
					t.Fatalf("%s in %+v", c, l.Colors)
				}
			}
			if len(got) > mostColors {
				t.Fatalf("too many: %+v", l.Colors)
			}
		})
	}
}

// The filters show exactly these colors.
func TestSwatchesAreTheLibrarysColors(t *testing.T) {
	for hex, want := range map[string]string{"#c8481f": "red", "#e3b23c": "yellow", "#2f7d5b": "green", "#1f7a8c": "teal", "#2c5fd4": "blue", "#3d2fa0": "violet", "#1c1a22": "black", "#f4f0e8": "paper", "#808080": ""} {
		c, ok := parseColor(hex)
		if !ok {
			t.Fatal(hex)
		}
		if got := swatchOf(c); got != want {
			t.Errorf("%s reads as %q, wants %q", hex, got, want)
		}
	}
}

func TestParseColorForms(t *testing.T) {
	for _, s := range []string{"#abc", "#abcd", "#aabbcc", "#aabbccdd", "rgb(1, 2, 3)", "rgb(1 2 3 / 50%)", "rgba(1,2,3,.9)", "hsl(120deg 50% 50%)", "hsla(120,50%,50%,1)", "White"} {
		if _, ok := parseColor(s); !ok {
			t.Errorf("%s didn't read", s)
		}
	}
	for _, s := range []string{"", "#ab", "#ggg", "rgb(1,2)", "rgba(1,2,3,0.1)", "#00000011", "url(x)"} {
		if _, ok := parseColor(s); ok {
			t.Errorf("%q read", s)
		}
	}
}
