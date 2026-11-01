package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/enowx/enowx/core/designbrief"
	"github.com/enowx/enowx/core/git"
	"github.com/enowx/enowx/core/unslop"
	"github.com/enowx/enowx/proto"
)

// Unslop in the Browser: the page a session's Browser shows, the
// project's own running on localhost, is read for what makes it look
// AI-made: its HTML and every stylesheet it loaded (a bundle's, a CSS
// framework's), with the colors it really shows. Started, a branch of its
// own is made in the session's folder, so the redesign is undone by going
// back, and the session's agent is told to redesign the project's code
// pass by pass, a commit each; it reads the page again at each, as the dev
// server reloads it.

// browserPage is what a run calls the page it reads, whatever its URL.
const browserPage = "page"

// pageScript gives the page shown as HTML whose styles are all of its
// stylesheets' rules, and its URL. The background it shows and its text
// color go in as the body's, so its theme reads true; the app's own
// marks (every element and stylesheet whose id starts __enx) are left
// out, so they're never read as the page's.
const pageScript = `(function(){var css=[];
for(var i=0;i<document.styleSheets.length;i++){var sh=document.styleSheets[i],n=sh.ownerNode;if(n&&n.id&&n.id.indexOf('__enx')===0)continue;
try{var rs=sh.cssRules;for(var j=0;j<rs.length;j++)css.push(rs[j].cssText)}catch(e){}}
var bg='',e=document.elementFromPoint(innerWidth/2,Math.min(innerHeight/2,200));
for(;e;e=e.parentElement){var c=getComputedStyle(e).backgroundColor;if(c&&c!=='transparent'&&!/rgba\([^)]*,\s*0\)$/.test(c)){bg=c;break}}
if(!bg)bg=getComputedStyle(document.body).backgroundColor;
css.push('body{background-color:'+bg+';color:'+getComputedStyle(document.body).color+'}');
var b=document.body.cloneNode(true);b.querySelectorAll('script,[id^="__enx"]').forEach(function(x){x.remove()});
return JSON.stringify({url:location.href,html:'<style>'+css.join('\n')+'</style>'+b.outerHTML})})()`

// localPage says page URL u is on this device: localhost, a loopback
// address, or a .localhost name.
func localPage(u string) bool {
	p, err := url.Parse(u)
	if err != nil || (p.Scheme != "http" && p.Scheme != "https") {
		return false
	}
	h := strings.ToLower(p.Hostname())
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// layoutScript measures the page as laid out now: its width, how far it
// scrolls sideways, and the elements (selectors, 8 a kind at most) that
// stick out past it, take touches under 44 px, or set text under 12 px.
// The app's own marks are hidden while it measures, as they were drawn
// at another width; what is hidden or a pixel big (a skip link until
// focused) isn't counted.
const layoutScript = `(function(){var ours=[].slice.call(document.querySelectorAll('[id^="__enx"]')),was=ours.map(function(e){return e.style.display});
ours.forEach(function(e){e.style.display='none'});
try{var de=document.documentElement,W=de.clientWidth||innerWidth;
function sel(e){if(e.id)return '#'+CSS.escape(e.id);var s=e.tagName.toLowerCase();for(var i=0;i<e.classList.length&&i<2;i++)s+='.'+CSS.escape(e.classList[i]);
var p=e.parentElement;return p&&p.id?'#'+CSS.escape(p.id)+' > '+s:s}
function shown(e,r){var st=getComputedStyle(e);return r.width>2&&r.height>2&&st.visibility!=='hidden'&&st.display!=='none'&&st.opacity!=='0'}
function scrolls(e){for(var p=e.parentElement;p&&p!==document.body;p=p.parentElement){var o=getComputedStyle(p).overflowX;if(o!=='visible')return true}return false}
var out={width:W,overflow:Math.max(0,de.scrollWidth-W),off:[],small:[],tiny:[]};
function add(a,s){if(a.length<8&&a.indexOf(s)<0)a.push(s)}
document.querySelectorAll('body *').forEach(function(e){if(e.closest('[id^="__enx"]'))return;var r=e.getBoundingClientRect();if(!shown(e,r))return;
if(r.right>W+1&&r.width<=W*2&&!scrolls(e))add(out.off,sel(e));
if(e.matches('a[href],button,input,select,textarea,[role=button]')&&getComputedStyle(e).display!=='inline'&&(r.width<44||r.height<44))add(out.small,sel(e));
for(var c=e.firstChild;c;c=c.nextSibling){if(c.nodeType===3&&c.textContent.trim()){if(parseFloat(getComputedStyle(e).fontSize)<12)add(out.tiny,sel(e));break}}});
return JSON.stringify(out)}finally{ours.forEach(function(e,i){e.style.display=was[i]})}})()`

// The widths a page is read at besides the one it shows: a tablet's and a
// phone's, those narrower than it.
var unslopWidths = []struct {
	px          int
	step, label string
}{{768, "At a tablet's width (768 px)", "Tablet"}, {390, "At a phone's width (390 px)", "Phone"}}

// marksHidden hides the app's own marks for a screenshot ("none"), or
// shows them again ("").
func marksHidden(display string) string {
	return `(function(){document.querySelectorAll('[id^="__enx"]').forEach(function(e){if(e.tagName!=='STYLE')e.style.display='` + display + `'});return true})()`
}

// shotNow is a PNG of the page as shown, the app's marks left out.
func (s *Server) shotNow(session string) []byte {
	if _, err := s.browserAct(proto.BrowserAction{Session: session, Action: proto.BrowserEval, Text: marksHidden("none")}); err != nil {
		return nil
	}
	defer s.browserAct(proto.BrowserAction{Session: session, Action: proto.BrowserEval, Text: marksHidden("")})
	res, err := s.browserAct(proto.BrowserAction{Session: session, Action: proto.BrowserScreenshot})
	if err != nil {
		return nil
	}
	return res.Image
}

// pageAsShown is the page the session's Browser shows: its URL and HTML,
// refused when it isn't on this device.
func (s *Server) pageAsShown(session string) (url, html string, err error) {
	res, err := s.browserAct(proto.BrowserAction{Session: session, Action: proto.BrowserEval, Text: pageScript})
	if err != nil {
		return "", "", err
	}
	var page struct{ URL, HTML string }
	if err := json.Unmarshal([]byte(res.Text), &page); err != nil {
		return "", "", fmt.Errorf("browser: the page didn't give its HTML: %w", err)
	}
	if !localPage(page.URL) {
		return "", "", fmt.Errorf("browser: unslop reads a page of the project running here (localhost), not %s", page.URL)
	}
	return page.URL, page.HTML, nil
}

// layoutNow measures the page as laid out now.
func (s *Server) layoutNow(session string) (unslop.Layout, error) {
	res, err := s.browserAct(proto.BrowserAction{Session: session, Action: proto.BrowserEval, Text: layoutScript})
	if err != nil {
		return unslop.Layout{}, err
	}
	var l unslop.Layout
	if err := json.Unmarshal([]byte(res.Text), &l); err != nil {
		return unslop.Layout{}, fmt.Errorf("browser: the page didn't give its layout: %w", err)
	}
	return l, nil
}

// layoutsAt measures the page as shown (its lint too) and at each width
// of unslopWidths narrower than it, the Browser shown at each in turn and
// then as it was;
// step is told each width's step as it begins (1, 2…), shot (when not
// nil) gets a screenshot at each width.
func (s *Server) layoutsAt(session string, step func(int), shot func(label string, width int, png []byte)) []proto.UnslopFinding {
	now, err := s.layoutNow(session)
	if err != nil || now.Width <= 0 {
		return nil // not laid out: nothing true to say of it
	}
	found := append(unslop.LayoutFindings(now), s.lintNow(session, now.Width)...)
	if shot != nil {
		shot("As shown", now.Width, s.shotNow(session))
	}
	was := ""
	for i, w := range unslopWidths {
		step(i + 1)
		if w.px >= now.Width {
			continue
		}
		res, err := s.browserAct(proto.BrowserAction{Session: session, Action: proto.BrowserWidth, Text: strconv.Itoa(w.px)})
		if err != nil {
			break // an older app shows one width only
		}
		if was == "" {
			was = res.Text
		}
		if l, err := s.layoutNow(session); err == nil && l.Width > 0 {
			found = append(found, unslop.LayoutFindings(l)...)
			if shot != nil {
				shot(w.label, l.Width, s.shotNow(session))
			}
		}
	}
	if was != "" {
		_, _ = s.browserAct(proto.BrowserAction{Session: session, Action: proto.BrowserWidth, Text: was})
	}
	return found
}

// browserUnslop reads the page of session p.Session's Browser.
func (s *Server) browserUnslop(p proto.BrowserUnslopParams) (proto.CanvasUnslop, error) {
	if p.Status {
		run, ok := s.browserUnslops.status(p.Session)
		if !ok {
			return proto.CanvasUnslop{}, errors.New("browser: no unslop under way in this session's Browser")
		}
		return proto.CanvasUnslop{Score: run.Score, Findings: run.Findings, Run: &run}, nil
	}
	if p.Start {
		return s.startBrowserUnslop(p.Session)
	}
	if p.Brief != "" {
		b, err := s.keepProjectBrief(p.Session, p.Brief)
		if err != nil {
			return proto.CanvasUnslop{}, err
		}
		if run, ok := s.browserUnslops.setBrief(p.Session, b); ok {
			s.broadcast(proto.EventBrowserUnslop, run)
		}
	}
	if p.Pass != "" && !strings.EqualFold(p.Pass, "done") && !s.browserUnslops.hasBrief(p.Session) {
		return proto.CanvasUnslop{}, errNoBrief(`"$ENOWX_BIN" browser unslop --brief -, the brief on stdin`)
	}
	u, html, err := s.pageAsShown(p.Session)
	if err != nil {
		return proto.CanvasUnslop{}, err
	}
	// Every width again for the mobile pass and at the end; else the
	// widths' findings as last measured.
	layout, measured := s.browserUnslops.layoutOf(p.Session)
	done := strings.EqualFold(p.Pass, "done")
	if p.Pass == "12" || done || !measured {
		var after func(string, int, []byte)
		if done {
			after = func(label string, w int, png []byte) { s.browserUnslops.addShot(p.Session, label, w, png, true) }
		}
		layout = s.layoutsAt(p.Session, func(int) {}, after)
		s.browserUnslops.setLayout(p.Session, layout)
	}
	theme := s.browserUnslops.theme(p.Session, browserPage)
	findings := browserFindings(html, theme, layout)
	out := proto.CanvasUnslop{Frame: u, Score: unslop.Score(findings), Findings: findings}
	run, ok, err := s.browserUnslops.update(p.Session, browserPage, proto.CanvasUnslopParams{Pass: p.Pass}, out.Score, findings, theme)
	if err != nil {
		return out, err
	}
	if ok {
		s.broadcast(proto.EventBrowserUnslop, run)
		out.Run = &run
		if done && run.Done {
			s.background.start(func() { s.reviewPage(p.Session, u) })
		}
	}
	return out, nil
}

// browserFindings are what a page's HTML and its rendered layouts and
// lint show, those measured replacing the HTML's guesses of the same, its
// theme first when it turned from theme.
func browserFindings(html, theme string, layout []proto.UnslopFinding) []proto.UnslopFinding {
	_, findings := unslop.Scan(html, proto.CanvasMockup)
	findings = unslop.WithRendered(findings, layout)
	if fd, turned := unslop.ThemeFinding(theme, html); turned {
		findings = append([]proto.UnslopFinding{fd}, findings...)
	}
	return findings
}

// startBrowserUnslop starts a run on session's page, step by step as the
// app shows: the page read as shown and at each width, its styles
// checked, a branch made; then what to tell the session's agent.
func (s *Server) startBrowserUnslop(session string) (proto.CanvasUnslop, error) {
	steps := []string{"Reading the page as shown"}
	for _, w := range unslopWidths {
		steps = append(steps, w.step)
	}
	steps = append(steps, "Checking its styles at every size", "Making a branch for the changes")
	step := func(i int) {
		if run, ok := s.browserUnslops.step(session, i); ok {
			s.broadcast(proto.EventBrowserUnslop, run)
		}
	}
	s.broadcast(proto.EventBrowserUnslop, s.browserUnslops.begin(session, browserPage, steps))
	u, html, err := s.pageAsShown(session)
	if err != nil {
		s.browserUnslops.drop(session)
		s.broadcast(proto.EventBrowserUnslop, proto.CanvasUnslopRun{ID: session})
		return proto.CanvasUnslop{}, err
	}
	layout := s.layoutsAt(session, step, func(label string, w int, png []byte) {
		s.browserUnslops.addShot(session, label, w, png, false)
	})
	s.browserUnslops.setLayout(session, layout)
	step(len(unslopWidths) + 1)
	theme := unslop.Theme(html)
	findings := browserFindings(html, "", layout)
	score := unslop.Score(findings)
	step(len(unslopWidths) + 2)
	branch, base, err := s.unslopBranch(session)
	if err != nil {
		s.browserUnslops.drop(session)
		s.broadcast(proto.EventBrowserUnslop, proto.CanvasUnslopRun{ID: session})
		return proto.CanvasUnslop{}, err
	}
	s.browserUnslops.setTheme(session, theme)
	brief := s.projectBrief(session)
	out := proto.CanvasUnslop{Frame: u, Score: score, Findings: findings, Ask: browserAsk(u, score, len(findings), theme, branch, base, brief != nil)}
	if _, ok, err := s.browserUnslops.update(session, browserPage, proto.CanvasUnslopParams{}, score, findings, theme); err != nil || !ok {
		return out, err
	}
	run := s.browserUnslops.setBranch(session, branch, base)
	if brief != nil {
		run, _ = s.browserUnslops.setBrief(session, brief)
	}
	s.broadcast(proto.EventBrowserUnslop, run)
	out.Run = &run
	return out, nil
}

// browserAsk is what the session's agent is told to redesign the page at
// u in the project's code, on branch (from base; "" outside git); briefed
// when the project keeps a brief already.
func browserAsk(u string, score, found int, theme, branch, base string, briefed bool) string {
	cmd := `"$ENOWX_BIN" browser unslop`
	keep := "Keep its look's direction: its theme, brand colors and mood stay; unslop changes its care, not its style."
	if theme != "" {
		keep = "It is a " + theme + " page and stays " + theme + ": never turn its theme. " + keep
	}
	where := "This folder isn't a git repository, so nothing is undone by a branch: change only what the passes call for."
	commit := ""
	if branch != "" {
		where = fmt.Sprintf("You are on branch %s, made for this from %s: the whole redesign goes there, so going back to %s undoes it.", branch, base, base)
		commit = ` After each pass, commit what it changed on this branch, the pass's name as the message ("Unslop: 03 Spacing"), so each can be undone alone; the brief goes in with the first.`
	}
	brief := "Write its brief"
	if briefed {
		brief = "The project keeps a brief (" + designbrief.Path + "): read it, and write it again where the product has moved on"
	}
	return fmt.Sprintf(`Unslop the page in your Browser (%s). Its code reads %d/100 with %d findings of what makes a design look AI-made, but rules read only the code: what a page looks like, and what it is for, you judge. The user watches the page live. %s
1. Understand the product first: the project's README and context ("$ENOWX_BIN" project context .), the page itself, and how it looks at a desktop's and a phone's width ("$ENOWX_BIN" browser screenshot). %s, with %s --brief - (Markdown on stdin, a ## section a field: %s). Tone as decisions (calm or lively, dense or airy, polished or raw), not adjectives; Remove names what repeats, explains the obvious or decorates without helping. Passes are refused until the brief is written.
2. Run: %s — the findings by pass, in the order to fix them, each with what to avoid, and the elements.
3. Look at the screenshots against the brief for what rules can't read: a screen with more than one focal point or main action, a label over every heading, a glow or gradient doing no work, rows of boxes alike, text saying what the layout already shows, a look any product of its kind could have. Each goes in the pass it belongs to. Then say honestly how much is off, and change that much: many signs across color, type, icons and copy, every pass in full; a designed page with a few signs left, fix only those, its palette, type, layout and pictures kept, bigger ideas told as suggestions; nothing off, say so, suggest at most three refinements with exact values, and end with %s --pass done unless the user wants them.
4. Redesign it in this project's code (its components, styles, tokens, copy), one pass at a time, in that order, keeping its content, purpose and behavior; never invent data, and follow the project's own design system where it has one. %s
5. Begin each pass with: %s --pass N (N its number, 1 to 12): it reads the page again, as the dev server reloaded it, and the app shows the pass under way. Change the code a part at a time, so each change shows.%s
6. Before done, take away once more: each screen one focal point and one main action, the rest quiet; then screenshots at both widths again, looked at. When what is left is what you'd keep on purpose: %s --pass done, then sum up what you changed, file by file, and how to undo it.`,
		u, score, found, where, brief, cmd, designbrief.Sections(), cmd, cmd, keep, cmd, commit, cmd)
}

// unslopBranch makes a branch of its own for a redesign of session id's
// code and checks it out: its name, and the branch it started from. A
// folder outside git gets none; one with changes not committed is refused,
// so the branch holds the redesign alone.
func (s *Server) unslopBranch(id string) (branch, base string, err error) {
	sess, err := s.sessions.Get(id)
	if err != nil {
		return "", "", err
	}
	dir := sess.Info().Dir
	if _, err := git.TopLevel(dir); err != nil {
		return "", "", nil
	}
	if status, err := git.Run(dir, "status", "--porcelain"); err != nil {
		return "", "", fmt.Errorf("browser: read the project's changes: %w", err)
	} else if status != "" {
		return "", "", errors.New("browser: the project has changes not committed: commit or stash them first, so the unslop's branch holds its changes alone and is undone cleanly")
	}
	base = git.CurrentBranch(dir)
	if base == "" {
		base, _ = git.Run(dir, "rev-parse", "--short", "HEAD")
	}
	branch = "unslop/" + time.Now().Format("20060102-1504")
	for n := 2; git.BranchExists(dir, branch); n++ {
		branch = fmt.Sprintf("unslop/%s-%d", time.Now().Format("20060102-1504"), n)
	}
	if _, err := git.Run(dir, "switch", "-c", branch); err != nil {
		return "", "", fmt.Errorf("browser: make the unslop's branch: %w", err)
	}
	return branch, base, nil
}

// browserUnslopShots are session's run's screenshots, before and after.
func (s *Server) browserUnslopShots(session string) (proto.UnslopShots, error) {
	shots, ok := s.browserUnslops.shotsOf(session)
	if !ok {
		return proto.UnslopShots{}, errors.New("browser: no unslop's screenshots in this session's Browser")
	}
	return proto.UnslopShots{Shots: shots}, nil
}
