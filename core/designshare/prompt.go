package designshare

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/net/html"
)

var (
	colorRe = regexp.MustCompile(`#[0-9a-fA-F]{6}\b`)
	fontRe  = regexp.MustCompile(`font-family\s*:\s*([^;"'}]+)`)
)

// DraftPrompt is the prompt a creator starts from, for an agent to make a
// design again: made of the design's own structure, colors and fonts and what
// the creator wrote of it. Nothing else is read, so a canvas's chat and agents'
// conversations can't reach it. The creator edits and approves it.
func DraftPrompt(b Bundle, title, description, kind string) string {
	what := "a full page"
	if kind == "component" {
		what = "a small component"
	}
	var s strings.Builder
	fmt.Fprintf(&s, "Build %s like \"%s\", %d by %d pixels.\n", what, title, b.Frame.Width, b.Frame.Height)
	if d := strings.TrimSpace(description); d != "" {
		s.WriteString(d + "\n")
	}
	if parts := structureOf(b.Frame.HTML); len(parts) > 0 {
		s.WriteString("\nIts structure, from the top:\n")
		for _, p := range parts {
			s.WriteString("- " + p + "\n")
		}
	}
	if colors := topMatches(colorRe, b.Frame.HTML, 6); len(colors) > 0 {
		s.WriteString("\nColors: " + strings.Join(colors, ", ") + "\n")
	}
	if fonts := fontsOf(b.Frame.HTML); len(fonts) > 0 {
		s.WriteString("Fonts: " + strings.Join(fonts, "; ") + "\n")
	}
	return strings.TrimSpace(s.String())
}

// structureOf lists a page's landmarks and headings, in order.
func structureOf(page string) []string {
	doc, err := html.Parse(strings.NewReader(base64Data.ReplaceAllString(page, "data:")))
	if err != nil {
		return nil
	}
	var out []string
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if len(out) >= 14 {
			return
		}
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style":
				return
			case "header", "nav", "main", "section", "aside", "footer", "form":
				out = append(out, "a "+n.Data)
			case "h1", "h2", "h3":
				if t := strings.Join(strings.Fields(textOf(n)), " "); t != "" {
					out = append(out, fmt.Sprintf("heading \"%s\"", clip(t, 80)))
				}
				return
			case "button":
				if t := strings.Join(strings.Fields(textOf(n)), " "); t != "" {
					out = append(out, fmt.Sprintf("button \"%s\"", clip(t, 40)))
				}
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return out
}

func textOf(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(textOf(c) + " ")
	}
	return b.String()
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// topMatches are re's matches in text, the most frequent first.
func topMatches(re *regexp.Regexp, text string, n int) []string {
	count := map[string]int{}
	var order []string
	for _, m := range re.FindAllString(base64Data.ReplaceAllString(text, ""), -1) {
		m = strings.ToLower(m)
		if count[m] == 0 {
			order = append(order, m)
		}
		count[m]++
	}
	slices.SortStableFunc(order, func(a, b string) int { return count[b] - count[a] })
	return order[:min(n, len(order))]
}

func fontsOf(page string) []string {
	var out []string
	for _, m := range fontRe.FindAllStringSubmatch(page, -1) {
		f := strings.TrimSpace(m[1])
		if f != "" && !slices.Contains(out, f) && len(out) < 3 {
			out = append(out, clip(f, 60))
		}
	}
	return out
}
