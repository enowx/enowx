# Canvas

The rail's **Canvas** page keeps canvases on the connected host: mockups of UIs in HTML and CSS, wireframes, diagrams (flows, architectures) and free boards. The user and agents design them together. An agent's changes reach every app connected to the host as it makes them, an element at a time, so the user watches the design build up, whether the agent runs on this Mac or on a server.

## Kinds and frames

A canvas is one of four kinds:

- **Mockup**: a UI in HTML and CSS, shown in a web view of this device, interactive.
- **Wireframe**: a UI in boxes and text.
- **Diagram**: shapes joined by arrows.
- **Board**: stickies, shapes and images, laid out freely.

A canvas has frames, its pages or the same page at a device's size. Frames named Desktop (1440×900), Tablet (768×1024) or Mobile (390×844) take those sizes; others take any. The toolbar's tabs switch frames, and **+** adds one at a device's size or removes the one shown.

A frame holds a mockup's HTML, its elements with `id` attributes, and shapes above it: rectangles, ellipses, decisions (diamonds), text, stickies, lines, arrows and images. Arrows join two shapes and follow them as they move.

## Designing

The stage shows the frame, scaled to fit.

- In **Design** mode, a click selects a shape and a drag moves it. The corner handle resizes it, and a double click edits its text in place. Delete removes it.
- The palette down the stage's side adds shapes. **Connect** draws an arrow from the selected shape to the next one clicked.
- **Layers** lists the frame's elements: the mockup's by id, and the shapes. The selected one shows its properties: a shape's text, fill and line colors, and a mockup element's HTML to edit and apply.
- A mockup previews at its frame's size, or at a desktop's, tablet's or phone's width.
- In **Interact** mode the mockup works as the real thing: buttons, links, forms and hover states.

Every change is made to one element of the canvas as the host has it now. A change the user makes while an agent designs never undoes the agent's work, and the agent's never undoes the user's.

## Many designs

A canvas's frames are its designs, numbered from 1: the variations an agent was asked for ("10 landing pages"), a flow's screens, or a page at a device's size. Each has a name, a line on what it tries, and a page and a variant to group by (Home · A, Home · B, Login · A).

- The strip beside the stage shows every design, small, grouped by page. A click shows one, and a drag puts it before another. The design an agent is making is dotted, and the page follows the agent to it.
- **‹** and **›**, or ← and →, step through them; 1 to 9 jump to one.
- **Grid** shows them all at once. **Pick to compare** on two to four of them, then **Compare** shows them side by side at one scale, scrolling together.
- A design is starred, picked the winner, duplicated as a variant, named and described, or deleted from its menu. Agents read what the user starred and picked.
- Comments go with the design they are on.
- `canvas open`, `goto` and `compare` never move the user: they raise a card ("An agent opened it") whose click goes there, and nothing when it already shows.
- In Interact mode, a mockup's links to other designs move between them: `href="#design-2"` or `data-goto="Login"`. The shared page does the same, showing the design it names alone (**Show all designs** goes back).
- `canvas://ID/design-3` links to a design. In a note or a chat, it opens the Canvas page there; **Copy link** in a design's menu gives it.

## Prototypes

A prototype is a canvas's designs linked together, one per page, so the strip, comments and versions work on each. Each page is its own document, so what it should remember goes through `enowx.state`, in every mockup: `data-set="plan=pro;step=2"` sets it on a click, `data-bind="email"` edits and shows a value, `data-show-if="plan=pro"` or `"!email"` shows by it, `data-reset` empties it, with `enowx.state.get/set/all/reset` and an `enowx:state` event for scripts. Interact mode and the shared page carry it from page to page alike, and each has a **Reset**, which empties it and returns to the first design. On the board a design shows as it starts.

Scripts stay in their design. On the shared page each design is a sandboxed frame with an origin of its own: it can't read the page around it or its link, reach the sharing server, or keep storage, and it only tells the page where to go and what state holds. In the app each load of a mockup gets an address of its own that leads nowhere, so the page counts as a stranger's: calls into the app are refused, the device's files can't be read, and what it stores isn't there for another design. The app only reads from the page the link clicked and the state. A design can still load from the internet, as a page does; that is how it gets its libraries.

## Layout grids

A design has a layout grid, as CSS grid lays one out: columns and rows of equal fractions, a column gap and a row gap, a margin, and named areas spanning cells. The grid button in the toolbar opens its panel.

- **Add a 12-column grid** starts one: 8 columns for a tablet's width, 4 for a phone's. Its numbers change in the fields, and **Apply** sets them.
- **Draw an area**, then drag across cells on the design, makes an area (`area-1`…). The list beside removes them.
- Guides show the columns and rows on the stage, and the areas outlined and named. The eye in the toolbar (Shift+G), or **Guides** in the panel, hides them at once: for this device and the whole canvas, kept when it is reopened, with the grid, its areas and Snap as they were, and no new version. An agent adding a grid doesn't bring them back.
- With **Snap** on, a shape moved or resized lands on the grid's lines, and so do the shapes agents place.
- **CSS** is the grid's: the container (`.grid`) and each area by its id, to copy. **Lay the mockup out on it** makes the design's HTML the grid, with an empty element per area to fill.

Agents set it with `canvas grid ID --cols 12 --rows 8 --gap 24 --margin 48 --area header:1,1,12,1 … --apply`. The canvas guide has them give a design its grid before placing anything, so every design of a canvas shares one rhythm.

## Libraries and icons in mockups

A mockup renders in the app's web view, so libraries load from CDNs when the device is online. The canvas guide has agents pin exact versions, with a snippet each:

- Tailwind CSS, shadcn-style tokens, DaisyUI and Bootstrap.
- Lucide and Tabler icons.
- Inter from Google Fonts.
- anime.js, Chart.js and three.js.

Offline, those don't load: a mockup keeps its layout and colors in its own CSS, and icons go in as inline SVG. `canvas icon lucide:rocket` gives one from the Lucide, Tabler and Phosphor icons enowX carries.

## At any window size

The page keeps the stage usable at any window size:

- In a narrow page, the canvases' list hides behind a button in the header.
- The designs' strip hides when the stage would be too narrow.
- Buttons show as icons alone.
- The agent's panel takes at most about a third of the window.
- Compared designs stack when they don't fit side by side.

## The agent beside the canvas

**Ask an agent** in the header opens a panel beside the page to design with an agent, as the Notes page's panel does. Drag its edge to change its width.

- Pick Claude Code, Codex or any other installed agent. The canvas keeps the pick, so every host has it.
- Each message starts with where the user is: the canvas, the design shown and the element selected. The chat shows the message without that line.
- `@design-3` names a design, `@#id` an element. **Ask agent** in an element's properties starts a message about it. **Send to agent** on a comment has the agent do it, then resolve it.
- Pictures pasted or attached go with the message. They are kept with the canvas as its assets, which the agent reads.
- Every change the agent makes shows live. In review mode (the history button in the panel's header), once the agent is done, a card shows the versions its message took the canvas through. **Keep**, **Undo** (back to the version when the message was sent) or **Compare**. In direct mode, changes stay with nothing asked.
- The list button shows the messages sent, each with the canvas's version then, to compare with or go back to.
- The agent runs in its own folder (`~/.enowx-app/canvas-chat/<id>`), out of the session list. It gets a guide of its own and the canvas skill. Claude Code is held to the canvas tools (MCP `canvas_*`), web search, and reading the canvas's folder and the projects the canvas belongs to (`--add-dir`), which it reads for their design system and components. It has no tools for writing files or running commands. Other agents are asked to keep to it.

The conversation is kept in `~/.enowx-app/canvas/<id>/chat.json`: the agent, the mode, and each message with the canvas's version then (`canvas.chat`, `canvas.chatLog`, `canvas.chatMark`, event `canvas.chat`).

## Agents designing live

When an agent changes a canvas, a bar under the header says who is designing and which element. The element changed is outlined a moment, with the agent's name beside it, and the page follows the agent to the frame it works on. Mockup elements go into the page in place, so it doesn't reload at each change. The list of canvases marks those being designed.

- **Pause** refuses the agent's changes until **Resume**. The agent is told it was paused, and goes on from what the canvas shows when resumed.
- **Stop** interrupts the agent, as Esc does in its terminal, and pauses.

Agents learn how to design from the canvas guide (`enowx-app canvas guide`), which the session guide points to rather than carrying it. It covers the steps, layout and type rules, a project's own design system first (Tailwind, shadcn/ui tokens), components to start from (navbar, card, form field, table, modal), responsive frames, and SVG icons only.

## Comments, versions, exports

- **Comments** go on the selected element, or on the frame. Agents read them (`canvas comments`), do them, and resolve them.
- **Versions** lists every change: whose it was, and what it changed. One compares with the version shown, the elements added, removed and changed, and **Restore** makes an old version the newest. Undo and Redo (⌘Z, Shift+⌘Z) step through versions.
- **Export** saves the frame as PNG, SVG, PDF or HTML into Downloads.
- **Share as a page** saves the whole canvas as one read-only HTML file, its images inside. There is no cloud: share the file.
- **Share a link** has the host serve that page, read-only, to whoever has the link: on this device (`127.0.0.1`) and on the user's tailnet (the host's Tailscale address), never on other networks, nothing going to the internet. The page reloads as the canvas changes. No canvas has a link until the user makes one; **Stop sharing** (or deleting the canvas) ends it. The links are the host's own, kept in `canvas-shares.json` beside the synced folders, and come back when the daemon restarts. `canvas share CANVAS`, `canvas unshare CANVAS` and `canvas shares` do the same from the command line.
- **Make code** starts an agent in one of the host's projects on building the canvas there, in the project's stack and components. It attaches the canvas to the project. **More** attaches a canvas to, or detaches it from, other projects.

## Rendering

The daemon draws a frame of shapes itself (PNG, PDF and SVG). Agents on a server see what they made without a screen or an app.

A frame with HTML needs a web engine. The daemon asks the app connected to the host and serving Browsers (`canvas.render` event), which loads the page in a web view behind its window, captures it, and answers (`canvas.renderReply`). Without such an app, rendering a mockup says to open it.

## Where they are kept

On the host, in `~/.enowx-app/canvas/<id>/`:

- `canvas.json`: the canvas as shown.
- `history/<n>.json`: every version, whole, the last 400.
- `history.json`: which version is shown, and what made each.
- `comments.json`.
- `chat.json`: the conversation with the agent beside it.
- `assets/`: images that shapes show (`--src NAME`) and HTML shows (`assets/NAME`).

## Protocol

| Method | Does |
|---|---|
| `canvas.list`, `canvas.get` | The canvases, a canvas with its frames |
| `canvas.create`, `canvas.delete` | Make, delete |
| `canvas.edit` | A change: `meta`, `frame.add`/`set`/`remove`/`meta`/`move`/`copy`/`grid`, `pick`, `html.set`, `html.patch` (replace element `target`, or add into `into`), `html.remove`, `html.text`, `el.set`, `el.remove`; `session` marks an agent's |
| `canvas.history`, `canvas.version`, `canvas.undo`, `canvas.redo`, `canvas.restore` | Versions |
| `canvas.comments`, `canvas.comment` | Comments: add, resolve, reopen, delete |
| `canvas.control` | `pause`, `resume`, `stop`, `open` (tells the apps of it, at design `frame`, as a card), `compare` (of designs `frames` side by side) |
| `canvas.chat`, `canvas.chatLog`, `canvas.chatMark` | The agent beside it: a message recorded with the canvas's version, the agent and mode picked; the conversation; a message's changes kept or undone |
| `canvas.asset`, `canvas.assetGet` | Keep and read an image |
| `canvas.render`, `canvas.renderReply` | A frame as PNG, PDF, SVG or HTML; the whole canvas as HTML |

Events: `canvas.changed` (a `CanvasChange`: the canvas, the op, frame and element, the session), `canvas.listed` (the list), `canvas.render` (to the app).

## The board

A design shows whole on an endless, dotted board, at its own width times the zoom: scrolling (two fingers, a wheel, Shift for sideways) moves it, pinching or scrolling with Command (Control) zooms at the pointer, dragging the board beside it (or with the middle button) moves it, and the controls at the bottom right zoom out and in, show it at its own size, fit its width or fit it all. Moving between designs keeps the zoom set, the new design centered from its top, and each canvas keeps its zoom while the app runs; a canvas's first design fits its width. Below 50% a mockup shows as its picture, rendered at its own width (a web view zoomed out that far would lay its page out anew, narrower), so its content shrinks whole with its frame; from 50% up, and until the picture comes, the live page shows, and a click on the picture zooms in there. In Design mode the mockup's page doesn't scroll and takes no pointer, so the board moves over it too; Interact mode shows it as a browser does, scrolling and taking clicks.

Shapes lie on the board itself, free form: a design without a page draws no sheet under them, so agents start a diagram, wireframe or board with a sheet of their own (a large light rect, behind everything) and keep text on it; screenshots and exports of such a design show the bare board's color around it, as the app does. A click on a mockup's page picks the component under it (the element with an id, or the nearest around it, outlined): a bar over it asks the AI about it or says what to change (sent to the canvas's agent with the component named), **Redesign** has the AI redesign it alone, and its note button keeps a note on it instead. Escape or × lets it go. While an agent works, the view stays where the user put it: the strip marks the design it works on.

A design must never be wider than its frame: nothing sticking out sideways, so its page never scrolls horizontally. The agents' guide says so, `canvas screenshot` and `canvas export` warn when a page is wider, and the board shows "Wider than its frame by N px" over such a design.

Interact mode shows a design whole in a centered box with a margin all round, scaled to fit the stage, never above its own size, so it clears the bars and its `100vh` is its frame's height. A page taller than its frame scrolls inside. In a window too small for 50%, the box stays at 50% and what's cut is reached by scrolling the page: a web view zoomed out further would lay the page out anew.
