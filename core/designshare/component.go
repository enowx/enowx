package designshare

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// MaxComponent is the most HTML and CSS a component holds (the owner's limit).
const MaxComponent = 64 << 10

var scriptTag = regexp.MustCompile(`(?i)<script[\s>/]`)
var eventAttr = regexp.MustCompile(`(?i)<[a-z][^>]*\son[a-z]+\s*=`)

// CheckComponent says why b can't be a component, a small piece people copy as
// HTML and CSS; nil when it can.
func CheckComponent(b Bundle) error {
	for _, f := range b.frames() {
		if scriptTag.MatchString(f.HTML) || eventAttr.MatchString(f.HTML) {
			return errors.New("a component has no scripts: publish this as a full page")
		}
		if len(f.HTML) > MaxComponent {
			return fmt.Errorf("a component's HTML and CSS are %d KiB at most: publish this as a full page", MaxComponent>>10)
		}
	}
	return nil
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
