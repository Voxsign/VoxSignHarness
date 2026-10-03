package asr

import "net/http"

// testPageHTML 是零依赖测试页（无框架/CDN/npm）。输入框用标准 <textarea>，
// **不做键盘拦截** ⇒ macOS 听写（连按两下 Fn / 麦克风键）可直接用。
const testPageHTML = `<!doctype html>
<html lang="zh"><head><meta charset="utf-8"><title>VoxSign 本地测试页</title>
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
<p style="color:#666">把光标放进下面的框，用 macOS 听写说话（连按两下 Fn），再点「处理」。</p>
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
<div id="metrics"></div>
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
  return r.json();
}
function esc(s){return (s||'').replace(/[<>&]/g,c=>({'<':'&lt;','>':'&gt;','&':'&amp;'}[c]))}
async function run(){
  const text = document.getElementById('t').value;
  const d = await j('/v1/testpage', {text});
  let h = '<div class="row"><span class="k">原始：</span><code>'+esc(d.raw)+'</code></div>'+
          '<div class="row"><span class="k">纠错：</span><code>'+esc(d.corrected)+'</code></div>'+
          '<div class="row"><span class="k">意图：</span>'+esc(d.intent)+
          ' ｜ 回问：'+(d.ask_back?'<b style="color:#c60">是</b>':'否')+
          ' ｜ 降级：'+(d.degraded?'<b style="color:#c60">是</b>':'否')+'</div>'+
          '<div class="row"><span class="k">层级：</span>'+esc(d.level)+' ｜ 耗时 '+d.ms+' ms ｜ 教的词命中：'+(d.taught_hit?'是':'否')+'</div>';
  if(d.degraded && d.degraded_reason){h += '<div class="row">降级原因：'+esc(d.degraded_reason)+'</div>';}
  if(d.ask_back){h += '<div class="row">候选（回问必须给候选）：'+JSON.stringify(d.candidates||[])+'</div>';}
  if(d.log_error){h += '<div class="row" style="color:#c00">台账写入失败：'+esc(d.log_error)+'</div>';}
  document.getElementById('out').innerHTML = h;
  log();
}
async function teach(){
  const term = prompt('教哪个词（如 哎欧劈艾斯）？'); if(!term) return;
  const canonical = prompt('规范化成什么（如 aiops）？'); if(!canonical) return;
  const r = await j('/v1/observe', {term, canonical});
  alert(JSON.stringify(r));
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
</script></body></html>`

func (s *Server) handleTestPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(testPageHTML))
}
