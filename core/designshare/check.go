package designshare

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"

	"github.com/enowx/enowx/core/autopush"
)

// What a check found. Refuse stops a publish; Warn goes on once the creator
// agreed to it. A finding says where and of what kind: never the value, which
// nothing may send, keep or show.
const (
	Refuse = "refuse"
	Warn   = "warn"
)

// Finding is one thing a check found.
type Finding struct {
	Severity string `json:"severity"`
	Kind     string `json:"kind"`
	Where    string `json:"where"`
}

// Text is what the creator wrote about a design, checked with it.
type Text struct {
	Title, Description, Prompt string
}

// base64Data is a file inlined in a page: its bytes would look like anything.
var base64Data = regexp.MustCompile(`data:[a-zA-Z0-9.+/-]+;base64,[A-Za-z0-9+/=]+`)

// trackers are hosts whose only job in a page is to report its viewers.
var trackers = []string{"google-analytics.com", "googletagmanager.com", "doubleclick.net", "connect.facebook.net", "hotjar.com", "cdn.segment.com",
	"segment.io", "mixpanel.com", "plausible.io", "clarity.ms", "fullstory.com", "amplitude.com", "matomo.cloud", "tiktok.com/i18n/pixel", "ads-twitter.com"}

// The patterns of what the creator has to look at before it goes public.
var (
	emailRe    = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}`)
	phoneRe    = regexp.MustCompile(`(^|[^0-9A-Za-z.#-])\+?[0-9][0-9 ()-]{8,16}[0-9]($|[^0-9A-Za-z.-])`)
	internalRe = regexp.MustCompile(`(?i)\b(localhost|127\.0\.0\.1|0\.0\.0\.0|10\.\d{1,3}\.\d{1,3}\.\d{1,3}|192\.168\.\d{1,3}\.\d{1,3}|172\.(1[6-9]|2\d|3[01])\.\d{1,3}\.\d{1,3}|[a-z0-9.-]+\.(local|test|internal|ts\.net)|staging\.[a-z0-9.-]+)\b`)
	outgoingRe = regexp.MustCompile(`\b(fetch|XMLHttpRequest|WebSocket|EventSource|sendBeacon|importScripts)\b`)
	navigateRe = regexp.MustCompile(`\b(location\s*(\.\s*href\s*)?=[^=]|location\.(assign|replace)\s*\()`)
)

// Check is what's wrong with sharing b with the text t. Anything Refuse, a
// publish doesn't go on; the cloud runs this again on what it gets.
func Check(b Bundle, t Text) []Finding {
	var out []Finding
	add := func(sev, kind, where string) {
		for _, f := range out {
			if f.Severity == sev && f.Kind == kind && f.Where == where {
				return
			}
		}
		out = append(out, Finding{sev, kind, where})
	}
	secrets := func(where string, data []byte) {
		for _, s := range autopush.ScanSecrets(where, data) {
			w := where
			if s.Line > 0 {
				w = fmt.Sprintf("%s, line %d", where, s.Line)
			}
			add(Refuse, s.Kind, w)
		}
	}
	secrets("title", []byte(t.Title))
	secrets("description", []byte(t.Description))
	secrets("prompt", []byte(t.Prompt))
	for name, data := range b.Assets {
		secrets("file "+name, data)
		add(Warn, "a file whose rights you have to hold", "file "+name)
	}
	for i, f := range b.frames() {
		where := "the design"
		if i > 0 {
			where = "the phone design"
		}
		secrets(where, []byte(f.HTML))
		secrets(where+"'s name", []byte(f.Name))
		for _, e := range f.Elements {
			secrets(where+", a shape", []byte(e.Text))
			if strings.HasPrefix(e.Src, "http://") || strings.HasPrefix(e.Src, "https://") {
				add(Warn, "a picture from another address, which won't load in the preview", where+", a shape")
			}
		}
		checkPage(f.HTML, where, add)
	}
	text := t.Title + "\n" + t.Description + "\n" + t.Prompt
	personal(text, "the text you wrote", add)
	return out
}

// checkPage looks at a design's page.
func checkPage(page, where string, add func(sev, kind, where string)) {
	bare := base64Data.ReplaceAllString(page, "data:")
	low := strings.ToLower(bare)
	for _, host := range trackers {
		if strings.Contains(low, host) {
			add(Refuse, "a known tracker", where)
			break
		}
	}
	personal(bare, where, add)
	if outgoingRe.MatchString(bare) {
		add(Warn, "a request out of the design, which the preview blocks", where)
	}
	if navigateRe.MatchString(bare) {
		add(Warn, "a script that changes the page's address", where)
	}
	doc, err := html.Parse(strings.NewReader(bare))
	if err != nil {
		return
	}
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			checkElement(n, where, add)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
}

// personal warns of what's about someone, or of a place that isn't public.
func personal(text, where string, add func(sev, kind, where string)) {
	if emailRe.MatchString(text) {
		add(Warn, "an email address", where)
	}
	if phoneRe.MatchString(text) {
		add(Warn, "a phone number", where)
	}
	if internalRe.MatchString(text) {
		add(Warn, "an internal address", where)
	}
}

func attr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, name) {
			return a.Val
		}
	}
	return ""
}

func absolute(u string) bool {
	u = strings.ToLower(strings.TrimSpace(u))
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "//")
}

func checkElement(n *html.Node, where string, add func(sev, kind, where string)) {
	switch n.Data {
	case "form":
		if absolute(attr(n, "action")) && holdsPassword(n) {
			add(Refuse, "a form sending a password to another address", where)
		}
	case "a":
		if absolute(attr(n, "href")) {
			add(Warn, "a link to another address", where)
		}
	case "meta":
		if strings.EqualFold(attr(n, "http-equiv"), "refresh") {
			add(Warn, "a page that sends its viewer elsewhere", where)
		}
	case "base":
		add(Warn, "a page that changes where its addresses lead", where)
	case "script", "img", "iframe", "video", "audio", "source", "embed", "object":
		src := attr(n, "src")
		if src == "" {
			src = attr(n, "data")
		}
		if absolute(src) {
			add(Warn, "something from another address, which won't load in the preview", where)
		}
	case "link":
		if absolute(attr(n, "href")) && strings.EqualFold(attr(n, "rel"), "stylesheet") {
			add(Warn, "a stylesheet from another address, which won't load in the preview", where)
		}
	}
}

func holdsPassword(n *html.Node) bool {
	if n.Type == html.ElementNode && n.Data == "input" && strings.EqualFold(attr(n, "type"), "password") {
		return true
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if holdsPassword(c) {
			return true
		}
	}
	return false
}

// Refused says whether any finding stops a publish.
func Refused(found []Finding) bool {
	for _, f := range found {
		if f.Severity == Refuse {
			return true
		}
	}
	return false
}
