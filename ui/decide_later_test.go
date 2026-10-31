package ui

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// laterApp is an app on a real daemon whose judge is a local model
// answering as a person would: a typo is easy, an outage urgent, lunch
// quiet; the uses route and chat on. Its main thread's work is posted.
func laterApp(t *testing.T) (*App, chan func()) {
	t.Helper()
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "judge"}}})
			return
		}
		var in struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		q := in.Messages[len(in.Messages)-1].Content
		answer := `{"verdict":"medium","confidence":0.9,"why":"ordinary work"}`
		switch {
		case strings.Contains(q, "fix the typo in the README"):
			answer = `{"verdict":"easy","confidence":0.92,"why":"A one-word fix in one file."}`
		case strings.Contains(q, "<message>") && strings.Contains(q, "down"):
			answer = `{"verdict":"urgent","confidence":0.95,"why":"checkout is down for users"}`
		case strings.Contains(q, "<message>"):
			answer = `{"verdict":"quiet","confidence":0.9,"why":"small talk"}`
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": answer}}}})
	}))
	t.Cleanup(model.Close)
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	a1, b1 := net.Pipe()
	go srv.ServeConn(a1)
	cl, err := client.New(b1, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cl.Close() })
	on := proto.DecideUse{On: true}
	if _, err := cl.SetDecideSettings(proto.DecideSettings{Policy: proto.DecideLocalOnly,
		Own:  []proto.Judge{{Kind: proto.JudgeLocal, URL: model.URL + "/v1", Model: "judge"}},
		Uses: map[string]proto.DecideUse{proto.DecideRoute: on, proto.DecideChat: on}}); err != nil {
		t.Fatal(err)
	}
	app := New(Options{})
	posted := make(chan func(), 64)
	app.onMain = func(fn func()) { posted <- fn }
	app.client, app.agents = cl, agent.List()
	return app, posted
}

// drain runs the main thread's posted work until done, drawing tt.
func drain(t *testing.T, tt *ui.Tester, posted chan func(), done func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for !done() {
		select {
		case fn := <-posted:
			fn()
		case <-time.After(30 * time.Millisecond):
		}
		if tt != nil {
			tt.Frame()
		}
		if time.Now().After(deadline) {
			t.Fatalf("never: %s", what)
		}
	}
}

// New session suggests a model for the first prompt when the use is on;
// the session starts on it only once the user takes it.
func TestNewSessionSuggestsAModel(t *testing.T) {
	app, posted := laterApp(t)
	app.host.Name = "this Mac"
	app.newSession = newSessionForm{open: true, loaded: true, agent: "claude", prompt: "fix the typo in the README",
		project: proto.ProjectDetail{ProjectInfo: proto.ProjectInfo{Name: "shop", Path: t.TempDir()}}}
	app.loadRouteOn(app.client, app.newSession.project.Path)
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).FillWidth().FillHeight().Background(theme.Background).Children(func() { app.newSessionDialog(c) })
	}, 1000, 900)
	drain(t, tt, posted, func() bool { return hasText(tt, "Suggest a model for this prompt") }, "the suggestion offered")
	var p proto.SessionCreateParams
	app.newSession.routeParams(&p)
	if p.Model != "" {
		t.Fatalf("a model before the user took it: %+v", p)
	}
	if err := tt.Click("Suggest a model for this prompt"); err != nil {
		t.Fatal(err)
	}
	drain(t, tt, posted, func() bool { return hasText(tt, "Suggested: Haiku 5.5 · low effort. A one-word fix in one file.") }, "the suggestion")
	shot(t, tt, "new-session-route.png")
	if err := tt.Click("Use it"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	app.newSession.routeParams(&p)
	if p.Model != "claude-haiku-5-5" || p.Effort != "low" || !hasText(tt, "Its own model") {
		t.Fatalf("taken: %+v %q", p, tt.Texts())
	}
}

// With the use chat on, an urgent message notifies though the chat is
// off; a quiet one doesn't, unless it names the user.
func TestChatUrgencyFromTheJudge(t *testing.T) {
	playSound = func([]byte, float64) error { return nil }
	app, posted := laterApp(t)
	app.cloudOf().st.AccountID = "me"
	app.ntf.settings, app.ntf.loaded = app.notifySettings(), true
	send := func(id, text string, mention bool) {
		m := proto.ChatMessage{ID: id, Team: "t1", At: time.Now(), Account: "bob", Name: "Bob Lee", Text: text}
		if mention {
			m.Mentions = []proto.ChatMention{{Kind: proto.MentionUser, ID: "me", Title: "Ada"}}
		}
		data, _ := json.Marshal([]proto.ChatMessage{m})
		app.cloudOf().st.AccountID = "me" // the cloud's status, read meanwhile, has none
		app.teamChatEvent(data)()
	}
	app.ntf.settings.Chat = "off"
	send("1", "checkout is down for everyone", false)
	drain(t, nil, posted, func() bool { return len(app.chatToasts) == 1 }, "the urgent card")
	if app.chatToasts[0].urgent != "checkout is down for users" {
		t.Fatalf("card %+v", app.chatToasts[0])
	}
	app.ntf.settings.Chat = ""
	send("2", "lunch at noon?", false)
	send("3", "lunch at noon, Ada?", true)
	drain(t, nil, posted, func() bool { return len(app.chatToasts) == 2 }, "the mention's card")
	drain(t, nil, posted, func() bool { l, _ := app.client.DecideLog(proto.DecideLogParams{}); return len(l) == 3 }, "both weighed")
	time.Sleep(100 * time.Millisecond) // the answer of the last on its way to the main thread
	for len(posted) > 0 {
		(<-posted)()
	}
	if len(app.chatToasts) != 2 || app.chatToasts[1].msg.ID != "3" {
		t.Fatalf("cards %d", len(app.chatToasts))
	}
}
