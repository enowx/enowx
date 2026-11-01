//go:build record

package ui

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	imgdraw "image/draw"
	"image/png"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	xdraw "golang.org/x/image/draw"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// The recordings of GOD of Agents, RAG and the judge for the website: the
// real app and its daemon with sample data, played through a scene and
// drawn frame by frame, a pointer and a camera on top, a caption under it.
// RECORD=folder go test -tags record ./ui -run TestRecordReel

const (
	reelW, reelH = 1400, 875
	reelCaption  = 72
	reelScale    = 2
	reelFPS      = 24
)

// reel is one recording: the app under test, what the camera and pointer
// are at, and the frames written so far.
type reel struct {
	t       *testing.T
	app     *App
	cl      *client.Client
	tt      *ui.Tester
	dir     string
	frame   int
	mu      sync.Mutex
	later   []func()
	cam     [3]float64 // centre x, y (DIPs) and zoom
	px, py  float64    // the pointer
	press   float64
	caption string
	capFade float64
	capTest *ui.Tester
	capText string
	capCol  float64
}

func newReel(t *testing.T, name string, dataDir string, ev func(*App, proto.Event)) *reel {
	t.Helper()
	out := os.Getenv("RECORD")
	if out == "" {
		t.Skip("RECORD names the folder the frames go to")
	}
	srv, err := daemon.NewServer(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	r := &reel{t: t, app: New(Options{Hosts: mustHosts(t, dataDir)}), dir: filepath.Join(out, name), cam: [3]float64{reelW / 2, reelH / 2, 1}, px: reelW * 0.6, py: reelH * 0.6}
	os.RemoveAll(r.dir)
	os.MkdirAll(r.dir, 0o755)
	r.app.onMain = func(fn func()) { r.mu.Lock(); r.later = append(r.later, fn); r.mu.Unlock() }
	a, b := net.Pipe()
	go srv.ServeConn(a)
	cl, err := client.New(b, func(e proto.Event) {
		r.app.handleEvent(1, e)
		if ev != nil {
			ev(r.app, e)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cl.Close() })
	r.cl, r.app.gen, r.app.client, r.app.agents = cl, 1, cl, agent.List()
	r.app.host = hosts.Host{ID: hosts.LocalID, Name: "this Mac"}
	r.tt = ui.NewTester(func(c *ui.Context) { r.app.view(c) }, reelW, reelH)
	r.tt.SetScale(reelScale)
	r.capTest = ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Row(c).Fill().Background(ui.RGBA(0, 0, 0, 1)).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
			ui.Text(c, r.capText).FontSize(25).FontWeight(500).TextColor(ui.RGBA(255, 255, 255, float32(r.capCol)))
		})
	}, reelW, reelCaption)
	r.capTest.SetScale(1)
	return r
}

func (r *reel) pump() {
	r.tt.Advance(time.Second / reelFPS)
	r.tt.Frame()
	time.Sleep(8 * time.Millisecond)
	r.mu.Lock()
	fns := r.later
	r.later = nil
	r.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
}

func (r *reel) settle(what string, done func() bool) {
	r.t.Helper()
	deadline := time.Now().Add(25 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			r.t.Fatalf("%s: never; shows %q", what, r.tt.Texts())
		}
		r.pump()
	}
	for range 4 {
		r.pump()
	}
}

// shoot writes the frame as it is, under the camera, with the pointer and
// the caption.
func (r *reel) shoot() {
	src := r.tt.Image()
	b := src.Bounds()
	zoom := r.cam[2]
	cw, ch := float64(b.Dx())/zoom, float64(b.Dy())/zoom
	cx := math.Min(math.Max(r.cam[0]*reelScale, cw/2), float64(b.Dx())-cw/2)
	cy := math.Min(math.Max(r.cam[1]*reelScale, ch/2), float64(b.Dy())-ch/2)
	from := image.Rect(int(cx-cw/2), int(cy-ch/2), int(cx+cw/2), int(cy+ch/2))
	out := image.NewRGBA(image.Rect(0, 0, reelW, reelH+reelCaption))
	view := image.Rect(0, 0, reelW, reelH)
	xdraw.CatmullRom.Scale(out, view, src, from, xdraw.Src, nil)
	// The pointer, where the camera shows it.
	f := float64(reelW) / float64(from.Dx()) * reelScale
	drawPointer(out, (r.px*reelScale-float64(from.Min.X))/reelScale*f, (r.py*reelScale-float64(from.Min.Y))/reelScale*f, r.press)
	r.capCol = r.capFade
	r.capText = r.caption
	r.capTest.Frame()
	imgdraw.Draw(out, image.Rect(0, reelH, reelW, reelH+reelCaption), r.capTest.Image(), image.Point{}, imgdraw.Src)
	r.frame++
	f2, err := os.Create(filepath.Join(r.dir, fmt.Sprintf("%05d.png", r.frame)))
	if err != nil {
		r.t.Fatal(err)
	}
	(&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(f2, out)
	f2.Close()
}

// hold keeps the scene running for seconds.
func (r *reel) hold(seconds float64) {
	for range int(seconds * reelFPS) {
		r.pump()
		if r.capFade < 1 {
			r.capFade = math.Min(1, r.capFade+0.125)
		}
		r.shoot()
	}
}

func ease(t float64) float64 { return t * t * (3 - 2*t) }

// say changes the caption, fading.
func (r *reel) say(text string) {
	if r.caption != "" {
		for r.capFade > 0 {
			r.capFade = math.Max(0, r.capFade-0.17)
			r.pump()
			r.shoot()
		}
	}
	r.caption = text
}

// glide moves the camera to (x, y, zoom) and the pointer to (px, py) over
// seconds; a negative px leaves the pointer.
func (r *reel) glide(seconds, x, y, zoom, px, py float64) {
	c0, p0, q0 := r.cam, r.px, r.py
	if px < 0 {
		px, py = p0, q0
	}
	n := int(seconds * reelFPS)
	for i := 1; i <= n; i++ {
		e := ease(float64(i) / float64(n))
		r.cam = [3]float64{c0[0] + (x-c0[0])*e, c0[1] + (y-c0[1])*e, c0[2] + (zoom-c0[2])*e}
		r.px, r.py = p0+(px-p0)*e, q0+(py-q0)*e
		r.pump()
		if r.capFade < 1 {
			r.capFade = math.Min(1, r.capFade+0.125)
		}
		r.shoot()
	}
}

// to moves the pointer onto label, the camera kept, and says where it is.
func (r *reel) to(label string, seconds float64) (float64, float64) {
	r.t.Helper()
	box, ok := r.tt.Find(label)
	if !ok {
		r.t.Fatalf("no %q: %q", label, r.tt.Texts())
	}
	x, y := float64(box.X+box.W/2), float64(box.Y+box.H/2)
	r.glide(seconds, r.cam[0], r.cam[1], r.cam[2], x, y)
	return x, y
}

// click moves to label and clicks it, the press seen.
func (r *reel) click(label string) {
	r.t.Helper()
	r.to(label, 0.55)
	r.press = 1
	r.hold(0.12)
	if err := r.tt.Click(label); err != nil {
		r.t.Fatalf("click %q: %v (%q)", label, err, r.tt.Texts())
	}
	r.press = 0
	r.hold(0.2)
}

func (r *reel) typed(s string, every float64) {
	for _, c := range s {
		r.tt.Type(string(c))
		r.hold(every)
	}
}

// drawPointer draws an arrow at (x, y), smaller while pressed.
func drawPointer(img *image.RGBA, x, y, press float64) {
	s := 1.5 * (1 - 0.14*press)
	poly := [][2]float64{{0, 0}, {0, 17}, {4.2, 13.2}, {7, 19.6}, {10, 18.3}, {7.2, 12}, {12.5, 12}}
	inside := func(px, py float64) bool {
		in := false
		for i, j := 0, len(poly)-1; i < len(poly); j, i = i, i+1 {
			xi, yi, xj, yj := poly[i][0], poly[i][1], poly[j][0], poly[j][1]
			if (yi > py) != (yj > py) && px < (xj-xi)*(py-yi)/(yj-yi)+xi {
				in = !in
			}
		}
		return in
	}
	for dy := -3.0; dy < 34; dy += 0.5 {
		for dx := -3.0; dx < 24; dx += 0.5 {
			px, py := dx/s, dy/s
			col := color.RGBA{}
			switch {
			case inside(px, py):
				col = color.RGBA{255, 255, 255, 255}
			case inside(px-1, py) || inside(px+1, py) || inside(px, py-1) || inside(px, py+1):
				col = color.RGBA{0, 0, 0, 255}
			default:
				continue
			}
			img.SetRGBA(int(x+dx), int(y+dy), col)
		}
	}
}

// reelJudge answers as a small local model would, a little after being asked.
func reelJudge(t *testing.T) string {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "qwen3.6-35b-fast"}}})
			return
		}
		var in struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		all := ""
		for _, m := range in.Messages {
			all += m.Content + "\n"
		}
		q := in.Messages[len(in.Messages)-1].Content
		time.Sleep(160 * time.Millisecond)
		answer := `{"verdict":"allow","confidence":0.97,"why":"runs the project's tests, changes nothing"}`
		switch {
		case strings.Contains(q, "How hard") && strings.Contains(q, "copy"):
			answer = `{"verdict":"easy","confidence":0.92,"why":"a small change in one place"}`
		case strings.Contains(q, "How hard") && strings.Contains(q, "igrate"):
			answer = `{"verdict":"hard","confidence":0.93,"why":"changes data other services rely on"}`
		case strings.Contains(q, "How hard"):
			answer = `{"verdict":"medium","confidence":0.88,"why":"a feature across a few files"}`
		case strings.Contains(all, "You write"):
			answer = "### Routing\n\n- Give frontend tasks to Codex, every other task to Claude Code.\n\n### Merging\n\n- Never merge into `main` before asking me and hearing yes.\n\n### Failures\n\n- When a worker fails a task twice, tell me with the failing output before trying again."
		case strings.Contains(q, "rm -rf"):
			answer = `{"verdict":"refuse","confidence":0.99,"why":"it wipes the home folder"}`
		case strings.Contains(q, "git status"):
			answer = `{"verdict":"allow","confidence":0.99,"why":"a read-only git command"}`
		case strings.Contains(q, "git push"):
			answer = `{"verdict":"ask","confidence":0.93,"why":"it publishes to the shared main branch"}`
		case strings.Contains(q, "Proceed? (y/n)"):
			answer = `{"verdict":"waiting","confidence":0.94,"why":"it asks whether to apply the migration"}`
		case strings.Contains(q, "<steps>"):
			answer = `{"verdict":"looping","confidence":0.92,"why":"the same failing test, the same edit, five times"}`
		case strings.Contains(q, "<output>"):
			answer = `{"verdict":"trimmed","confidence":0.91,"why":"one test failed","text":"--- FAIL: TestInvoiceTotal\n    invoice_test.go:42: got 118.00, want 110.00\nFAIL"}`
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": answer}}},
			"usage": map[string]int{"prompt_tokens": 180, "completion_tokens": 24}})
	}))
	// Where a local model server listens, as the recording shows it.
	l, err := net.Listen("tcp", "127.0.0.1:11434")
	if err != nil {
		t.Fatal(err)
	}
	srv.Listener.Close()
	srv.Listener = l
	srv.Start()
	t.Cleanup(srv.Close)
	return "http://localhost:11434/v1"
}

func reelRepo(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755)
		os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644)
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "."}, {"-c", "user.name=demo", "-c", "user.email=demo@example.com", "commit", "-qm", "First"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
}

// reelFakeAgents puts agents on PATH that wait, as one between prompts.
func reelFakeAgents(t *testing.T, names ...string) {
	bin := t.TempDir()
	for _, n := range names {
		os.WriteFile(filepath.Join(bin, n), []byte("#!/bin/sh\nwhile read -r line; do :; done\n"), 0o755)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func reelTranscript(t *testing.T, home, dir string, records ...[2]string) {
	folder := filepath.Join(home, ".claude", "projects", strings.NewReplacer("/", "-", ".", "-", "_", "-").Replace(dir))
	os.MkdirAll(folder, 0o755)
	var b strings.Builder
	parent := ""
	for i, rec := range records {
		id := fmt.Sprintf("r%d", i)
		kind := "user"
		if rec[0] == "a" {
			kind = "assistant"
		}
		line, _ := json.Marshal(map[string]any{"type": kind, "uuid": id, "parentUuid": parent, "cwd": dir, "message": map[string]any{"role": kind, "content": rec[1]}})
		b.Write(line)
		b.WriteByte('\n')
		parent = id
	}
	if err := os.WriteFile(filepath.Join(folder, "goa-demo.jsonl"), []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

// GOD of Agents: started, handed work, its board, a session pointed at, its
// rules written with an AI.
func TestRecordReelGOA(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	reelFakeAgents(t, "claude", "codex")
	data := t.TempDir()
	r := newReel(t, "goa", data, func(a *App, ev proto.Event) { a.goaEvent(ev) })
	web, api := filepath.Join(home, "code", "shop-web"), filepath.Join(home, "code", "shop-api")
	reelRepo(t, web, map[string]string{"README.md": "# Shop web\n"})
	reelRepo(t, api, map[string]string{"README.md": "# Shop API\n"})
	if _, err := r.cl.SetDecideSettings(proto.DecideSettings{Policy: proto.DecideLocalOnly,
		Own: []proto.Judge{{Kind: proto.JudgeLocal, URL: reelJudge(t), Model: "qwen3.6-35b-fast", Label: "Qwen 3.6"}}}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{web, api} {
		if _, err := r.cl.AddProject(p); err != nil {
			t.Fatal(err)
		}
	}
	r.app.setProjects([]proto.ProjectInfo{{Path: web, Name: "shop-web", IsGit: true, Branch: "main"}, {Path: api, Name: "shop-api", IsGit: true, Branch: "main"}})
	r.app.state.RailWide = true
	r.settle("the chip", func() bool { return r.tt.HasText("GOD of Agents") && r.app.goa.host == hosts.LocalID })

	r.say("GOD of Agents: one session that drives all the others")
	r.hold(0.8)
	r.click("One session controlling every session on every host")
	r.settle("the agents", func() bool { return r.tt.HasText("Start GOD of Agents") && r.tt.HasText("enforced") })
	r.hold(1.8)
	r.click("Start GOA")
	r.settle("GOA", func() bool { s, ok := r.app.goaSession(); return ok && r.app.goaOn() && r.app.selected == s.ID })
	g, _ := r.app.goaSession()
	r.hold(0.8)

	r.say("Say what you want. It splits the work and hands it over")
	if err := r.tt.Click("Message"); err != nil {
		t.Fatal(err)
	}
	ask := "Fix the copy on the empty cart, add dark mode to the web app, and migrate the orders table"
	r.typed(ask, 0.022)
	r.hold(0.35)
	r.tt.Key(0, ui.KeyEnter)
	reelTranscript(t, home, g.Dir, [2]string{"u", ask})
	r.hold(0.7)
	delegate := func(project, agentID, title, prompt string) proto.GOATask {
		var task proto.GOATask
		if err := r.cl.Call(proto.MethodGOADelegate, proto.GOADelegateParams{From: g.ID, Agent: agentID, Project: project, Title: title, Prompt: prompt}, &task); err != nil {
			t.Fatal(err)
		}
		return task
	}
	reply := "Three tasks. The copy goes to a small model, dark mode to Codex, the migration to the strongest model, as it touches data other services read."
	reelTranscript(t, home, g.Dir, [2]string{"u", ask}, [2]string{"a", reply})
	r.hold(0.6)
	first := delegate(web, "claude", "Fix the empty cart copy", "Fix the copy on the empty cart page")
	r.hold(0.5)
	delegate(web, "codex", "Dark mode", "Add dark mode to the web app, following the system's setting")
	r.hold(0.5)
	delegate(api, "claude", "Migrate orders", "Migrate the orders table to the new schema")
	r.settle("the workers", func() bool { return r.tt.HasText("Migrate orders · working") })
	r.glide(0.8, 1130, 300, 1.45, -1, 0)
	r.hold(1.6)

	r.say("Each task gets the model it needs, and the reason why")
	r.click("Tasks")
	r.settle("the board", func() bool { return r.tt.HasText("Migrate orders") })
	r.hold(3)
	if err := r.cl.Call(proto.MethodGOATask, proto.GOATaskParams{ID: first.ID, Status: proto.GOATaskDone, Result: "Copy changed on the empty cart page"}, nil); err != nil {
		t.Fatal(err)
	}
	r.hold(1.6)

	r.say("Drag a session into the chat to point an agent at it")
	r.click("Sessions")
	r.settle("the sessions", func() bool { return r.tt.HasText("Dark mode · working") })
	r.glide(0.7, reelW/2, reelH/2, 1, -1, 0)
	x, y := r.to("Dark mode · working", 0.5)
	r.tt.Press(float32(x), float32(y))
	r.press = 1
	r.hold(0.15)
	tx, ty := 420.0, 700.0
	n := 22
	for i := 1; i <= n; i++ {
		e := ease(float64(i) / float64(n))
		r.px, r.py = x+(tx-x)*e, y+(ty-y)*e
		r.tt.Move(float32(r.px), float32(r.py))
		r.pump()
		r.shoot()
	}
	r.hold(0.3)
	r.tt.Release(float32(tx), float32(ty))
	r.press = 0
	r.settle("the pointer", func() bool { return strings.Contains(r.app.chats[g.ID].Draft(), "[session ") })
	r.glide(0.7, 470, 690, 1.5, -1, 0)
	r.hold(1.9)
	r.glide(0.6, reelW/2, reelH/2, 1, -1, 0)
	r.app.chats[g.ID].SetDraft("")

	r.say("Give it rules, in your words: an AI writes them clearly")
	r.click("Rules")
	r.settle("the rules", func() bool { return r.app.goa.rules != nil && r.app.goa.rules.loaded && r.tt.HasText("Write with AI") })
	r.glide(0.6, reelW/2, reelH/2, 1.22, -1, 0)
	for _, c := range "frontend goes to codex, the rest to claude. ask me before merging to main. tell me if a worker fails twice" {
		r.app.goa.rules.draft.ask += string(c)
		r.hold(0.018)
	}
	r.hold(0.4)
	r.click("Write with AI")
	r.settle("the rules written", func() bool { return r.app.goa.rules.draft.by != "" })
	r.hold(3.2)
	r.click("Save")
	r.settle("saved", func() bool { return r.app.goa.rules == nil })
	r.glide(0.6, reelW/2, reelH/2, 1, -1, 0)
	r.hold(1)
	t.Logf("%d frames", r.frame)
}

// RAG: a project indexed once the user agrees, searched by meaning, a result
// opened at its lines, a memory kept.
func TestRecordReelRAG(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	reelFakeAgents(t, "claude")
	data := t.TempDir()
	r := newReel(t, "rag", data, func(a *App, ev proto.Event) { a.ragEvent(ev) })
	// An embedder on this machine, as a TEI server would be: words hashed
	// into a vector, so what shares words comes close.
	l, err := net.Listen("tcp", "127.0.0.1:8080")
	if err != nil {
		t.Fatal(err)
	}
	emb := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		var in struct {
			Inputs []string `json:"inputs"`
		}
		json.NewDecoder(q.Body).Decode(&in)
		out := [][]float32{}
		for _, text := range in.Inputs {
			v := make([]float32, 256)
			for _, word := range strings.FieldsFunc(strings.ToLower(text), func(c rune) bool { return c < 'a' || c > 'z' }) {
				h := fnv.New32a()
				h.Write([]byte(word))
				v[h.Sum32()%256]++
			}
			out = append(out, v)
		}
		time.Sleep(120 * time.Millisecond)
		json.NewEncoder(w).Encode(out)
	})}
	go emb.Serve(l)
	t.Cleanup(func() { emb.Close() })
	repo := filepath.Join(home, "code", "shop-api")
	reelRepo(t, repo, map[string]string{
		"README.md":             "# Shop API\n\nOrders, invoices and refunds for the shop.\n\n## Taxes\n\nVAT is added to an invoice when it is totalled, by the customer's country.\n",
		"billing/invoice.go":    "package billing\n\n// Total sums an invoice's lines and adds the VAT of the customer's country.\nfunc Total(inv Invoice) Money {\n\tsum := inv.Subtotal()\n\treturn sum.Add(VAT(inv.Customer.Country, sum))\n}\n",
		"billing/vat.go":        "package billing\n\n// VAT is the tax on an amount for a country; orders outside the EU pay none.\nfunc VAT(country string, amount Money) Money {\n\treturn amount.Times(rates[country])\n}\n",
		"orders/refund.go":      "package orders\n\n// Refund gives a customer's money back and restocks the items.\nfunc Refund(o Order) error {\n\treturn payments.Reverse(o.Payment)\n}\n",
		"orders/checkout.go":    "package orders\n\n// Checkout turns a cart into an order and charges the card.\nfunc Checkout(c Cart) (Order, error) {\n\treturn place(c)\n}\n",
		"auth/session.go":       "package auth\n\n// Session keeps a signed-in customer for thirty days.\nfunc NewSession(id string) Session {\n\treturn Session{ID: id}\n}\n",
		"docs/decisions/tax.md": "# Taxes\n\nWe charge VAT at checkout, not at shipping, as finance asked.\n",
	})
	if _, err := r.cl.SetRAGSettings(proto.RAGSettings{Embedder: proto.RAGTEI, Model: "bge-m3", BaseURL: "http://localhost:8080"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.cl.AddProject(repo); err != nil {
		t.Fatal(err)
	}
	r.app.setProjects([]proto.ProjectInfo{{Path: repo, Name: "shop-api", IsGit: true, Branch: "main"}})
	s, err := r.cl.CreateSession(proto.SessionCreateParams{Project: repo, Agent: "claude", Name: "Tax on refunds"})
	if err != nil {
		t.Fatal(err)
	}
	r.app.upsert(s)
	r.app.selectSession(s)
	r.app.state.RailWide = true
	r.app.state.Panel.Open, r.app.state.Panel.Tab = true, panelIndex
	r.settle("offered to index", func() bool { return r.tt.HasText("Index this project…") })

	r.say("RAG: agents find a project's code by what it means")
	r.glide(0.8, 1130, 330, 1.4, -1, 0)
	r.hold(0.7)
	r.click("Index this project…")
	r.settle("the consent", func() bool { return r.tt.HasText("Agree and index") })
	r.say("Nothing leaves until you agree: where it goes, how much, what stays out")
	r.glide(0.7, reelW/2, reelH/2, 1.45, -1, 0)
	r.hold(3.4)
	r.click("Agree and index")
	r.glide(0.6, 1130, 330, 1.4, -1, 0)
	r.say("Indexed in SQLite, or PostgreSQL with pgvector, and kept up to date")
	st := func() proto.RAGStatus { v, _ := r.cl.RAGStatus(proto.RAGRef{Project: repo}); return v }
	for st().Indexed.IsZero() || st().Indexing {
		r.hold(0.2)
	}
	r.hold(2.2)

	r.say("Search by meaning, as agents do: file, line and the part found")
	if err := r.tt.Click("Search by meaning: where are invoices totalled"); err != nil {
		t.Fatal(err)
	}
	r.typed("where is the tax added to an invoice", 0.03)
	r.settle("the results", func() bool { return r.tt.HasText("billing/invoice.go") || len(r.app.ragPanel.hits) > 0 })
	r.hold(3)
	t.Logf("%d frames; hits %+v", r.frame, r.app.ragPanel.hits)
}

// The judge: what can judge on this host, its uses, its log of real
// decisions, and a loop it stopped.
func TestRecordReelDecide(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	reelFakeAgents(t, "claude")
	data := t.TempDir()
	r := newReel(t, "decide", data, func(a *App, ev proto.Event) { a.decideEvent(ev) })
	url := reelJudge(t)
	if _, err := r.cl.SetDecideSettings(proto.DecideSettings{Policy: proto.DecideLocalOnly,
		Own: []proto.Judge{{Kind: proto.JudgeLocal, URL: url, Model: "qwen3.6-35b-fast", Label: "Qwen 3.6, on this Mac"}}}); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(home, "code", "shop-api")
	reelRepo(t, repo, map[string]string{"README.md": "# Shop API\n"})
	if _, err := r.cl.AddProject(repo); err != nil {
		t.Fatal(err)
	}
	r.app.setProjects([]proto.ProjectInfo{{Path: repo, Name: "shop-api", IsGit: true, Branch: "main"}})
	sess, err := r.cl.CreateSession(proto.SessionCreateParams{Project: repo, Agent: "claude", Name: "Fix the invoice total"})
	if err != nil {
		t.Fatal(err)
	}
	r.app.upsert(sess)
	r.app.page, r.app.settingsSection = pageSettings, sectionDecide
	r.app.state.RailWide = true
	r.settle("the judges", func() bool { return r.tt.HasText("Qwen 3.6, on this Mac") })

	r.say("Decision models: a small model answers instead of guessing")
	r.hold(1)
	r.app.testJudges(&r.app.decideSet, "local:"+url+":qwen3.6-35b-fast")
	r.settle("tested", func() bool { return len(r.app.decideSet.testing) == 0 })
	r.hold(2.2)

	r.say("On by default, each use with its own switch and how sure it must be")
	r.click("Uses")
	r.settle("the uses", func() bool { return r.tt.HasText("Approving commands") })
	r.glide(0.7, 800, 360, 1.3, -1, 0)
	r.hold(3.2)
	r.glide(0.6, reelW/2, reelH/2, 1, -1, 0)

	r.say("Every decision is logged with its reason, to mark what was wrong")
	ask := func(a proto.DecideAsk) proto.Decision {
		var d proto.Decision
		if err := r.cl.Call(proto.MethodDecideAsk, a, &d); err != nil {
			t.Fatalf("ask %+v: %v", a, err)
		}
		return d
	}
	ask(proto.DecideAsk{Use: proto.DecideApprove, Command: "go test ./..."})
	ask(proto.DecideAsk{Use: proto.DecideApprove, Command: "git status"})
	ask(proto.DecideAsk{Use: proto.DecideApprove, Command: "git push origin main"})
	ask(proto.DecideAsk{Use: proto.DecideApprove, Command: "rm -rf ~/code"})
	loop := ask(proto.DecideAsk{Use: proto.DecideLoop, Agent: "claude", Steps: []string{"edit invoice.go", "go test ./billing: FAIL TestInvoiceTotal", "edit invoice.go", "go test ./billing: FAIL TestInvoiceTotal", "edit invoice.go", "go test ./billing: FAIL TestInvoiceTotal"}})
	r.click("Log")
	r.settle("the log", func() bool { return r.tt.HasText("it wipes the home folder") })
	r.glide(0.7, 800, 330, 1.25, -1, 0)
	r.hold(3.6)
	r.glide(0.6, reelW/2, reelH/2, 1, -1, 0)

	r.say("It stops a loop before more tokens go, and leaves the call to you")
	loop.Acted, loop.Session = proto.ActedPaused, sess.ID
	r.app.decisions = []*proto.Decision{&loop}
	r.settle("the dialog", func() bool { return r.tt.HasText("Leave it") })
	r.glide(0.6, reelW/2, reelH/2, 1.45, -1, 0)
	r.hold(3)
	t.Logf("%d frames", r.frame)
}
