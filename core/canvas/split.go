package canvas

import (
	"bytes"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var cssURL = regexp.MustCompile(`url\(\s*['"]?([^'")]+)`)

// SplitStyles is a design's page as its markup and its CSS apart, for copying:
// every style block's text in css, and in markup what the body holds and the
// stylesheet links of the head, without the document around them. Attributes,
// style="" among them, stay on their elements.
func SplitStyles(page string) (markup, css string, err error) {
	d, err := parseDoc(page)
	if err != nil {
		return "", "", err
	}
	var blocks []string
	var out []*html.Node
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		for ch := n.FirstChild; ch != nil; {
			next := ch.NextSibling
			if ch.Type == html.ElementNode && ch.DataAtom == atom.Style {
				blocks = append(blocks, strings.TrimSpace(text(ch)))
				n.RemoveChild(ch)
			} else {
				walk(ch)
			}
			ch = next
		}
	}
	walk(d.root)
	if head := find(d.root, func(n *html.Node) bool { return n.Type == html.ElementNode && n.DataAtom == atom.Head }); head != nil {
		for ch := head.FirstChild; ch != nil; ch = ch.NextSibling {
			if ch.Type == html.ElementNode && ch.DataAtom == atom.Link && strings.EqualFold(attr(ch, "rel"), "stylesheet") {
				out = append(out, ch)
			}
		}
	}
	if body := d.body(); body != nil {
		for ch := body.FirstChild; ch != nil; ch = ch.NextSibling {
			out = append(out, ch)
		}
	}
	var b bytes.Buffer
	for _, n := range out {
		_ = html.Render(&b, n)
	}
	return strings.TrimSpace(b.String()), strings.TrimSpace(strings.Join(blocks, "\n\n")), nil
}

// StyleText is all the CSS a page holds: its style blocks and its style=""
// attributes, one a line. Scripts and text stay out, so what's read from it is
// what the page declares.
func StyleText(page string) string {
	d, err := parseDoc(page)
	if err != nil {
		return ""
	}
	var parts []string
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if n.DataAtom == atom.Style {
				parts = append(parts, text(n))
			}
			if s := attr(n, "style"); s != "" {
				parts = append(parts, s)
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(d.root)
	return strings.Join(parts, "\n")
}

// FilesNamed are the local files a page's markup and CSS name: src and poster
// attributes and url()s that aren't addresses of the web or data. What a copy
// without its files has to be given.
func FilesNamed(page string) []string {
	d, err := parseDoc(page)
	if err != nil {
		return nil
	}
	var out []string
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, key := range []string{"src", "poster", "href"} {
				v := attr(n, key)
				if v != "" && (key != "href" || n.DataAtom == atom.Link) {
					out = append(out, v)
				}
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(d.root)
	for _, m := range cssURL.FindAllStringSubmatch(StyleText(page), -1) {
		out = append(out, m[1])
	}
	seen := map[string]bool{}
	files := []string{}
	for _, f := range out {
		f = localFile(f)
		if f != "" && !seen[f] {
			seen[f] = true
			files = append(files, f)
		}
	}
	return files
}

// localFile is what a page names as a file of its own, "" for an address.
func localFile(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.HasPrefix(ref, "#") || strings.HasPrefix(ref, "//") || strings.Contains(ref, ":") {
		return ""
	}
	if i := strings.IndexAny(ref, "?#"); i >= 0 {
		ref = ref[:i]
	}
	return strings.TrimPrefix(strings.TrimPrefix(ref, "./"), "assets/")
}
