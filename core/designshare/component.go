package designshare

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"golang.org/x/net/html"
)

// MaxComponent is the most HTML and CSS a component holds (the owner's limit).
const MaxComponent = 64 << 10

// CheckComponent says why b can't be a component, a small piece people copy as
// HTML and CSS; nil when it can. People paste these into their own pages, so it's
// walked as a browser reads it, and refused for anything that runs.
func CheckComponent(b Bundle) error {
	for _, f := range b.frames() {
		if len(f.HTML) > MaxComponent {
			return fmt.Errorf("a component's HTML and CSS are %d KiB at most: publish this as a full page", MaxComponent>>10)
		}
		if runs(f.HTML) {
			return errors.New("a component has no scripts: publish this as a full page")
		}
	}
	return nil
}

// eventInTag finds an event attribute in a tag as text: a parser moves what doesn't
// belong where it stands, and not every browser moves the same things.
var eventInTag = regexp.MustCompile(`(?i)<[a-z][^>]*[\s/"']on[a-z]+\s*=`)

// runs says whether page holds anything that runs or brings in another page.
func runs(page string) bool {
	if eventInTag.MatchString(page) {
		return true
	}
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		return true
	}
	var walk func(n *html.Node) bool
	walk = func(n *html.Node) bool {
		if n.Type == html.ElementNode && elementRuns(n) {
			return true
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if walk(c) {
				return true
			}
		}
		return false
	}
	return walk(doc)
}

func elementRuns(n *html.Node) bool {
	switch strings.ToLower(n.Data) {
	case "script", "embed", "object", "iframe", "frame", "frameset", "applet", "base", "noscript":
		return true
	case "meta":
		if attr(n, "http-equiv") != "" {
			return true
		}
	}
	for _, a := range n.Attr {
		key := strings.ToLower(a.Key)
		if strings.HasPrefix(key, "on") || key == "srcdoc" || runningAddress(a.Val) {
			return true
		}
	}
	return false
}

// runningAddress says whether an attribute's value is an address that runs. The
// parser has undone its entities; browsers also drop tabs, newlines and leading
// controls from a scheme, and a copy may be read again after being percent-decoded.
func runningAddress(val string) bool {
	for range 3 {
		bare := strings.ToLower(strings.Map(func(r rune) rune {
			if r <= ' ' || r == 0x7f || unicode.Is(unicode.Cf, r) {
				return -1
			}
			return r
		}, val))
		if strings.HasPrefix(bare, "javascript:") || strings.HasPrefix(bare, "vbscript:") || strings.HasPrefix(bare, "data:text/html") || strings.HasPrefix(bare, "data:image/svg") || strings.HasPrefix(bare, "data:application/xhtml") {
			return true
		}
		decoded, err := url.PathUnescape(val)
		if err != nil || decoded == val {
			return false
		}
		val = decoded
	}
	return false
}

// cdnScript is a script from a CDN, at an exact version. What it names is
// fetched once, as the design is packed, since a preview loads nothing else.
var cdnScript = regexp.MustCompile(`<script\b[^>]*\bsrc=["'](https://(?:cdn\.tailwindcss\.com/\d[^"']*|cdn\.jsdelivr\.net/npm/[a-z0-9@._/-]*@\d[^"']*|cdnjs\.cloudflare\.com/ajax/libs/[a-zA-Z0-9._/-]*/\d[^"']*|unpkg\.com/[a-z0-9@._/-]*@\d[^"']*))["'][^>]*>`)

// maxEmbedded is the most a library put in a design may weigh.
const maxEmbedded = 4 << 20

// EmbedLibraries puts the scripts of known CDNs in page, so they run in the
// preview. A script it can't get, or isn't one of those, stays as it is, and
// the check warns of it. fetch is nil for http's; tests give their own.
func EmbedLibraries(ctx context.Context, page string, fetch func(ctx context.Context, url string) ([]byte, error)) string {
	if fetch == nil {
		fetch = fetchLibrary
	}
	return cdnScript.ReplaceAllStringFunc(page, func(tag string) string {
		url := cdnScript.FindStringSubmatch(tag)[1]
		// These go with enowX: documents put them in by themselves.
		if strings.Contains(url, "animejs") || strings.Contains(url, "three") {
			return tag
		}
		data, err := fetch(ctx, url)
		if err != nil {
			return tag
		}
		return strings.Replace(tag, url, "data:text/javascript;base64,"+base64.StdEncoding.EncodeToString(data), 1)
	})
}

var libraryClient = &http.Client{Timeout: 30 * time.Second}

func fetchLibrary(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	res, err := libraryClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("designshare: %s answered %d", url, res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxEmbedded+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxEmbedded {
		return nil, errors.New("designshare: a library too big to put in a design")
	}
	return data, nil
}
