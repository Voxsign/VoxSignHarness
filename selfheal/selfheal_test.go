package selfheal

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"voicesign-harness/config"
	"voicesign-harness/contract"
	"voicesign-harness/provider"
)

// newDiagProvider 用 httptest 造一个指向测试端点的 diag openai provider（零真实网络）。
// 返回 diag provider 与请求计数（断言 KB 命中时 0 调用 / 未命中时调用次数）。
func newDiagProvider(t *testing.T, h http.HandlerFunc) (provider.Provider, *int32) {
	t.Helper()
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		h(w, r)
	}))
	t.Cleanup(ts.Close)
	trueVal := true
	cfg := config.Config{
		Global: config.Global{LLMTimeoutMs: 5000},
		Providers: []config.Provider{{
			Name: "diag", Kind: config.OpenAIKind, Endpoint: ts.URL, Model: "jev-diagnose",
			APIKey: "test-key", ResponseFormat: &trueVal,
			Params: map[string]any{"use_max_completion_tokens": true},
		}},
	}
	reg, err := provider.NewRegistry(&cfg)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	p, err := reg.Get("diag")
	if err != nil {
		t.Fatalf("get diag: %v", err)
	}
	return p, &calls
}

// diagJSONBody 是一个合法的诊断模型响应。
const diagJSONBody = `{"choices":[{"message":{"content":"{\"category\":\"network\",\"root_cause\":\"模型中心超时抖动\",\"confidence\":0.8,\"recoverable\":true,\"suggestion\":\"退避后重试\",\"action\":\"retry\",\"retry_params\":{}}","finish_reason":"stop"}}],"usage":{}}`

func okJSON(w http.ResponseWriter, content string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"choices":[{"message":{"content":`+jsonString(content)+`,"finish_reason":"stop"}}],"usage":{}}`)
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestFingerprintAndErrSegment(t *testing.T) {
	// 确定性：同输入同指纹。
	a := Fingerprint("QUERY", "search", "dial tcp: i/o timeout\nsecond line")
	b := Fingerprint("QUERY", "search", "dial tcp: i/o timeout\nother noise")
	if a != b {
		t.Fatalf("首行相同应同指纹: %s vs %s", a, b)
	}
	// 工具/意图不同 → 指纹不同。
	if Fingerprint("EDIT", "search", "x") == Fingerprint("QUERY", "search", "x") {
		t.Fatal("intent 不同指纹应不同")
	}
	// errFirstSegment：首行去空白 + 80 截断。
	long := strings.Repeat("字", 100)
	if got := errFirstSegment(long); len(got) != 80 {
		t.Fatalf("应截断到 80 字符, got %d", len(got))
	}
	if got := errFirstSegment("  hello world  \n tail "); got != "hello world" {
		t.Fatalf("应取首行去空白, got %q", got)
	}
}

func TestKBLoadDedupeAndRemember(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exceptions.jsonl")
	// 两行同指纹：后写的应覆盖先写的（updated 更新）。
	old := KBEntry{Fingerprint: "fp1", Category: CatNetwork, Suggestion: "old", Hits: 1, Updated: "2020-01-01T00:00:00Z"}
	new := KBEntry{Fingerprint: "fp1", Category: CatBudget, Suggestion: "预算已用尽", Recoverable: true, Action: ActionFallback, Hits: 2, Updated: "2026-10-02T00:00:00Z"}
	b1, _ := json.Marshal(old)
	b2, _ := json.Marshal(new)
	seed := append(append(append(b1, '\n'), b2...), '\n')
	_ = os.WriteFile(path, seed, 0o600)

	kb := OpenKB(path)
	if kb.Count() != 1 {
		t.Fatalf("同指纹应去重为 1 条, got %d", kb.Count())
	}
	d, ok := kb.Lookup("fp1")
	if !ok {
		t.Fatal("应命中 fp1")
	}
	// 去重取最新：Category 应为后写的 budget（而非 network）。
	if d.Category != CatBudget {
		t.Fatalf("应取最新一条的 category, got %q", d.Category)
	}
	if !strings.Contains(d.Suggestion, "预算已用尽") {
		t.Fatalf("应取最新一条的建议, got %q", d.Suggestion)
	}
	if d.Source != "kb" {
		t.Fatalf("KB 命中 source 应为 kb, got %q", d.Source)
	}

	// Remember 新指纹 → 回写磁盘。
	kb.Remember(Diagnosis{Fingerprint: "fp2", Category: CatParam, RootCause: "max_tokens 400", Suggestion: "换 max_completion_tokens", Action: ActionModify, Recoverable: true})
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"fp2"`) {
		t.Fatalf("回写应含 fp2: %s", data)
	}
	// 再加载：fp2 hits=1。
	kb2 := OpenKB(path)
	if _, ok := kb2.Lookup("fp2"); !ok {
		t.Fatal("回写后应能查到 fp2")
	}
}

func TestDiagnoseKbHitNoModelCall(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exceptions.jsonl")
	fp := Fingerprint("QUERY", "search", "dial tcp: i/o timeout")
	kb := OpenKB(path)
	kb.Remember(Diagnosis{Fingerprint: fp, Category: CatNetwork, RootCause: "已知抖动", Suggestion: "复用知识库建议", Action: ActionRetry, Recoverable: true})

	diag, calls := newDiagProvider(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("KB 命中不应调用诊断模型")
		okJSON(w, diagJSONBody)
	})
	svc := NewService(kb, diag, nil)
	d := svc.Diagnose(context.Background(), "查一下", "QUERY", []Trace{NewTrace("search", "", nil, "dial tcp: i/o timeout")})
	if d == nil {
		t.Fatal("KB 应命中")
	}
	if d.Source != "kb" {
		t.Fatalf("source 应为 kb, got %q", d.Source)
	}
	if *calls != 0 {
		t.Fatalf("KB 命中应 0 模型调用, got %d", *calls)
	}
}

func TestDiagnoseModelHappyPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exceptions.jsonl")

	var body map[string]any
	diag, calls := newDiagProvider(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		okJSON(w, `{"category":"network","root_cause":"超时","confidence":0.9,"recoverable":true,"suggestion":"退避重试","action":"retry","retry_params":{}}`)
	})
	svc := NewService(OpenKB(path), diag, nil)
	d := svc.Diagnose(context.Background(), "查一下订单", "QUERY", []Trace{
		NewTrace("llm", "fast", map[string]any{"model": "fast"}, "HTTP 500: boom"),
	})
	if d == nil {
		t.Fatal("应返回诊断")
	}
	if *calls != 1 {
		t.Fatalf("应调用诊断模型 1 次, got %d", *calls)
	}
	// 请求体：模型名 jev-diagnose + response_format=json_object + user 负载含 task/intent/traces/model。
	if body["model"] != "jev-diagnose" {
		t.Fatalf("model 应为 jev-diagnose, got %v", body["model"])
	}
	rf, _ := body["response_format"].(map[string]any)
	if rf["type"] != "json_object" {
		t.Fatalf("应带 response_format=json_object, got %v", body["response_format"])
	}
	msgs, _ := body["messages"].([]any)
	if len(msgs) < 2 {
		t.Fatal("应有 system+user 两条消息")
	}
	userContent := msgs[1].(map[string]any)["content"].(string)
	var up diagnoseRequest
	if err := json.Unmarshal([]byte(userContent), &up); err != nil {
		t.Fatalf("user 负载应为 JSON: %v", err)
	}
	if up.Task != "查一下订单" || up.Intent != "QUERY" {
		t.Fatalf("user 负载 task/intent 不符: %+v", up)
	}
	if len(up.Traces) != 1 || up.Traces[0].Model != "fast" || up.Traces[0].Tool != "llm" {
		t.Fatalf("traces 应带 model=fast/tool=llm: %+v", up.Traces)
	}
	if up.Traces[0].Error == nil || up.Traces[0].Error.Code != "500" || up.Traces[0].Error.Type != "overload" {
		t.Fatalf("error.code/type 应从 HTTP 500 归类为 overload: %+v", up.Traces[0].Error)
	}
	if d.Category != CatNetwork || d.Action != ActionRetry || !d.Recoverable {
		t.Fatalf("诊断解析异常: %+v", d)
	}
}

func TestDiagnoseDegradesOnBadEndpoint(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"404": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":"not found"}`)
		},
		"500": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `boom`)
		},
		"malformed-json": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `this is not json {{{`)
		},
		"missing-fields": func(w http.ResponseWriter, r *http.Request) { okJSON(w, `{"root_cause":"x"}`) }, // 缺 category/action
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			diag, _ := newDiagProvider(t, h)
			svc := NewService(OpenKB(filepath.Join(t.TempDir(), "exceptions.jsonl")), diag, nil)
			d := svc.Diagnose(context.Background(), "t", "QUERY", []Trace{NewTrace("llm", "fast", nil, "x")})
			if d != nil {
				t.Fatalf("%s 应返回 nil（跳过不扩散）, got %+v", name, d)
			}
		})
	}
}

func TestDiagnoseNoDiagProviderSkips(t *testing.T) {
	// diag=nil（未配置）→ 返回 nil，零开销跳过。
	svc := NewService(OpenKB(filepath.Join(t.TempDir(), "exceptions.jsonl")), nil, nil)
	if d := svc.Diagnose(context.Background(), "t", "QUERY", []Trace{NewTrace("x", "", nil, "y")}); d != nil {
		t.Fatalf("未配置 diag 应 nil, got %+v", d)
	}
}

func TestSafeRetryReadOnlySuccessWritesKB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exceptions.jsonl")
	diag, _ := newDiagProvider(t, func(w http.ResponseWriter, r *http.Request) {
		okJSON(w, `{"category":"transient","root_cause":"瞬时抖动","confidence":0.7,"recoverable":true,"suggestion":"重试","action":"retry"}`)
	})

	called := 0
	runner := func(tool string, args map[string]any) contract.Receipt {
		called++
		return contract.Receipt{Tool: tool, OK: true, Stdout: "hits: 3"}
	}
	svc := NewService(OpenKB(path), diag, runner)
	att := Attempt{Tool: "search", Args: map[string]any{"pattern": "x"}, Receipt: contract.Receipt{Tool: "search", OK: false, Err: "dial tcp: i/o timeout"}}
	nr, d := svc.SafeRetry(context.Background(), "查 x", "QUERY", att)
	if nr == nil {
		t.Fatal("只读工具 retry 应重放成功并返回新回执")
	}
	if !nr.OK || called != 1 {
		t.Fatalf("runner 应被调用 1 次且成功: called=%d", called)
	}
	if d == nil || d.Action != ActionRetry {
		t.Fatalf("诊断结论应保留: %+v", d)
	}
	// 修复成功 → 回写知识库（指纹应落盘）。
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"fingerprint"`) {
		t.Fatalf("修复成功应回写 exceptions.jsonl: %s", data)
	}
}

// TestSafeRetryReplayFailureDoesNotWriteKB（负向）：重放后仍失败 → 绝不回写知识库，
// 避免把未验证的结论固化成"已知修复"。
func TestSafeRetryReplayFailureDoesNotWriteKB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exceptions.jsonl")
	diag, _ := newDiagProvider(t, func(w http.ResponseWriter, r *http.Request) {
		okJSON(w, `{"category":"transient","root_cause":"瞬时抖动","confidence":0.7,"recoverable":true,"suggestion":"重试","action":"retry"}`)
	})
	runner := func(tool string, args map[string]any) contract.Receipt {
		return contract.Receipt{Tool: tool, OK: false, Err: "still down"} // 重放仍失败
	}
	svc := NewService(OpenKB(path), diag, runner)
	att := Attempt{Tool: "search", Args: map[string]any{"pattern": "x"}, Receipt: contract.Receipt{Tool: "search", OK: false, Err: "dial tcp: i/o timeout"}}
	nr, d := svc.SafeRetry(context.Background(), "查 x", "QUERY", att)
	if nr != nil {
		t.Fatal("重放仍失败不应返回成功回执")
	}
	if d == nil || d.Action != ActionRetry {
		t.Fatalf("诊断结论应保留: %+v", d)
	}
	// KB 应为空（未修复成功不得回写）。
	if got := svc.KB.Count(); got != 0 {
		t.Fatalf("重放失败不得回写 KB, KB 条目数=%d", got)
	}
	if data, _ := os.ReadFile(path); strings.Contains(string(data), `"fingerprint"`) {
		t.Fatalf("exceptions.jsonl 不应含任何回写: %s", data)
	}
}

func TestSafeRetryNeverReplaysIrreversible(t *testing.T) {
	dir := t.TempDir()
	diag, _ := newDiagProvider(t, func(w http.ResponseWriter, r *http.Request) {
		// 模型即使说 retry，git commit 是不可逆工具也绝不自动重放。
		okJSON(w, `{"category":"transient","root_cause":"抖动","confidence":0.9,"recoverable":true,"suggestion":"重试","action":"retry"}`)
	})
	called := 0
	runner := func(tool string, args map[string]any) contract.Receipt {
		called++
		return contract.Receipt{Tool: tool, OK: true}
	}
	svc := NewService(OpenKB(filepath.Join(dir, "exceptions.jsonl")), diag, runner)

	att := Attempt{Tool: "git", Args: map[string]any{"args": []string{"commit"}}, Receipt: contract.Receipt{Tool: "git", OK: false, Err: "git commit: exit 1"}}
	nr, d := svc.SafeRetry(context.Background(), "提交", "COMMIT", att)
	if nr != nil {
		t.Fatal("git commit 失败绝不自动重放")
	}
	if called != 0 {
		t.Fatalf("不可逆工具 runner 不应被调用, got %d", called)
	}
	if d == nil || d.Action != ActionRetry {
		t.Fatal("诊断结论仍应供归因")
	}
}

func TestSafeRetryTwoRoundLimit(t *testing.T) {
	dir := t.TempDir()
	diag, _ := newDiagProvider(t, func(w http.ResponseWriter, r *http.Request) {
		okJSON(w, `{"category":"network","root_cause":"超时","confidence":0.9,"recoverable":true,"suggestion":"重试","action":"retry"}`)
	})
	called := 0
	runner := func(tool string, args map[string]any) contract.Receipt {
		called++
		return contract.Receipt{Tool: tool, OK: false, Err: "still failing"}
	}
	svc := NewService(OpenKB(filepath.Join(dir, "exceptions.jsonl")), diag, runner)
	att := Attempt{Tool: "search", Args: map[string]any{"pattern": "x"}, Receipt: contract.Receipt{Tool: "search", OK: false, Err: "dial tcp: i/o timeout"}}
	fp := Fingerprint("QUERY", "search", "dial tcp: i/o timeout")

	for i := 0; i < 4; i++ {
		svc.SafeRetry(context.Background(), "查 x", "QUERY", att)
	}
	if called != MaxAutoRetries {
		t.Fatalf("自动重放应恰好 %d 轮, got %d", MaxAutoRetries, called)
	}
	if svc.RetryCount(fp) != MaxAutoRetries {
		t.Fatalf("计数器应到上限, got %d", svc.RetryCount(fp))
	}
}

func TestRecoverableFalseForcesStop(t *testing.T) {
	dir := t.TempDir()
	diag, _ := newDiagProvider(t, func(w http.ResponseWriter, r *http.Request) {
		// action=retry 但 recoverable=false → 冲突，以 recoverable 为准转 stop。
		okJSON(w, `{"category":"auth","root_cause":"key 无效","confidence":0.99,"recoverable":false,"suggestion":"检查 key","action":"retry"}`)
	})
	called := 0
	runner := func(tool string, args map[string]any) contract.Receipt {
		called++
		return contract.Receipt{Tool: tool, OK: true}
	}
	svc := NewService(OpenKB(filepath.Join(dir, "exceptions.jsonl")), diag, runner)
	att := Attempt{Tool: "search", Args: map[string]any{"pattern": "x"}, Receipt: contract.Receipt{Tool: "llm", OK: false, Err: "401"}}
	nr, d := svc.SafeRetry(context.Background(), "t", "QUERY", att)
	if nr != nil || called != 0 {
		t.Fatal("recoverable=false 必须转 stop，绝不重放")
	}
	if d.Action != ActionStop {
		t.Fatalf("应归一为 stop, got %q", d.Action)
	}
}

// TestSafeRetryModifyReplaysReadOnlyWithFilteredParams：param→modify 正向重放。
// 只读工具首次失败 → 诊断 modify → 应用 retry_params 后重放成功；
// 断言业务键合入工具 args，而 harness 控制键（backoff/cooldown/max_retries）被过滤不注入。
func TestSafeRetryModifyReplaysReadOnlyWithFilteredParams(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exceptions.jsonl")
	diag, _ := newDiagProvider(t, func(w http.ResponseWriter, r *http.Request) {
		okJSON(w, `{"category":"param","root_cause":"pattern 拼写错误","confidence":0.9,"recoverable":true,"suggestion":"修正 pattern 后重试","action":"modify","retry_params":{"backoff_seconds":0.05,"cooldown_seconds":0.01,"max_retries":1,"pattern":"fixed-pattern"}}`)
	})
	var gotArgs map[string]any
	called := 0
	runner := func(tool string, args map[string]any) contract.Receipt {
		called++
		gotArgs = args
		return contract.Receipt{Tool: tool, OK: true, Stdout: "hits: 9"}
	}
	svc := NewService(OpenKB(path), diag, runner)
	att := Attempt{Tool: "search", Args: map[string]any{"pattern": "wrong*"}, Receipt: contract.Receipt{Tool: "search", OK: false, Err: "param invalid"}}
	nr, d := svc.SafeRetry(context.Background(), "查 x", "QUERY", att)

	// (a) 重放发生且成功。
	if nr == nil || !nr.OK || called != 1 {
		t.Fatalf("modify 应重放一次并成功: called=%d nr=%+v", called, nr)
	}
	if d == nil || d.Action != ActionModify || d.Category != CatParam {
		t.Fatalf("诊断结论应为 param/modify: %+v", d)
	}
	// (b) 业务键合入工具 args。
	if gotArgs["pattern"] != "fixed-pattern" {
		t.Fatalf("合入业务键 pattern 应为 fixed-pattern, got %v", gotArgs["pattern"])
	}
	// (c) harness 控制键未注入工具 args。
	for _, k := range []string{"backoff_seconds", "cooldown_seconds", "max_retries"} {
		if _, leaked := gotArgs[k]; leaked {
			t.Fatalf("控制键 %s 不应注入工具 args: %+v", k, gotArgs)
		}
	}
	// (d) 修复成功回写 KB。
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"fingerprint"`) {
		t.Fatalf("modify 修复成功应回写 KB: %s", data)
	}
}

// TestSafeRetryUnknownAskDoesNotReplay：unknown→ask 路由。
// runner 零调用（不重放）、无错误扩散，结论保留供归因。
func TestSafeRetryUnknownAskDoesNotReplay(t *testing.T) {
	dir := t.TempDir()
	diag, _ := newDiagProvider(t, func(w http.ResponseWriter, r *http.Request) {
		okJSON(w, `{"category":"unknown","root_cause":"无法判定","confidence":0.5,"recoverable":true,"suggestion":"人工介入","action":"ask"}`)
	})
	called := 0
	runner := func(tool string, args map[string]any) contract.Receipt {
		called++
		return contract.Receipt{Tool: tool, OK: true}
	}
	svc := NewService(OpenKB(filepath.Join(dir, "exceptions.jsonl")), diag, runner)
	att := Attempt{Tool: "search", Args: map[string]any{"pattern": "x"}, Receipt: contract.Receipt{Tool: "search", OK: false, Err: "weird"}}
	nr, d := svc.SafeRetry(context.Background(), "t", "QUERY", att)
	if nr != nil || called != 0 {
		t.Fatalf("action=ask 绝不重放, called=%d nr=%+v", called, nr)
	}
	if d == nil || d.Action != ActionAsk {
		t.Fatalf("结论应保留为 ask 供归因: %+v", d)
	}
}

func TestReadOnlyClassification(t *testing.T) {
	cases := []struct {
		tool string
		args map[string]any
		want bool
	}{
		{"search", nil, true},
		{"get_time", nil, true},
		{"verify", nil, true},
		{"file", map[string]any{"action": "read"}, true},
		{"file", map[string]any{"action": "exists"}, true},
		{"file", map[string]any{"action": "append"}, false}, // NOTE append 写
		{"file", map[string]any{"action": "write"}, false},  // 文件写
		{"git", map[string]any{"args": []string{"commit"}}, false},
		{"run", map[string]any{"command": []string{"make"}}, false},
		{"deploy", nil, false},
	}
	for _, c := range cases {
		if got := IsReadOnlyTool(c.tool, c.args); got != c.want {
			t.Errorf("IsReadOnlyTool(%s) = %v, want %v", c.tool, got, c.want)
		}
	}
}

func TestBudgetFriendlyAndParamSuggestion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exceptions.jsonl")
	// budget → 友好文案含「预算」。
	kb := OpenKB(path)
	kb.Remember(Diagnosis{Fingerprint: "fp-budget", Category: CatBudget, RootCause: "额度用尽", Suggestion: "今日额度耗尽", Action: ActionFallback, Recoverable: false})
	d, _ := kb.Lookup("fp-budget")
	if !strings.Contains(d.Suggestion, "预算") {
		t.Fatalf("budget 类建议应友好含「预算」: %q", d.Suggestion)
	}
}

func TestNormalizeActionWhitelist(t *testing.T) {
	// 越界 category/action 归一。
	d := Diagnosis{Category: "nonsense", Action: "explode", Recoverable: true}
	d.normalizeAction()
	if d.Category != CatUnknown || d.Action != ActionStop {
		t.Fatalf("越界应归一 unknown/stop: %+v", d)
	}
	// parseDiagnosis 缺字段报错。
	if _, err := parseDiagnosis(`{"root_cause":"x"}`); err == nil {
		t.Fatal("缺 category/action 应报错")
	}
}

func TestPrepareDiagKeyPreservesExplicit(t *testing.T) {
	cfg := config.Config{
		Providers: []config.Provider{
			{Name: "diag", Kind: config.OpenAIKind, Endpoint: "https://model.peterzou.com/v1", Model: "jev-diagnose", APIKey: "explicit-123"},
		},
	}
	PrepareDiagKey(&cfg)
	if cfg.Providers[0].APIKey != "explicit-123" {
		t.Fatal("显式 api_key 必须优先保留，不被覆盖")
	}
	// diag 未显式配 TimeoutMs → 默认 30000ms（避免干等 60s）。
	if cfg.Providers[0].TimeoutMs != DiagDefaultTimeoutMs {
		t.Fatalf("diag 未配 TimeoutMs 应默认 %dms, got %d", DiagDefaultTimeoutMs, cfg.Providers[0].TimeoutMs)
	}
	// 显式 TimeoutMs 不被覆盖。
	cfg3 := config.Config{Providers: []config.Provider{{Name: "diag", Kind: config.OpenAIKind, Endpoint: "https://x", Model: "jev-diagnose", APIKey: "k", TimeoutMs: 12345}}}
	PrepareDiagKey(&cfg3)
	if cfg3.Providers[0].TimeoutMs != 12345 {
		t.Fatalf("显式 TimeoutMs 应保留, got %d", cfg3.Providers[0].TimeoutMs)
	}
	// 未声明 diag → 零动作。
	cfg2 := config.Config{Providers: []config.Provider{{Name: "fast", Kind: config.OpenAIKind, Endpoint: "https://x", Model: "m"}}}
	PrepareDiagKey(&cfg2) // 不应 panic
	if len(cfg2.Providers) != 1 {
		t.Fatal("无 diag provider 应零动作")
	}
}
