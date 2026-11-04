package server

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/core/designshare"
	"github.com/enowx/enowx/proto"
)

// submitPath is where version n of item id is submitted.
func submitPath(id string, n int) string {
	return strings.NewReplacer("{id}", id, "{version}", string(rune('0'+n))).Replace(api.PathLibrarySubmit)
}

// sendBundle makes a design of data's bundle, its objects sent, and submits it.
func (e *libEnv) sendBundle(m signedInMember, p api.LibraryPublishParams, data []byte) (int, string, api.LibraryPublished) {
	e.t.Helper()
	var made api.LibraryPublished
	if code, msg := send(e.t, "POST", e.url+api.PathLibraryItems, m.token, p, &made); code != http.StatusOK {
		e.t.Fatalf("create: %d %s", code, msg)
	}
	if code, msg := e.put(m.token, partPath(made.ID, made.Version, api.PartSource), data); code != http.StatusNoContent {
		e.t.Fatalf("source: %d %s", code, msg)
	}
	if code, msg := e.put(m.token, partPath(made.ID, made.Version, api.PartThumb), thumb(e.t)); code != http.StatusNoContent {
		e.t.Fatalf("thumb: %d %s", code, msg)
	}
	code, msg := send(e.t, "POST", e.url+submitPath(made.ID, made.Version), m.token, nil, nil)
	return code, msg, made
}

// A small bundle can't grow without end on the server: what a design's document
// would come to is refused before it's made, however the growth is asked for. The
// sizes here are far below what an attacker sends, so that running this before a
// limit existed cost a test run and not the machine.
func TestLibraryDocumentsHaveABudget(t *testing.T) {
	e := newLibEnv(t, Config{})
	ada := e.signIn(1, "ada", "")
	e.live(ada, "First")                      // trusted, so more than three can wait
	file := bytes.Repeat([]byte{0, 1}, 1<<19) // 1 MiB, 1.4 inlined
	read := func(string) ([]byte, error) { return file, nil }
	const anime = "https://cdnjs.cloudflare.com/ajax/libs/animejs/3.2.2/anime.min.js"
	shapes := make([]proto.CanvasElement, 64)
	for i := range shapes {
		shapes[i] = proto.CanvasElement{ID: "s", Type: proto.ShapeImage, Src: "a.png"}
	}
	lines := make([]proto.CanvasElement, 250)
	for i := range lines {
		lines[i] = proto.CanvasElement{ID: "t", Type: proto.ShapeText, Text: "x" + strings.Repeat("\n", 7990) + "x"}
	}
	for name, frame := range map[string]proto.CanvasFrame{
		"a file named many times":  {Name: "x", Width: 800, Height: 600, HTML: strings.Repeat(`assets/a.png `, 64)},
		"a shape showing it, many": {Name: "x", Width: 800, Height: 600, Elements: shapes},
		"a known library, many":    {Name: "x", Width: 800, Height: 600, HTML: strings.Repeat(anime+" ", 3000)},
		"shapes' lines":            {Name: "x", Width: 800, Height: 600, Elements: lines},
	} {
		data, err := designshare.Build(frame, nil, read).Marshal()
		if err != nil {
			t.Fatal(err)
		}
		code, msg, made := e.sendBundle(ada, publishParams(name), data)
		if code != http.StatusUnprocessableEntity {
			t.Errorf("%s: %d %s", name, code, msg)
		}
		if _, err := e.blobs.Get(context.Background(), "library/items/"+made.ID+"/v1/doc"); err == nil {
			t.Errorf("%s: a document was made", name)
		}
	}
	// A design showing its file a few times is what the library is for.
	data, err := designshare.Build(proto.CanvasFrame{Name: "x", Width: 800, Height: 600, HTML: strings.Repeat(`<img src="assets/a.png">`, 4)}, nil, read).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if code, msg, _ := e.sendBundle(ada, publishParams("Fine"), data); code != http.StatusOK {
		t.Errorf("a fair design: %d %s", code, msg)
	}
}
