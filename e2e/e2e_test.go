// Package e2e 是 V0 技术底座的端到端演示与验证：
// 跑通「ASR 模糊文本 → 清洗/纠错 → 意图 JSON → 模型路由 → 工具执行 → 回执 → 轨迹落盘」。
// 工具执行用最小演示执行器（demo shim，真实 tools 注册表在后续里程碑按定稿架构实现）。
package e2e

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"voicesign-harness/config"
	"voicesign-harness/contract"
	"voicesign-harness/input"
	"voicesign-harness/memory"
	"voicesign-harness/provider"
	"voicesign-harness/router"
	"voicesign-harness/trajectory"
)

// demoExecute 最小演示执行器（demo shim）：按意图执行真实动作并产收回执。
// 正式 tools 注册表（shell/read_file/…/风险分级）在后续里程碑实现。
func demoExecute(intent contract.Intent, root string) []contract.Receipt {
	switch intent.Intent {
	case contract.IntentTime:
		return []contract.Receipt{{
			Seq: 1, Tool: "get_time", OK: true,
			Stdout: time.Now().Format("2006-01-02 15:04:05 MST"),
		}}
	case contract.IntentFileList:
		p := intent.Slots["path"]
		if p == "" || p == "." {
			p = root
		} else {
			p = filepath.Join(root, p)
		}
		entries, err := os.ReadDir(p)
		if err != nil {
			return []contract.Receipt{{Seq: 1, Tool: "list_dir", OK: false, Err: err.Error()}}
		}
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		return []contract.Receipt{{Seq: 1, Tool: "list_dir", OK: true, Stdout: strings.Join(names, ", ")}}
	}
	return []contract.Receipt{{Seq: 1, Tool: "none", OK: false, Err: "演示执行器未支持意图 " + intent.Intent}}
}

// writeDemoConfig 写一份指向临时目录的演示配置（mock provider + 基础路由）。
func writeDemoConfig(t *testing.T, dir string) config.Config {
	t.Helper()
	logDir := filepath.Join(dir, "logs")
	memDir := filepath.Join(dir, "mem")
	cfgPath := filepath.Join(dir, "harness.json")
	js := `{
  "global": {"log_dir": "` + filepath.ToSlash(logDir) + `", "max_turns_default": 2},
  "providers": [{"name": "mock", "kind": "mock"}],
  "routes": [
    {"name": "time", "intent": ["TIME"], "provider": "local"},
    {"name": "file", "intent": ["FILE_READ", "FILE_WRITE", "FILE_LIST"], "provider": "mock", "max_turns": 1},
    {"name": "default", "provider": "mock", "default": true}
  ],
  "memory": {"dir": "` + filepath.ToSlash(memDir) + `"}
}`
	if err := os.WriteFile(cfgPath, []byte(js), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VHS_CONFIG", cfgPath)
	for _, k := range []string{"VHS_API_KEY", "VHS_PROVIDER", "VHS_ROUTE", "VHS_LOG_DIR", "VHS_ADDR", "VHS_MAX_TURNS", "VHS_ALLOW_HIGH_RISK", "VHS_INTENT_CONF", "VHS_LOW_CONF_ACTION"} {
		t.Setenv(k, "")
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// TestCorrectionEndToEndDemo 硬性验收：美墅→Mansour 纠错端到端。
// 链路：ASR 原文 → 清洗/纠错(内置词典) → 意图 FILE_LIST → 路由(file→mock) →
//
//	mock provider 连通 → 演示执行器 list_dir → 回执 → 轨迹落盘（含原文/纠错/意图/回执）。
func TestCorrectionEndToEndDemo(t *testing.T) {
	dir := t.TempDir()
	cfg := writeDemoConfig(t, dir)

	// 造一个真实目录，供 list_dir 执行。
	manDir := filepath.Join(dir, "Mansour")
	if err := os.MkdirAll(manDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(manDir, "resume.pdf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 认识层：词典（内置默认，含 美墅→Mansour）→ 管线。
	dict, err := memory.LoadDictionary(cfg.DictionaryPath())
	if err != nil {
		t.Fatal(err)
	}
	pipe := input.NewPipeline(&cfg, dict)
	res, err := pipe.Process("帮我打开美墅的文件夹看看有什么")
	if err != nil {
		t.Fatal(err)
	}

	// 断言：原文保留 / 纠错 / 意图。
	if res.Raw != "帮我打开美墅的文件夹看看有什么" {
		t.Fatalf("raw 必须原样保留: %q", res.Raw)
	}
	if !strings.Contains(res.Corrected, "Mansour") {
		t.Fatalf("纠错后应含 Mansour: %q", res.Corrected)
	}
	hasCorrection := false
	for _, c := range res.Corrections {
		if c.From == "美墅" && c.To == "Mansour" && c.Rule == "dict" {
			hasCorrection = true
		}
	}
	if !hasCorrection {
		t.Fatalf("应记录 美墅→Mansour 纠错: %+v", res.Corrections)
	}
	if res.Intent.Intent != contract.IntentFileList {
		t.Fatalf("意图应为 FILE_LIST: %+v", res.Intent)
	}
	if res.Intent.NeedsClarification() {
		t.Fatalf("高置信意图不应回问: %+v", res.Intent)
	}
	if res.Intent.Confidence < cfg.Input.IntentConf {
		t.Fatalf("置信度应不低于阈值: %v", res.Intent.Confidence)
	}

	// 路由：file → mock。
	rt, err := router.Resolve(&cfg, res.Intent, res.Corrected)
	if err != nil {
		t.Fatal(err)
	}
	if rt.Provider != "mock" || rt.MaxTurns != 1 {
		t.Fatalf("路由应为 file→mock(max_turns=1): %+v", rt)
	}

	// 接口层：mock provider 连通（闭环内确有模型调用）。
	reg, err := provider.NewRegistry(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	mock, err := reg.Get(rt.Provider)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := mock.Chat(context.Background(), provider.ChatRequest{
		Messages: []contract.Message{{Role: contract.RoleUser, Content: res.Corrected}},
	})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := contract.ParseActionPlan(resp.Content)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) == 0 {
		t.Fatalf("mock 应返回动作计划: %+v", plan)
	}

	// 演示执行器：真实 list_dir → 回执。
	receipts := demoExecute(res.Intent, dir)
	if len(receipts) != 1 || !receipts[0].OK {
		t.Fatalf("list_dir 回执应为成功: %+v", receipts)
	}
	if !strings.Contains(receipts[0].Stdout, "resume.pdf") {
		t.Fatalf("回执应包含目录内容 resume.pdf: %q", receipts[0].Stdout)
	}

	// 轨迹：原文 / 纠错 / 意图 / 回执 全部落盘。
	tr, err := trajectory.Open(cfg.Global.LogDir)
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	rid := "e2e-demo-1"
	for _, e := range []trajectory.Entry{
		{RequestID: rid, Kind: trajectory.KindInputRaw, Content: res.Raw},
		{RequestID: rid, Kind: trajectory.KindInputCorrec, Content: res.Corrected},
		{RequestID: rid, Kind: trajectory.KindIntent, Intent: &res.Intent},
		{RequestID: rid, Kind: trajectory.KindReceipts, Receipts: receipts},
		{RequestID: rid, Kind: trajectory.KindFinal, Content: "已查看 Mansour 文件夹，共 1 个文件"},
	} {
		if err := tr.Write(e); err != nil {
			t.Fatal(err)
		}
	}

	data, err := os.ReadFile(filepath.Join(cfg.Global.LogDir, "trajectory-"+time.Now().Format("20060102")+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	joined := string(data)
	if !strings.Contains(joined, "帮我打开美墅的文件夹看看有什么") ||
		!strings.Contains(joined, "Mansour") ||
		!strings.Contains(joined, "FILE_LIST") ||
		!strings.Contains(joined, "resume.pdf") {
		t.Fatalf("轨迹应包含 原文/纠错/意图/回执: %s", joined)
	}
}

// TestTimeLocalDirect 时间意图本地直通：零 LLM、route=local、get_time 执行。
func TestTimeLocalDirect(t *testing.T) {
	dir := t.TempDir()
	cfg := writeDemoConfig(t, dir)
	dict, err := memory.LoadDictionary(cfg.DictionaryPath())
	if err != nil {
		t.Fatal(err)
	}
	pipe := input.NewPipeline(&cfg, dict)
	res, err := pipe.Process("现在几点")
	if err != nil {
		t.Fatal(err)
	}
	if res.Intent.Intent != contract.IntentTime || res.Intent.NeedsClarification() {
		t.Fatalf("TIME 意图不应回问: %+v", res.Intent)
	}
	rt, err := router.Resolve(&cfg, res.Intent, res.Corrected)
	if err != nil {
		t.Fatal(err)
	}
	if rt.Provider != config.LocalProvider || rt.MaxTurns != 0 {
		t.Fatalf("TIME 应路由 local 且 0 轮: %+v", rt)
	}
	receipts := demoExecute(res.Intent, dir)
	if len(receipts) != 1 || !receipts[0].OK || !strings.Contains(receipts[0].Stdout, time.Now().Format("2006")) {
		t.Fatalf("get_time 回执异常: %+v", receipts)
	}
}

// TestLowConfidenceAskBack 低置信回问：不执行任何动作。
func TestLowConfidenceAskBack(t *testing.T) {
	dir := t.TempDir()
	cfg := writeDemoConfig(t, dir)
	dict, err := memory.LoadDictionary(cfg.DictionaryPath())
	if err != nil {
		t.Fatal(err)
	}
	pipe := input.NewPipeline(&cfg, dict)

	// UNKNOWN → 必回问。
	res, err := pipe.Process("随便来点什么")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Intent.NeedsClarification() || res.Intent.Ask == "" {
		t.Fatalf("UNKNOWN 应回问: %+v", res.Intent)
	}
	// 回问即止：演示执行器不得被调用（此处直接断言 Ask 语义，不产生任何回执）。

	// INFO → 不回问，交模型（Ask 空）。
	res2, err := pipe.Process("帮我翻译这句话")
	if err != nil {
		t.Fatal(err)
	}
	if res2.Intent.Intent != contract.IntentInfo || res2.Intent.NeedsClarification() {
		t.Fatalf("INFO 应交模型且不回问: %+v", res2.Intent)
	}
}

// TestIntentJSONMarshal 意图 JSON 可被轨迹与回执消费。
func TestIntentJSONMarshal(t *testing.T) {
	i := contract.Intent{Intent: contract.IntentFileList, Slots: map[string]string{"path": "Mansour"}, Confidence: 0.8, CorrectedText: "打开 Mansour 的文件夹"}
	b, err := json.Marshal(i)
	if err != nil {
		t.Fatal(err)
	}
	var out contract.Intent
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.Intent != contract.IntentFileList {
		t.Fatal("round-trip failed")
	}
}
