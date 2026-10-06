package asr

import "net/http"

// testPageHTML is dependency   (no  /CDN/npm).  in usetgtapprove <textarea>, 
// **    block** ⇒ macOS  write(linkby under Fn /     )  connectuse. 
const testPageHTML = `<!doctype html>
<html lang="zh"><head><meta charset="utf-8"><title>VoxSign 本地测试页（录音 → 纠错 → 台账）</title>
<style>
 body{font-family:-apple-system,sans-serif;max-width:820px;margin:24px auto;padding:0 16px}
 textarea{width:100%;height:90px;font-size:18px;padding:10px}
 button{font-size:16px;padding:8px 14px;margin:6px 6px 0 0}
 .row{margin:10px 0;font-size:17px}
 .k{color:#666;display:inline-block;min-width:66px}
 code{background:#f4f4f4;padding:2px 6px;border-radius:4px}
 table{border-collapse:collapse;width:100%;font-size:14px}td,th{border:1px solid #ddd;padding:4px 6px}
</style></head><body>
<h2>VoxSign 本地测试页</h2>
<div id="banner"></div>
<p style="color:#666">点 <b>〔🎤 说话〕</b> 录音（浏览器内置识别，零后端）；也可粘贴文本或用系统听写（Fn）。说完点「处理」，每次都会记进真实台账。</p>
<textarea id="t" placeholder="点这里，用〔🎤 说话〕录音，或手动粘贴/系统听写……"></textarea>
<div>
 <button id="mic" type="button">🎤 说话</button>
 <span id="michint" style="color:#666;font-size:14px"></span>
</div>
<div>
 <button onclick="run()">处理</button>
 <button onclick="teach()">教一个词</button>
 <button onclick="clearTaught()">清空教的词</button>
 <button onclick="log()">看台账</button>
</div>
<div id="out"></div>
<div id="fbrow" style="margin-top:6px">
 <button id="fbok" type="button" onclick="feedbackOk()">✔ 对</button>
 <button id="fbno" type="button" onclick="showNeg()">✘ 不对</button>
 <button id="fbwrong" type="button" onclick="markWrong()">这个改错了</button>
 <span id="fbstatus" style="color:#666;font-size:14px"></span>
</div>
<div id="fbneg" hidden>
 <textarea id="fbneg_reason" placeholder="哪里不对？（可留空；留空记 user_marked_wrong）" style="height:40px"></textarea>
 <button type="button" onclick="submitNeg()">提交 ✘ 原因</button>
</div>
<div id="metrics"></div>
<h3>丢一个文档 + 一个难任务，看它打算怎么干</h3>
<p style="color:#666"><b>本轮只规划，不执行</b>（不会真的改你的文件）。document 仅作规划输入、不落盘。</p>
<textarea id="doc" placeholder="① 粘贴文档内容，或选择文件（本地读取，不上传服务器）……" style="height:120px"></textarea>
<div><input type="file" id="file" onchange="loadFile()"> <span style="color:#666;font-size:14px">（文件只在浏览器本地读入文本框）</span></div>
<textarea id="task" placeholder="② 写任务，可以很复杂：例如“把这个文档里的 TODO 整理成一份计划”……" style="height:60px"></textarea>
<div><button onclick="planTask()">规划（只规划，不执行）</button></div>
<div id="planout"></div>
<h3>最近台账</h3><div id="recent"></div>
<script>
// ---- 浏览器原生语音识别（零后端、零 key、零依赖）----
// ⚠️ 语音经 Apple/Google 的识别服务，不是本地识别（见文档说明）。
var rec = null;
function recSupported(){ return typeof window !== 'undefined' && ('webkitSpeechRecognition' in window); }
function initRec(){
  var btn = document.getElementById('mic');
  var hint = document.getElementById('michint');
  if(!recSupported()){
    // 优雅降级：置灰 + 明确提示；**不白屏、不静默失败**
    btn.disabled = true;
    hint.textContent = '此浏览器不支持语音识别，请手动粘贴或使用系统听写（Fn）';
    return;
  }
  var r = new webkitSpeechRecognition();
  r.lang = 'zh-CN';
  r.continuous = false;
  r.interimResults = true;   // 边说边出字
  r.onresult = function(e){
    var ta = document.getElementById('t');
    var text = '';
    for(var i = e.resultIndex; i < e.results.length; i++){ text += e.results[i][0].transcript; }
    ta.value = (ta.dataset.recBase || '') + text;   // **只填字，不自动提交**
  };
  r.onend = function(){ document.getElementById('mic').textContent = '🎤 说话'; };
  rec = r;
  btn.onclick = function(){
    var ta = document.getElementById('t');
    ta.dataset.recBase = ta.value;        // 以点击时的文本为基底（保留手动编辑）
    btn.textContent = '⏺ 识别中…';
    r.start();                             // 唯一动作：开始识别（不发任何请求）
  };
}
window.addEventListener('load', initRec);

async function j(url, body){
  const r = await fetch(url, body?{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)}:{});
  if(!r.ok){ throw new Error('HTTP '+r.status+' '+url); }
  return r.json();
}
// ③ 服务可用性自检：不可用 ⇒ 顶部红色横幅（可操作提示）
async function probe(){
  var b = document.getElementById('banner');
  try{
    var r = await fetch('/v1/health');
    if(!r.ok){ throw new Error('HTTP '+r.status); }
    b.innerHTML = '';
  }catch(err){
    b.innerHTML = '<div style="background:#c00;color:#fff;padding:8px;border-radius:4px">'+
      '<b>服务未运行</b>：请执行 <code>sh scripts/dev.sh</code>（'+esc(String(err))+'）</div>';
  }
}
function esc(s){return (s||'').replace(/[<>&]/g,c=>({'<':'&lt;','>':'&gt;','&':'&amp;'}[c]))}
async function run(){
  const out = document.getElementById('out');
  // ② 立刻给"处理中"，让用户能区分"在跑"和"死了"
  out.innerHTML = '<div class="row">处理中…</div>';
  const text = document.getElementById('t').value;
  try{
  const d = await j('/v1/testpage', {text});
  lastResult = d;   // 供 〔✔/✘/这个改错了〕 指认本次结果
  let h = '<div class="row"><span class="k">原始：</span><code>'+esc(d.raw)+'</code></div>'+
          '<div class="row"><span class="k">纠错：</span><code>'+esc(d.corrected)+'</code></div>'+
          '<div class="row"><span class="k">标点：</span><code>'+esc(d.punctuated)+'</code></div>'+
          '<div class="row"><span class="k">意图：</span>'+esc(d.intent)+
          ' ｜ 回问：'+(d.ask_back?'<b style="color:#c60">是</b>':'否')+
          ' ｜ 降级：'+(d.degraded?'<b style="color:#c60">是</b>':'否')+'</div>'+
          '<div class="row"><span class="k">层级：</span>'+esc(d.level)+' ｜ 耗时 '+d.ms+' ms ｜ 教的词命中：'+(d.taught_hit?'是':'否')+'</div>';
  if(d.degraded && d.degraded_reason){h += '<div class="row">降级原因：'+esc(d.degraded_reason)+'</div>';}
  if(d.ask_back){h += '<div class="row">候选（回问必须给候选）：'+JSON.stringify(d.candidates||[])+'</div>';}
  if(d.log_error){h += '<div class="row" style="color:#c00">台账写入失败：'+esc(d.log_error)+'</div>';}
  out.innerHTML = h;
  log();
  }catch(err){
    // ① **失败必须可见**（静默失败是这次要钉住的东西）
    out.innerHTML = '<div class="row" style="color:#c00"><b>处理失败</b>：'+esc(String(err))+
      '<br>请求：<code>POST /v1/testpage</code>'+
      '<br>服务可能已停止，请重新运行 <code>sh scripts/dev.sh</code></div>';
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
  out.innerHTML = '<div class="row">规划中…</div>';
  try{
    const r = await fetch('/v1/task', {method:'POST', headers:{'Content-Type':'application/json'},
      body: JSON.stringify({task: document.getElementById('task').value, document: document.getElementById('doc').value})});
    if(!r.ok){ throw new Error('HTTP '+r.status+' /v1/task'); }
    const d = await r.json();
    const p = d.plan;
    let h = '<div class="row"><b>'+(d.execute?'':'【只规划，不执行】')+'</b> 目标：<code>'+esc(d.goal)+'</code></div>';
    h += '<div class="row"><span class="k">来源：</span>'+esc(p.source||'-')+
         ' ｜ 降级：'+(p.degraded?('是（'+esc(p.degraded_reason||'')+'）'):'否')+
         ' ｜ 拒绝：'+(p.refused?'<b style="color:#c60">是</b>':'否')+'</div>';
    if(p.reason){ h += '<div class="row">原因：'+esc(p.reason)+'</div>'; }
    if(p.missing && p.missing.length){ h += '<div class="row">做不到 / 找谁：<ul>'+p.missing.map(x=>'<li>'+esc(x)+'</li>').join('')+'</ul></div>'; }
    if(p.steps && p.steps.length){
      h += '<table><tr><th>#</th><th>工具</th><th>cap</th><th>动作</th><th>产出</th><th>域</th></tr>';
      p.steps.forEach(s=>{ h += '<tr><td>'+s.index+'</td><td>'+esc(s.tool)+'</td><td>'+esc((s.caps||[]).join(','))+'</td><td>'+esc(s.action)+'</td><td>'+esc(s.output)+'</td><td>'+esc(s.domain||'')+'</td></tr>'; });
      h += '</table>';
    } else { h += '<div class="row" style="color:#666">没有可执行步骤（见上面的拒绝原因/找谁）。</div>'; }
    h += '<details><summary>轨迹（考虑了哪些要素 / 工作记忆）</summary><pre style="white-space:pre-wrap">'+
         esc(JSON.stringify({considered:p.considered, wm:p.wm}, null, 1))+'</pre></details>';
    out.innerHTML = h;
  }catch(err){
    out.innerHTML = '<div class="row" style="color:#c00"><b>规划失败</b>：'+esc(String(err))+
      '<br>请求：<code>POST /v1/task</code><br>服务可能已停止，请重新运行 <code>sh scripts/dev.sh</code></div>';
  }
}
async function teach(){
  const term = prompt('教哪个词（如 哎欧劈艾斯）？'); if(!term) return;
  const canonical = prompt('规范化成什么（如 aiops）？'); if(!canonical) return;
  const r = await j('/v1/observe', {term, canonical});
  alert(JSON.stringify(r));
}
// ---- 回馈（✔/✘）与「这个改错了」（A8/A9/A10）----
var lastResult = null;
function fbstatus(msg, isErr){
  const el = document.getElementById('fbstatus');
  el.style.color = isErr ? '#c00' : '#666';
  el.textContent = msg;
}
async function feedbackOk(){
  if(!lastResult){ fbstatus('先点〔处理〕再给评价', true); return; }
  try{
    const d = await j('/v1/feedback', {text_raw: lastResult.raw, text_final: lastResult.corrected, accepted: true, reason: '', source: 'testpage'});
    fbstatus('✔ 已记录（feedback.jsonl 第 ' + d.feedback.lines + ' 条）' + (d.feedback.log_error?('，落盘失败：'+d.feedback.log_error):''));
  }catch(err){ fbstatus('✔ 记录失败：'+String(err), true); }
}
function showNeg(){
  document.getElementById('fbneg').hidden = false;
  document.getElementById('fbneg_reason').focus();
}
async function submitNeg(){
  const reason = document.getElementById('fbneg_reason').value.trim();
  if(!lastResult){ fbstatus('先点〔处理〕再给评价', true); return; }
  try{
    const d = await j('/v1/feedback', {text_raw: lastResult.raw, text_final: lastResult.corrected, accepted: false, reason: reason, source: 'testpage'});
    document.getElementById('fbneg').hidden = true;
    fbstatus('✘ 已记录（feedback.jsonl 第 ' + d.feedback.lines + ' 条，原因：' + d.feedback.reason + '）');
  }catch(err){ fbstatus('✘ 记录失败：'+String(err), true); }
}
async function markWrong(){
  if(!lastResult){ fbstatus('先点〔处理〕再标记', true); return; }
  const corrs = (lastResult.corrections||[]).filter(c=>c.From && c.To);
  if(corrs.length === 0){
    fbstatus('本次没有可标记的改写（没有发生 From→To 纠错改写）', true);
    return;
  }
  const term = corrs[0].From;
  try{
    const d = await j('/v1/blacklist', {op:'add', term: term, note: lastResult.raw+' → '+lastResult.corrected});
    fbstatus('已把「'+d.term+'」加入改写黑名单并落盘，重新〔处理〕看效果');
    run();  // 立即重跑一次，让"生效"可见
  }catch(err){ fbstatus('黑名单记录失败：'+String(err), true); }
}
async function clearTaught(){ alert(JSON.stringify(await j('/v1/lexicon', {op:'clear_taught'}))); }
async function log(){
  const d = await j('/v1/testlog?n=20');
  document.getElementById('metrics').innerHTML = '<div class="row"><span class="k">真实样本：</span>'+d.total+
    ' 条 ｜ L0 比例 '+d.metrics.l0_share.toFixed(2)+' ｜ 回问率 '+d.metrics.ask_back_rate.toFixed(2)+
    ' ｜ 降级率 '+d.metrics.degraded_rate.toFixed(2)+'</div><div style="color:#666;font-size:14px">'+esc(d.metrics.note)+'<br>台账：'+esc(d.path)+'</div>';
  let h = '<table><tr><th>时间</th><th>原始</th><th>纠错</th><th>意图</th><th>回问</th><th>层级</th><th>ms</th></tr>';
  (d.recent||[]).slice().reverse().forEach(r=>{h+='<tr><td>'+esc((r.at||'').slice(11,19))+'</td><td>'+esc(r.raw)+'</td><td>'+esc(r.corrected)+'</td><td>'+esc(r.intent)+'</td><td>'+(r.ask_back?'是':'否')+'</td><td>'+esc(r.level)+'</td><td>'+r.ms+'</td></tr>';});
  document.getElementById('recent').innerHTML = h+'</table>';
}
log();
probe();  // 加载后自检服务是否活着
</script></body></html>`

func (s *Server) handleTestPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(testPageHTML))
}
