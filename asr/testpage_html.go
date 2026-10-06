package asr

import "net/http"

// testPageHTML is dependency   (no  /CDN/npm).  in usetgtapprove <textarea>, 
// **    block** ⇒ macOS  write(linkby under Fn /     )  connectuse. 
const testPageHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>VoxSign local test page (record -> correct -> ledger)</title>
<style>
 body{font-family:-apple-system,sans-serif;max-width:820px;margin:24px auto;padding:0 16px}
 textarea{width:100%;height:90px;font-size:18px;padding:10px}
 button{font-size:16px;padding:8px 14px;margin:6px 6px 0 0}
 .row{margin:10px 0;font-size:17px}
 .k{color:#666;display:inline-block;min-width:66px}
 code{background:#f4f4f4;padding:2px 6px;border-radius:4px}
 table{border-collapse:collapse;width:100%;font-size:14px}td,th{border:1px solid #ddd;padding:4px 6px}
</style></head><body>
<h2>VoxSign local test page</h2>
<div id="banner"></div>
<p style="color:#666">Click <b>[🎤 Speak]</b> to record (browser built-in recognition, zero backend); or paste text / use system dictation (Fn). Click "Process" after speaking; every run is logged to the real ledger.</p>
<textarea id="t" placeholder="Click here, use [🎤 Speak] to record, or paste / system dictation..."></textarea>
<div>
 <button id="mic" type="button">🎤 Speak</button>
 <span id="michint" style="color:#666;font-size:14px"></span>
</div>
<div>
 <button onclick="run()">Process</button>
 <button onclick="teach()">Teach a word</button>
 <button onclick="clearTaught()">Clear taught words</button>
 <button onclick="log()">View ledger</button>
</div>
<div id="out"></div>
<div id="fbrow" style="margin-top:6px">
 <button id="fbok" type="button" onclick="feedbackOk()">✔ OK</button>
 <button id="fbno" type="button" onclick="showNeg()">✘ Wrong</button>
 <button id="fbwrong" type="button" onclick="markWrong()">This correction is wrong</button>
 <span id="fbstatus" style="color:#666;font-size:14px"></span>
</div>
<div id="fbneg" hidden>
 <textarea id="fbneg_reason" placeholder="What is wrong? (optional; blank records user_marked_wrong)" style="height:40px"></textarea>
 <button type="button" onclick="submitNeg()">Submit ✘ reason</button>
</div>
<div id="metrics"></div>
<h3>Drop a document + a hard task and see how it plans</h3>
<p style="color:#666"><b>This round plans only, no execution</b> (it will not modify your files). The document is plan input only, not written to disk.</p>
<textarea id="doc" placeholder="1) Paste document content, or choose a file (read locally, not uploaded)..." style="height:120px"></textarea>
<div><input type="file" id="file" onchange="loadFile()"> <span style="color:#666;font-size:14px">(file is read into the text box locally in the browser)</span></div>
<textarea id="task" placeholder="2) Write a task, can be complex: e.g. \"turn the TODOs in this doc into a plan\"..." style="height:60px"></textarea>
<div><button onclick="planTask()">Plan (plan only, no execution)</button></div>
<div id="planout"></div>
<h3>Recent ledger</h3><div id="recent"></div>
<script>
// ---- browser native speech recognition (zero backend, zero key, zero deps) ----
// Warning: audio goes through Apple/Google recognition, not on-device (see docs).
var rec = null;
function recSupported(){ return typeof window !== 'undefined' && ('webkitSpeechRecognition' in window); }
function initRec(){
  var btn = document.getElementById('mic');
  var hint = document.getElementById('michint');
  if(!recSupported()){
    // graceful fallback: gray out + clear hint; never blank screen or silent failure
    btn.disabled = true;
    hint.textContent = 'Speech recognition not supported in this browser; please paste manually or use system dictation (Fn)';
    return;
  }
  var r = new webkitSpeechRecognition();
  r.lang = 'zh-CN';
  r.continuous = false;
  r.interimResults = true;   // show words while speaking
  r.onresult = function(e){
    var ta = document.getElementById('t');
    var text = '';
    for(var i = e.resultIndex; i < e.results.length; i++){ text += e.results[i][0].transcript; }
    ta.value = (ta.dataset.recBase || '') + text;   // fill text only, no auto-submit
  };
  r.onend = function(){ document.getElementById('mic').textContent = '🎤 Speak'; };
  rec = r;
  btn.onclick = function(){
    var ta = document.getElementById('t');
    ta.dataset.recBase = ta.value;        // base = text at click time (preserve manual edits)
    btn.textContent = '⏺ Listening...';
    r.start();                             // only action: start recognition (no requests)
  };
}
window.addEventListener('load', initRec);

async function j(url, body){
  const r = await fetch(url, body?{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)}:{});
  if(!r.ok){ throw new Error('HTTP '+r.status+' '+url); }
  return r.json();
}
// 3) health self-check: unavailable -> red banner with actionable hint
async function probe(){
  var b = document.getElementById('banner');
  try{
    var r = await fetch('/v1/health');
    if(!r.ok){ throw new Error('HTTP '+r.status); }
    b.innerHTML = '';
  }catch(err){
    b.innerHTML = '<div style="background:#c00;color:#fff;padding:8px;border-radius:4px">'+
      '<b>Service not running</b>: please run <code>sh scripts/dev.sh</code> ('+esc(String(err))+')</div>';
  }
}
function esc(s){return (s||'').replace(/[<>&]/g,c=>({'<':'&lt;','>':'&gt;','&':'&amp;'}[c]))}
async function run(){
  const out = document.getElementById('out');
  // 2) immediately show "processing" so users can tell "running" from "dead"
  out.innerHTML = '<div class="row">Processing...</div>';
  const text = document.getElementById('t').value;
  try{
  const d = await j('/v1/testpage', {text});
  lastResult = d;   // lets the ✔/✘/correction-wrong buttons target this result
  let h = '<div class="row"><span class="k">Raw: </span><code>'+esc(d.raw)+'</code></div>'+
          '<div class="row"><span class="k">Corrected: </span><code>'+esc(d.corrected)+'</code></div>'+
          '<div class="row"><span class="k">Punctuated: </span><code>'+esc(d.punctuated)+'</code></div>'+
          '<div class="row"><span class="k">Intent: </span>'+esc(d.intent)+
          ' | Ask-back: '+(d.ask_back?'<b style="color:#c60">yes</b>':'no')+
          ' | Degraded: '+(d.degraded?'<b style="color:#c60">yes</b>':'no')+'</div>'+
          '<div class="row"><span class="k">Level: </span>'+esc(d.level)+' | took '+d.ms+' ms | taught-word hit: '+(d.taught_hit?'yes':'no')+'</div>';
  if(d.degraded && d.degraded_reason){h += '<div class="row">Degraded reason: '+esc(d.degraded_reason)+'</div>';}
  if(d.ask_back){h += '<div class="row">Candidates (ask-back must offer candidates): '+JSON.stringify(d.candidates||[])+'</div>';}
  if(d.log_error){h += '<div class="row" style="color:#c00">Ledger write failed: '+esc(d.log_error)+'</div>';}
  out.innerHTML = h;
  log();
  }catch(err){
    // 1) failures must be visible (silent failure is what we are pinning down)
    out.innerHTML = '<div class="row" style="color:#c00"><b>Processing failed</b>: '+esc(String(err))+
      '<br>Request: <code>POST /v1/testpage</code>'+
      '<br>The service may have stopped; please rerun <code>sh scripts/dev.sh</code></div>';
  }
}
function loadFile(){
  const f = document.getElementById('file').files[0]; if(!f) return;
  const rd = new FileReader();
  rd.onload = ()=>{ document.getElementById('doc').value = rd.result; };
  rd.readAsText(f);
}
async function planTask(){
  const out = document.getElementById('planout');
  out.innerHTML = '<div class="row">Planning...</div>';
  try{
    const r = await fetch('/v1/task', {method:'POST', headers:{'Content-Type':'application/json'},
      body: JSON.stringify({task: document.getElementById('task').value, document: document.getElementById('doc').value})});
    if(!r.ok){ throw new Error('HTTP '+r.status+' /v1/task'); }
    const d = await r.json();
    const p = d.plan;
    let h = '<div class="row"><b>'+(d.execute?'':'[plan only, no execution]')+'</b> Goal: <code>'+esc(d.goal)+'</code></div>';
    h += '<div class="row"><span class="k">Source: </span>'+esc(p.source||'-')+
         ' | Degraded: '+(p.degraded?('yes ('+esc(p.degraded_reason||'')+')'):'no')+
         ' | Refused: '+(p.refused?'<b style="color:#c60">yes</b>':'no')+'</div>';
    if(p.reason){ h += '<div class="row">Reason: '+esc(p.reason)+'</div>'; }
    if(p.missing && p.missing.length){ h += '<div class="row">Cannot do / who to ask: <ul>'+p.missing.map(x=>'<li>'+esc(x)+'</li>').join('')+'</ul></div>'; }
    if(p.steps && p.steps.length){
      h += '<table><tr><th>#</th><th>Tool</th><th>cap</th><th>Action</th><th>Output</th><th>Domain</th></tr>';
      p.steps.forEach(s=>{ h += '<tr><td>'+s.index+'</td><td>'+esc(s.tool)+'</td><td>'+esc((s.caps||[]).join(','))+'</td><td>'+esc(s.action)+'</td><td>'+esc(s.output)+'</td><td>'+esc(s.domain||'')+'</td></tr>'; });
      h += '</table>';
    } else { h += '<div class="row" style="color:#666">No executable steps (see refusal reason / who to ask above).</div>'; }
    h += '<details><summary>Trace (factors considered / working memory)</summary><pre style="white-space:pre-wrap">'+
         esc(JSON.stringify({considered:p.considered, wm:p.wm}, null, 1))+'</pre></details>';
    out.innerHTML = h;
  }catch(err){
    out.innerHTML = '<div class="row" style="color:#c00"><b>Planning failed</b>: '+esc(String(err))+
      '<br>Request: <code>POST /v1/task</code><br>The service may have stopped; please rerun <code>sh scripts/dev.sh</code></div>';
  }
}
async function teach(){
  const term = prompt('Which word to teach (e.g. aiopisi)?'); if(!term) return;
  const canonical = prompt('Normalize to what (e.g. aiops)?'); if(!canonical) return;
  const r = await j('/v1/observe', {term, canonical});
  alert(JSON.stringify(r));
}
// ---- feedback (OK/wrong) and correction-wrong (A8/A9/A10) ----
var lastResult = null;
function fbstatus(msg, isErr){
  const el = document.getElementById('fbstatus');
  el.style.color = isErr ? '#c00' : '#666';
  el.textContent = msg;
}
async function feedbackOk(){
  if(!lastResult){ fbstatus('Click Process first, then give feedback', true); return; }
  try{
    const d = await j('/v1/feedback', {text_raw: lastResult.raw, text_final: lastResult.corrected, accepted: true, reason: '', source: 'testpage'});
    fbstatus('✔ recorded (feedback.jsonl line ' + d.feedback.lines + ')' + (d.feedback.log_error?(', write failed: '+d.feedback.log_error):''));
  }catch(err){ fbstatus('✔ record failed: '+String(err), true); }
}
function showNeg(){
  document.getElementById('fbneg').hidden = false;
  document.getElementById('fbneg_reason').focus();
}
async function submitNeg(){
  const reason = document.getElementById('fbneg_reason').value.trim();
  if(!lastResult){ fbstatus('Click Process first, then give feedback', true); return; }
  try{
    const d = await j('/v1/feedback', {text_raw: lastResult.raw, text_final: lastResult.corrected, accepted: false, reason: reason, source: 'testpage'});
    document.getElementById('fbneg').hidden = true;
    fbstatus('✘ recorded (feedback.jsonl line ' + d.feedback.lines + ', reason: ' + d.feedback.reason + ')');
  }catch(err){ fbstatus('✘ record failed: '+String(err), true); }
}
async function markWrong(){
  if(!lastResult){ fbstatus('Click Process first, then mark', true); return; }
  const corrs = (lastResult.corrections||[]).filter(c=>c.From && c.To);
  if(corrs.length === 0){
    fbstatus('No correction to mark this run (no From->To rewrite happened)', true);
    return;
  }
  const term = corrs[0].From;
  try{
    const d = await j('/v1/blacklist', {op:'add', term: term, note: lastResult.raw+' → '+lastResult.corrected});
    fbstatus('Added "'+d.term+'" to the correction blacklist and persisted it; re-run Process to see the effect');
    run();  // immediately re-run so the effect is visible
  }catch(err){ fbstatus('Blacklist record failed: '+String(err), true); }
}
async function clearTaught(){ alert(JSON.stringify(await j('/v1/lexicon', {op:'clear_taught'}))); }
async function log(){
  const d = await j('/v1/testlog?n=20');
  document.getElementById('metrics').innerHTML = '<div class="row"><span class="k">Real samples: </span>'+d.total+
    ' | L0 share '+d.metrics.l0_share.toFixed(2)+' | ask-back rate '+d.metrics.ask_back_rate.toFixed(2)+
    ' | degraded rate '+d.metrics.degraded_rate.toFixed(2)+'</div><div style="color:#666;font-size:14px">'+esc(d.metrics.note)+'<br>Ledger: '+esc(d.path)+'</div>';
  let h = '<table><tr><th>Time</th><th>Raw</th><th>Corrected</th><th>Intent</th><th>Ask-back</th><th>Level</th><th>ms</th></tr>';
  (d.recent||[]).slice().reverse().forEach(r=>{h+='<tr><td>'+esc((r.at||'').slice(11,19))+'</td><td>'+esc(r.raw)+'</td><td>'+esc(r.corrected)+'</td><td>'+esc(r.intent)+'</td><td>'+(r.ask_back?'yes':'no')+'</td><td>'+esc(r.level)+'</td><td>'+r.ms+'</td></tr>';});
  document.getElementById('recent').innerHTML = h+'</table>';
}
log();
probe();  // after load, self-check whether the service is alive
</script></body></html>`

func (s *Server) handleTestPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(testPageHTML))
}
