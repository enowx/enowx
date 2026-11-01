package canvas

// shareMotionBar is a moving design's controls on the shared page, hidden until
// the design says it has motion.
const shareMotionBar = `<div class="enowx-motion" hidden><button type="button" data-act="toggle">Pause</button> <button type="button" data-act="restart">Restart</button> <input type="range" min="0" max="1000" value="0" aria-label="Move through the animation"> <output></output></div>`

// shareMotion runs those controls: a design's frame says where its motion is and
// takes orders by postMessage, as its sandbox leaves nothing else. Only its own
// frames are listened to, and all they can change is their own bar's numbers.
const shareMotion = `<script>(function(){
function secs(ms){return (ms/1000).toFixed(1)+' s'}
document.querySelectorAll('section iframe').forEach(function(f){
var bar=f.parentNode.querySelector('.enowx-motion');if(!bar)return;
var toggle=bar.querySelector('[data-act=toggle]'),range=bar.querySelector('input'),out=bar.querySelector('output'),d=0,playing=false,held=false;
function tell(act,t){if(f.contentWindow)f.contentWindow.postMessage({enowx:'motion',act:act,t:t},'*')}
window.addEventListener('message',function(e){var m=e.data;if(!m||m.enowx!=='motion'||e.source!==f.contentWindow)return;
d=+m.d||0;var t=+m.t||0;playing=!!m.p;bar.hidden=!(d>0);toggle.textContent=playing?'Pause':'Play';if(!held)range.value=d?Math.round(t/d*1000):0;out.textContent=secs(t)+' / '+secs(d)});
f.addEventListener('load',function(){tell('state')});tell('state');
toggle.addEventListener('click',function(){tell(playing?'pause':'play')});
bar.querySelector('[data-act=restart]').addEventListener('click',function(){tell('restart')});
range.addEventListener('input',function(){held=true;tell('seek',range.value/1000*d)});
range.addEventListener('change',function(){held=false})})})()</script><style>.enowx-motion{display:flex;gap:6px;align-items:center;margin-top:8px;max-width:640px;font-size:12px;color:#475569}.enowx-motion[hidden]{display:none}.enowx-motion button{font:inherit;padding:4px 10px;border:1px solid #cbd5e1;border-radius:6px;background:#fff;color:#0f172a;cursor:pointer}.enowx-motion input{flex:1;min-width:80px}.enowx-motion output{font-variant-numeric:tabular-nums;min-width:84px;text-align:right}</style>`
