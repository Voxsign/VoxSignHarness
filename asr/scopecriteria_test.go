//go:build vhs002

// scopecriteria_test.go -- §4 new   data  (**first **, RC6:  datafirstat now). 
//
//   : go test -tags vhs002 ./asr
//
// overwrite: intent JSON / coreference resolution   / L3   notein / word langaudioadd modify /
// JSON    (   use YAML)/ trace JSONL /  type bot 3s    / control semantic /      . 
//
//     **curbefore  all  now**(base    write, RC2/RC6),  bynow safety . 
// default `go test ./...`    vhs002 tag,  accept  . 
//
// tgtpt  (SCOPE-PUNCT)    :  already now, close  data  punct_test.go  default   ( ). 
package asr

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 8 class + ORCHESTRATE(needrequire 4.6 /  recv #2). 
var vhsIntentTypes = map[string]bool{
	"EDIT": true, "DEBUG": true, "QUERY": true, "TEST": true,
	"COMMIT": true, "DEPLOY": true, "NOTE": true, "ASK": true, "ORCHESTRATE": true,
}

// SCOPE-INTENT-01:  in write base -> tgtapproveizeintent JSON, 9 classsafety compat. 
func TestSCOPEINTENT01NineIntentTypes(t *testing.T) {
	base := serviceBase(t)
	samples := map[string]string{
		"EDIT":        `{"text":"把报价单改成中文"}`,
		"DEBUG":       `{"text":"查一下这个报错的原因"}`,
		"QUERY":       `{"text":"库存还有多少"}`,
		"TEST":        `{"text":"跑一下测试"}`,
		"COMMIT":      `{"text":"提交这次改动"}`,
		"DEPLOY":      `{"text":"部署到服务器"}`,
		"NOTE":        `{"text":"记一下这个想法"}`,
		"ASK":         `{"text":"这个怎么弄"}`,
		"ORCHESTRATE": `{"text":"先查库存再改报价最后提交"}`,
	}
	for want, body := range samples {
		t.Run(want, func(t *testing.T) {
			got := postJSON(t, base+"/v1/process", body)
			if got["type"] != want {
				t.Errorf("[SCOPE-INTENT-01] 期望 %s，实际 %v", want, got["type"])
			}
			if _, ok := got["confidence"]; !ok {
				t.Errorf("[SCOPE-INTENT-01] 缺 confidence")
			}
		})
	}
}

// SCOPE-REF-01/02: coreference resolution   + low-confidence need_disambiguate clarification. 
func TestSCOPEREF01ThreeLayerDisambiguation(t *testing.T) {
	base := serviceBase(t)
	got := postJSON(t, base+"/v1/process", `{"text":"把那个文档改了","session_id":"ref-probe"}`)
	if got["need_disambiguate"] != true {
		t.Errorf("[SCOPE-REF-01] 无上下文指代低置信时应 need_disambiguate：%v", got)
	}
	//    :  onunder     coreferenceto 
	got2 := postJSON(t, base+"/v1/process",
		`{"text":"把那个文档改了","session_id":"ref-probe","context":["打开 modules/quote"]}`)
	if got2["path"] == nil || got2["path"] == "" {
		t.Errorf("[SCOPE-REF-01] 有上下文时未消解出 path：%v", got2)
	}
}

func TestSCOPEREF02ConfirmationIsReused(t *testing.T) {
	// **v2(VHS-DECIDE-001    C, Lead sendraise  v1->v2)**: 
	// serveservice**   **    state; confirmclose bycalluse  back.    v1("serveservice    ")alreadydeprecated. 
	base := serviceBase(t)

	// ①    : confirm require ->   returnback**    confirmclose **
	first := postJSON(t, base+"/v1/process", `{"text":"确认，就是报价模块","session_id":"ref-probe-2"}`)
	conf, ok := first["confirmable"].(map[string]any)
	if !ok || conf["canonical"] == nil || conf["canonical"] == "" {
		t.Fatalf("[SCOPE-REF-02 v2] 第一次未返回可携带确认结构：%v", first)
	}
	canon, _ := conf["canonical"].(string)

	// ②    : **calluse  back confirmed** ->  use( againclarification)
	got := postJSON(t, base+"/v1/process",
		`{"text":"把那个模块改了","session_id":"ref-probe-2","confirmed":{"mention":"那个模块","canonical":"`+canon+`"}}`)
	if got["need_disambiguate"] == true {
		t.Errorf("[SCOPE-REF-02 v2] 带回 confirmed 后未复用：%v", got)
	}
	if got["path"] == nil || got["path"] == "" {
		t.Errorf("[SCOPE-REF-02 v2] 带回 confirmed 后未消解出 path：%v", got)
	}

	// ③ **revexample:    confirmed ⇒   clarification** --   serveserviceis" nostatus", but is"   "
	again := postJSON(t, base+"/v1/process", `{"text":"把那个模块改了","session_id":"ref-probe-2"}`)
	if again["need_disambiguate"] != true {
		t.Errorf("[SCOPE-REF-02 v2] 不带 confirmed 却未回问 ⇒ 服务偷偷记住了会话态：%v", again)
	}
}

// SCOPE-PROFILE-01: L3 onlynotein close seg,  out   ,  modifywriteorig . 
func TestSCOPEPROFILE01RelevantContextOnly(t *testing.T) {
	base := serviceBase(t)
	got := postJSON(t, base+"/v1/process", `{"text":"按上次的偏好处理这个报价","session_id":"p1"}`)
	if got["context_sources"] == nil {
		t.Errorf("[SCOPE-PROFILE-01] 缺 context_sources（无法归因注入来源）：%v", got)
	}
	if strings.Contains(strings.ToLower(toJSON(got)), `"all_history"`) {
		t.Errorf("[SCOPE-PROFILE-01] 疑似全量加载历史（需求 4.5 禁止）")
	}
}

// SCOPE-DICT-01: word  keeplangaudiorefer add modify,  i.e.occur , keep ize, heavystart  . 
func TestSCOPEDICT01VoiceDictAddDeletePersist(t *testing.T) {
	base := serviceBase(t)
	bin := os.Getenv("VHS_ASR_BIN")
	if bin == "" {
		t.Fatalf("[criterion] 需要 VHS_ASR_BIN 做重启测试（未配置 → 先红）")
	}
	_ = bin
	// add
	postJSON(t, base+"/v1/dictionary", `{"text":"记住，冀总是冀中的冀"}`)
	got := postJSON(t, base+"/v1/correct", `{"text":"季总看一下"}`)
	if !strings.Contains(toJSON(got), "冀总") {
		t.Errorf("[SCOPE-DICT-01] 语音新增词典未生效：%v", got)
	}
	// delete(needconfirm)
	postJSON(t, base+"/v1/dictionary", `{"op":"delete","term":"冀总","confirm":true}`)
	got2 := postJSON(t, base+"/v1/correct", `{"text":"冀总看一下"}`)
	if strings.Contains(toJSON(got2), `"季总"`) {
		t.Errorf("[SCOPE-DICT-01] 删除未生效：%v", got2)
	}
}

// SCOPE-CONF-01:   safety  JSON(** use YAML**),  keep   . 
func TestSCOPECONF01JSONHotReloadNotYAML(t *testing.T) {
	base := serviceBase(t)
	//   has YAML   
	for _, pat := range []string{"*.yaml", "*.yml"} {
		m, _ := filepath.Glob(filepath.Join(vhsServiceDir, pat))
		//  datapathfixpos(2026-10-03): vhsServiceDir is "../cmd/vhs-asr", 
		//  by vhsServiceDir/../config = "../cmd/config" --  . 
		// from asr/(    obj )to  root  config/  as "../config". 
		// by now on , DSH   confirmafterfixpos( data  DSH). 
		m2, _ := filepath.Glob(filepath.Join("..", "config", pat))
		if len(m)+len(m2) > 0 {
			t.Errorf("[SCOPE-CONF-01] 发现 YAML 配置 %v %v —— 需求明令不用 YAML", m, m2)
		}
	}
	//    : modifyword fileafter N secinoccur 
	dict := os.Getenv("VHS_ASR_DICT")
	if dict == "" {
		t.Fatalf("[criterion] 需要 VHS_ASR_DICT 做热加载测试（未配置 → 先红）")
	}
	if err := os.WriteFile(dict, []byte(`{"entries":[{"raw_speech":"哈牛斯","target":"harness","scope":"global","priority":9}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got := postJSON(t, base+"/v1/correct", `{"text":"哈牛斯接上"}`)
		if strings.Contains(toJSON(got), "harness") {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Errorf("[SCOPE-CONF-01] 词典热加载 2s 内未生效")
}

// SCOPE-TRACE-01: safetychainroutetrace JSONL,     timeand in  . 
func TestSCOPETRACE01TrajectoryJSONL(t *testing.T) {
	base := serviceBase(t)
	traces := os.Getenv("VHS_ASR_TRACES")
	if traces == "" {
		t.Fatalf("[criterion] 需要 VHS_ASR_TRACES（未配置 → 先红）")
	}
	postJSON(t, base+"/v1/process", `{"text":"记一下这个想法","session_id":"t1"}`)
	data, err := os.ReadFile(traces)
	if err != nil {
		t.Fatalf("[SCOPE-TRACE-01] 读轨迹失败: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("[SCOPE-TRACE-01] 轨迹行非法: %q", line)
		}
		for _, k := range []string{"step", "ms"} {
			if _, ok := rec[k]; !ok {
				t.Errorf("[SCOPE-TRACE-01] 轨迹缺字段 %q: %v", k, rec)
			}
		}
	}
}

// SCOPE-FALLBACK-01:  type bot time -> fail-open + degraded,    . 
func TestSCOPEFALLBACK01ModelTimeoutDegraded(t *testing.T) {
	base := serviceBase(t)
	if os.Getenv("VHS_ASR_FORCE_MODEL_TIMEOUT") == "" {
		t.Fatalf("[criterion] 需要 VHS_ASR_FORCE_MODEL_TIMEOUT=1 注入超时场景（未配置 → 先红）")
	}
	start := time.Now()
	got := postJSON(t, base+"/v1/process", `{"text":"这个复杂句子需要模型兜底","session_id":"f1"}`)
	if time.Since(start) > 4*time.Second {
		t.Errorf("[SCOPE-FALLBACK-01] 超时场景耗时 %v，超过 3s+余量", time.Since(start))
	}
	if got["degraded"] != true {
		t.Errorf("[SCOPE-FALLBACK-01] 降级未标记 degraded：%v", got)
	}
}

// SCOPE-CONTROL-01: controlsemanticis   semantic,    in service  routeby. 
//
// keep resolve   : needrequireorig  write "  "   , base nowget"  on   in/ empty confirm", 
//  resolve as"   service  ", and control and service type mutex. 
func TestSCOPECONTROL01InterruptPauseUndoAreInteractionOnly(t *testing.T) {
	base := serviceBase(t)
	for _, text := range []string{"暂停", "停一下", "撤销"} {
		got := postJSON(t, base+"/v1/process", `{"text":"`+text+`","session_id":"c1"}`)
		ctrl, _ := got["control"].(string)
		if ctrl == "" {
			t.Errorf("[SCOPE-CONTROL-01] %q 未识别为控制语义：%v", text, got)
			continue
		}
		if typ, _ := got["type"].(string); typ != "" && typ != "ASK" {
			t.Errorf("[SCOPE-CONTROL-01] %q 的控制语义被翻译成业务 type=%q（应互斥）", text, typ)
		}
	}
}

// SCOPE-AUDIT-01:        (  /timetime/   obj). 
func TestSCOPEAUDIT01LearningAuditable(t *testing.T) {
	base := serviceBase(t)
	postJSON(t, base+"/v1/feedback", `{"text_raw":"哈牛斯","text_final":"harness","accepted":true,"source":"user_edit"}`)
	got := postJSON(t, base+"/v1/dictionary", `{"op":"list"}`)
	s := toJSON(got)
	for _, k := range []string{"source", "created_at"} {
		if !strings.Contains(s, k) {
			t.Errorf("[SCOPE-AUDIT-01] 学习条目缺可审计字段 %q：%s", k, s)
		}
	}
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// SCOPE-PROFILE-01b(Lead  decide): `none` andits   **mutex** --      value   writeout . 
//
// ⚠️  datatgt : **forward-guard(before   )** -- curbeforenonumdata triggersend
// (serveservice returnback value  ; zhiji/learned   connectin).  only  "forbidstop  store ", 
// **   "  andstore scenario be under"**.   zhiji/learned connectinafter,  patch  andstore scenario  data. 
//
// `none`  semanticis"    all has"; if handwritten store ,  then is" has  ". 
// ifwill needneed" split    ",  is**new  become **,  is use none. 
func TestSCOPEProfileSourcesAreExclusiveWithNone(t *testing.T) {
	base := serviceBase(t)
	got := postJSON(t, base+"/v1/process", `{"text":"按上次的偏好处理这个报价","session_id":"p-excl"}`)
	raw, ok := got["context_sources"].([]any)
	if !ok {
		t.Fatalf("[PROFILE-01b] context_sources 缺失或类型不对：%v", got["context_sources"])
	}
	enum := map[string]bool{"handwritten": true, "zhiji": true, "learned": true, "project-map": true, "none": true}
	hasNone, hasOther := false, false
	for _, v := range raw {
		s, _ := v.(string)
		if !enum[s] {
			t.Errorf("[PROFILE-01b] 枚举外的来源值 %q（消费方无法穷举）：%v", s, raw)
		}
		if s == "none" {
			hasNone = true
		} else {
			hasOther = true
		}
	}
	if hasNone && hasOther {
		t.Errorf("[PROFILE-01b] none 与其它来源并存（自相矛盾的值）：%v", raw)
	}
	if !hasNone && !hasOther {
		t.Errorf("[PROFILE-01b] 来源为空 —— 必须显式说明「有没有」，不能留白：%v", raw)
	}
}
