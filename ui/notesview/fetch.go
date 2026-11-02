package notesview

import (
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/enowx/enowx/core/notes"
)

// What a page's embeds show, fetched once each from where they point, off
// the main thread: an image, or a page's title for its link's card.

// fetchPoll is how often a page looks again while an embed loads.
const fetchPoll = 250 * time.Millisecond

// remote is an embed's link as fetched so far.
type remote struct {
	done  bool
	img   *ui.Bitmap
	title string
	err   string
	// retry is when a note's file not on the host yet is asked for again:
	// a sync may bring the note before its file.
	retry time.Time
	used  time.Time // last asked for
}

// size is the image's size, in pixels.
func (r remote) size() (float32, float32) {
	if r.img == nil {
		return 0, 0
	}
	w, h := r.img.Size()
	return float32(w), float32(h)
}

var remotes struct {
	sync.Mutex
	m map[string]*remote
}

// remotesKept is how many fetched images and titles stay in memory: a note's images
// run to 16 MB each, and one scrolled back to is fetched again.
const remotesKept = 96

// forgetRemotes drops the links least recently asked for once over most, those still
// on their way aside. The caller holds remotes.
func forgetRemotes(m map[string]*remote, most int) {
	if len(m) <= most {
		return
	}
	links := make([]string, 0, len(m))
	for link, r := range m {
		if r.done {
			links = append(links, link)
		}
	}
	slices.SortFunc(links, func(x, y string) int { return m[x].used.Compare(m[y].used) })
	for _, link := range links[:max(len(m)-most, 0)] {
		delete(m, link)
	}
}

var fetchClient = &http.Client{Timeout: 15 * time.Second}

// fetch is link as fetched so far, fetching it the first time it is
// asked for: an image when image, else the title of the page there. A
// note's own file ("assets/…") comes through local, from its host.
func fetch(link string, image bool, local func(ref string) ([]byte, error)) remote {
	link = imageSource(link)
	remotes.Lock()
	defer remotes.Unlock()
	if remotes.m == nil {
		remotes.m = map[string]*remote{}
	}
	if r, ok := remotes.m[link]; ok && (r.retry.IsZero() || time.Now().Before(r.retry)) {
		r.used = time.Now()
		return *r
	}
	r := &remote{used: time.Now()}
	remotes.m[link] = r
	forgetRemotes(remotes.m, remotesKept)
	go func() {
		var got remote
		if strings.HasPrefix(link, "assets/") {
			got = loadLocal(link, local)
		} else {
			got = load(link, image)
		}
		got.done = true
		remotes.Lock()
		got.used = r.used
		*r = got
		remotes.Unlock()
	}()
	return *r
}

func load(link string, image bool) remote {
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "http" && u.Scheme != "https" {
		return remote{err: "only http(s) links are fetched"}
	}
	resp, err := fetchClient.Get(link)
	if err != nil {
		return remote{err: "unreachable"}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return remote{err: fmt.Sprintf("the server answered %d", resp.StatusCode)}
	}
	limit := int64(512 << 10)
	if image {
		limit = 16 << 20
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return remote{err: "cut off"}
	}
	if image {
		img, err := ui.DecodeBitmap(data)
		if err != nil {
			return remote{err: "not an image this can show"}
		}
		return remote{img: img}
	}
	if m := titleRe.FindSubmatch(data); m != nil {
		return remote{title: strings.TrimSpace(html.UnescapeString(string(m[1])))}
	}
	return remote{}
}

func loadLocal(ref string, local func(string) ([]byte, error)) remote {
	if local == nil {
		return remote{err: "this host doesn't share the note's files"}
	}
	data, err := local(ref)
	if err != nil {
		return remote{err: "not on this host", retry: time.Now().Add(10 * time.Second)}
	}
	img, err := ui.DecodeBitmap(data)
	if err != nil {
		return remote{err: "not an image this can show"}
	}
	return remote{img: img}
}

// imageToken finds images even when a block also contains text.
var imageToken = regexp.MustCompile(`!\[([^\]]*)\]\((<[^>\r\n]+>|[^)\s]+)\)`)

func imageSource(link string) string {
	if ref, ok := notes.AssetReference(link); ok {
		return ref
	}
	if strings.HasPrefix(link, "<") && strings.HasSuffix(link, ">") {
		return link[1 : len(link)-1]
	}
	return link
}

var titleRe = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

// isImage says link names an image by its extension.
func isImage(link string) bool {
	link = imageSource(link)
	u, err := url.Parse(link)
	if err != nil {
		return false
	}
	switch strings.ToLower(path.Ext(u.Path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp":
		return true
	}
	return false
}

// hostOf is link's host, or link when it has none.
func hostOf(link string) string {
	if u, err := url.Parse(link); err == nil && u.Host != "" {
		return strings.TrimPrefix(u.Host, "www.")
	}
	return link
}
