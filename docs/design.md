# Designing with agents: brief, unslop, review, build

How enowX has agents design a UI and judge one, on a canvas or a project's running page.

## The brief

Before an agent designs or redesigns, it writes what the design is for (`proto.DesignBrief`, `core/designbrief`):

- the product and its category;
- its users and the task they come for;
- the tone as decisions, not adjectives;
- the world it belongs to (a place or material, like paper and ink for notes), which tints its neutrals and gives its accent and pictures;
- what the first screen is for, with its one focal point and one main action;
- its signature, one small detail only this product would have, used the same way wherever it belongs;
- peers looked at;
- what to remove;
- what is fixed.

Where the brief is kept:

- **A canvas:** on the canvas itself (`canvas brief`, `canvas_brief`). The user edits it in the canvas's Brief sheet.
- **A project:** in `.enowx/brief.md` (`browser unslop --brief -`).

An unslop pass and a build's later stages are refused until the brief is written.

## Unslop

Unslop reads a design for what makes it look AI-made, in 12 passes (`core/unslop`). It reads the design two ways:

- **The HTML and CSS, statically.**
- **The page as rendered.** The lint script (`LintScript`) measures computed styles, and a layout read takes the page at a tablet's and a phone's width. A Browser page is measured in the app's Browser. A canvas design is measured in the app's renderer (`CanvasRenderRequest.Script`).

Rules see only the code, so the agent is also told to look at screenshots against the brief.

The agent then says how much is off and changes that much. A screen with many signs gets every pass in full. A designed screen with a few signs left gets only those fixed: its palette, type, layout and pictures stay, and bigger ideas are told as suggestions. A screen with nothing off is left as it is, with at most three refinements suggested.

A Browser unslop works on a page served from localhost. Its changes go on a branch of their own, `unslop/…`.

Screenshots are taken at each width when the run starts and again when it ends, for the Compare slider.

### A canvas redesign's steps

The chevron beside **Unslop** picks the passes to run (`CanvasUnslopParams.Only`); the agent is told to skip the others.

As each pass ends (the next begins, or "done"), the daemon keeps the design as it left it: a step (`proto.UnslopStep`), in `~/.enowx-app/unslop/<canvas>-<design>.json`, with the design it started from.
- **Versions:** the canvas's version at the step's end is labelled with its pass ("Unslop · 03 Spacing"). Passes region agents did at once are one step.
- **Measured:** the app renders the step's design and the one before with `unslop.MeasureScript`. For each element with an id, it reads the box, font, size, weight, line height, tracking, colors, padding, gap, radius, shadow and border. `unslop.Changes` lists what changed, before → after, leaving to an element what it only inherits; the 40 elements most changed first, and those added and removed. Without an app connected, a step is measured when the Replay asks.

The panel lists the steps and what each changed. **Replay** plays them back:
- **Playback (`unslop.StoryHTML`):** each step's design over the last, its changed elements boxed in red with their sizes counting from the old value to the new, then revealed by a wave from the top left, the step's name in a chip.
- **Compare:** two steps, with a slider between them.
- **Changes:** the list of what each step changed.
- **Leave this step out (`canvas.unslopLeaveOut`, `canvas.LeaveOut`):** a copy of the design where what the step changed, and nothing changed since, is put back as it was before. What the step added is taken out. Elements changed again since stay, and are named.
- **Export:** the play as a video in the Downloads folder.
  - MP4 with ffmpeg when the device has it, else a GIF.
  - Square (720, cropped close on what the busiest step changed) or wide (960×540).
  - At most five steps (`unslop.StoryGroup`), 2.2 s each, ending on the score.
  - Played in a hidden window and captured frame by frame at 15 fps.

Its card keeps to the area it is over: its title and score stay in place, the rest scrolls inside it, new rows leaving what's being read where it is, and a button folds it to its head to free the design.

## Independent review

When an unslop or a build is done, the daemon starts a fresh agent (`core/designreview`, `daemon/design_review.go`). It gets a folder holding only the screenshots and the brief.

1. The reviewer scores the design on a fixed rubric: vetoes, a three-second look, then each dimension, with score anchors.
2. It sends its verdict with `enowx-app review -`.
3. If the score is below 8/10, the design's agent is told what to fix. It says done again, and the review runs again.
4. There are at most 3 rounds.

## The UI builder

The Canvas page's **Build** dialog starts a build (`proto.CanvasBuildRun`, `daemon/canvas_build.go`). It takes:

- what to build;
- what it starts from: a new design, a change to the design shown, a project's pages with the kind of work, or a screenshot;
- the devices;
- either one recommended direction, or 2 to 4 directions to compare.

The canvas's agent then goes through these steps:

1. **Brief.** It writes the brief.
2. **Directions.** With one direction, it writes its card (`canvas_direction`) and, if asked, waits for the user's OK. With several, it fans out (`canvas_build --fanout N`): fresh agents each build one direction on a design only they may change. Each starts from the brief alone and a different kind of source.
3. **Distinctness check.** When every direction is done, the daemon compares each pair (`core/designcompare`). Two directions may share at most one of skeleton, type, palette and hero. Their first screens are also compared squinted at. A direction too close to another is sent back once.
4. **Compare and pick.** The current design (a Browser page can be put on the canvas as one) comes first. Every design shows at a desktop's or a phone's width. Pick sends the design chosen, with what to keep and what to change, to the agent.
5. **Finish.** The agent refines the design, unslops it, and calls `canvas_build --done`, which starts the independent review.

A recreated screenshot is matched against its picture cell by cell (`canvas_match`).

Make code turns the chosen design into the project's code. It keeps the design's tokens and rules in `.enowx/design.md`.
