package ui

import (
	"bytes"
	"runtime"
	"time"

	"github.com/egoist/mygo"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/diag"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/core/selfprofile"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/transport"
	"github.com/enowx/enowx/version"
)

const (
	// A call this slow or this large is logged, a method at most once in rpcLogEvery.
	rpcLogSlow  = 2 * time.Second
	rpcLogLarge = 4 << 20
	rpcLogEvery = 30 * time.Second
	// A frame this slow is logged, at most once in frameLogEvery, with what was on
	// screen; the watchdog counts the others.
	frameLogSlow  = 250 * time.Millisecond
	frameLogEvery = 30 * time.Second
	// Web views made for renders come in runs.
	webViewLogEvery = 10 * time.Second
	logsSweepEvery  = 6 * time.Hour
)

// startDiag opens the app's log, and has every call through a client noted when slow
// or large. Before the first connection: it says it is the app's.
func (a *App) startDiag() {
	client.Kind = "app"
	dir, err := transport.Dir()
	if err != nil {
		return
	}
	l := diag.Open(dir, diag.AppLog)
	a.diag = l
	l.Start("app", version.String, version.Build(), "os", runtime.GOOS, "arch", runtime.GOARCH)
	mygo.App.OnQuit(func() { l.End("app") })
	trace := func(t client.CallTrace) {
		if t.Took >= rpcLogSlow || t.Sent >= rpcLogLarge || t.Got >= rpcLogLarge {
			l.Every("rpc:"+t.Method, rpcLogEvery, "rpc.slow", "method", t.Method, "took", t.Took, "sentBytes", t.Sent, "gotBytes", t.Got, "err", t.Err)
		}
	}
	client.Trace.Store(&trace)
	go func() {
		for {
			if s := diag.Sweep(dir, time.Now()); s.Files > 0 {
				l.Event("logs.swept", "files", s.Files, "bytes", s.Bytes)
			}
			time.Sleep(logsSweepEvery)
		}
	}()
}

// logPage notes the user moving to another page.
func (a *App) logPage() {
	if a.page == a.diagPage {
		return
	}
	a.diag.Event("page.change", "from", pageName(a.diagPage), "to", pageName(a.page))
	a.diagPage = a.page
}

func pageName(page string) string {
	if page == "" {
		return "sessions"
	}
	return page
}

// frameEnded ends the frame the watchdog watches, noting a slow one.
func (a *App) frameEnded() {
	if d := a.watchdog.FrameEnd(); d >= frameLogSlow {
		a.diag.Every("frame.slow", frameLogEvery, "frame.slow", "took", d, "screen", a.watchdogScreen())
	}
}

// logClosed notes connection c to host h having ended, and why.
func (a *App) logClosed(what string, h hosts.Host, c *client.Client) {
	a.diag.Event("conn.closed", "use", what, "host", h.ID, "name", h.Name, "conn", c.ID(), "lasted", c.Since(), "by", c.ClosedBy(), "why", c.Err())
}

// selfProfile measures this app (`enowx-app profile`). It runs off the main thread.
func (a *App) selfProfile(p proto.ProfileParams) proto.ProcessProfile {
	return selfprofile.Take(selfprofile.Options{Kind: "app", Version: version.String, Params: p, Areas: a.profileAreas})
}

// profileFile is the profile a bundle holds: this app and the connected daemon's, or
// this app alone with a daemon that can't say.
func (a *App) profileFile(cl *client.Client) proto.DiagFile {
	var got proto.Profile
	if cl == nil || cl.Call(proto.MethodDaemonProfile, proto.ProfileParams{}, &got) != nil {
		me := a.selfProfile(proto.ProfileParams{})
		return proto.DiagFile{Name: diag.ProfileFile, Text: diag.Scrub(selfprofile.TextOf(me))}
	}
	return proto.DiagFile{Name: diag.ProfileFile, Text: diag.Scrub(selfprofile.Text(got))}
}

// saveDiagnostics writes this device's logs and the connected host's as a zip for a
// bug report, and says where.
func (a *App) saveDiagnostics() {
	cl, host := a.client, a.host
	a.diag.Event("diagnostics.bundle", "host", host.ID)
	a.diag.Flush()
	go func() {
		sets := map[string][]proto.DiagFile{}
		if dir, err := transport.Dir(); err == nil {
			sets["this device"] = diag.Collect(dir)
		}
		sets["this device"] = append(sets["this device"], a.profileFile(cl))
		about := "enowX " + version.String + " " + runtime.GOOS + "/" + runtime.GOARCH
		if cl != nil && cl.Remote() {
			var got proto.Diagnostics
			if err := cl.Call(proto.MethodDaemonDiagnostics, nil, &got); err == nil {
				sets[host.Name] = got.Files
				about += "\n" + host.Name + ": enowxd " + got.Version
			} else {
				about += "\n" + host.Name + ": its logs couldn't be read: " + diag.Scrub(client.Explain(err))
			}
		}
		var buf bytes.Buffer
		err := diag.WriteZip(&buf, about, sets)
		path := ""
		if err == nil {
			path, err = saveDownload("enowx-diagnostics-"+time.Now().Format("20060102-150405")+".zip", buf.Bytes())
		}
		a.onMain(func() {
			if err != nil {
				a.notify("Diagnostics weren't saved: " + err.Error())
				return
			}
			a.notify("Diagnostics saved to " + path + ". Logs hold names, IDs, sizes and errors, not your content.")
		})
	}()
}

// profileAreas are what the app keeps per session and per host. Those maps belong to
// the main thread; a window that doesn't answer in time gets a profile without them.
func (a *App) profileAreas() []proto.ProfileArea {
	got := make(chan []proto.ProfileArea, 1)
	a.quietly(func() {
		pictures, held := a.canvases.PicturesHeld()
		got <- []proto.ProfileArea{
			{Name: "canvas pictures", Count: pictures, Bytes: held},
			{Name: "terminals (a session's or a tab's)", Count: len(a.panes)},
			{Name: "chat views", Count: len(a.chats)},
			{Name: "diff views", Count: len(a.diffs)},
			{Name: "file views", Count: len(a.fileViews)},
			{Name: "files open in tabs", Count: len(a.docs)},
			{Name: "browsers", Count: len(a.browsers)},
			{Name: "activity views", Count: len(a.activityViews)},
			{Name: "check views", Count: len(a.checkViews)},
			{Name: "port views", Count: len(a.portViews)},
			{Name: "other hosts watched", Count: len(a.watches)},
		}
	})
	select {
	case areas := <-got:
		return areas
	case <-time.After(3 * time.Second):
		return nil
	}
}
