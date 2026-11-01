package canvas

import (
	"strings"
)

// A prototype is a canvas's designs linked together, so the strip, comments and
// versions keep working on its pages. Each page is its own document, so what it
// needs to remember goes through the runtime here, in every mockup: the host
// (the app's Interact mode, the shared page) carries it from one to the next.
//
//	enowx.state.set("plan", "pro"), .get("plan"), .all(), .reset()
//	<button data-set="plan=pro;step=2">   sets state when clicked
//	<button data-reset>                   empties it
//	<input data-bind="email">, <b data-bind="email"></b>   shows and edits a value
//	<div data-show-if="plan=pro">, data-show-if="!plan"    shown by state
//
// "enowx:state" fires on document as it changes.

// prototypeRuntime is the state a design's scripts and attributes use;
// PrototypeSeed goes before it.
const prototypeRuntime = `<style>[data-show-if][hidden]{display:none!important}</style><script>(function(){
var state=window.__enowxSeed||{},framed=window.parent!==window;
function matches(rule){var not=rule[0]==='!';if(not)rule=rule.slice(1);var i=rule.indexOf('='),v=i<0?state[rule]:state[rule.slice(0,i)],ok=i<0?(v!==undefined&&v!==''&&v!==false):String(v)===rule.slice(i+1);return not?!ok:ok}
function render(){if(!document.body)return;
document.querySelectorAll('[data-bind]').forEach(function(e){var v=state[e.getAttribute('data-bind')];if(v===undefined)v='';if(e.type==='checkbox'||e.type==='radio'){if(e.type==='radio')e.checked=String(v)===e.value;else e.checked=!!v}else if('value' in e&&e.tagName!=='BUTTON'&&e.tagName!=='LI'){if(e.value!==String(v))e.value=v}else e.textContent=v});
document.querySelectorAll('[data-show-if]').forEach(function(e){e.hidden=!matches(e.getAttribute('data-show-if'))});
document.dispatchEvent(new CustomEvent('enowx:state',{detail:state}))}
function changed(){window.__enowxState=JSON.stringify(state);if(framed)parent.postMessage({enowx:'state',state:state},'*');render()}
window.enowx={state:{get:function(k){return state[k]},set:function(k,v){state[k]=v;changed()},all:function(){return JSON.parse(JSON.stringify(state))},reset:function(){state={};changed()}}};
window.__enowxState=JSON.stringify(state);
window.addEventListener('message',function(e){var d=e.data;if(d&&d.enowx==='state'&&e.source===parent){state=d.state||{};window.__enowxState=JSON.stringify(state);render()}});
document.addEventListener('click',function(e){var t=e.target.closest&&e.target.closest('[data-set],[data-reset]');if(!t)return;if(t.hasAttribute('data-reset')){state={}}else{t.getAttribute('data-set').split(';').forEach(function(p){var i=p.indexOf('=');if(i>0)state[p.slice(0,i).trim()]=p.slice(i+1).trim()})}changed()},true);
function bound(e){var t=e.target;if(!t.getAttribute||!t.hasAttribute('data-bind'))return;state[t.getAttribute('data-bind')]=t.type==='checkbox'?t.checked:t.value;window.__enowxState=JSON.stringify(state);if(framed)parent.postMessage({enowx:'state',state:state},'*');render()}
document.addEventListener('input',bound);document.addEventListener('change',bound);
document.addEventListener('DOMContentLoaded',render)})()</script>`

// usesPrototype says whether a design's HTML asks for the runtime, so that a
// plain design's document stays as it was written.
func usesPrototype(html string) bool {
	for _, mark := range []string{"data-set", "data-reset", "data-bind", "data-show-if", "enowx.state", "enowx:state"} {
		if strings.Contains(html, mark) {
			return true
		}
	}
	return false
}

// PrototypeSeed is the script giving a page the state earlier pages left, JSON
// ("" for none), for the head of its document, before the runtime.
func PrototypeSeed(state string) string {
	if strings.TrimSpace(state) == "" {
		return ""
	}
	state = strings.NewReplacer("<", `\u003c`, "\u2028", `\u2028`, "\u2029", `\u2029`).Replace(state)
	return "<script>window.__enowxSeed=" + state + "</script>"
}

// WithHead puts add at the start of doc's head, before any script of its own.
func WithHead(doc, add string) string {
	low := strings.ToLower(doc)
	if i := strings.Index(low, "<head>"); i >= 0 {
		return doc[:i+6] + add + doc[i+6:]
	}
	if i := strings.Index(low, "<html"); i >= 0 {
		if j := strings.IndexByte(doc[i:], '>'); j >= 0 {
			return doc[:i+j+1] + add + doc[i+j+1:]
		}
	}
	return add + doc
}

// shareFrameSandbox is what a design may do on the shared page. Without
// allow-same-origin its scripts have an origin of their own: no reaching the
// page around them, its link, or what else that server serves.
const shareFrameSandbox = "allow-scripts allow-forms allow-popups allow-modals"

// shareGoto has a design's links between designs tell the shared page.
const shareGoto = `<script>document.addEventListener('click',function(e){var a=e.target.closest&&e.target.closest('[data-goto],a[href^="#"]');if(!a)return;var t=a.getAttribute('data-goto')||a.getAttribute('href').slice(1);if(!t)return;e.preventDefault();parent.postMessage({enowx:'goto',target:t},'*')},true)</script>`

// shareHost runs a shared page's prototype: a link shows the design it names
// alone, as Interact mode does, and state goes between the designs. Only its
// own frames are listened to.
const shareHost = `<script>(function(){
var frames=[].slice.call(document.querySelectorAll('section iframe')),state={};
function own(w){return frames.some(function(f){return f.contentWindow===w})}
function send(from){frames.forEach(function(f){if(f.contentWindow&&f.contentWindow!==from)f.contentWindow.postMessage({enowx:'state',state:state},'*')})}
function show(s){document.body.classList.add('one');document.querySelectorAll('section').forEach(function(x){x.classList.toggle('on',x===s)});window.scrollTo(0,0)}
window.addEventListener('message',function(e){var d=e.data;if(!d||!own(e.source))return;
if(d.enowx==='state'){state=d.state||{};send(e.source)}
if(d.enowx==='goto'){var t=String(d.target),s=document.getElementById(t),all=document.querySelectorAll('section');if(!s)all.forEach(function(x){if(x.getAttribute('data-name')===t)s=x});if(s)show(s)}});
frames.forEach(function(f){f.addEventListener('load',function(){f.contentWindow.postMessage({enowx:'state',state:state},'*')})});
var reset=document.getElementById('enowx-reset'),all=document.getElementById('enowx-all');
if(reset)reset.addEventListener('click',function(){state={};send(null);var first=document.querySelector('section');if(document.body.classList.contains('one')&&first)show(first)});
if(all)all.addEventListener('click',function(){document.body.classList.remove('one')})})()</script>`

// shareControls are the shared page's way back to the start.
const shareControls = `<nav><button id="enowx-reset" type="button">Reset the prototype</button> <button id="enowx-all" type="button">Show all designs</button></nav>`

// shareStyle shows one design at a time once a link was followed.
const shareStyle = `<style>nav{margin:10px 0 0}nav button{font:inherit;font-size:12px;padding:4px 10px;border:1px solid #cbd5e1;border-radius:6px;background:#fff;color:#0f172a;cursor:pointer}body.one section{display:none}body.one section.on{display:block}</style>`
