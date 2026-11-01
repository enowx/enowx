package ui

import (
	"testing"

	"github.com/enowx/enowx/proto"
)

// A path a program prints opens in a panel tab of its session, named as
// the Files tool names it.
func TestLinksOpenFilesInThePanel(t *testing.T) {
	a := New(Options{})
	a.daemonInfo.Home = "/home/enowx"
	parent := proto.SessionInfo{ID: "s1", Dir: "/home/enowx/test"}
	tab := proto.SessionInfo{ID: "s2", Dir: "/home/enowx/test", Parent: "s1"}
	a.sessions = []proto.SessionInfo{parent, tab}

	for link, want := range map[string]string{
		"/home/enowx/test/my-app/main.go:12:3": "my-app/main.go",
		"./my-app/index.html":                  "my-app/index.html",
		"~/notes/todo.md":                      "/home/enowx/notes/todo.md",
		"/tmp/shots/08-fixed.png":              "/tmp/shots/08-fixed.png",
		// As a chat's Markdown links them.
		"sample-web/index.html":             "sample-web/index.html",
		"file:///home/enowx/test/web/a.css": "web/a.css",
	} {
		if !a.openLink("s2", link) {
			t.Fatalf("%s: not opened", link)
		}
		if a.selected != "s1" || a.centerOf("s1").front != fileTabPrefix+want {
			t.Fatalf("%s: selected %q, in front %q, want tab %q", link, a.selected, a.centerOf("s1").front, want)
		}
	}
	if a.openLink("s1", "https://example.com/") {
		t.Fatal("a web page opened in the app")
	}
}

func TestWindowsImagePathsOpenOutsideProject(t *testing.T) {
	a := New(Options{})
	a.daemonInfo.Home = `C:\Users\Julian Fernando`
	a.sessions = []proto.SessionInfo{{ID: "s1", Dir: `C:\Contribute\enowx`}}
	const want = "C:/Users/Julian Fernando/.enowx-app/notes/assets/1fdcf71820cc/icon.png"
	for _, link := range []string{
		`C:\Users\Julian Fernando/.enowx-app/notes/assets/1fdcf71820cc/icon.png`,
		want,
		"<" + want + ">",
		"file:///C:/Users/Julian%20Fernando/.enowx-app/notes/assets/1fdcf71820cc/icon.png",
		"~/.enowx-app/notes/assets/1fdcf71820cc/icon.png",
	} {
		if !a.openLink("s1", link) || a.centerOf("s1").front != fileTabPrefix+want {
			t.Fatalf("%s opened as %q", link, a.centerOf("s1").front)
		}
	}
	for link, expected := range map[string]string{
		`C:\Contribute\enowx\images\icon.png`: "images/icon.png",
		`c:/contribute/enowx/images/icon.png`: "images/icon.png",
		`.\images\icon.png`:                   "images/icon.png",
	} {
		if got := a.linkPath(a.sessions[0], link); got != expected {
			t.Errorf("%s: %q", link, got)
		}
	}
}
