package bench

// Issue #30 Phase 0 baseline runner.
//
// Usage (real model center):
//
//	export PATH="/opt/homebrew/bin:$PATH"
//	source <(grep -E 'export (AIOPS_KEY|STRATA_API_KEY|VHS_API_KEY)=' ~/.zshrc)
//	go test -count=3 ./bench/ -run TestBaseline -v -timeout 90m
//
// Each -count run gets a fresh server + temp logDir. The runner drives the real
// HTTP/SSE surface (POST /v1/tasks -> GET /v1/tasks/{id}/events), records
// N1 (first visible event), N2 (done/failed latency per class), N3 (first-pass
// success), N4 (clarification rate), N5 (empty/false receipts), plus the
// additive timing payload (stage breakdown + LLM calls) for top-cost stages.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"voicesign-harness/cache"
	"voicesign-harness/config"
	"voicesign-harness/memory"
	"voicesign-harness/pipeline"
	"voicesign-harness/provider"
	"voicesign-harness/refer"
	"voicesign-harness/server"
	"voicesign-harness/space"
	"voicesign-harness/tools"
	"voicesign-harness/trajectory"
	"voicesign-harness/verify"
)

type benchTask struct {
	ID        string   `json:"id"`
	Class     string   `json:"class"`
	Utterance string   `json:"utterance"`
	Tags      []string `json:"tags"`
}

func loadBenchTasks(t *testing.T) []benchTask {
	t.Helper()
	b, err := os.ReadFile("tasks.json")
	if err != nil {
		b, err = os.ReadFile(filepath.Join("..", "bench", "tasks.json"))
	}
	if err != nil {
		t.Fatalf("read tasks.json: %v", err)
	}
	var set struct {
		Tasks []benchTask `json:"tasks"`
	}
	if err := json.Unmarshal(b, &set); err != nil {
		t.Fatalf("parse tasks.json: %v", err)
	}
	return set.Tasks
}

type runResult struct {
	TaskID, Class string
	FirstEventMs  int64
	TerminalMs    int64
	TerminalEvent string // done | failed | ask | canceled | timeout
	BizState      string
	Ask           string
	Receipt       string
	Timing        *pipeline.TimingInfo
	RawEvents     []string // event types seen, in order (for M7/M8 symptom judgement)
	Err           string
}

func (r runResult) done() bool { return r.TerminalEvent == "done" }
func (r runResult) asked() bool {
	if r.TerminalEvent == "ask" || r.Ask != "" {
		return true
	}
	// Clarification may land inside a done receipt instead of an ask event
	// (M7 current form on main) — count it as a clarification too (baseline
	// honesty).
	rc := strings.ToLower(r.Receipt)
	return strings.Contains(rc, "needs clarification") ||
		strings.Contains(rc, "need clarification") ||
		strings.Contains(rc, "not sure what you meant") ||
		strings.Contains(rc, "待澄清") ||
		strings.Contains(rc, "没太确定") ||
		strings.Contains(rc, "我没太确定")
}

// firstPass (N3): done, has a real executed result, no clarification, no FAILED.
// Receipts that carry FAILED / 未执行 / 待澄清 signals are NOT first-pass even
// though the event type is done.
func (r runResult) firstPass() bool {
	if !r.done() || r.asked() {
		return false
	}
	rc := strings.ToLower(r.Receipt)
	if strings.Contains(rc, "not executed") || strings.Contains(rc, "need clarification") ||
		strings.Contains(rc, "pending confirm") ||
		strings.Contains(rc, "failed") || strings.Contains(rc, "失败") ||
		strings.Contains(rc, "未解析到") || strings.Contains(rc, "未识别") ||
		strings.Contains(rc, "未执行") || strings.Contains(rc, "待澄清") {
		return false
	}
	return strings.TrimSpace(r.Receipt) != ""
}

// fakeDone (N5): done event whose receipt shows nothing was actually done —
// empty shell, a bare OK, or an explicit FAILED / 待澄清 / 未执行 receipt.
// Failures reported as done count as dishonest done.
func (r runResult) fakeDone() bool {
	if !r.done() {
		return false
	}
	trimmed := strings.TrimSpace(r.Receipt)
	rc := strings.ToLower(r.Receipt)
	return trimmed == "" || trimmed == "OK（自动执行）" ||
		strings.Contains(rc, "failed") || strings.Contains(rc, "失败") ||
		strings.Contains(rc, "未解析到") || strings.Contains(rc, "未识别") ||
		strings.Contains(rc, "未执行") || strings.Contains(rc, "待澄清")
}

func buildServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	logDir := t.TempDir()
	// config.Load injects per-provider API keys from VHS_API_KEY / AIOPS_KEY env
	// (config.Default alone does not). Fall back to Default + manual injection so
	// the benchmark also runs in a no-key sandbox.
	cfg, err := config.Load("")
	if err != nil {
		cfg = config.Default()
		for i := range cfg.Providers {
			if cfg.Providers[i].APIKey == "" {
				cfg.Providers[i].APIKey = os.Getenv("AIOPS_KEY")
			}
		}
	}
	cfg.Global.LogDir = logDir
	cfg.Server.Token = "bench-token"
	cfg.Memory.Dir = filepath.Join(logDir, "mem")
	cfg.Spaces.Dir = filepath.Join(logDir, "spaces")
	cfg.Contracts.Dir = filepath.Join(logDir, "contracts")
	cfg.Cache.Dir = filepath.Join(logDir, "cache")
	for _, d := range []string{cfg.Memory.Dir, cfg.Spaces.Dir, cfg.Contracts.Dir, cfg.Cache.Dir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	reg, err := provider.NewRegistry(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	dict, _ := memory.LoadDictionary(filepath.Join(cfg.Memory.Dir, "dictionary.json"))
	spaces, _ := space.Load(cfg.Spaces.Dir)
	treg, _ := tools.LoadContracts(cfg.Contracts.Dir)
	tr, _ := trajectory.Open(cfg.Global.LogDir)
	st, _ := cache.Open(filepath.Join(cfg.Cache.Dir, "quad.json"), time.Hour)
	opts := &pipeline.Options{
		Cfg:       &cfg,
		Dict:      dict,
		Spaces:    spaces,
		Refer:     refer.New(dict),
		Cache:     st,
		Providers: reg,
		Tools:     treg,
		Exec:      &tools.Executor{BaseDir: logDir},
		Verifier:  &verify.Verifier{BaseDir: logDir},
		ConfirmFn: func(string, string) (bool, error) { return true, nil },
		Trace:     tr,
	}
	srv := server.New(&cfg, opts)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, "bench-token"
}

func postTask(ts *httptest.Server, token, text string) (string, error) {
	body, _ := json.Marshal(map[string]string{"text": text})
	req, _ := http.NewRequest("POST", ts.URL+"/v1/tasks", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return "", fmt.Errorf("POST status %d: %s", resp.StatusCode, string(b))
	}
	var out struct {
		TaskID string `json:"task_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.TaskID, nil
}

func streamEvents(ts *httptest.Server, token, taskID string, postedAt time.Time, perTaskTimeout time.Duration) runResult {
	res := runResult{}
	deadline := time.Now().Add(perTaskTimeout)
	after := 0
	seenTerminal := false
	for !seenTerminal && time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Until(deadline))
		req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/v1/tasks/"+taskID+"/events", nil)
		q := req.URL.Query()
		if after > 0 {
			q.Set("after", fmt.Sprintf("%d", after))
		}
		req.URL.RawQuery = q.Encode()
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			res.Err = "events GET: " + err.Error()
			cancel()
			break
		}
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024)
		var dataBuf strings.Builder
		first := true
		for sc.Scan() {
			line := sc.Text()
			if strings.HasPrefix(line, "data:") {
				dataBuf.WriteString(strings.TrimPrefix(line, "data:"))
				continue
			}
			if line == "" {
				payload := strings.TrimSpace(dataBuf.String())
				dataBuf.Reset()
				if payload == "" {
					continue
				}
				var ev map[string]any
				if err := json.Unmarshal([]byte(payload), &ev); err != nil {
					continue
				}
				typ, _ := ev["type"].(string)
				seq, _ := ev["seq"].(float64)
				now := time.Since(postedAt).Milliseconds()
				if first && after == 0 {
					res.FirstEventMs = now
					first = false
				}
				res.RawEvents = append(res.RawEvents, typ)
				if int(seq) > after {
					after = int(seq)
				}
				switch typ {
				case "done", "failed", "canceled":
					seenTerminal = true
					res.TerminalEvent = typ
					res.TerminalMs = now
					res.BizState, _ = ev["biz_state"].(string)
					res.Receipt, _ = ev["receipt"].(string)
					if a, ok := ev["ask"].(string); ok && a != "" {
						res.Ask = a
					}
					if tm, ok := ev["timing"].(map[string]any); ok {
						tj, _ := json.Marshal(tm)
						var ti pipeline.TimingInfo
						if err := json.Unmarshal(tj, &ti); err == nil {
							res.Timing = &ti
						}
					}
				case "ask":
					if res.TerminalEvent == "" {
						res.TerminalEvent = "ask"
						res.TerminalMs = now
					}
					if a, ok := ev["ask"].(string); ok && a != "" {
						res.Ask = a
					}
				}
			}
		}
		resp.Body.Close()
		cancel()
		if !seenTerminal && sc.Err() != nil {
			res.Err = "scan: " + sc.Err().Error()
		}
	}
	if res.TerminalEvent == "" {
		res.TerminalEvent = "timeout"
		res.TerminalMs = time.Since(postedAt).Milliseconds()
	}
	return res
}

func TestBaseline(t *testing.T) {
	if os.Getenv("AIOPS_KEY") == "" {
		t.Skip("baseline requires AIOPS_KEY (real model center); skipped in sandbox/CI")
	}
	tasks := loadBenchTasks(t)
	if len(tasks) != 40 {
		t.Fatalf("expected 40 tasks, got %d", len(tasks))
	}
	ts, token := buildServer(t)
	results := make([]runResult, 0, len(tasks))
	for _, tk := range tasks {
		postedAt := time.Now()
		taskID, err := postTask(ts, token, tk.Utterance)
		if err != nil {
			t.Logf("task %s POST failed: %v", tk.ID, err)
			results = append(results, runResult{TaskID: tk.ID, Class: tk.Class, TerminalEvent: "post-error", Err: err.Error(), TerminalMs: time.Since(postedAt).Milliseconds()})
			continue
		}
		res := streamEvents(ts, token, taskID, postedAt, 90*time.Second)
		res.TaskID = tk.ID
		res.Class = tk.Class
		results = append(results, res)
		t.Logf("task %-6s %-9s %-10s first=%dms terminal=%dms ask=%q", tk.ID, tk.Class, res.TerminalEvent, res.FirstEventMs, res.TerminalMs, truncate(res.Ask, 40))
	}

	// ---- aggregate report ----
	var b strings.Builder
	b.WriteString("=== Issue #30 Phase 0 baseline (run " + time.Now().Format("15:04:05") + ") ===\n")
	// N1: first visible feedback (first event) + first meaningful (terminal/ask) event
	var n1, n1m []int64
	firstTypes := map[string]int{}
	for _, r := range results {
		if len(r.RawEvents) > 0 {
			firstTypes[r.RawEvents[0]]++
		}
		if r.FirstEventMs > 0 {
			n1 = append(n1, r.FirstEventMs)
		}
		if r.TerminalMs > 0 {
			n1m = append(n1m, r.TerminalMs)
		}
	}
	sort.Slice(n1, func(i, j int) bool { return n1[i] < n1[j] })
	sort.Slice(n1m, func(i, j int) bool { return n1m[i] < n1m[j] })
	b.WriteString(fmt.Sprintf("N1 first-event-ms: n=%d p50=%d p90=%d (first event types: %v)\n", len(n1), pct(n1, 50), pct(n1, 90), firstTypes))
	b.WriteString(fmt.Sprintf("N1b first-terminal/ask-ms: n=%d p50=%d p90=%d\n", len(n1m), pct(n1m, 50), pct(n1m, 90)))

	// N2: per-class done/failed terminal latency
	for _, cls := range []string{"qa", "simple", "complex", "ambiguous"} {
		var d []int64
		for _, r := range results {
			if r.Class == cls && r.TerminalMs > 0 {
				d = append(d, r.TerminalMs)
			}
		}
		sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
		b.WriteString(fmt.Sprintf("N2 %-9s terminal-ms: n=%d p50=%d p90=%d\n", cls, len(d), pct(d, 50), pct(d, 90)))
	}

	// N3: first-pass success (evidence-backed done, no clarify, no retry)
	doneCount, fp := 0, 0
	for _, r := range results {
		if r.done() {
			doneCount++
		}
		if r.firstPass() {
			fp++
		}
	}
	b.WriteString(fmt.Sprintf("N3 first-pass: %d/%d (%.0f%%) done=%d\n", fp, len(results), 100*float64(fp)/float64(len(results)), doneCount))

	// N4: clarification rate among non-ambiguous tasks
	nonAmb, askedNonAmb := 0, 0
	for _, r := range results {
		if r.Class == "ambiguous" {
			continue
		}
		nonAmb++
		if r.asked() {
			askedNonAmb++
		}
	}
	b.WriteString(fmt.Sprintf("N4 clarify-rate (non-ambiguous): %d/%d (%.0f%%)\n", askedNonAmb, nonAmb, 100*float64(askedNonAmb)/float64(nonAmb)))

	// N5: dishonest done (empty shell / bare OK / FAILED-in-done receipts)
	emptyN := 0
	for _, r := range results {
		if r.fakeDone() {
			emptyN++
		}
	}
	b.WriteString(fmt.Sprintf("N5 dishonest done receipts: %d\n", emptyN))

	// top-cost stages across all tasks with timing payloads
	stageSum := map[string]int64{}
	stageN := map[string]int{}
	llmSum := map[string]int64{}
	llmN := map[string]int{}
	for _, r := range results {
		if r.Timing == nil {
			continue
		}
		for _, s := range r.Timing.Stages {
			stageSum[s.Name] += s.Ms
			stageN[s.Name]++
		}
		for _, c := range r.Timing.LLMCalls {
			llmSum[c.Provider] += c.Ms
			llmN[c.Provider]++
		}
	}
	type sc struct {
		name string
		sum  int64
		n    int
	}
	var stages []sc
	for k, v := range stageSum {
		stages = append(stages, sc{k, v, stageN[k]})
	}
	sort.Slice(stages, func(i, j int) bool { return stages[i].sum > stages[j].sum })
	b.WriteString("top-cost stages (sum ms over timed tasks):\n")
	for i, s := range stages {
		if i >= 5 {
			break
		}
		b.WriteString(fmt.Sprintf("  %-12s sum=%dms n=%d\n", s.name, s.sum, s.n))
	}
	var llms []sc
	for k, v := range llmSum {
		llms = append(llms, sc{k, v, llmN[k]})
	}
	sort.Slice(llms, func(i, j int) bool { return llms[i].sum > llms[j].sum })
	b.WriteString("LLM calls by provider (sum ms / count / avg):\n")
	for _, l := range llms {
		avg := int64(0)
		if l.n > 0 {
			avg = l.sum / int64(l.n)
		}
		b.WriteString(fmt.Sprintf("  %-16s sum=%dms n=%d avg=%dms\n", l.name, l.sum, l.n, avg))
	}

	// M7/M8 symptom rows
	b.WriteString("M7/M8 symptom per tagged task (terminal event + ask + first event ms):\n")
	for _, tk := range tasks {
		for _, r := range results {
			if r.TaskID != tk.ID {
				continue
			}
			tags := strings.Join(tk.Tags, ",")
			b.WriteString(fmt.Sprintf("  %-6s [%s] %-10s first=%dms ask=%q\n", tk.ID, tags, r.TerminalEvent, r.FirstEventMs, truncate(r.Ask, 30)))
		}
	}
	t.Log("\n" + b.String())
}

func pct(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := (len(sorted) - 1) * p / 100
	return sorted[idx]
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
