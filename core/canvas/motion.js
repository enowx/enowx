// enowx.motion: one clock for a design's motion, so that play, pause, restart
// and seek(t) show the same picture at the same t. Playing is seeking on every
// frame: anime.js instances, CSS and Web Animations, and draw functions all take
// the time from here, none keeps a clock of its own.
(function () {
  var E = (window.enowx = window.enowx || {});
  var sources = [], known = [], options = {};
  var t = 0, playing = false, raf = 0, last = 0, drawn = 0, posted = 0;
  var started = false, seen = true, hold = !!window.__enowxMotionHold;
  var framed = window.parent !== window;

  function animeSource(a) {
    var primed = false;
    function loops() {
      if (a.loop === true) return Infinity;
      var n = +a.loop || 1;
      return a.direction === 'alternate' && n < 2 ? 2 : n;
    }
    function prime() {
      // A fresh instance writes nothing at 0 until it has been past it, so the
      // first frame would differ from every later one at the same time.
      if (primed || !a.duration) return;
      primed = true;
      a.seek(a.duration - 0.001);
      a.seek(0);
    }
    return {
      end: function () { return loops() === Infinity ? Infinity : a.duration * loops(); },
      cycle: function () { return a.duration; },
      apply: function (at) {
        if (!a.paused) a.pause();
        prime();
        var d = a.duration, n = loops();
        if (!d) return;
        var k = Math.floor(at / d), local = at - k * d;
        if (k >= n) { k = n - 1; local = d; }
        if (a.direction === 'alternate' && k % 2) local = d - local;
        if (a.direction === 'reverse') local = d - local;
        // Reaching its very end counts a loop down and turns it around; a loop
        // is driven by this clock alone, so it stops just short.
        a.seek(n > 1 ? Math.min(local, d - 0.001) : local);
      }
    };
  }

  function waapiSource(a) {
    return {
      end: function () {
        var c = a.effect ? a.effect.getComputedTiming() : null;
        return c ? c.endTime : 0;
      },
      cycle: function () {
        var c = a.effect ? a.effect.getComputedTiming() : null;
        return c ? c.delay + (+c.duration || 0) : 0;
      },
      apply: function (at) {
        if (a.playState !== 'paused') a.pause();
        a.currentTime = at;
      }
    };
  }

  // A draw function is a pure function of time: heavy ones say their element,
  // and aren't called while it can't be seen, except to seek.
  function drawSource(fn, o) {
    var visible = true;
    if (o.element && window.IntersectionObserver) {
      new IntersectionObserver(function (es) {
        visible = es[es.length - 1].isIntersecting;
      }).observe(o.element);
    }
    return {
      end: function () { return o.loop ? Infinity : o.duration || Infinity; },
      cycle: function () { return o.duration || 10000; },
      apply: function (at, force) {
        if (!force && !visible) return;
        drawn++;
        fn(o.duration && !o.loop ? Math.min(at, o.duration) : at);
      }
    };
  }

  function add(x, o) {
    if (!x || known.indexOf(x) >= 0) return E.motion;
    known.push(x);
    o = o || {};
    var s = typeof x === 'function' ? drawSource(x, o)
      : x.seek && x.pause ? animeSource(x)
      : waapiSource(x);
    sources.push(s);
    if (started) draw(true);
    return E.motion;
  }

  // scan takes what the page set going by itself as it loaded: what starts later
  // is a click's feedback, and stays its own. A transition is a state changing,
  // never part of the design's motion.
  function scan() {
    if (window.anime && window.anime.running) window.anime.running.slice().forEach(function (a) { add(a); });
    if (!document.getAnimations) return;
    document.getAnimations().forEach(function (a) {
      if (a.playState === 'idle') return;
      if (window.CSSTransition && a instanceof window.CSSTransition) return;
      add(a);
    });
  }

  function finiteEnd() {
    return sources.reduce(function (m, s) {
      var e = s.end();
      return e === Infinity ? m : Math.max(m, e);
    }, 0);
  }

  function endless() {
    return sources.some(function (s) { return s.end() === Infinity; });
  }

  function length() {
    if (options.duration) return options.duration;
    return finiteEnd() || sources.reduce(function (m, s) { return Math.max(m, s.cycle()); }, 0);
  }

  function looping() {
    if (options.loop !== undefined) return !!options.loop;
    return sources.length > 0 && !finiteEnd();
  }

  function draw(force) {
    sources.forEach(function (s) {
      try { s.apply(t, force); } catch (err) { console.warn('enowx.motion:', err); }
    });
  }

  function state() {
    var d = length();
    return { d: d, t: Math.min(t, d), p: playing, n: sources.length, f: drawn, l: looping() };
  }

  function report(now) {
    if (!framed) return;
    if (now !== undefined && now - posted < 120) return;
    if (now !== undefined) posted = now;
    window.parent.postMessage({ enowx: 'motion', d: length(), t: state().t, p: playing }, '*');
  }

  function tick(now) {
    raf = 0;
    if (!playing) return;
    // Added up by frame, so that time hidden or behind other windows isn't jumped.
    var dt = Math.min(now - last, 250);
    last = now;
    if (seen && document.visibilityState !== 'hidden') {
      t += dt;
      var d = length();
      if (t >= d) {
        if (looping()) {
          t = d ? t % d : 0;
        } else if (!endless()) {
          t = d;
          playing = false;
          draw(false);
          report();
          return;
        }
      }
      draw(false);
      report(now);
    }
    raf = requestAnimationFrame(tick);
  }

  function run() {
    if (!raf) {
      last = performance.now();
      raf = requestAnimationFrame(tick);
    }
  }

  function seek(ms) {
    t = Math.max(0, Math.min(+ms || 0, length()));
    draw(true);
    report();
    return state();
  }

  function play() {
    begin();
    if (t >= length()) t = 0;
    playing = true;
    report();
    run();
    return state();
  }

  function pause() {
    playing = false;
    draw(true);
    report();
    return state();
  }

  function restart() {
    begin();
    t = 0;
    draw(true);
    return play();
  }

  // settle shows the one moment a still should: where it was asked for, else
  // the finished composition, else a loop's start.
  function settle() {
    begin();
    playing = false;
    var at = window.__enowxMotionAt, d = length();
    if (at) return seek(at.ms !== undefined ? at.ms : (d * at.pct) / 100);
    return seek(looping() ? 0 : d);
  }

  function reduced() {
    return !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
  }

  function begin() {
    if (started) return;
    started = true;
    scan();
  }

  // On load a design's motion plays by itself, unless it is shown as it is
  // (the board, a still) or the viewer asked for less motion, when it shows
  // finished. An explicit play() plays either way.
  function auto() {
    begin();
    if (!sources.length) return report();
    if (hold || window.__enowxMotionAt || reduced() || options.autoplay === false) settle();
    else restart();
  }

  E.motion = {
    add: add,
    set: function (o) { for (var k in o) options[k] = o[k]; return E.motion; },
    play: play, pause: pause, restart: restart, seek: seek, settle: settle, state: state,
    hold: function (on) { hold = !!on; auto(); return state(); }
  };

  if (window.IntersectionObserver) {
    addEventListener('DOMContentLoaded', function () {
      new IntersectionObserver(function (es) { seen = es[es.length - 1].isIntersecting; }).observe(document.documentElement);
    });
  }
  addEventListener('message', function (e) {
    var m = e.data;
    if (e.source !== window.parent || !m || m.enowx !== 'motion') return;
    if (m.act === 'play') play();
    else if (m.act === 'pause') pause();
    else if (m.act === 'restart') restart();
    else if (m.act === 'seek') { playing = false; seek(m.t); }
    else report();
  });
  if (document.readyState === 'complete') setTimeout(auto, 0);
  else addEventListener('load', function () { setTimeout(auto, 0); });
})();
