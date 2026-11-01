//go:build !windows

package ui

import (
	"bytes"
	"image"
	"image/png"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/cloud/hostauth"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// The side panel's Team group: the team's chat, a message typed in it
// mentioning the team's note with "@", sent with Enter, edited and
// deleted; the group's projects and members.
func TestTeamChatInTheSidePanel(t *testing.T) {
	dir := t.TempDir()
	const team = "0123456789abcdef"
	hostauth.Open(filepath.Join(dir, "cloud")).Put(hostauth.State{URL: "https://cloud.test", Token: "enxc_test", AccountID: "acc1", Account: "Ada"})
	os.MkdirAll(filepath.Join(dir, "teams", team), 0o700)
	os.WriteFile(filepath.Join(dir, "teams", team, "team.json"), []byte(`{"team":"`+team+`","name":"Design","role":"owner","account":"acc1"}`), 0o600)
	// Bob wrote first, twice in a row, mentioning Ada.
	chat := filepath.Join(dir, "teams", team, "chat")
	os.MkdirAll(chat, 0o700)
	at := time.Now().Add(-20 * time.Minute).UTC()
	os.WriteFile(filepath.Join(chat, "0000000000000000001-aaaaaaaa.json"), []byte(`{"at":"`+at.Format(time.RFC3339)+`","account":"acc2","name":"Bob","github":"bob","text":"morning @Ada, the canvas is ready","mentions":[{"kind":"user","id":"acc1","title":"Ada"}]}`), 0o600)
	os.WriteFile(filepath.Join(chat, "0000000000000000002-bbbbbbbb.json"), []byte(`{"at":"`+at.Add(time.Minute).Format(time.RFC3339)+`","account":"acc2","name":"Bob","github":"bob","text":"have a look when you can"}`), 0o600)
	srv, err := daemon.NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	app := New(Options{})
	var mu sync.Mutex
	var later []func()
	app.onMain = func(fn func()) { mu.Lock(); later = append(later, fn); mu.Unlock() }
	a, b := net.Pipe()
	go srv.ServeConn(a)
	cl, err := client.New(b, func(ev proto.Event) { app.handleEvent(1, ev) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cl.Close() })
	app.gen, app.client = 1, cl
	app.host = hosts.Host{ID: hosts.LocalID, Name: "this Mac"}
	app.state.Space = team
	cl.SaveNote(proto.Note{NoteInfo: proto.NoteInfo{Title: "Roadmap", Team: team}})
	app.state.Panel.Open, app.state.Panel.Tab = true, panelChat
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).Width(460).Height(640).Background(theme.Background).TextColor(theme.Text).Children(func() {
			app.panelStrip(c)
			switch app.panelTab() {
			case panelTeamProjects:
				app.teamProjectsTab(c)
			case panelTeamMembers:
				app.teamMembersTab(c)
			default:
				app.chatTab(c)
			}
		})
	}, 460, 640)
	settle := func(what string, done func() bool) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for {
			tt.Frame()
			if done() {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: never; shows %q", what, tt.Texts())
			}
			time.Sleep(10 * time.Millisecond)
			mu.Lock()
			fns := later
			later = nil
			mu.Unlock()
			for _, fn := range fns {
				fn()
			}
		}
	}
	settle("the team's chat", func() bool { return tt.HasText("Team") && tt.HasText("have a look when you can") })
	if !tt.HasText("Bob") || !tt.HasText("@Ada") {
		t.Fatalf("Bob's messages, under his name, mentioning Ada: %q", tt.Texts())
	}
	app.notesOf()
	settle("the team's note to mention", func() bool { return len(app.notesOf().list) == 1 })
	// "@" and a few letters offer the note; Enter picks it, then sends.
	if err := tt.Click("Mention (@)"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Type("road")
	settle("the mention menu", func() bool { return tt.HasText("Roadmap") && tt.HasText("MENTION") })
	shot(t, tt, "team-chat-mention.png")
	tt.Key(0, ui.KeyEnter)
	settle("the note mentioned", func() bool { return app.chatsOf().draft == "@Roadmap " })
	tt.Type("is up")
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	settle("the message", func() bool { return tt.HasText("is up") && len(app.chatsOf().byTeam[team]) == 3 })
	if m := app.chatsOf().byTeam[team][2]; m.Text != "@Roadmap is up" || len(m.Mentions) != 1 || m.Mentions[0].Title != "Roadmap" {
		t.Fatalf("the message: %+v", m)
	}
	if app.chatsOf().draft != "" {
		t.Fatalf("the draft after sending: %q", app.chatsOf().draft)
	}
	// Up in the empty composer edits the last message; hovering it offers
	// to delete it, once confirmed.
	// Which messages are the user's waits on the account, read in the
	// background: Up before it is known edits nothing.
	settle("the account", func() bool { return app.cloudOf().st.AccountID == "acc1" })
	tt.Key(0, ui.KeyUp)
	settle("editing", func() bool { return tt.HasText("Editing your message · Esc to cancel") })
	tt.Type(" now")
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	settle("the edit", func() bool {
		m := app.chatsOf().byTeam[team]
		return len(m) == 3 && m[2].Text == "@Roadmap is up now" && m[2].Edited != nil && tt.HasText("  (edited)")
	})
	shot(t, tt, "team-chat.png")
	r, ok := tt.Find("@Roadmap is up now  (edited)")
	if !ok {
		t.Fatal("the edited message isn't shown")
	}
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	tt.Frame()
	shot(t, tt, "team-chat-hover.png")
	if err := tt.Click("Delete"); err != nil {
		t.Fatalf("no Delete on hover: %v; shows %q", err, tt.Texts())
	}
	if app.confirming == nil {
		t.Fatal("deleted without asking")
	}
	app.confirming.do()
	settle("the message gone", func() bool { return len(app.chatsOf().byTeam[team]) == 2 && !tt.HasText("is up now") })

	// A message whose mention shows as a chip, not in its text, keeps it
	// through an edit.
	if _, err := cl.SendChat(proto.ChatSendParams{Team: team, Text: "ni bagus", Mentions: []proto.ChatMention{{Kind: proto.MentionCanvas, ID: "c1", Title: "Untitled canvas"}}}); err != nil {
		t.Fatal(err)
	}
	settle("the message with a chip", func() bool { return len(app.chatsOf().byTeam[team]) == 3 })
	app.startEditing(app.chatsOf().byTeam[team][2])
	app.chatsOf().draft = "ni bagus banget"
	app.sendChat(team, app.chatsOf().draft)
	settle("the edit keeping its chip", func() bool {
		m := app.chatsOf().byTeam[team][2]
		return m.Text == "ni bagus banget" && len(m.Mentions) == 1 && m.Mentions[0].Title == "Untitled canvas"
	})
	// Bob's message answered: the answer quotes it. Reacted to, the
	// reaction counts under it; Bob typing shows under the chat.
	asked := app.chatsOf().byTeam[team][1]
	app.startReply(asked)
	settle("replying", func() bool { return tt.HasText("Replying to") })
	app.chatsOf().draft = "on it"
	app.sendChat(team, "on it")
	settle("the answer, quoting", func() bool {
		m := app.chatsOf().byTeam[team]
		n := 0
		for _, x := range tt.Texts() {
			if x == "have a look when you can" {
				n++
			}
		}
		return len(m) == 4 && m[3].ReplyTo == asked.ID && n == 2 && !tt.HasText("Replying to")
	})
	if _, err := cl.ReactChat(proto.ChatReactParams{Team: team, ID: asked.ID, Emoji: "👍"}); err != nil {
		t.Fatal(err)
	}
	settle("the reaction", func() bool { return tt.HasText("👍") && len(app.chatsOf().byTeam[team][1].Reactions) == 1 })
	hc := app.chatsOf()
	hc.members[team] = []api.Member{{Account: api.Account{ID: "acc2", Name: "Bob"}, Role: api.RoleMember}}
	hc.membersAt[team] = time.Now().Add(time.Hour)
	app.hereOf().byTeam[team] = api.Presence{Online: []string{"acc1", "acc2"}, Typing: []string{"acc1", "acc2"}}
	settle("who types", func() bool { return tt.HasText("Bob is typing…") && tt.HasText("2 online") })
	shot(t, tt, "team-chat-reactions.png")

	// An image in a message shows in its bubble, and opens large on a
	// click; one that doesn't upload says why, and isn't sent.
	var pic bytes.Buffer
	red := image.NewNRGBA(image.Rect(0, 0, 80, 40))
	for i := 0; i < len(red.Pix); i += 4 {
		red.Pix[i], red.Pix[i+3] = 220, 255
	}
	png.Encode(&pic, red)
	att := proto.ChatAttachment{ID: strings.Repeat("ab", 16), Name: "screen.webp", Type: "image/webp", Size: pic.Len(), Width: 80, Height: 40, Epoch: 1}
	bmp, _ := ui.DecodeBitmap(pic.Bytes())
	hc.images[att.ID] = &chatImage{bmp: bmp, data: pic.Bytes(), asked: true}
	if _, err := cl.SendChat(proto.ChatSendParams{Team: team, Attachments: []proto.ChatAttachment{att}}); err != nil {
		t.Fatal(err)
	}
	settle("the image", func() bool { _, ok := tt.Find("Image screen.webp"); return ok })
	if err := tt.Click("Image screen.webp"); err != nil {
		t.Fatal(err)
	}
	settle("the image opened", func() bool { return hc.viewing != nil && tt.HasText("screen.webp") })
	shot(t, tt, "team-chat-image.png")
	tt.Key(0, ui.KeyEscape)
	settle("the image closed", func() bool { return hc.viewing == nil })
	app.addChatImage(team, "photo.png", pic.Bytes())
	settle("the upload failing", func() bool { return len(hc.pending) == 1 && hc.pending[0].err != "" })
	if app.sendChat(team, "") {
		t.Fatal("a message of a failed image only was sent")
	}
	hc.pending = nil

	// A canvas:// link in a message opens the design it names.
	if _, err := cl.SendChat(proto.ChatSendParams{Team: team, Text: "cek canvas://cb04ed75ff0e/design-14 ya"}); err != nil {
		t.Fatal(err)
	}
	settle("the link", func() bool { return tt.HasText("canvas://cb04ed75ff0e/design-14") })
	if err := tt.Click("canvas://cb04ed75ff0e/design-14"); err != nil {
		t.Fatal(err)
	}
	if cv := app.canvasesOf(); app.page != pageCanvas || cv.open != "cb04ed75ff0e" || cv.frame != "design-14" {
		t.Fatalf("opened page %v, canvas %q, design %q", app.page, cv.open, cv.frame)
	}
	app.page = ""
	settle("back to the chat", func() bool { return tt.HasText("ya") })

	// The group's other tools: the team's projects, its members.
	if err := tt.Click("Projects"); err != nil {
		t.Fatal(err)
	}
	settle("the projects", func() bool { return tt.HasText("No projects yet") })
	if err := tt.Click("Members"); err != nil {
		t.Fatal(err)
	}
	settle("the members, who is online", func() bool { return tt.HasText("Bob") && tt.HasText("online") })
}

// A message coming before the chat was ever opened isn't its history: the
// chat still reads what came before it, and says so while it reads.
func TestTeamChatReadsItsHistoryAfterAnEvent(t *testing.T) {
	dir := t.TempDir()
	const team = "0123456789abcdef"
	hostauth.Open(filepath.Join(dir, "cloud")).Put(hostauth.State{URL: "https://cloud.test", Token: "enxc_test", AccountID: "acc1", Account: "Ada"})
	os.MkdirAll(filepath.Join(dir, "teams", team, "chat"), 0o700)
	os.WriteFile(filepath.Join(dir, "teams", team, "team.json"), []byte(`{"team":"`+team+`","name":"Design","role":"owner","account":"acc1"}`), 0o600)
	at := time.Now().Add(-20 * time.Minute).UTC().Format(time.RFC3339)
	os.WriteFile(filepath.Join(dir, "teams", team, "chat", "0000000000000000001-aaaaaaaa.json"), []byte(`{"at":"`+at+`","account":"acc2","name":"Bob","text":"the old message"}`), 0o600)
	srv, err := daemon.NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	app := New(Options{})
	var mu sync.Mutex
	var later []func()
	app.onMain = func(fn func()) { mu.Lock(); later = append(later, fn); mu.Unlock() }
	a, b := net.Pipe()
	go srv.ServeConn(a)
	cl, err := client.New(b, func(proto.Event) {})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cl.Close() })
	app.gen, app.client = 1, cl
	app.host = hosts.Host{ID: hosts.LocalID, Name: "this Mac"}
	app.state.Space = team
	app.teamChatEvent([]byte(`[{"id":"0000000000000000002-bbbbbbbb","team":"` + team + `","account":"acc2","name":"Bob","text":"the new message"}]`))()
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).Width(460).Height(640).Background(theme.Background).TextColor(theme.Text).Children(func() { app.chatTab(c) })
	}, 460, 640)
	deadline := time.Now().Add(10 * time.Second)
	for !(tt.HasText("the old message") && tt.HasText("the new message")) {
		tt.Frame()
		mu.Lock()
		fns := later
		later = nil
		mu.Unlock()
		for _, fn := range fns {
			fn()
		}
		if time.Now().After(deadline) {
			t.Fatalf("the history never read: %q", tt.Texts())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := app.chatsOf().byTeam[team]; len(got) != 2 || got[0].Text != "the old message" {
		t.Fatalf("the messages: %+v", got)
	}
}
