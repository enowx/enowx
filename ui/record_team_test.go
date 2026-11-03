//go:build record

package ui

import (
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net"
	"os"
	"os/exec"
	"path/filepath"
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
)

// The recording of the team's tools for the README and the website: the
// real app, its daemon and a team's space, played through a scene and
// drawn frame by frame (RECORD=folder, frames at 30 a second, made into a
// video by ffmpeg). go test -tags record ./ui -run TestRecordTeam
func TestRecordTeam(t *testing.T) {
	out := os.Getenv("RECORD")
	if out == "" {
		t.Skip("RECORD names the folder the frames go to")
	}
	// A copy of this repository, a team's project, in ~/code as the
	// recording's home shows it.
	src, _ := filepath.Abs("..")
	home := t.TempDir()
	repo := filepath.Join(home, "code", "enowx")
	if out, err := exec.Command("git", "clone", "-q", "--depth", "40", "file://"+src, repo).CombinedOutput(); err != nil {
		t.Fatalf("clone: %v %s", err, out)
	}
	exec.Command("git", "-C", repo, "remote", "set-url", "origin", "https://github.com/enowx/enowx.git").Run()
	exec.Command("git", "-C", repo, "checkout", "-q", "-b", "release-0.4").Run()
	dir := t.TempDir()
	const team = "3043d082da38fa04"
	hostauth.Open(filepath.Join(dir, "cloud")).Put(hostauth.State{URL: "http://127.0.0.1:9", Token: "enxc_demo", AccountID: "acc-enow", Account: "Enow", GitHub: "enowdev"})
	space := filepath.Join(dir, "teams", team)
	os.MkdirAll(filepath.Join(space, "chat"), 0o700)
	os.WriteFile(filepath.Join(space, "team.json"), []byte(`{"team":"`+team+`","name":"enowX","role":"owner","account":"acc-enow"}`), 0o600)
	projects, _ := json.Marshal([]map[string]any{{"id": "p1", "name": "enowx", "repo": "https://github.com/enowx/enowx", "added": time.Now().Add(-72 * time.Hour), "addedBy": "Enow"}})
	os.WriteFile(filepath.Join(space, "projects.json"), projects, 0o600)
	start := time.Now().Add(-40 * time.Minute)
	say := func(i int, account, name, github, text string, mentions []proto.ChatMention) {
		at := start.Add(time.Duration(i) * 3 * time.Minute).UTC()
		m := proto.ChatMessage{At: at, Account: account, Name: name, GitHub: github, Text: text, Mentions: mentions}
		data, _ := json.Marshal(m)
		os.WriteFile(filepath.Join(space, "chat", fmt.Sprintf("%019d-%08x.json", at.UnixNano(), i)), data, 0o600)
	}
	say(0, "acc-rina", "Rina", "rinadev", "Morning! The invite page is live, joining works from the browser now", nil)
	say(1, "acc-rina", "Rina", "rinadev", "Can someone look at the release notes before we tag?", nil)
	say(2, "acc-enow", "Enow", "enowdev", "On it after lunch", nil)
	say(4, "acc-dimas", "Dimas", "dimas", "@Enow I shared my build box with the team, the canvas review is there", []proto.ChatMention{{Kind: proto.MentionUser, ID: "acc-enow", Title: "Enow"}})

	srv, err := daemon.NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	app := New(Options{Hosts: mustHosts(t, dir)})
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
	cl.SaveNote(proto.Note{NoteInfo: proto.NoteInfo{Title: "Release plan", Team: team}, Body: "# 0.4.0\n\n- [x] team cloud\n- [ ] website"})
	cl.SaveNote(proto.Note{NoteInfo: proto.NoteInfo{Title: "Release checklist", Team: team}, Body: "- [ ] tag"})
	if _, err := cl.AddProject(repo); err != nil {
		t.Fatal(err)
	}
	app.setProjects([]proto.ProjectInfo{{Path: repo, Name: "enowx", IsGit: true, Branch: "release-0.4"}})
	app.daemonInfo.Home = home
	for _, s := range []struct{ name, cmd string }{
		{"release notes", "git --no-pager log --oneline --no-decorate -16; PS1='~/code/enowx $ ' exec sh"},
		{"invite page", "git --no-pager log --oneline --no-decorate -8 -- cloud; PS1='~/code/enowx $ ' exec sh"},
	} {
		info, err := cl.CreateSession(proto.SessionCreateParams{Project: repo, Name: s.name, Cols: 100, Rows: 30, Command: []string{"sh", "-c", s.cmd}, Env: []string{"HOME=" + home, "TERM=xterm-256color"}})
		if err != nil {
			t.Fatal(err)
		}
		app.upsert(info)
	}
	app.selectSession(app.sessions[0])
	app.state.Panel.Open, app.state.Panel.Tab = true, panelChat

	const w, h, scale = 1400, 875, 2
	tt := ui.NewTester(func(c *ui.Context) { app.view(c) }, w, h)
	tt.SetScale(scale)
	pump := func() {
		tt.Advance(time.Second / 30) // the app's motion, as on screen
		tt.Frame()
		time.Sleep(15 * time.Millisecond)
		mu.Lock()
		fns := later
		later = nil
		mu.Unlock()
		for _, fn := range fns {
			fn()
		}
	}
	settle := func(what string, done func() bool) {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for !done() {
			if time.Now().After(deadline) {
				t.Fatalf("%s: never; shows %q", what, tt.Texts())
			}
			pump()
		}
		for range 5 {
			pump()
		}
	}
	frame := 0
	os.MkdirAll(out, 0o755)
	keep := func(img image.Image, seconds float64) {
		path := filepath.Join(out, "f.png")
		f, _ := os.Create(path)
		png.Encode(f, img)
		f.Close()
		for range int(seconds * 30) {
			frame++
			os.Link(path, filepath.Join(out, fmt.Sprintf("%05d.png", frame)))
		}
		os.Remove(path)
	}
	hold := func(seconds float64) { pump(); keep(tt.Image(), seconds) }

	settle("the team's chat", func() bool { return tt.HasText("Can someone look at the release notes before we tag?") })
	app.chatsOf().members[team] = []api.Member{
		{Account: api.Account{ID: "acc-enow", Name: "Enow", GitHub: "enowdev"}, Role: api.RoleOwner},
		{Account: api.Account{ID: "acc-rina", Name: "Rina", GitHub: "rinadev"}, Role: api.RoleAdmin},
		{Account: api.Account{ID: "acc-dimas", Name: "Dimas", GitHub: "dimas"}, Role: api.RoleMember},
		{Account: api.Account{ID: "acc-sari", Name: "Sari", GitHub: "sari-codes"}, Role: api.RoleViewer},
	}
	app.chatsOf().membersAt[team] = time.Now().Add(time.Hour)
	app.presenceOf().visitors = []proto.Visitor{{Account: "acc-dimas", Name: "Dimas", GitHub: "dimas", Team: team, Role: api.RoleMember, Watching: []string{app.sessions[1].ID}}}
	settle("the sessions drawn", func() bool { return tt.HasText("release notes") })
	hold(2.5)

	// A message comes from the team.
	say(6, "acc-rina", "Rina", "rinadev", "Pushed the changelog, 0.4.0 is ready to tag", nil)
	app.loadChat(team) // as the chat reads what the team's space holds
	settle("Rina's message", func() bool { return tt.HasText("Pushed the changelog, 0.4.0 is ready to tag") })
	hold(1.8)

	// The user answers, mentioning the team's note.
	tt.Click("Message")
	typed := func(s string, every float64) {
		for _, r := range s {
			tt.Type(string(r))
			hold(every)
		}
	}
	typed("Checked the notes against @rel", 0.06)
	settle("the mention menu", func() bool { return tt.HasText("Mention") && tt.HasText("Release plan") })
	hold(1.4)
	tt.Key(0, ui.KeyEnter)
	settle("the note mentioned", func() bool { return app.chatsOf().draft == "Checked the notes against @Release plan " })
	hold(0.5)
	typed("and they match. Tagging now", 0.06)
	hold(0.6)
	tt.Key(0, ui.KeyEnter)
	settle("the message sent", func() bool { return tt.HasText("Tagging now") && app.chatsOf().draft == "" })
	hold(2.5)

	// The team's projects, and who is on this Mac.
	tt.Click("Projects")
	settle("the projects", func() bool { return tt.HasText("enowx") && tt.HasText("on this Mac") })
	hold(2.6)
	tt.Click("Members")
	settle("the members", func() bool { return tt.HasText("Sari") })
	hold(2.6)
	tt.Click("Chat")
	settle("the chat again", func() bool { return tt.HasText("Tagging now") })
	hold(1.5)
	t.Logf("%d frames in %s", frame, out)
}

func mustHosts(t *testing.T, dir string) *hosts.Store {
	s, err := hosts.Open(filepath.Join(dir, "hosts.json"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}
