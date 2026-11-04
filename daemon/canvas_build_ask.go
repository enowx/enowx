package daemon

import (
	"fmt"
	"strings"

	"github.com/enowx/enowx/core/designbrief"
	"github.com/enowx/enowx/core/designcompare"
	"github.com/enowx/enowx/proto"
)

// What the UI builder's agents are told: the lead, from the brief to a
// finished design; each direction's fresh agent, from the brief alone.

// sources are where directions grow from, one each, so several never
// start from the same kind of thing.
var sources = []string{
	"a material and the place it is used in (ruled paper and a workshop bench, frosted glass and a lab, enamel and a train station…), chosen from the product's own world",
	"a scene: one time, place and light where the product's users are, seen precisely",
	"the visual language of another field the product touches (signage, print editorial, instruments, maps, packaging, broadcast graphics…): its grammar, not its decoration",
	"one convention of the product's category, broken on purpose and kept usable",
}

// buildAsk is what the canvas's agent is told when the user starts a
// build.
func buildAsk(cv proto.Canvas, p proto.CanvasBuildParams) string {
	at := fmt.Sprintf("canvas %q", cv.ID)
	var b strings.Builder
	fmt.Fprintf(&b, "Build a UI on this canvas (%s): %s\n", at, nonEmpty(p.What, "recreate the screenshot the user gave"))
	fmt.Fprintf(&b, "Devices: %s. The user watches every step on the canvas. Say where you are with canvas_build (%s, stage …) as you go: brief, directions, building, compare, refining.\n\n", strings.Join(p.Devices, " and "), at)

	b.WriteString("1. What to deliver: if it is unclear which page, which device, or whether it is a new design or a change, ask the user once, all at once, and stop; otherwise start at once, saying what you assumed.\n")
	brief := "Write the brief with canvas_brief (" + at + ", brief): Markdown, a ## section a field (" + designbrief.Sections() + ")."
	if cv.Brief != nil {
		brief = "The canvas has a brief (canvas_brief, " + at + "): read it, and write it again where this build changes it."
	}
	fmt.Fprintf(&b, "2. Understand the product before designing: its category, who uses it and for what. Look at two or three products of its kind with the most distinctive looks, not the most famous (web search, their pages): what to keep (what users expect), what's the category's habit to break, what to do differently. Turn words like clean, premium or modern into decisions (calm or lively, dense or airy, polished or raw; then type, space, how much of a screen carries color). Decide what the first screen is for (the work itself for apps, dashboards, shops; a designed opening for pages that persuade), its one focal point and one main action, and list what to remove. %s The user can edit it in the canvas's Brief; read it again before each round.\n", brief)

	switch {
	case p.From == proto.BuildScreenshot:
		fmt.Fprintf(&b, "3. Recreate the screenshot (the canvas's picture %q, canvas_show lists it) as a mockup, faithfully: no new style. Note its size (a 2x picture is half that in CSS pixels) and make a design that size; match positions, proportions and type first, then borders, shadows and detail. After each round compare: canvas_match (%s, design N, image %q), and fix the cells that differ most first. Then make its phone layout as a design of its own. Improve or unslop it only if the user asks.\n", p.Image, at, p.Image)
	case p.From == proto.BuildProject:
		fmt.Fprintf(&b, "3. The project (%s) first: read its design system, tokens, components and conventions, and follow them. Recreate the page as it is now as a design named “Current”, the baseline. %s\n", p.Project, workAsk(p.Work, at))
	case p.From == proto.BuildChange:
		fmt.Fprintf(&b, "3. Change design %s: copy it first (canvas_new_design, from it), keep what the brief says is fixed, and change the copy.\n", p.Frame)
	}
	if p.Work != proto.WorkReview && p.From != proto.BuildScreenshot {
		if p.Directions > 1 {
			fmt.Fprintf(&b, "4. Directions: the user wants %d to compare. When the brief is written, canvas_build (%s, fanout %d): fresh agents each build one on a design of its own, from the brief alone, each from a different kind of source; the daemon checks they differ (of skeleton, type, palette and hero any two share at most one; their first screens squinted at must differ) and sends back the too-alike. Change nothing meanwhile and end your turn: you're told when they're built.\n", p.Directions, at, p.Directions)
		} else {
			fmt.Fprintf(&b, "4. One direction, from concepts you didn't write: when the brief is written, canvas_build (%s, fanout 3, concepts true): three fresh agents each write a concept's card on a design of its own, from the brief alone, each from a different kind of source. End your turn: you're told when they're in. Then check each against the brief (fits the users and task; its main action works; what it needs exists; it works on the devices), pick one, and build on its design.", at)
			if p.Confirm {
				b.WriteString(" Before building, tell the user the direction picked in three or four lines (how it feels, the first screen, the main action, type, color, the main picture) and why, and end your turn: build it once they agree.")
			}
			b.WriteString("\n")

		}
		b.WriteString("5. Build it in the order a designer would: the structure first, in grays (the first screen's focal point, main action and states), checked before any color or type; then the representative screen with its key states; then the rest. " + skeletons + " A page presenting a product shows what it really delivers (a generated report, an exported file, real processed data), not a list of what it can do. " + moments + " One memorable moment, the rest quiet. Pictures made for it, kept with the canvas (canvas_image: an SVG illustration you draw, or a picture chosen from the web), never gray boxes. Content that reads real (1,238 not 1,000; real names; specific buttons); nothing saying “sample” or “placeholder”.\n")
		fmt.Fprintf(&b, "6. Compare: canvas_compare the designs, the current one first when there is one, and tell the user which you recommend and why; they pick in Compare, with what to keep and change. Refine the one picked around that; keep the others.\n")
	}
	fmt.Fprintf(&b, "7. Check and finish: look at the design at desktop and phone sizes (canvas_screenshot, frame desktop and mobile); then unslop it as the last pass: canvas_unslop (%s, design N) and fix what it finds that you wouldn't keep on purpose; take away once more (one focal point a screen, no copy, lines or boxes that don't help). Then canvas_build (%s, done, design N): a fresh agent reviews it independently, and you're told its verdict; below the bar, fix what it lists and say done again.\n", at, at)
	b.WriteString("8. When the user is happy: Make code (the canvas's toolbar) builds it in their project, its tokens and rules kept as the project's design file.")
	return b.String()
}

// workAsk is how each kind of work on a project's pages goes.
func workAsk(work, at string) string {
	switch work {
	case proto.WorkRefresh:
		return "A UI refresh: first tidy the noise (what repeats, explains the obvious, decorates without helping), then upgrade the layout. Brand assets stay; defaults nobody chose may go. Show the work as designs here before any code changes: the code changes only once the user picks."
	case proto.WorkFlow:
		return "A flow fix: count the steps and clicks of the flow now, write them down, then cut them, keeping the project's look. Show the new flow as designs, a screen each, with the counts before and after; the code changes only once the user picks."
	case proto.WorkFeature:
		return "A new feature: fit what is there, its look, components and patterns; nothing new where something existing serves. Show it as designs before any code changes."
	case proto.WorkReview:
		return "A review only: change nothing. On the Current design, leave a comment on each element worth changing (canvas_comment, element …): what and why, the worst first, at most ten. Then canvas_build (" + at + ", done, design 1) and tell the user the three that matter most."
	}
	return ""
}

// directionAsk is what a direction's fresh agent is told: the brief alone,
// its source, its design; a concept's writes the card alone.
func directionAsk(cv proto.Canvas, f proto.CanvasFrame, n int, letter, source string, run proto.CanvasBuildRun, concept bool) string {
	at := fmt.Sprintf("canvas %q", cv.ID)
	brief := ""
	if cv.Brief != nil {
		brief = designbrief.Markdown(*cv.Brief)
	}
	if len(cv.References) > 0 {
		brief += "\n## References\n" + strings.Join(cv.References, "\n") + "\n"
	}
	kind, steps := "Direction", fmt.Sprintf(`2. Build its structure in grays first (the first screen's focal point, main action and states), then the real design, on your design only (canvas_write or canvas_patch, frame %q). %s One memorable moment, the rest quiet. Pictures made for it in its own style, kept with the canvas (canvas_image: an SVG you draw, or one chosen from the web), never gray boxes; content that reads real.
3. Look at it at desktop and phone width (canvas_screenshot, frame %q) and fix what reads wrong.
4. Then canvas_build (%s, done). If you're told it is too close to another direction, change what you're told and say done again.`, f.ID, skeletons, f.ID, at)
	if concept {
		kind, steps = "Concept", fmt.Sprintf(`2. Design nothing else: the lead picks one concept to build. Then canvas_build (%s, done). If you're told it is too close to another, change what you're told and say done again.`, at)
	}
	return fmt.Sprintf(`You design %s %s of a UI, one of several agents each designing one at once, the user watching live. Yours is design %d (frame %q) of %s, and only it: changes to anything else are refused. You know only the brief; never look at the others.

What to build: %s. Devices: %s.
%s
Grow it from %s, found in this product's own world. Never from an adjective, never from the category's usual look (%s), and never a retro prop unrelated to the product (receipts, cassettes, old consoles). Its colors come from the product (its content, users or brand), not from the source's material; not just a lighter or darker version of another.
%s

1. Write your card first: canvas_direction (%s, design %d, card), in this form:
%s
%s`,
		kind, letter, n, f.ID, at, nonEmpty(run.What, "see the brief"), strings.Join(run.Devices, " and "), brief, source, cliches, moments, at, n, designcompare.CardFormat, steps)
}

// skeletons is how a first screen is laid out by what the page is for.
const skeletons = `The first screen by what the page is for: one that persuades (landing, brand, launch) takes a skeleton chosen on purpose (a full-bleed picture with the words over it, one object centered, the product's own screen as the page, full-bleed type, stacked full-width bands, a collage grid, a vertical or diagonal flow, or a split, the split only with a reason and a strong picture); one for content or work (a gallery, a shop, a feed, docs, a tool, a dashboard) shows the first row of content or the workspace itself, the nav, title and filters in a line or two, no slogan, no paragraph saying what the page is. Copy is written after the structure, to fit it.`

// cliches are categories' usual layouts, which a direction must not fall
// into unthinkingly.
const cliches = "an admin's sidebar, top bar and grid of cards; a shop's banner, product grid and footer; a social feed with bottom tabs and a floating button; a landing page's hero, features, testimonials and call to action; a chat's contacts left and messages right; a dev or AI tool's black background, purple-blue glows, floating particles and pill buttons"

// moments are where a memorable moment is found.
const moments = "The memorable moment is found in what the user does (a press, a drag, a switch, a submit), a key moment (a payment done, a first task finished, a result arriving), a corner nobody expects (an empty state, a 404, loading), or AI at work made visible: the thing affected changes on the spot, numbers drawn as amounts, the domain's own objects carrying the result."

func nonEmpty(s, or string) string {
	if strings.TrimSpace(s) == "" {
		return or
	}
	return s
}
