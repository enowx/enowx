package unslop

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/enowx/enowx/proto"
)

// A page's lint, as rendered: what LintScript measured in the browser,
// turned into findings here, where the thresholds are, so they're tested
// without a browser.

// lintItem is an element LintScript met for a rule: its selector (S), its
// label as an example (D), and what was measured of it, by rule.
type lintItem struct {
	S, D string
	// contrast-low
	Ratio float64
	Large bool
	Plate bool // a plate not its ancestor may sit under it, or unknown
	// gray-on-color: the backdrop's chroma, the text's, its luminance
	BG, Text, Lum float64
	// body text: px, line-height ÷ font-size, characters per line
	Size, Leading float64
	CPL           int
	CJK           bool
	// kicker-label: how it is styled; the heading it's above
	How, H string
	// heading-gap: the space above and below, px
	Above, Below float64
	// nested-cards: the card around it
	Outer string
}

// lintOrigin is a left edge text starts from: its x (px), how many blocks
// start there, and the first of them (its label D, its selector S).
type lintOrigin struct {
	X    float64
	N    int
	D, S string
}

type lintReport struct {
	VW, VH                           int
	Contrast, Gray, Body, Stuck      []lintItem
	Kickers, Numbered, Gaps, Stripes []lintItem
	GradText                         []lintItem `json:"gradtext"`
	Nested, Emoji                    []lintItem
	Anchors                          []lintOrigin
	Boxed                            struct {
		Texts, InBox, Boxes int
		Ex, At              []string // the first boxes' labels and selectors
	}
}

// maxLintExamples and maxLintTargets bound a rendered finding.
const (
	maxLintExamples = 3
	maxLintTargets  = 24
)

// lintFinding is a finding of items, one per element: counted once each,
// the first few as examples.
func lintFinding(pass, rule, severity, title, fix string, items []lintItem, example func(lintItem) string) proto.UnslopFinding {
	var ex, at []string
	seen := map[string]bool{}
	n := 0
	for _, it := range items {
		if seen[it.S] {
			continue
		}
		seen[it.S] = true
		n++
		if len(ex) < maxLintExamples {
			ex = append(ex, example(it))
		}
		if len(at) < maxLintTargets {
			at = append(at, it.S)
		}
	}
	f := finding(pass, rule, severity, title, fix, ex, at...)
	f.Count = n
	return f
}

// distinct is how many elements items name.
func distinct(items []lintItem) int {
	seen := map[string]bool{}
	for _, it := range items {
		seen[it.S] = true
	}
	return len(seen)
}

func keep(items []lintItem, ok func(lintItem) bool) []lintItem {
	var out []lintItem
	for _, it := range items {
		if ok(it) {
			out = append(out, it)
		}
	}
	return out
}

func label(it lintItem) string { return it.D }

// LintFindings are what LintScript's result raw shows of a page rendered
// width CSS pixels wide (0: the width it measured), in pass order;
// none when raw can't be read.
func LintFindings(raw string, width int) []proto.UnslopFinding {
	var r lintReport
	if json.Unmarshal([]byte(raw), &r) != nil {
		return nil
	}
	if r.VW <= 0 {
		return nil // a page at no width (a web view not on screen) shows nothing true
	}
	if width <= 0 {
		width = r.VW
	}
	var out []proto.UnslopFinding
	add := func(items []lintItem, min int, f func([]lintItem) proto.UnslopFinding) {
		if distinct(items) >= min {
			out = append(out, f(items))
		}
	}

	add(keep(r.Contrast, func(it lintItem) bool {
		limit := 4.5
		if it.Large {
			limit = 3
		}
		return it.Ratio < limit && !(it.Ratio < 1.5 && it.Plate)
	}), 1, func(items []lintItem) proto.UnslopFinding {
		return lintFinding("access", "contrast-low", proto.UnslopHigh, "Text below its contrast as rendered",
			"Darken or lighten the text, or its background, until body text reaches 4.5:1 and large text 3:1; faded and translucent text counts as shown.",
			items, func(it lintItem) string { return fmt.Sprintf("%s %.2f:1", it.D, it.Ratio) })
	})

	body := func(ok func(lintItem) bool) []lintItem { return keep(r.Body, ok) }
	add(body(func(it lintItem) bool { return it.Size < 12.5 }), 1, func(items []lintItem) proto.UnslopFinding {
		return lintFinding("type", "small-body-text", proto.UnslopHigh, "Body text too small to read",
			"Set what people read at a size made for reading; keep small sizes for labels and captions.",
			items, func(it lintItem) string { return fmt.Sprintf("%s %gpx", it.D, it.Size) })
	})
	add(body(func(it lintItem) bool {
		if it.CJK {
			return it.Leading < 1.4
		}
		return it.Leading < 1.3
	}), 1, func(items []lintItem) proto.UnslopFinding {
		return lintFinding("type", "tight-leading", proto.UnslopHigh, "Paragraphs with lines set too tight",
			"Give running text room between its lines; tight leading is for headings and single lines.",
			items, func(it lintItem) string { return fmt.Sprintf("%s line-height %.2f", it.D, it.Leading) })
	})
	add(body(func(it lintItem) bool {
		if it.CJK {
			return it.CPL > 46
		}
		return it.CPL > 95
	}), 1, func(items []lintItem) proto.UnslopFinding {
		return lintFinding("type", "long-measure", proto.UnslopLow, "Lines too long to read comfortably",
			"Hold running text to a readable measure with a max width, not the container's.",
			items, func(it lintItem) string { return fmt.Sprintf("%s ~%d characters a line", it.D, it.CPL) })
	})

	add(r.Stuck, 1, func(items []lintItem) proto.UnslopFinding {
		return lintFinding("motion", "stuck-hidden", proto.UnslopHigh, fmt.Sprintf("Content invisible in the first screen at %d px", width),
			"Show content without waiting for an animation: an entrance that never runs (no scroll, reduced motion, a script that failed) leaves it hidden.",
			items, label)
	})

	add(keep(r.Gray, func(it lintItem) bool { return it.BG >= 0.25 && it.Text < 0.06 && it.Lum >= 0.08 && it.Lum <= 0.6 }), 1, func(items []lintItem) proto.UnslopFinding {
		return lintFinding("color", "gray-on-color", proto.UnslopMedium, "Gray text on a saturated color",
			"On a strong color, set text in a tint or shade of that color, or in white or near-black; a neutral gray looks dirty there.",
			items, label)
	})
	add(r.GradText, 1, func(items []lintItem) proto.UnslopFinding {
		return lintFinding("color", "gradient-text", proto.UnslopMedium, "Gradient text",
			"Set text in a solid color; size, weight and space make a heading stand out.", items, label)
	})

	add(r.Kickers, 2, func(items []lintItem) proto.UnslopFinding {
		return lintFinding("structure", "kicker-label", proto.UnslopMedium, "Small labels above headings, section after section",
			"Let headings stand without a label above them; keep one only where it adds something the heading doesn't say.",
			items, func(it lintItem) string { return fmt.Sprintf("%s (%s) above %s", it.D, it.How, it.H) })
	})
	add(r.Numbered, 2, func(items []lintItem) proto.UnslopFinding {
		return lintFinding("structure", "numbered-sections", proto.UnslopLow, "Sections numbered for decoration",
			"Drop numbers from sections that aren't steps or an order the reader follows.",
			items, func(it lintItem) string { return fmt.Sprintf("%s above %s", it.D, it.H) })
	})
	add(keep(r.Gaps, func(it lintItem) bool { return it.Above >= 0 && it.Below >= 12 && it.Above+4 < it.Below }), 1, func(items []lintItem) proto.UnslopFinding {
		return lintFinding("spacing", "heading-gap", proto.UnslopMedium, "Headings nearer the block before than their own",
			"Put more space above a heading than below it, so it groups with what it introduces.",
			items, func(it lintItem) string { return fmt.Sprintf("%s %gpx above, %gpx below", it.D, it.Above, it.Below) })
	})

	add(r.Stripes, 1, func(items []lintItem) proto.UnslopFinding {
		return lintFinding("surfaces", "side-stripe", proto.UnslopMedium, "Colored stripes on the side of boxes",
			"Remove the stripe, or make it a real state (an error, a selection) shown with a label too.",
			items, func(it lintItem) string { return it.D + " " + it.How })
	})
	add(r.Nested, 1, func(items []lintItem) proto.UnslopFinding {
		return lintFinding("surfaces", "nested-cards", proto.UnslopMedium, "Cards inside cards",
			"Let the inner parts sit on their card's surface: one level of card, grouped by space or a divider.",
			items, func(it lintItem) string { return it.Outer + " › " + it.D })
	})
	if b := r.Boxed; b.Texts > 0 && float64(b.InBox) >= 0.8*float64(b.Texts) && b.Boxes >= 6 {
		f := finding("surfaces", "boxed-everything", proto.UnslopLow, fmt.Sprintf("Every block on its own plate at %d px", width),
			"Group by space and alignment; keep boxes for what is really a separate object.", b.Ex, b.At...)
		f.Count = b.Boxes
		f.Examples = append([]string{fmt.Sprintf("%d of %d text blocks in %d boxes", b.InBox, b.Texts, b.Boxes)}, f.Examples...)
		if len(f.Examples) > maxLintExamples {
			f.Examples = f.Examples[:maxLintExamples]
		}
		out = append(out, f)
	}

	add(r.Emoji, 2, func(items []lintItem) proto.UnslopFinding {
		return lintFinding("icons", "emoji-icons", proto.UnslopMedium, "Emoji standing in for icons",
			"Use icons of one set, at one stroke, in the text's color; or no icon where it adds nothing.",
			items, label)
	})

	if len(r.Anchors) > 4 {
		var ex, at []string
		for _, o := range r.Anchors {
			if len(ex) < maxLintExamples {
				ex = append(ex, fmt.Sprintf("x %g px: %d blocks (%s)", o.X, o.N, o.D))
			}
			if len(at) < maxLintTargets {
				at = append(at, o.S)
			}
		}
		f := finding("spacing", "text-anchors", proto.UnslopLow, fmt.Sprintf("Text starts from %d left edges at %d px", len(r.Anchors), width),
			"Align text to a few shared edges of one grid; a page that reads in order starts its blocks from two or three.", ex, at...)
		f.Count = len(r.Anchors)
		out = append(out, f)
	}
	sortByPass(out)
	return out
}

// sortByPass orders findings by their pass, keeping the order within one.
func sortByPass(findings []proto.UnslopFinding) {
	order := func(f proto.UnslopFinding) int {
		return slices.IndexFunc(Passes, func(p Pass) bool { return p.Name == f.Pass })
	}
	slices.SortStableFunc(findings, func(a, b proto.UnslopFinding) int { return order(a) - order(b) })
}

// supersedes are the static rules a rendered one replaces when both fire:
// the rendered page measures what the static read guessed.
var supersedes = map[string]string{
	"kicker-label": "eyebrow-pill",
	"nested-cards": "cards-in-cards",
	"contrast-low": "contrast",
}

// WithRendered are a page's static findings with those measured rendered
// (its lint, its layouts at each width), in pass order: a static finding
// a rendered one supersedes, or of the same rule, is left out.
func WithRendered(static, rendered []proto.UnslopFinding) []proto.UnslopFinding {
	drop := map[string]bool{}
	for _, f := range rendered {
		if f.State != "" {
			continue // of a state the first screen doesn't show
		}
		drop[f.Rule] = true
		if old, ok := supersedes[f.Rule]; ok {
			drop[old] = true
		}
	}
	out := make([]proto.UnslopFinding, 0, len(static)+len(rendered))
	for _, f := range static {
		if !drop[f.Rule] {
			out = append(out, f)
		}
	}
	out = append(out, rendered...)
	sortByPass(out)
	return out
}
