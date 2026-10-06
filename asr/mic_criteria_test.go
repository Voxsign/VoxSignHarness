//go:build vhsui

// mic_criteria_test.go --  audioby   data(close disconnectlang + node  asdisconnectlang). 
package asr

import (
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func fetchPage(t *testing.T) string {
	t.Helper()
	base, _ := newUIServer(t)
	resp, err := http.Get(base + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// ① close : has audioby  + has  branch + has rec.start() + onresult write textarea. 
func TestMicPageStructure(t *testing.T) {
	page := fetchPage(t)
	for _, want := range []string{
		`id="mic"`, "webkitSpeechRecognition", "btn.disabled = true",
		"r.start()", "onresult", "ta.value",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("[mic] 页面缺少 %q", want)
		}
	}
	// **     **: onresult    outnow run() / fetch( / testpage
	seg := page[strings.Index(page, "r.onresult"):]
	seg = seg[:strings.Index(seg, "r.onend")]
	for _, bad := range []string{"run()", "fetch(", "/v1/testpage"} {
		if strings.Contains(seg, bad) {
			t.Errorf("[mic] onresult 里出现 %q ⇒ 可能自动提交", bad)
		}
	}
}

// ②  as(node  ): pt  ⇒ rec.start(); onresult ⇒  in textarea;      ;   keep ⇒   + show. 
func TestMicBehaviorWithNodeStub(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("无 node，跳过行为断言（结构断言仍然生效）")
	}
	page := fetchPage(t)
	i := strings.Index(page, "<script>")
	j := strings.Index(page[i:], "</script>")
	script := page[i+len("<script>") : i+j]

	harness := `
const assert = require('assert');
let started = 0, posted = 0;
function makeEl(id){ return {id, value:'', dataset:{}, disabled:false, textContent:'',
  set innerHTML(v){}, onclick:null, addEventListener(){}}; }
const els = {t:makeEl('t'), mic:makeEl('mic'), michint:makeEl('michint'), out:makeEl('out'),
  metrics:makeEl('metrics'), recent:makeEl('recent')};
global.document = { getElementById:id=>els[id]||makeEl(id) };
global.window = { addEventListener:(ev,fn)=>{ if(ev==='load') global.__onload=fn; } };
global.fetch = (url, opts)=>{ if(opts && opts.method === 'POST'){ posted++; } return Promise.resolve({ok:true, json:()=>Promise.resolve({total:0, recent:[], metrics:{l0_share:0, ask_back_rate:0, degraded_rate:0, note:''}})}); };
function FakeRec(){ this.start=()=>{ started++; }; this.onresult=null; this.onend=null; }
global.webkitSpeechRecognition = FakeRec;
window.webkitSpeechRecognition = FakeRec; // 页面用 ('webkitSpeechRecognition' in window) 探测
global.alert = ()=>{};
global.prompt = ()=>null;
` + script + `
// 支持分支
__onload();
els.mic.onclick();
assert.strictEqual(started, 1, '点击后应调用 start()');
// 触发 onresult：页面脚本的 var rec 在同一作用域内可直接访问
els.t.dataset.recBase = '前缀';
// 直接调用页面的 onresult 逻辑：重新挂一个假实例不可行 ⇒ 用真实实例 path:
// 页面把实例存在 rec 变量里（脚本内 var rec）——在 global 作用域下我们可访问
assert.ok(typeof rec !== 'undefined', 'rec 应存在于脚本作用域');
rec.onresult({resultIndex:0, results:[[{transcript:'哎欧劈艾斯'}]]});
assert.strictEqual(els.t.value, '前缀哎欧劈艾斯', 'onresult 应填入 textarea');
assert.strictEqual(posted, 0, '不得自动提交（不应发 POST）');
console.log('NODE_OK');
`
	dir := t.TempDir()
	f := filepath.Join(dir, "harness.js")
	if err := os.WriteFile(f, []byte(harness), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("node", f).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "NODE_OK") {
		t.Fatalf("[mic] node 行为断言失败: %v\n%s", err, out)
	}
}
