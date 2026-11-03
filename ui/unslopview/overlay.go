package unslopview

import (
	"encoding/json"
	"time"

	"github.com/enowx/enowx/proto"
)

// A redesign's reads, shown in the page as they come (a canvas's design,
// the Browser's page): a line sweeps down, and each element a finding is
// about lights up as it passes, in its severity's color, with what is
// off. The next read draws them again, so what a pass fixed is no longer
// marked. Regions an agent each redesigns are outlined in their colors,
// and an element an agent changes is tagged with its pass.

type scanItem struct {
	Sel   string `json:"sel"`
	Sev   string `json:"sev"`
	Label string `json:"label"`
}

// enxStyle are the marks' animations, in CSS and the Web Animations API,
// so they run without requestAnimationFrame, which a page out of view
// doesn't call.
const enxStyle = `if(!document.getElementById('__enx_style')){var st=document.createElement('style');st.id='__enx_style';
st.textContent='@keyframes enxSweep{from{transform:translateY(-80px)}to{transform:translateY(var(--enx-h))}}'+
'@keyframes enxFade{to{opacity:0}}@keyframes enxShow{from{opacity:0}to{opacity:1}}'+
'@keyframes enxIn{0%{opacity:0;transform:scale(1.06)}60%{opacity:1;transform:scale(.99)}100%{opacity:1;transform:none}}'+
'@keyframes enxType{from{max-width:0}to{max-width:var(--enx-w)}}'+
'@keyframes enxTag{0%{opacity:0;transform:translateY(4px)}12%{opacity:1;transform:none}85%{opacity:1}100%{opacity:0}}'+
'@keyframes enxPulse{0%,100%{box-shadow:0 0 0 0 rgba(129,140,248,.6)}50%{box-shadow:0 0 0 6px rgba(129,140,248,0)}}';
document.head.appendChild(st)}`

// ScanScript plays a read over the page, as a scan: the page dims, a line
// sweeps down it, then a reticle goes from one finding to the next (top
// to bottom, ten at most), each box landing as it arrives with its label
// typed out, a count of what it found below; then the page brightens and
// the boxes stay. Items are ScanItems.
const ScanScript = `(function(items){` + enxStyle + `
['__enx_scan','__enx_count'].forEach(function(i){var o=document.getElementById(i);if(o)o.remove()});
var de=document.documentElement,H=Math.max(de.scrollHeight,innerHeight),W=de.scrollWidth,SW=1.4,HOP=.42;
var root=document.createElement('div');root.id='__enx_scan';
root.style.cssText='position:absolute;left:0;top:0;width:'+W+'px;height:'+H+'px;pointer-events:none;z-index:2147483646;overflow:hidden';
root.style.setProperty('--enx-h',H+'px');document.body.appendChild(root);
var colors={high:'#ef4444',medium:'#f59e0b',low:'#60a5fa'};
var marks=[];items.forEach(function(it){var els=[];try{els=document.querySelectorAll(it.sel)}catch(e){}
for(var i=0,n=0;i<els.length&&n<6;i++){var r=els[i].getBoundingClientRect();if(r.width<2||r.height<2)continue;n++;
marks.push({x:r.left+scrollX,y:r.top+scrollY,w:r.width,h:r.height,c:colors[it.sev]||'#818cf8',label:it.label})}});
marks.sort(function(a,b){return a.y-b.y||a.x-b.x});
var hops=Math.min(marks.length,10),end=SW+hops*HOP+.6;
var dim=document.createElement('div');
dim.style.cssText='position:absolute;inset:0;background:rgba(8,8,20,.32);animation:enxShow .3s both,enxFade .5s '+end.toFixed(2)+'s both';
root.appendChild(dim);
var line=document.createElement('div');
line.style.cssText='position:absolute;left:0;right:0;top:0;height:80px;background:linear-gradient(180deg,rgba(129,140,248,0),rgba(129,140,248,.22));border-bottom:2px solid #a5b4fc;box-shadow:0 8px 24px rgba(129,140,248,.55);animation:enxSweep '+SW+'s cubic-bezier(.45,0,.2,1) both,enxFade .25s '+SW+'s both';
root.appendChild(line);
var count=document.createElement('div');count.id='__enx_count';count.textContent='Scanning\u2026';if(!marks.length)count.style.display='none';
count.style.cssText='position:fixed;left:16px;bottom:16px;z-index:2147483647;pointer-events:none;font:500 12px/28px system-ui,sans-serif;padding:0 12px;border-radius:14px;color:#e0e7ff;background:rgba(30,27,75,.92);border:1px solid rgba(165,180,252,.5);box-shadow:0 6px 20px rgba(0,0,0,.35);animation:enxShow .3s both,enxFade .5s '+(end+1.4).toFixed(2)+'s both';
document.body.appendChild(count);
setTimeout(function(){count.remove()},(end+2)*1000);
if(hops&&root.animate){var ret=document.createElement('div');
ret.style.cssText='position:absolute;border:2px solid #c7d2fe;border-radius:6px;animation:enxPulse .9s infinite;opacity:0';root.appendChild(ret);
var frames=[],o=marks[0];frames.push({left:o.x-40+'px',top:o.y-40+'px',width:o.w+80+'px',height:o.h+80+'px',opacity:0,offset:0});
for(var k=0;k<hops;k++){var m=marks[k];frames.push({left:m.x-4+'px',top:m.y-4+'px',width:m.w+8+'px',height:m.h+8+'px',opacity:1,offset:(k+1)/(hops+1)})}
var lm=marks[hops-1];frames.push({left:lm.x-4+'px',top:lm.y-4+'px',width:lm.w+8+'px',height:lm.h+8+'px',opacity:0,offset:1});
ret.animate(frames,{duration:(hops+1)*HOP*1000,delay:SW*1000,fill:'both',easing:'cubic-bezier(.6,0,.2,1)'})}
marks.forEach(function(m,k){var at=k<hops?SW+(k+1)*HOP:SW+hops*HOP+.2;
var d=document.createElement('div');
d.style.cssText='position:absolute;left:'+m.x+'px;top:'+m.y+'px;width:'+m.w+'px;height:'+m.h+'px;border:1.5px solid '+m.c+';background:'+m.c+'1f;border-radius:4px;animation:enxIn .35s '+at.toFixed(2)+'s both';
var t=document.createElement('span');t.textContent=m.label;t.style.setProperty('--enx-w',(m.label.length*7+16)+'px');
t.style.cssText+='position:absolute;left:-1.5px;bottom:100%;margin-bottom:2px;font:500 12px/16px system-ui,sans-serif;padding:0 6px;border-radius:4px;color:#fff;white-space:nowrap;overflow:hidden;background:'+m.c+';animation:enxType .45s steps('+Math.min(m.label.length,24)+') '+(at+.15).toFixed(2)+'s both';
d.appendChild(t);root.appendChild(d);
setTimeout(function(){count.textContent='Scanning\u2026 '+(k+1)+(k?' issues':' issue')},at*1000)});
setTimeout(function(){count.textContent=marks.length?marks.length+(marks.length>1?' issues marked':' issue marked'):'Nothing to mark on this view'},end*1000);
return marks.length})`

// RegionColors tell a fanned-out unslop's regions apart, by order.
var RegionColors = []string{"#f472b6", "#22d3ee", "#a3e635", "#fb923c", "#c084fc", "#facc15"}

// RegionsScript outlines each region, dashed in its color, with its
// agent and pass on a tag; a region done goes solid, then fades.
const RegionsScript = `(function(rs){
var old=document.getElementById('__enx_regions');if(old)old.remove();if(!rs.length)return;
var root=document.createElement('div');root.id='__enx_regions';
root.style.cssText='position:absolute;left:0;top:0;width:0;height:0;pointer-events:none;z-index:2147483645';
document.body.appendChild(root);
rs.forEach(function(r){var e=document.getElementById(r.id);if(!e)return;var b=e.getBoundingClientRect();
var d=document.createElement('div');
d.style.cssText='position:absolute;left:'+(b.left+scrollX-3)+'px;top:'+(b.top+scrollY-3)+'px;width:'+(b.width+6)+'px;height:'+(b.height+6)+'px;border:2px '+(r.done?'solid':'dashed')+' '+r.color+';border-radius:6px;opacity:'+(r.done?.45:1)+';transition:opacity .4s';
var t=document.createElement('span');t.textContent=r.label;
t.style.cssText='position:absolute;right:-2px;top:-2px;transform:translateY(-100%);font:500 12px/18px system-ui,sans-serif;padding:0 7px;border-radius:9px 9px 0 9px;color:#111;white-space:nowrap;background:'+r.color;
d.appendChild(t);root.appendChild(d)})})`

// PassTagScript tags element id, just changed, with label in color.
const PassTagScript = `(function(id,label,color){` + enxStyle + `var e=document.getElementById(id);if(!e||!label)return;
var r=e.getBoundingClientRect(),t=document.createElement('div');t.textContent=label;
t.style.cssText='position:absolute;left:'+(r.left+scrollX)+'px;top:'+Math.max(0,r.top+scrollY-20)+'px;font:500 12px/18px system-ui,sans-serif;padding:0 7px;border-radius:9px;color:'+(color?'#111':'#fff')+';background:'+(color||'#6366f1')+';box-shadow:0 2px 8px rgba(0,0,0,.25);pointer-events:none;z-index:2147483647;animation:enxTag 2.1s both';
document.body.appendChild(t);setTimeout(function(){t.remove()},2200)})`

// ScanItems are run's findings' elements, as ScanScript takes them.
func ScanItems(run *proto.CanvasUnslopRun) string {
	var items []scanItem
	for _, f := range run.Findings {
		for _, sel := range f.Targets {
			items = append(items, scanItem{Sel: sel, Sev: f.Severity, Label: f.Title})
		}
	}
	data, _ := json.Marshal(items)
	return string(data)
}

// Tag is how an agent's changes are tagged: its label and color ("" the
// accent's).
type Tag struct{ Label, Color string }

type regionMark struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Color string `json:"color"`
	Done  bool   `json:"done"`
}

// RegionMarks are run's regions as RegionsScript takes them, and the tag
// of each region agent's session.
func RegionMarks(run *proto.CanvasUnslopRun, agentName func(string) string) (string, map[string]Tag) {
	tags := map[string]Tag{}
	var marks []regionMark
	if run != nil && !run.Done {
		for i, r := range run.Regions {
			color := RegionColors[i%len(RegionColors)]
			label := "#" + r.ID
			if agentName != nil {
				if name := agentName(r.Session); name != "" {
					label = name + " · " + label
				}
			}
			switch {
			case r.Done:
				label += " · done"
			case r.Pass != "":
				label += " · " + r.Pass
			}
			marks = append(marks, regionMark{ID: r.ID, Label: label, Color: color, Done: r.Done})
			tags[r.Session] = Tag{Label: label, Color: color}
		}
	}
	data, _ := json.Marshal(marks)
	return string(data), tags
}

// JS quotes s as a JavaScript string.
func JS(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// scanReady wraps a page script to run once the page is loaded: a page
// still loading (the design just shown, the dev server reloading it)
// would drop what it draws.
func scanReady(code string) string {
	return `(function(){var go=function(){` + code + `};if(document.readyState==='complete'&&document.body){go()}else{addEventListener('load',go,{once:true})}return true})()`
}

// scanStill says the page still holds a read's marks.
const scanStill = `!!document.getElementById('__enx_scan')`

// DrawScan draws a read's marks (ScanItems) in a page through eval,
// once it is loaded, and again when a reload took them, a few times at
// most. It waits, so it runs off the main thread.
func DrawScan(eval func(code string) (any, error), items string) {
	code := scanReady(ScanScript + "(" + items + ")")
	for try := 0; try < 4; try++ {
		time.Sleep(300 * time.Millisecond)
		if _, err := eval(code); err != nil {
			continue
		}
		// The sweep takes 1.6 s; a page reloaded meanwhile lost it.
		time.Sleep(1200 * time.Millisecond)
		if still, err := eval(scanStill); err == nil && still == true {
			return
		}
	}
}
