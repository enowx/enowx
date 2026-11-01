package agent

// CanvasGuide is the canvas skill: how an agent designs on a canvas, which
// it reads when asked for a mockup, a wireframe or a diagram ("enowx-app
// canvas guide"), not in every session's guide, as it is long.
const CanvasGuide = `# Designing on an enowX canvas

A canvas is a mockup of a UI (HTML and CSS), a wireframe, a diagram or a free board, kept on this host and shown live in the user's enowX app (Canvas page): they watch each element appear as you write it, comment on elements, edit them, pause or stop you. Work an element at a time, so they see the design build up and their changes to other elements stay.

## Start
    "$ENOWX_BIN" canvas new "Checkout" --kind mockup --frame desktop --frame mobile   # prints its ID
    "$ENOWX_BIN" canvas open ID          # tells the user in the app it's there, with a card they may click; it never moves them
    "$ENOWX_BIN" canvas show ID          # frames, elements by id, open comments
Kinds: mockup (HTML, real UI), wireframe (boxes and text, grey, no styling), diagram (shapes and arrows: flows, architecture, sequence), board (stickies, shapes, images). Frames named desktop (1440×900), tablet (768×1024) or mobile (390×844) take those sizes; --frame "Home:1280x800" any.

## Mockups: HTML an element at a time
1. The skeleton with the page's CSS, its regions empty, each with an id:
    "$ENOWX_BIN" canvas write ID --html - <<'EOF'
    <style>/* tokens and layout: see below */</style>
    <header id="nav"></header><main id="main"></main><footer id="footer"></footer>
    EOF
2. Then each piece, in reading order, in its own call: patch replaces element E, or adds the HTML at the end of --into P when no element has id E:
    "$ENOWX_BIN" canvas patch ID --element hero --into main --html '<section id="hero">…</section>'
3. Small changes: --text sets what an element holds; remove takes one out.
    "$ENOWX_BIN" canvas patch ID --element hero-title --text "Ship faster"
Give every element you may change later an id (kebab-case, unique). One <style> block in the skeleton; add rules with a patch of an element <style id="styles-cards">. Small JavaScript for what a page does (tabs, a menu, a modal, a form's states): the canvas is interactive in the app's Interact mode and on its shared page, so buttons, links between designs, inputs and :hover states should work. Scripts have no storage or server of their own: no localStorage, cookies or calls to enowX.
Responsive: one HTML per frame. Write desktop first, then the tablet and mobile frames (--frame mobile) as their own layouts, not a squeezed desktop: stacked columns, a menu button, 16px gutters, 44px tap targets.

## Prototypes
Asked for a prototype, or for something to click through, build it so by default: one design per page (--page Home, Pricing, Checkout), linked, so the strip, comments and versions still work on each. Every page carries the same navigation. Move between them with data-goto="Pricing" or href="#design-2". What the pages share lives in enowX's state, not in a script's variable or localStorage, which end with the page:
    <button data-set="plan=pro;step=2" data-goto="Checkout">Choose Pro</button>   sets state as it goes
    <input data-bind="email">  <b data-bind="email"></b>   edits and shows a value, on any page
    <div data-show-if="plan=pro">…</div>  <div data-show-if="!email">…</div>   shows by state
    <button data-reset>  enowx.state.get/set/all/reset  document.addEventListener('enowx:state',…)
Build its real states with these, in the page: empty (no items yet), loading (set it, then setTimeout to clear it), error (a form left wrong), filled. Menus and dialogs open for real. Keep it a design: the unslop check applies as to any other, and a screenshot shows each page as it starts, so check a state by setting it. Interact's Reset takes it back to the start.

## Many designs in one canvas
Asked for several designs or variations ("10 landing pages", "3 versions of the hero"), make them designs of one canvas, not one long page: each is a frame the user browses, compares side by side, stars and picks from. Number them from 1.
    "$ENOWX_BIN" canvas new-design ID --title "Bold hero" --description "Big type, one call to action" --page Home --variant B   # prints its number
    "$ENOWX_BIN" canvas write ID --frame 2 --html -          # then build design 2 as any mockup
    "$ENOWX_BIN" canvas new-design ID --from 5 --variant C   # a copy of design 5 to vary
    "$ENOWX_BIN" canvas designs ID      # numbers, pages × variants, the favorites and the winner the user marked
    "$ENOWX_BIN" canvas goto ID 3 | compare ID 2 5 | pick ID 4   # goto and compare raise a card too: the user stays where they are until they click it
Group a flow's screens with --page (Home, Login, Dashboard) and alternatives with --variant (A, B, C); give each design a title and a one-line description of what it tries. Work on what the user starred or picked when asked to "develop designs 3 and 7", and read the comments on each design. Between a flow's screens, link buttons to other designs so Interact mode moves between them: href="#design-2" or data-goto="Login". A design's link for notes and chats is canvas://ID/design-N.

## Wireframes, diagrams, boards: shapes
The board is bare and dark, as a design tool's: nothing is under your shapes until you put it there. Start each diagram, wireframe or board with a sheet of its own, the first shape, behind everything: a rect as large as the content plus 48–64 around it, a light fill (#ffffff or #f8fafc) or the project's surface, radius 16, a subtle stroke (#e2e8f0); then its title as text on it, top left, and only then the content, all on the sheet. Never put text on the bare board, where dark text can't be read: a title, a label or a note goes on the sheet or in a shape. Several groups each take a section (a rect of their own on the sheet, a slightly darker fill such as #f1f5f9, their heading inside). Make the sheet first so later shapes draw above it; grow it (canvas shape ID --id sheet --w … --h …) when the content outgrows it.
    "$ENOWX_BIN" canvas shape ID --type rect --id sheet --x 0 --y 0 --w 1200 --h 760 --fill "#ffffff" --stroke "#e2e8f0" --radius 16
    "$ENOWX_BIN" canvas shape ID --type rect --id api --x 80 --y 120 --w 200 --h 80 --text "API" --fill "#dbeafe" --radius 10
    "$ENOWX_BIN" canvas shape ID --type arrow --from api --to db --text "SQL"
Types: rect, ellipse, diamond (decisions), text, sticky, line, arrow, image (--src an asset kept with "canvas asset ID logo.png"). Arrows follow the shapes they join when those move. Lay out on a grid of 8: equal gaps (40–80 between boxes), flows left to right or top to bottom, at most 7–9 boxes per frame; split bigger diagrams into frames. A legend sticky when colors mean something.

## Look at what you made
    "$ENOWX_BIN" canvas screenshot ID /tmp/canvas.png --frame mobile   # then read the PNG
Check spacing, alignment, overflow and contrast; fix and look again before saying it is done. The screenshot is the design at its frame's size, as the board shows it at any zoom: if it looks right there and uses no viewport sizes (above), it looks right zoomed out too.

## Unslop: what makes it look AI-made
    "$ENOWX_BIN" canvas unslop ID --frame N   # findings by pass, each with its fix, and a score
Run it on a mockup before saying it is done, and fix what it finds unless you keep it on purpose. Asked to unslop a design (the app's Unslop button copies it and asks you), redesign the copy one pass at a time in the order printed, never turning its theme (dark stays dark, light stays light) nor its brand's colors, (truth, structure, spacing, type, color, surfaces, icons, copy, data, motion, access, mobile), keeping its content and purpose, describing the design with what each pass changed, and run the command again after each.

## The user's comments, pause and stop
"$ENOWX_BIN" canvas comments ID lists what they asked for on which element; do each, then "canvas resolve ID COMMENT". A change refused with "paused" means the user paused you: stop writing, tell them, and go on from "canvas show" when they resume. Every change is a version: "canvas history", "undo", "restore N".

## From design to code
Asked to make it real: read the project's components and tokens first, then build it in the project's stack (React, Vue, Svelte or plain HTML) matching the frames, one component per region, and compare with "canvas screenshot" against the app running in the Browser. A frame's page starts from html,body{margin:0} at the frame's width, with its background: start the code's page the same (reset the body's margin), or everything sits a few pixels off.

## Grid first
Before placing anything, give the design a layout grid; the user sees it as guides, shapes snap to it, and every design of the canvas then shares one rhythm:
    "$ENOWX_BIN" canvas grid ID --frame 1 --cols 12 --rows 8 --gap 24 --margin 48 \
      --area header:1,1,12,1 --area hero:1,2,7,3 --area visual:8,2,5,3 --area features:1,5,12,3 --area footer:1,8,12,1 --apply
--apply lays the design's HTML out on it (a <style> with the grid's CSS and an empty element per area): then fill each area with canvas patch --element hero …. Desktop: 12 columns, gap 24, margin 48–80; tablet (768): 8 columns, gap 16, margin 32; mobile (390): 4 columns, gap 16, margin 16. Shapes (wireframes, boards) snap to the grid's lines; place them by column and row, never at ad-hoc pixels. canvas grid ID prints the grid and its CSS again.

## Libraries in mockups
A mockup renders in the user's enowX app (a web view of their device), so libraries load from a CDN when the device is online. Pin an exact version, as below; never "latest". Offline, a CDN's script or stylesheet doesn't load: the page falls back to its own CSS, so keep the layout, colors and type in your own <style> (or Tailwind's classes with the CDN) and use libraries for what is extra (charts). anime.js and three.js are the exception: at exactly these addresses they go with enowX and work offline. Icons work offline as inline SVG.
- Tailwind CSS (utility classes, in the browser):
    <script src="https://cdn.tailwindcss.com/3.4.16"></script>
    <script>tailwind.config={theme:{extend:{colors:{accent:'var(--accent)'},fontFamily:{sans:['var(--font-text)','sans-serif']}}}}</script>
    <section id="hero" class="mx-auto max-w-6xl px-6 py-20 grid gap-6 md:grid-cols-2"><h1 class="text-4xl font-semibold tracking-tight">…</h1></section>
- shadcn/ui-style tokens (with Tailwind or plain CSS), light and dark:
    <style>:root{--background:0 0% 100%;--foreground:222 47% 11%;--primary:221 83% 53%;--primary-foreground:0 0% 100%;--muted:210 40% 96%;--muted-foreground:215 16% 47%;--border:214 32% 91%;--radius:0.5rem}
    .dark{--background:222 47% 11%;--foreground:210 40% 98%;--muted:217 33% 17%;--border:217 33% 17%}
    body{background:hsl(var(--background));color:hsl(var(--foreground))} .btn{background:hsl(var(--primary));color:hsl(var(--primary-foreground));border-radius:var(--radius)}</style>
  with Tailwind: tailwind.config={theme:{extend:{colors:{background:'hsl(var(--background))',foreground:'hsl(var(--foreground))',primary:'hsl(var(--primary))',border:'hsl(var(--border))'},borderRadius:{lg:'var(--radius)'}}}}
- DaisyUI (components on Tailwind): <link href="https://cdn.jsdelivr.net/npm/daisyui@4.12.14/dist/full.min.css" rel="stylesheet"> before the Tailwind script; <button class="btn btn-primary">, <div class="card bg-base-100 shadow">, data-theme="light" on <html>.
- Bootstrap: <link href="https://cdn.jsdelivr.net/npm/bootstrap@5.3.3/dist/css/bootstrap.min.css" rel="stylesheet"> and <script src="https://cdn.jsdelivr.net/npm/bootstrap@5.3.3/dist/js/bootstrap.bundle.min.js"></script> for its dropdowns and modals.
- Icons: prefer inline SVG, which works offline and takes the text's color: "$ENOWX_BIN" icons search "rocket" finds names in Lucide, Tabler and Phosphor, "$ENOWX_BIN" canvas icon lucide:rocket --size 20 prints the SVG to paste. From a CDN instead: Lucide <script src="https://cdn.jsdelivr.net/npm/lucide@0.468.0/dist/umd/lucide.min.js"></script> with <i data-lucide="rocket"></i> and lucide.createIcons() after it; a Tabler icon as <img src="https://cdn.jsdelivr.net/npm/@tabler/icons@3.24.0/icons/outline/rocket.svg" width="20" height="20" alt="">. No emoji or icon fonts as icons.
- Fonts: <link rel="preconnect" href="https://fonts.googleapis.com"><link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">, with font-family:Inter,system-ui,sans-serif so it falls back offline. Two families at most: one for text, one for headings or code.
- Motion (anime.js, goes with enowX, works offline; see Motion below): <script src="https://cdnjs.cloudflare.com/ajax/libs/animejs/3.2.2/anime.min.js"></script>
    <script>anime({targets:'#hero h1',translateY:[16,0],opacity:[0,1],duration:600,easing:'easeOutCubic'})</script>
- Charts (Chart.js): <script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.1/dist/chart.umd.min.js"></script>
    <canvas id="chart-revenue" height="240"></canvas>
    <script>new Chart(document.getElementById('chart-revenue'),{type:'line',data:{labels:['Jan','Feb','Mar','Apr'],datasets:[{label:'Revenue',data:[12,19,15,24],borderColor:getComputedStyle(document.documentElement).getPropertyValue('--accent'),tension:.3}]},options:{plugins:{legend:{display:false}}}})</script>
- 3D (three.js, goes with enowX, works offline), as a module:
    <script type="importmap">{"imports":{"three":"https://cdn.jsdelivr.net/npm/three@0.160.0/build/three.module.js"}}</script>
    <div id="scene" style="height:360px"></div>
    <script type="module">import * as THREE from 'three';const el=document.getElementById('scene');const r=new THREE.WebGLRenderer({antialias:true,alpha:true});r.setSize(el.clientWidth,el.clientHeight);el.append(r.domElement);const s=new THREE.Scene(),c=new THREE.PerspectiveCamera(45,el.clientWidth/el.clientHeight,.1,100);c.position.z=4;const m=new THREE.Mesh(new THREE.TorusKnotGeometry(.8,.28,128,16),new THREE.MeshNormalMaterial());s.add(m);enowx.motion.add(t=>{m.rotation.y=Math.PI*2*t/6000;r.render(s,c)},{element:el,duration:6000,loop:true})</script>
Screenshots (canvas screenshot) are made by the app too, so they show libraries as the user sees them; give scripts that draw (charts, 3D) a fixed height, and draw on load, not on a click.

## Motion
A design can really move: a product's animation in its mockup, a logo's reveal, a loader. Use the least that does it: CSS @keyframes for one element or a loader, an anime.js timeline for several in sequence (no longer limited to entrances), three.js only for what really is 3D. Each is for explaining something; the unslop check still applies. Short feedback and entrances stay short (150–600 ms).
All of it runs on one clock, enowx.motion, so the user can play, pause, restart and scrub it (Interact's bar, the shared page's) and it looks the same each time. CSS animations and anime.js running at load join by themselves; add what doesn't, once it is built:
    enowx.motion.add(timeline)     enowx.motion.add(t => { mesh.rotation.y = t / 1000; renderer.render(scene, camera) }, {element: el, duration: 6000})
Draw from t alone: no += each frame, no setAnimationLoop, no Math.random or Date.now in what moves. Pass a scene's element, so it stops rendering when it can't be seen. Viewers with prefers-reduced-motion get the finished frame until they press Play, so write no media query of your own taking it away.
A screenshot shows one frame, the end. To check motion, look at moments of it: "$ENOWX_BIN" canvas frames ID 3 --at 0,25,50,75,100 gives them in order on one sheet, and says when nothing moved. Only HTML exports and the shared page keep the motion; PNG, PDF and SVG catch one frame; there is no video export yet.
Asked for more than an entrance, read "$ENOWX_BIN" canvas guide motion first: timing, easing, choreography, what looks cheap, and how to check it.

## Design rules
- Sized to its frame, not to a screen: a design is seen at every zoom, small on the board and full size in Interact, so lay it out in pixels at the frame's width (a 1440 frame holds a centered container of 1120–1200, a 390 frame a 358 one) and let it not depend on the viewport: no vw or vh sizes, no clamp() on vw for type, no 100vh sections (use the frame's height in px when a section must fill it), and no media queries that change the layout inside a frame (each device is a design of its own). Then it shows the same at 10% as at 100%.
- Never wider than its frame: no horizontal scroll, ever. Nothing may stick out past the frame's width: no fixed widths beyond it (a 1440 frame holds a container of 1200 at most), no rows of cards or nav items that don't wrap, no negative margins or translations past the edge, no 100vw plus padding (use 100% and box-sizing:border-box), images and long words held in (max-width:100%; overflow-wrap:anywhere). A carousel scrolls inside its own box (overflow-x:auto on it), never the page. Check it before saying it is done: the page's width must be the frame's (the app warns "wider than its frame" on the design when it isn't).
- Grid first (above): every block starts and ends on the grid's columns; no element positioned by guesswork.
- One spacing scale, multiples of 4, the same gap between siblings of a kind and more space between sections than inside them; no value picked by eye.
- One type scale (a steady ratio), one or two families chosen for this product, line heights that suit them, few sizes on a screen; numbers that change in tabular figures.
- Contrast: text at least 4.5:1 on its background (3:1 for large text and icons); test both themes when there are two.
- Responsive: each device a design of its own on its own grid (desktop 12, tablet 8, mobile 4 columns), not a squeezed desktop; nothing wider than its frame; tap targets 44 px on mobile.
- Use the project's design system when there is one: Tailwind config (theme colors, fonts, radius), shadcn/ui (components.json, CSS variables), a tokens or theme file, existing components. Read them and reuse the names and values.
- Without one, there is no house style: choose a direction for this product (its mood, type, color, density) from what it is and who uses it, set it as CSS variables, keep it consistent, and say it in the design's description. The first idea that comes to mind is everyone's; don't settle for it.
- Consistency over taste: one radius system, one way to separate surfaces (a border, a shadow or a tint), one icon set at one stroke, one accent that means something, states in their own colors.
- Hierarchy: one primary action per region; size, weight and position order the page, not color, emoji or badges. Real copy, never lorem ipsum or invented names and numbers: the product's own, or a labelled placeholder.
- Visible focus, labels on every input; empty, loading and error states where they matter.
- Icons are inline SVG in currentColor, never emoji or icon fonts.
- Layout with flexbox and grid; no absolute positioning for layout; no fixed heights on text containers. Sections come from the content, not from a template.

## Avoid: what makes a design look AI-made
Violet, indigo or purple-to-blue gradients by default; gradient text; colored glows, glass everywhere, colored side stripes on cards; emoji as icons or in labels; the same few faces as everything's display type (Inter, Geist, Space Grotesk…) with no reason; bold everywhere; badges and pills on everything; cards in cards; a centered hero over three equal feature cards; greetings, exclamation marks and hype words; invented stats, testimonials and logos; a +N% with no period; rainbow or gradient charts; entrance animations on everything. "$ENOWX_BIN" canvas unslop ID --frame N finds these in a mockup.

## Structure of common parts (style them with the design's own tokens)
Give every part an id, so it can be changed alone:
    <header id="nav"><a id="nav-logo">…</a><nav id="nav-links">…</nav><button id="nav-cta">…</button></header>
    <article id="card-revenue"><span>label</span><strong>value</strong><span>change, and its period</span></article>
    <label id="field-email">Email <input type="email"></label>
    <table id="orders"><thead>…</thead><tbody>…</tbody></table>
    <div id="modal" role="dialog">…</div>
`
