package ui

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"mime"
	"net"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// An HTML file of a session shows as a page in its file tab: a web view
// of the app loads it from a server of the app's on localhost, which reads
// the file, and what it loads next to it (styles, scripts, images, fonts),
// from the session's host through its daemon (file.bytes). So a page of a
// server shows as one of this device does.
//
// Each folder pages show from is a site of the server, at
// /<token>/<site>/: the token, made at random as the app starts, keeps
// other pages on this device from reading the folders, and a site serves
// only what is under its folder.

// pageLimit is the largest file a page loads.
const pageLimit = 64 << 20

// pageSite is a folder pages show from.
type pageSite struct {
	cl      *client.Client
	session string
	dir     string // in the session's folder, or absolute on the host
	abs     bool
}

// pageServer serves the sites. It runs on a goroutine of its own from its
// first use; sites is guarded by mu.
type pageServer struct {
	start sync.Once
	addr  string // host:port, "" when it couldn't listen
	token string
	mu    sync.Mutex
	sites map[string]pageSite
}

var pages pageServer

// pageURL is the address of the file at p of session s (absolute on its
// host when it starts with /) on the page server, for a web view.
func (a *App) pageURL(s proto.SessionInfo, p string) (string, error) {
	if a.client == nil {
		return "", errors.New("not connected to the host")
	}
	pages.start.Do(pages.listen)
	if pages.addr == "" {
		return "", errors.New("couldn't serve the page on this device")
	}
	dir, name := path.Split(p)
	site := pageSite{cl: a.client, session: s.ID, dir: path.Clean("/" + dir), abs: strings.HasPrefix(p, "/")}
	if !site.abs {
		site.dir = strings.TrimPrefix(site.dir, "/")
	}
	sum := sha256.Sum256([]byte(s.ID + "\x00" + site.dir))
	id := hex.EncodeToString(sum[:8])
	pages.mu.Lock()
	pages.sites[id] = site
	pages.mu.Unlock()
	return "http://" + pages.addr + "/" + pages.token + "/" + id + "/" + name, nil
}

func (ps *pageServer) listen() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	ps.token, ps.sites, ps.addr = hex.EncodeToString(b), map[string]pageSite{}, ln.Addr().String()
	go func() { _ = http.Serve(ln, ps) }()
}

func (ps *pageServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 3)
	if len(parts) < 2 || parts[0] != ps.token || r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}
	ps.mu.Lock()
	site, ok := ps.sites[parts[1]]
	ps.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	rel := ""
	if len(parts) == 3 {
		rel = parts[2]
	}
	// Cleaned from a root, the path can't climb out of the site's folder.
	rel = strings.TrimPrefix(path.Clean("/"+rel), "/")
	if rel == "" || strings.HasSuffix(r.URL.Path, "/") {
		rel = path.Join(rel, "index.html")
	}
	file := path.Join(site.dir, rel)
	if site.abs {
		file = path.Join("/", file)
	}
	data, err := site.cl.ReadBytes(site.session, file, site.abs, pageLimit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	kind := mime.TypeByExtension(path.Ext(rel))
	if kind == "" {
		kind = http.DetectContentType(data)
	}
	w.Header().Set("Content-Type", kind)
	w.Header().Set("Cache-Control", "no-store") // the agent may change it any time
	_, _ = w.Write(data)
}

// pageView is the web view of a file tab showing its page.
type pageView struct {
	wv       *mygo.WebView
	err      string
	opening  bool
	modified time.Time // the file's, as last loaded
}

// filePage draws the file at p of session s as a page, reloading it when
// the file changes (modified).
func (a *App) filePage(c *ui.Context, s proto.SessionInfo, p string, modified time.Time) {
	if a.pageViews == nil {
		a.pageViews = map[string]*pageView{}
	}
	key := s.ID + "\x00" + p
	v := a.pageViews[key]
	if v == nil {
		v = &pageView{modified: modified}
		a.pageViews[key] = v
	}
	switch {
	case v.err != "":
		pageNote(c, v.err, theme.Error)
		return
	case v.wv == nil && !v.opening:
		v.opening = true
		url, err := a.pageURL(s, p)
		if err != nil {
			v.err = err.Error()
			return
		}
		win := a.win
		if win == nil {
			v.err = "there is no window to show the page in"
			return
		}
		go func() {
			wv, err := win.NewWebView(mygo.WebViewOptions{URL: url, Page: mygo.PageOptions{DevTools: mygo.DevToolsEnabled}})
			a.onMain(func() {
				v.opening = false
				if err != nil {
					v.err = err.Error()
					return
				}
				if a.pageViews[key] != v { // the tab closed meanwhile
					wv.Destroy()
					return
				}
				v.wv = wv
			})
		}()
	}
	if v.wv == nil {
		pageNote(c, "Opening the page…", theme.Muted)
		return
	}
	if !modified.IsZero() && !modified.Equal(v.modified) {
		v.modified = modified
		v.wv.Page().Reload()
	}
	ui.WebView(c, v.wv).Grow(1).FillWidth().MinHeight(0)
}

func pageNote(c *ui.Context, text string, color ui.Color) {
	ui.Column(c).Grow(1).FillWidth().Center().Padding(16).Children(func() {
		ui.Text(c, text).FontSize(theme.TextLabel).TextColor(color)
	})
}

// closePage drops the web view of session id's file at p.
func (a *App) closePage(id, p string) {
	key := id + "\x00" + p
	if v := a.pageViews[key]; v != nil && v.wv != nil {
		v.wv.Destroy()
	}
	delete(a.pageViews, key)
}
