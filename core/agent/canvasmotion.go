package agent

// CanvasMotionGuide is motion on a canvas in depth, which an agent reads when
// asked for an animation ("enowx-app canvas guide motion"), not with every design.
const CanvasMotionGuide = `# Motion on an enowX canvas

Motion earns its place by explaining something: where a thing came from, what changed, what to look at next, that work is under way. A design with one moment done well reads as made; ten effects read as a template. If you can't say what a movement explains, take it out.

## Which tool
- CSS @keyframes: one element, a loader, a pulse on a state. No library, smallest page.
- anime.js timeline: several elements in sequence, numbers counting, SVG paths drawing, a logo's reveal. Most product animation is this.
- three.js: only what really is 3D, a product turning, a scene the eye moves through. A flat logo in a 3D engine is a slower flat logo. It adds about 900 KB to each design that uses it, and needs WebGL.
anime.js 3.2.2 and three.js 0.160.0 (its core) go with enowX at the addresses in the guide's Libraries: offline, on the shared page and in HTML exports alike. Three's addons (OrbitControls, GLTFLoader) aren't bundled: from a CDN they need the internet and the whole scene fails without it, so write the little you need. Other versions, and GSAP, Lottie or Rive from a CDN, load online only and aren't on the clock unless you add them.

## The clock
enowx.motion is one clock for the page. What is on it can be played, paused, restarted and scrubbed by the user, and looks the same each time. CSS animations and anime.js instances running as the page loads join by themselves.
    enowx.motion.add(timeline)                 anime.js: build the timeline whole, then add it
    enowx.motion.add(t => { … }, {element, duration, loop})   a draw function, t in ms
    enowx.motion.set({duration, loop, autoplay})   the whole design's, when its parts don't say it
    enowx.motion.play() .pause() .restart() .seek(ms) .state()   for the design's own Replay button
Everything must be a function of t:
- Set values from t (mesh.rotation.y = t / 1000), never add to them each frame (+=), and no renderer.setAnimationLoop: the user couldn't scrub it, and it would never stop rendering.
- No Math.random() or Date.now() in what moves. Take random values once, from a fixed list or a small seeded generator, before the first frame.
- Don't start things from anime's complete or update callbacks, or from setTimeout: scrubbing runs past them. Put it on the timeline.
- Pass element to a scene's add, so it stops rendering when it can't be seen. Draw at a fixed size, setPixelRatio(Math.min(devicePixelRatio, 2)), a few thousand triangles.
- A loop's last frame must equal its first. Parts of a loop take the same period, or multiples of it: turn by Math.PI * 2 * t / duration.

## Timing
- Feedback (a press, a toggle): 100–200 ms. Entrance of one element: 300–600 ms. A sequence of a few: 1–2 s in all. A logo: 1.2–2.5 s, then rest. A product story: scenes of 2–4 s with a beat of stillness between; under about 15 s.
- Nothing waits on an animation to be read or used. Text holds still for as long as it takes to read.
- Exits are shorter than entrances, by about a third.

## Easing
- Entering: easeOutCubic or easeOutQuint, fast then settling. Leaving: easeInCubic. Moving across: easeInOutCubic. Linear only for what really turns at a constant rate: a spinner, a progress line.
- One or two curves for the whole design. Springs (spring(1, 90, 12, 0)) for what is handled by hand; a bounce on a serious product is a joke at its expense.
- Move little. 8–24 px of travel says "arrived"; 200 px says "look at me". Fade with it, and let the eye lead with position or opacity, not both at full strength.

## Choreography
- One thing leads, the rest follow: decide what the eye watches at every moment.
- Overlap steps ('-=200' on a timeline) so it flows; steps that wait for each other read as a slideshow.
- Stagger 30–60 ms between siblings, in reading order, and no more than five or six; past that show the rest together.
- Layers move in the same direction at different distances, not each its own way.
- End on the design's real composition, exactly where its layout puts everything. Animate from an offset to 0, so the page's HTML and CSS are the finished frame.

## What looks cheap, and AI-made
Every section fading up as it scrolls in; everything at the same 600 ms and the same delay; floating blobs and drifting gradients; pulsing glows on buttons; particles behind a hero; text typed letter by letter; a card tilting to the pointer; a 3D torus knot for no reason; numbers counting up to figures you made up; motion that never rests. canvas unslop's motion pass finds some of these; loops are for real progress alone.

## Reduced motion
enowX honors prefers-reduced-motion for you: such a viewer gets the finished frame at load, and it still plays when they press Play. So write no media query of your own taking the animation away (it would stop Play too), and make the finished frame read whole: nothing important only visible mid-way. On the board too a design shows finished, and plays in Interact.

## Checking it
A still catches one moment: canvas screenshot, PNG, PDF and SVG exports show the end (a loop's start). Sample it instead:
    "$ENOWX_BIN" canvas frames ID 3 --at 0,25,50,75,100   # files, and one sheet of them in order; MCP canvas_frames answers the sheet
    "$ENOWX_BIN" canvas frames ID 3 --at 400ms,800ms,1200ms   # around a moment that matters
- 0: is the start composed, or a blank page or a flash of hidden content?
- The middle: is one thing clearly leading, or is it all half-faded at once? Is anything overlapping or off its frame?
- 100: exactly the design, nothing left offset or at partial opacity?
- Read what it prints: "the same picture" means nothing moved there, so it's a dead stretch, or the motion isn't on the clock. "No motion" means the page never reported any.
- Run canvas unslop too: it applies as to any design.
Only HTML exports and the shared page keep the motion. There is no video export yet; say so if asked for one.

## Examples
A logo's reveal, in SVG:
    <script src="https://cdnjs.cloudflare.com/ajax/libs/animejs/3.2.2/anime.min.js"></script>
    <svg id="logo" viewBox="0 0 120 120" width="120"><path id="mark" d="M20 60a40 40 0 1 1 80 0" fill="none" stroke="currentColor" stroke-width="10" stroke-linecap="round"/></svg><span id="name">Halden</span>
    <script>
    const tl=anime.timeline({easing:'easeOutCubic'});
    tl.add({targets:'#mark',strokeDashoffset:[anime.setDashoffset,0],duration:900,easing:'easeInOutCubic'})
      .add({targets:'#name',translateX:[-12,0],opacity:[0,1],duration:500},'-=250');
    enowx.motion.add(tl);
    </script>
A product turning, on the clock, looping without a seam:
    <script type="importmap">{"imports":{"three":"https://cdn.jsdelivr.net/npm/three@0.160.0/build/three.module.js"}}</script>
    <div id="scene" style="width:640px;height:400px"></div>
    <script type="module">
    import * as THREE from 'three';
    const el=document.getElementById('scene'),r=new THREE.WebGLRenderer({antialias:true,alpha:true});
    r.setPixelRatio(Math.min(devicePixelRatio,2));r.setSize(640,400);el.append(r.domElement);
    const s=new THREE.Scene(),c=new THREE.PerspectiveCamera(35,640/400,.1,100);c.position.z=5;
    const key=new THREE.DirectionalLight(0xffffff,2);key.position.set(2,3,4);s.add(new THREE.HemisphereLight(0xffffff,0x334155,1),key);
    const m=new THREE.Mesh(new THREE.BoxGeometry(1.6,1,.2),new THREE.MeshStandardMaterial({color:0xd4d4d8,roughness:.4}));s.add(m);
    const d=8000;enowx.motion.add(t=>{m.rotation.y=Math.PI*2*t/d;r.render(s,c)},{element:el,duration:d,loop:true});
    </script>
`
