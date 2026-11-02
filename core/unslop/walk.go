package unslop

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/enowx/enowx/proto"
)

// A prototype's walk: its tabs, menus, dialogs and pages opened, each state it
// reaches measured as the first screen is. It runs in the copy of the design
// the app renders out of sight, so nothing of the canvas changes, and only
// there: a Browser page's clicks can start real actions, so it is never walked.
//
// It runs in one go, as a page's script is evaluated once: what a click changes
// at once is seen, and what takes a timer or a fetch isn't. The steps are every
// click that changed something, from the first screen.

// WalkScript clicks through a design and returns its states, each with its
// lint, as JSON. It stops at 24 states, 6 seconds or 200 clicks, keeping the
// states it reached.
const WalkScript = `(function(){
var LINT=function(){return ` + LintScript + `};
var MAXS=24,MAXMS=6000,MAXT=200,MAXP=12,t0=performance.now();
var out={states:[],tried:0,left:0,limit:''};
try{window.alert=window.confirm=window.prompt=window.open=function(){return false}}catch(x){}
document.addEventListener('click',function(e){var a=e.target&&e.target.closest&&e.target.closest('a[href]');if(a&&!/^#/.test(a.getAttribute('href')||''))e.preventDefault()},true);
document.addEventListener('submit',function(e){e.preventDefault()},true);
var SEL='[data-go],[data-tool],[data-open],[data-menu],[data-section],[role=tab],[role=menuitem],[aria-haspopup],[aria-expanded],[aria-controls],summary,button,a[href^="#"],[onclick]';
function sign(){var b=document.body;return (b?b.innerText:'')+'|'+document.querySelectorAll('dialog[open],[role=dialog]').length}
function name(e){var t=(e.getAttribute('aria-label')||e.innerText||e.getAttribute('title')||'').replace(/\s+/g,' ').trim().slice(0,40);return t||e.tagName.toLowerCase()}
function shown(e){var r=e.getBoundingClientRect();if(!r.width||!r.height||e.disabled)return false;var s=getComputedStyle(e);return s.display!=='none'&&s.visibility==='visible'}
var tried={},seen={},path=[];seen[sign()]=1;
function key(e){return e.tagName+'|'+name(e)}
function next(){var all=document.querySelectorAll(SEL);for(var i=0;i<all.length;i++){var e=all[i];if(!tried[key(e)]&&shown(e))return e}return null}
while(true){
if(out.states.length>=MAXS){out.limit='states';break}
if(performance.now()-t0>MAXMS){out.limit='time';break}
if(out.tried>=MAXT){out.limit='tries';break}
var c=next();if(!c)break;
tried[key(c)]=1;out.tried++;
var was=sign(),nm=name(c);
try{c.click()}catch(x){continue}
var now=sign();if(now===was)continue;
path.push(nm);if(path.length>MAXP)path=path.slice(-MAXP);
if(seen[now])continue;seen[now]=1;
var lint='';try{lint=LINT()}catch(x){}
out.states.push({name:nm,steps:path.slice(),lint:lint});
}
var rest=document.querySelectorAll(SEL);for(var j=0;j<rest.length;j++){if(!tried[key(rest[j])]&&shown(rest[j]))out.left++}
out.ms=Math.round(performance.now()-t0);
return JSON.stringify(out)})()`

// walkable finds what a click could change, for a design not to be loaded again
// for nothing.
var walkable = regexp.MustCompile(`(?i)\bonclick\b|data-(go|tool|open|menu|section)\b|<button\b|<summary\b|<dialog\b|role=["']?(tab|menuitem)|aria-(haspopup|expanded|controls)|href=["']#|<script\b`)

// Walkable says html has something a walk could open.
func Walkable(html string) bool { return walkable.MatchString(html) }

// ParseWalk reads what WalkScript returned of a design at width: the walk's
// report, and each state's findings, naming it and how to reach it, those of a
// state being only what the first screen didn't show already.
func ParseWalk(raw string, width int, first []proto.UnslopFinding) (*proto.UnslopWalk, []proto.UnslopFinding) {
	var in struct {
		States []struct {
			Name  string   `json:"name"`
			Steps []string `json:"steps"`
			Lint  string   `json:"lint"`
		} `json:"states"`
		Tried int    `json:"tried"`
		Left  int    `json:"left"`
		Limit string `json:"limit"`
		MS    int    `json:"ms"`
	}
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		return nil, nil
	}
	known := map[string]bool{}
	for _, f := range first {
		known[f.Rule] = true
	}
	walk := &proto.UnslopWalk{Tried: in.Tried, Left: in.Left, Limit: in.Limit, Millis: in.MS}
	var out []proto.UnslopFinding
	for _, st := range in.States {
		found := LintFindings(st.Lint, width)
		state := proto.UnslopWalkState{Name: st.Name, Steps: st.Steps}
		for _, f := range found {
			state.Findings++
			if known[f.Rule] {
				continue // the first screen has it: one fix, one finding
			}
			known[f.Rule] = true
			f.State, f.Steps = st.Name, st.Steps
			out = append(out, f)
		}
		walk.States = append(walk.States, state)
	}
	return walk, out
}

// ScoreStates is findings' score when some are of states a walk reached: that
// of the worst state, each scored with what every state shows. A design is as
// good as the worst thing it can show, which an average would hide behind tidy
// screens, the more of them the better.
func ScoreStates(findings []proto.UnslopFinding) int {
	var base []proto.UnslopFinding
	states := map[string][]proto.UnslopFinding{}
	for _, f := range findings {
		if f.State == "" {
			base = append(base, f)
		} else {
			states[f.State] = append(states[f.State], f)
		}
	}
	worst := Score(base)
	for _, own := range states {
		worst = min(worst, Score(append(append([]proto.UnslopFinding{}, base...), own...)))
	}
	return worst
}

// WalkReport says what a walk reached and what it didn't, for the agent.
func WalkReport(w *proto.UnslopWalk) string {
	if w == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("\nThe walk clicked what the design offers, ")
	b.WriteString(plural(w.Tried, "control", "controls"))
	b.WriteString(", and measured " + plural(len(w.States), "state", "states") + " beyond the first screen.")
	for _, st := range w.States {
		b.WriteString("\n- " + st.Name + ": " + strings.Join(st.Steps, " > "))
	}
	if w.Limit != "" {
		b.WriteString("\nIt stopped at its limit of " + w.Limit + ", with " + plural(w.Left, "control", "controls") + " untried: what they open wasn't measured.")
	} else if w.Left > 0 {
		b.WriteString("\n" + plural(w.Left, "control", "controls") + " weren't tried.")
	}
	b.WriteString("\nWhat a click only shows after a timer or a fetch isn't seen: look at those states yourself.\n")
	return b.String()
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}
