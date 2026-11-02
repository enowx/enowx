// Package unslop reads a mockup's HTML for what makes a design look made
// by AI without a designer's care: off-grid spacing, decorative gradients
// and glows, emoji for icons, default type, hype and exclamation in the
// copy, badges and radii without a system, gratuitous motion, unreadable
// contrast. What it finds is grouped by pass, in the order a redesign best
// fixes them, so an agent can redesign one pass at a time and read the
// design again after each.
package unslop

import (
	"fmt"
	"slices"
	"strings"

	"github.com/enowx/enowx/proto"
)

// Pass is a step of a redesign: its name as a version shows it, and what
// it changes.
type Pass struct {
	ID, Name, Goal string
}

// Passes are the redesign's steps, in order: honesty first (it removes
// whole sections), then the structure, the grid, type, color, surfaces,
// icons, words, charts, motion, and the checks last, since every earlier
// pass can break contrast or reflow.
var Passes = []Pass{
	{"truth", "01 Truth", "Nothing invented: stats, names, testimonials, logos and lorem become the product's own, labelled placeholders, or go; a change names its period."},
	{"structure", "02 Structure", "The content leads the layout, not a template: no cards in cards, no row of equal cards by habit, one primary action per region, body text not centered."},
	{"spacing", "03 Spacing", "One spacing scale, no value picked by eye; less space within a group than between groups, more between sections."},
	{"type", "04 Type", "Faces chosen for this product with a reason, not the first that comes; one scale, few sizes and weights, numbers that change in tabular figures."},
	{"color", "05 Color", "Color with a meaning: the design's own direction kept, no decorative gradients, gradient text or default violet; states in their own colors."},
	{"surfaces", "06 Surfaces", "One way to separate surfaces and one radius system; no glows, little glass, no colored side stripes; badges only where they say something."},
	{"icons", "07 Icons", "One icon set at one stroke in currentColor; no emoji standing in for icons, no icons where they add nothing."},
	{"copy", "08 Copy", "Plain, specific words: no exclamation marks, hype, greetings in the way or emoji in labels; buttons say what they do. Never invent facts."},
	{"data", "09 Data", "A chart answers one question: labelled, with units, color used for what matters, no decoration."},
	{"motion", "10 Motion", "Motion only where it explains a change: no loops, no entrances on everything, no hover that grows cards."},
	{"access", "11 Access", "Contrast measured, not guessed: 4.5:1 for text, 3:1 for large text and controls; a visible focus; status by label as well as color."},
	{"mobile", "12 Mobile", "On a phone: one column, nothing overflowing, 44 px touch targets, type that reads, a mobile navigation."},
}

// wireframePasses are those a wireframe gets: its gray boxes and
// placeholders are the point.
var wireframePasses = []string{"truth", "structure", "spacing"}

// PassNamed is the pass ref names: its number ("3", "03"), its name
// ("03 Spacing") or its ID or word ("spacing").
func PassNamed(ref string) (string, bool) {
	ref = strings.ToLower(strings.TrimSpace(ref))
	for i, p := range Passes {
		n := strings.ToLower(p.Name)
		if ref == fmt.Sprint(i+1) || ref == fmt.Sprintf("%02d", i+1) || ref == n || ref == p.ID || ref == strings.Fields(n)[1] {
			return p.Name, true
		}
	}
	return "", false
}

// Names are the passes' names, in order.
func Names() []string {
	out := make([]string, len(Passes))
	for i, p := range Passes {
		out[i] = p.Name
	}
	return out
}

// passName is pass id's name.
func passName(id string) string {
	for _, p := range Passes {
		if p.ID == id {
			return p.Name
		}
	}
	return id
}

// Weights of a finding in the score.
var weight = map[string]int{proto.UnslopHigh: 15, proto.UnslopMedium: 7, proto.UnslopLow: 3}

// Scan reads a design's HTML: its findings, the first pass's first, and
// its score, 100 when nothing was found. A wireframe (kind
// proto.CanvasWireframe) is read for the passes it gets only.
func Scan(html, kind string) (score int, findings []proto.UnslopFinding) {
	p := parse(html)
	for _, check := range checks {
		if f, ok := check(p); ok {
			if kind == proto.CanvasWireframe && !slices.ContainsFunc(wireframePasses, func(id string) bool { return passName(id) == f.Pass }) {
				continue
			}
			if len(f.Examples) > maxExamples {
				f.Examples = f.Examples[:maxExamples]
			}
			findings = append(findings, f)
		}
	}
	order := func(f proto.UnslopFinding) int {
		return slices.IndexFunc(Passes, func(p Pass) bool { return p.Name == f.Pass })
	}
	slices.SortStableFunc(findings, func(a, b proto.UnslopFinding) int { return order(a) - order(b) })
	score = 100
	for _, f := range findings {
		score -= weight[f.Severity]
	}
	return max(score, 0), findings
}

const maxExamples = 4

// Score is findings' score: 100 less each one's weight, 0 at the least.
func Score(findings []proto.UnslopFinding) int {
	score := 100
	for _, f := range findings {
		score -= weight[f.Severity]
	}
	return max(score, 0)
}

// Report is findings as an agent reads them: each pass with what it found,
// and what to do.
func Report(score int, findings []proto.UnslopFinding) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Score %d/100", score)
	if len(findings) == 0 {
		b.WriteString(": nothing found. Still look at the design as a designer would, pass by pass.\n")
		return b.String()
	}
	fmt.Fprintf(&b, ", %d findings.\n", len(findings))
	for _, p := range Passes {
		var in []proto.UnslopFinding
		for _, f := range findings {
			if f.Pass == p.Name {
				in = append(in, f)
			}
		}
		if len(in) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n%s: %s\n", p.Name, p.Goal)
		for _, f := range in {
			fmt.Fprintf(&b, "  [%s] %s (%d)", f.Severity, f.Title, f.Count)
			if len(f.Examples) > 0 {
				fmt.Fprintf(&b, ": %s", strings.Join(f.Examples, " · "))
			}
			if f.State != "" {
				fmt.Fprintf(&b, "\n    in the state “%s”, reached by: %s", f.State, strings.Join(f.Steps, " > "))
			}
			fmt.Fprintf(&b, "\n    fix: %s\n", f.Fix)
		}
	}
	return b.String()
}
