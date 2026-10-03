//go:build vhsui

// uifail_criteria_test.go —— **失败路径**判据（这次修的是静默失败，所以判据钉失败路径）。
package asr

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func pageScript(t *testing.T) string {
	page := fetchPage(t)
	i := strings.Index(page, "<script>")
	j := strings.Index(page[i:], "</script>")
	return page[i+len("<script>") : i+j]
}

// 固定桩：元素保存 innerHTML；fetch 可按用例注入失败。
const failHarnessPrelude = `
const assert = require('assert');
function el(id){ return {id, value:'', dataset:{}, disabled:false, textContent:'', innerHTML:'', onclick:null, addEventListener(){}}; }
const els = {t:el('t'), mic:el('mic'), michint:el('michint'), out:el('out'),
  metrics:el('metrics'), recent:el('recent'), banner:el('banner')};
global.document = { getElementById:id=>els[id]||el(id) };
global.window = { addEventListener:(ev,fn)=>{ if(ev==='load') global.__onload=fn; } };
global.alert=()=>{}; global.prompt=()=>null;
// 预置安全 fetch：页面脚本末尾会立即调用 log()/probe()（否则 node 里用原生 fetch 解析相对 URL 会崩）
global.fetch = ()=>Promise.resolve({ok:true, json:()=>Promise.resolve({total:0,recent:[],metrics:{l0_share:0,ask_back_rate:0,degraded_rate:0,note:''}})});
function FakeRec(){ this.start=()=>{}; this.onresult=null; this.onend=null; }
global.webkitSpeechRecognition = FakeRec; window.webkitSpeechRecognition = FakeRec;
`

func runNode(t *testing.T, script string) string {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("无 node，跳过行为断言")
	}
	f := filepath.Join(t.TempDir(), "h.js")
	if err := os.WriteFile(f, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("node", f).CombinedOutput()
	if err != nil {
		t.Fatalf("node 断言失败: %v\n%s", err, out)
	}
	return string(out)
}

// ① **失败可见**：/v1/testpage 失败 ⇒ #out 必须非空且含错误信息（不得空白）。
func TestUIFailureIsVisible(t *testing.T) {
	js := failHarnessPrelude + pageScript(t) + `
global.fetch = (url, opts)=>{
  if(String(url).includes('/v1/testpage')){ return Promise.reject(new Error('boom')); }
  return Promise.resolve({ok:true, json:()=>Promise.resolve({total:0,recent:[],metrics:{l0_share:0,ask_back_rate:0,degraded_rate:0,note:''}})});
};
global.__onload();
run().then(()=>{
  assert.ok(els.out.innerHTML.length > 0, '#out 不得空白（静默失败）');
  assert.ok(els.out.innerHTML.includes('处理失败'), '#out 应含"处理失败"');
  assert.ok(els.out.innerHTML.includes('/v1/testpage'), '应给出请求 URL');
  assert.ok(els.out.innerHTML.includes('dev.sh'), '应给出可操作提示');
  console.log('NODE_OK_FAILVISIBLE');
}).catch(e=>{ console.error(e); process.exit(1); });
`
	out := runNode(t, js)
	if !strings.Contains(out, "NODE_OK_FAILVISIBLE") {
		t.Fatalf("未通过: %s", out)
	}
}

// ② 处理中状态：点击后、响应前 ⇒ 必须出现"处理中"。
func TestUIProcessingState(t *testing.T) {
	js := failHarnessPrelude + pageScript(t) + `
let resolveIt; global.fetch = (url, opts)=>{
  if(String(url).includes('/v1/testpage')){ return new Promise(r=>{ resolveIt = ()=>r({ok:true,json:()=>Promise.resolve({raw:'x',corrected:'x',intent:'ASK',ask_back:false,degraded:false,level:'L0',ms:1,taught_hit:false,candidates:[],corrections:[]})}); }); }
  return Promise.resolve({ok:true, json:()=>Promise.resolve({total:0,recent:[],metrics:{l0_share:0,ask_back_rate:0,degraded_rate:0,note:''}})});
};
global.__onload();
const p = run();
assert.ok(els.out.innerHTML.includes('处理中'), '点击后应立即显示"处理中"');
resolveIt();
p.then(()=>{ assert.ok(!els.out.innerHTML.includes('处理中'), '响应后应替换结果'); console.log('NODE_OK_PENDING'); })
 .catch(e=>{ console.error(e); process.exit(1); });
`
	out := runNode(t, js)
	if !strings.Contains(out, "NODE_OK_PENDING") {
		t.Fatalf("未通过: %s", out)
	}
}

// ③ 服务不可用横幅 + ④ 文案（主路径是〔🎤 说话〕）。
func TestUIBannerAndCopy(t *testing.T) {
	page := fetchPage(t)
	if !strings.Contains(page, "🎤 说话") || !strings.Contains(page, "点 <b>〔🎤 说话〕</b>") {
		t.Errorf("[文案] 页面未把〔🎤 说话〕写成主路径")
	}
	js := failHarnessPrelude + pageScript(t) + `
global.fetch = ()=>{ return Promise.reject(new Error('conn refused')); };
global.__onload();
probe().then(()=>{
  assert.ok(els.banner.innerHTML.includes('服务未运行'), '探测失败应出现横幅');
  assert.ok(els.banner.innerHTML.includes('dev.sh'), '横幅应给可操作提示');
  console.log('NODE_OK_BANNER');
}).catch(e=>{ console.error(e); process.exit(1); });
`
	out := runNode(t, js)
	if !strings.Contains(out, "NODE_OK_BANNER") {
		t.Fatalf("未通过: %s", out)
	}
}
