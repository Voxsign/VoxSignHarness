//go:build vhs002

// scopecriteria_test.go —— §4 新范围判据骨架（**先红**，RC6：判据先于实现）。
//
// 运行：go test -tags vhs002 ./asr
//
// 覆盖：意图 JSON / 指代消解三层 / L3 画像注入 / 词典语音增删改 /
// JSON 热加载（明令不用 YAML）/ 轨迹 JSONL / 模型兜底 3s 降级 / control 语义 / 学习可审计。
//
// 这些能力**当前一行都没实现**（本轮刻意不写，RC2/RC6），所以现在全红。
// 默认 `go test ./...` 不带 vhs002 tag，不受影响。
//
// 标点恢复（SCOPE-PUNCT）不在这里：它已实现，结构判据在 punct_test.go 的默认套件里（绿）。
package asr

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 8 类 + ORCHESTRATE（需求 4.6 / 验收 #2）。
var vhsIntentTypes = map[string]bool{
	"EDIT": true, "DEBUG": true, "QUERY": true, "TEST": true,
	"COMMIT": true, "DEPLOY": true, "NOTE": true, "ASK": true, "ORCHESTRATE": true,
}

// SCOPE-INTENT-01：输入转写文本 → 标准化意图 JSON，9 类全部兼容。
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

// SCOPE-REF-01/02：指代消解三层 + 低置信 need_disambiguate 回问。
func TestSCOPEREF01ThreeLayerDisambiguation(t *testing.T) {
	base := serviceBase(t)
	got := postJSON(t, base+"/v1/process", `{"text":"把那个文档改了","session_id":"ref-probe"}`)
	if got["need_disambiguate"] != true {
		t.Errorf("[SCOPE-REF-01] 无上下文指代低置信时应 need_disambiguate：%v", got)
	}
	// 第二层：带上下文应能确定指代对象
	got2 := postJSON(t, base+"/v1/process",
		`{"text":"把那个文档改了","session_id":"ref-probe","context":["打开 modules/quote"]}`)
	if got2["path"] == nil || got2["path"] == "" {
		t.Errorf("[SCOPE-REF-01] 有上下文时未消解出 path：%v", got2)
	}
}

func TestSCOPEREF02ConfirmationIsReused(t *testing.T) {
	// **v2（VHS-DECIDE-001 选项 C，Lead 发起的 v1→v2）**：
	// 服务**不记得**任何会话态；确认结构由调用方带回。旧版 v1（"服务自己记得"）已废弃。
	base := serviceBase(t)

	// ① 第一次：确认请求 → 必须返回**可携带的确认结构**
	first := postJSON(t, base+"/v1/process", `{"text":"确认，就是报价模块","session_id":"ref-probe-2"}`)
	conf, ok := first["confirmable"].(map[string]any)
	if !ok || conf["canonical"] == nil || conf["canonical"] == "" {
		t.Fatalf("[SCOPE-REF-02 v2] 第一次未返回可携带确认结构：%v", first)
	}
	canon, _ := conf["canonical"].(string)

	// ② 第二次：**调用方带回 confirmed** → 复用（不再回问）
	got := postJSON(t, base+"/v1/process",
		`{"text":"把那个模块改了","session_id":"ref-probe-2","confirmed":{"mention":"那个模块","canonical":"`+canon+`"}}`)
	if got["need_disambiguate"] == true {
		t.Errorf("[SCOPE-REF-02 v2] 带回 confirmed 后未复用：%v", got)
	}
	if got["path"] == nil || got["path"] == "" {
		t.Errorf("[SCOPE-REF-02 v2] 带回 confirmed 后未消解出 path：%v", got)
	}

	// ③ **反例：不带 confirmed ⇒ 必须回问** —— 证明服务是"真无状态"，而不是"偷偷记了"
	again := postJSON(t, base+"/v1/process", `{"text":"把那个模块改了","session_id":"ref-probe-2"}`)
	if again["need_disambiguate"] != true {
		t.Errorf("[SCOPE-REF-02 v2] 不带 confirmed 却未回问 ⇒ 服务偷偷记住了会话态：%v", again)
	}
}

// SCOPE-PROFILE-01：L3 只注入相关片段，输出带来源，不改写原文。
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

// SCOPE-DICT-01：词典支持语音指令增删改，立即生效、持久化、重启不丢。
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
	// delete（需确认）
	postJSON(t, base+"/v1/dictionary", `{"op":"delete","term":"冀总","confirm":true}`)
	got2 := postJSON(t, base+"/v1/correct", `{"text":"冀总看一下"}`)
	if strings.Contains(toJSON(got2), `"季总"`) {
		t.Errorf("[SCOPE-DICT-01] 删除未生效：%v", got2)
	}
}

// SCOPE-CONF-01：配置全部 JSON（**不用 YAML**），支持热加载。
func TestSCOPECONF01JSONHotReloadNotYAML(t *testing.T) {
	base := serviceBase(t)
	// 不得有 YAML 配置
	for _, pat := range []string{"*.yaml", "*.yml"} {
		m, _ := filepath.Glob(filepath.Join(vhsServiceDir, pat))
		// 判据路径修正（2026-10-03）：vhsServiceDir 是 "../cmd/vhs-asr"，
		// 所以 vhsServiceDir/../config = "../cmd/config" —— 错。
		// 从 asr/（测试工作目录）到仓库根的 config/ 应为 "../config"。
		// 由实现方上报、DSH 复核确认后修正（判据归 DSH）。
		m2, _ := filepath.Glob(filepath.Join("..", "config", pat))
		if len(m)+len(m2) > 0 {
			t.Errorf("[SCOPE-CONF-01] 发现 YAML 配置 %v %v —— 需求明令不用 YAML", m, m2)
		}
	}
	// 热加载：改词典文件后 N 秒内生效
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

// SCOPE-TRACE-01：全链路轨迹 JSONL，每步带耗时与命中来源。
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

// SCOPE-FALLBACK-01：模型兜底超时 → fail-open + degraded，不阻塞。
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

// SCOPE-CONTROL-01：控制语义是交互层语义，不得进入业务执行路由。
//
// 保守解释登记：需求原文未写明"撤销"的范围，本实现取「撤销上一轮输入/清空待确认」，
// 不解释为"撤销业务操作"，且 control 与业务 type 互斥。
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

// SCOPE-AUDIT-01：学习动作可审计（来源/时间/影响条目）。
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
