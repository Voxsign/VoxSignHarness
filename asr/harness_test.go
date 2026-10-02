//go:build asrharness

// ASR 识别 harness —— C1–C4 判据 + 性能判据。
//
// **P1 阶段：本套桩跑在基线 Passthrough 上，应当大量是红的。**
// 红 = 能力还没做，不是"测试写错了"。判据先于实现（技能文档 §0）。
//
// 语料：asr/corpus/real-dialogue.jsonl —— **全部来自 Peter 与 DSH 的真实对话**，
// 每条带 source 出处。每轮真实对话往里加。
//
// 运行：go test -tags asrharness ./asr -v
package asr

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Case 是一条语料。字段与 jsonl 对应。
type Case struct {
	ID     string   `json:"id"`
	Raw    string   `json:"raw"`
	Issue  string   `json:"issue"`
	Source string   `json:"source"`
	Tags   []string `json:"tags"`
	Expect struct {
		Intent      string `json:"intent"`
		Corrected   string `json:"corrected"`
		Fidelity    bool   `json:"fidelity"`
		AskEmpty    bool   `json:"ask_empty"`
		AskNonempty bool   `json:"ask_nonempty"`
	} `json:"expect"`
}

func loadCorpus(t *testing.T) []Case {
	t.Helper()
	// 单一权威源：asr/corpus/real-dialogue.jsonl（eval/asr-corpus 副本已撤销）
	for _, p := range []string{
		filepath.Join("corpus", "real-dialogue.jsonl"),
	} {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var out []Case
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var c Case
			if err := json.Unmarshal([]byte(line), &c); err != nil {
				t.Fatalf("语料 %s 有非法行: %v", p, err)
			}
			out = append(out, c)
		}
		return out
	}
	t.Fatal("找不到语料 real-dialogue.jsonl —— 语料是这套桩的地基，不能缺")
	return nil
}

// engine 是本套桩的测试对象（**接线点**，不是判据本身）。
// P2 起接生产引擎；基线 Passthrough 单独在 TestBaselinePassthrough 里对照。
func engine() Engine { return Passthrough{} }

// hasTag 报告语料是否带某标签。
func hasTag(c Case, tag string) bool {
	for _, t := range c.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// C1 不该纠的不能纠 —— 保真（safety 侧）
//
// 反向的那一半：纠错**不得**把否定、条件、长句改掉。
// 基线 Passthrough 天然通过；实现阶段一旦过度纠正，这条会红。
// ---------------------------------------------------------------------------
func TestC1FidelityNeverOverCorrect(t *testing.T) {
	eng := engine()
	for _, c := range loadCorpus(t) {
		// 保真类：correction 必须原样等于 raw
		if c.Expect.Fidelity || hasTag(c, "negation") || hasTag(c, "safety") || hasTag(c, "long") {
			got := eng.Correct(CorrectRequest{Raw: c.Raw})
			if got.Text != c.Raw {
				t.Errorf("C1 保真被破坏 [%s] %q → %q\n    这是安全侧红线：纠错不得改动否定/条件/长句",
					c.ID, c.Raw, got.Text)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// C2 触发词同音容错 —— 该纠的必须纠
//
// 反向的那一半：多字触发词被听错后必须能还原意图。
// 基线 Passthrough 在这条上必然红。
// ---------------------------------------------------------------------------
func TestC2TriggerWordHomophone(t *testing.T) {
	eng := engine()
	for _, c := range loadCorpus(t) {
		if c.Expect.Corrected == "" || c.Expect.Corrected == c.Raw {
			continue
		}
		got := eng.Correct(CorrectRequest{Raw: c.Raw})
		if got.Text != c.Expect.Corrected {
			t.Errorf("C2 未纠正 [%s] %s\n    原文: %q\n    期望: %q\n    实际: %q\n    问题: %s",
				c.ID, c.Tags, c.Raw, c.Expect.Corrected, got.Text, c.Issue)
		}
	}
}

// ---------------------------------------------------------------------------
// C3 噪声不脑补 —— 纯噪声不得被纠成"看起来合理"的句子
//
// 双向：①噪声不得产出缩短/改写成命令的文本 ②非噪声不得被判为噪声。
// ---------------------------------------------------------------------------
func TestC3NoiseNeverHallucinated(t *testing.T) {
	eng := engine()
	for _, c := range loadCorpus(t) {
		if !hasTag(c, "noise") {
			continue
		}
		got := eng.Correct(CorrectRequest{Raw: c.Raw})
		// 噪声纠正后不得变得更"像命令"（这里用长度做代理：不得凭空变长）
		if len([]rune(got.Text)) > len([]rune(c.Raw)) {
			t.Errorf("C3 脑补 [%s] %q → %q（凭空变长 = 在猜）", c.ID, c.Raw, got.Text)
		}
		// 若引擎给出候选，说明它知道该问人；这不算红。
		// 但**不得**给出唯一且高置信的改法去执行。
		if got.Text != c.Raw && len(got.Candidates) == 0 {
			t.Errorf("C3 脑补 [%s] 噪声被唯一确定地改写为 %q，且无候选供人确认", c.ID, got.Text)
		}
	}
}

// ---------------------------------------------------------------------------
// C4 可观测 —— 每次纠错必须带证据，raw 必须留底
//
// 双向：①改了必须记录 ②没改不得凭空记录。
// ---------------------------------------------------------------------------
func TestC4CorrectionsAreObservable(t *testing.T) {
	eng := engine()
	for _, c := range loadCorpus(t) {
		got := eng.Correct(CorrectRequest{Raw: c.Raw})

		// 没改就不该有记录
		if got.Text == c.Raw && len(got.Corrections) > 0 {
			t.Errorf("C4 凭空记录 [%s] 文本未改却有 %d 条 Correction", c.ID, len(got.Corrections))
		}
		// 改了就必须有记录，且区间可回溯
		if got.Text != c.Raw {
			if len(got.Corrections) == 0 {
				t.Errorf("C4 不可观测 [%s] 改成了 %q 却没有 Correction 记录", c.ID, got.Text)
				continue
			}
			for _, cor := range got.Corrections {
				if cor.Start < 0 || cor.End > len(c.Raw) || cor.Start >= cor.End {
					t.Errorf("C4 区间非法 [%s] start=%d end=%d (raw len=%d)", c.ID, cor.Start, cor.End, len(c.Raw))
				}
				if cor.Evidence == "" || cor.Kind == "" {
					t.Errorf("C4 缺证据 [%s] kind=%q evidence=%q", c.ID, cor.Kind, cor.Evidence)
				}
			}
		}
		// 期望有纠正的条目，必须真的有纠正记录
		if c.Expect.Corrected != "" && c.Expect.Corrected != c.Raw && len(got.Corrections) == 0 {
			t.Errorf("C4 未记录 [%s] 期望纠正为 %q 但一条 Correction 都没有", c.ID, c.Expect.Corrected)
		}
	}
}

// ---------------------------------------------------------------------------
// 性能判据 —— "快"必须可测
//
// 基线 Passthrough 会通过；但实现阶段一旦引入暴力全表扫描，这条会红。
// ---------------------------------------------------------------------------
func TestPerfFastPathLatency(t *testing.T) {
	eng := engine()
	raw := strings.Repeat("把报价单改成中文", 6) // 约 48 字
	if n := len([]rune(raw)); n < 40 {
		t.Fatalf("测试前提：样本应为 50 字量级，实际 %d 字", n)
	}

	const N = 2000
	start := time.Now()
	for i := 0; i < N; i++ {
		eng.Correct(CorrectRequest{Raw: raw})
	}
	avg := time.Since(start) / N

	// 目标：远低于 1ms（快路必须是微秒级）
	if avg > time.Millisecond {
		t.Errorf("快路过慢：平均 %v/次（目标 < 1ms，实为快路应达微秒级）", avg)
	}
	t.Logf("快路平均延迟 %v/次（%d 次）", avg, N)
}

// TestPerfOfflineCapable 断网可跑：快路不得依赖外部调用。
//
// 判据形式：在无网络配置的零值 Engine 上必须仍能 Correct（不 panic、不阻塞）。
func TestPerfOfflineCapable(t *testing.T) {
	eng := engine()
	done := make(chan struct{})
	go func() {
		defer close(done)
		eng.Correct(CorrectRequest{Raw: "查一下库存"})
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("快路阻塞超过 2s —— 疑似依赖了外部调用（违背「断网可跑」）")
	}
}

// ---------------------------------------------------------------------------
// 自比对：把基线（零纠正）跑一遍，作为「改动前」那条臂。
//
// 这个测试**不断言通过**——它只把基线的得失打印出来，
// 与 engine() 的结果并排，构成「自己和自己比」。
// ---------------------------------------------------------------------------
func TestBaselinePassthroughComparison(t *testing.T) {
	base := Passthrough{}
	prod := engine()
	var baseFixed, prodFixed, total int
	for _, c := range loadCorpus(t) {
		if c.Expect.Corrected == "" || c.Expect.Corrected == c.Raw {
			continue
		}
		total++
		if base.Correct(CorrectRequest{Raw: c.Raw}).Text == c.Expect.Corrected {
			baseFixed++
		}
		if prod.Correct(CorrectRequest{Raw: c.Raw}).Text == c.Expect.Corrected {
			prodFixed++
		}
	}
	t.Logf("自比对（%d 条期望纠正）：改动前 %d 条 / 改动后 %d 条", total, baseFixed, prodFixed)
	if prodFixed <= baseFixed {
		t.Errorf("改动后没有提升：基线 %d 条，现状 %d 条", baseFixed, prodFixed)
	}
}

// ---------------------------------------------------------------------------
// C3-bis 噪声必须发出"该问人"的信号（补判据空洞）
//
// 异源盲评指出：原 C3 只测"不变长 + 有候选"，而 Passthrough（零纠正基线）
// 也能 100% 通过 —— 那条绿对实现质量**零信息量**（技能文档 §9：无样本就报绿）。
// 且 Case 里的 AskNonempty/AskEmpty/Intent 三个字段**解析了却从无断言**。
//
// 本测试补上断言。按 asr.go 的约定：**Candidates 非空 = 该问人/该问外部**。
// 于是"噪声必须回问"在 ASR 层的可观测形式就是：噪声样本必须给出非空 Candidates。
//
// 判据先于实现：本测试在实现方给出信号前应当是**红的**。
// ---------------------------------------------------------------------------
func TestC3BisNoiseMustSignalAsk(t *testing.T) {
	eng := engine()
	var checked int
	for _, c := range loadCorpus(t) {
		if !c.Expect.AskNonempty {
			continue
		}
		checked++
		got := eng.Correct(CorrectRequest{Raw: c.Raw})
		// 噪声不得被改写成"看起来合理"的文本
		if got.Text != c.Raw {
			t.Errorf("C3-bis [%s] 噪声被改写：%q → %q", c.ID, c.Raw, got.Text)
		}
		// 必须发出"该问人"的信号 —— 否则上层无从知道这是噪声。
		// 约定（实现方 2026-10-03 冻结）：噪声返回**恰好一条**候选，
		// Text == Raw（不给改写建议）、Confidence == 0（没有可信改写）、
		// Reason 以 "ask:noise" 开头（稳定机器可判前缀）。
		if len(got.Candidates) == 0 {
			t.Errorf("C3-bis [%s] 噪声 %q 既未改写**也未给出候选** —— "+
				"上层拿不到任何「这是噪声、该回问」的信号（判据空洞，Passthrough 也能过）",
				c.ID, c.Raw)
			continue
		}
		cand := got.Candidates[0]
		if cand.Text != c.Raw {
			t.Errorf("C3-bis [%s] 噪声候选不得给出改写建议：Text=%q 应等于原文 %q", c.ID, cand.Text, c.Raw)
		}
		if cand.Confidence != 0 {
			t.Errorf("C3-bis [%s] 噪声候选置信度应为 0（无可信改写），实际 %v", c.ID, cand.Confidence)
		}
		if !strings.HasPrefix(cand.Reason, "ask:noise") {
			t.Errorf("C3-bis [%s] 噪声候选 Reason 应以 ask:noise 开头（稳定机器可判前缀），实际 %q",
				c.ID, cand.Reason)
		}
	}
	if checked == 0 {
		t.Fatal("语料里没有 AskNonempty 的条目 —— 该判据无样本，等于没测")
	}
	t.Logf("C3-bis 覆盖 %d 条噪声样本", checked)

	// ---- 反向：有实义的句子**不得**给出候选（否则就是无谓地打扰用户）----
	var reverse int
	for _, c := range loadCorpus(t) {
		if !c.Expect.AskEmpty {
			continue
		}
		reverse++
		if got := eng.Correct(CorrectRequest{Raw: c.Raw}); len(got.Candidates) != 0 {
			t.Errorf("C3-bis 反向 [%s] 有实义的句子 %q 不该给出候选，实际 %d 条",
				c.ID, c.Raw, len(got.Candidates))
		}
	}
	if reverse == 0 {
		t.Error("C3-bis 反向无样本 —— 只测「该问的问了」，没测「不该问的没问」，是单向判据")
	}
	t.Logf("C3-bis 反向覆盖 %d 条 ask_empty 样本", reverse)
}

// TestCaseExpectFieldsAllExercised 防止"字段解析了却没人用"再次发生。
func TestCaseExpectFieldsAllExercised(t *testing.T) {
	// 这三个字段在 Case 结构里存在，必须至少各有一条语料在用，
	// 否则说明判据材料里有"死字段"——被解析但不参与判定。
	var nIntent, nAskEmpty, nAskNonempty, nCorrected, nFidelity int
	for _, c := range loadCorpus(t) {
		if c.Expect.Intent != "" {
			nIntent++
		}
		if c.Expect.AskEmpty {
			nAskEmpty++
		}
		if c.Expect.AskNonempty {
			nAskNonempty++
		}
		if c.Expect.Corrected != "" {
			nCorrected++
		}
		if c.Expect.Fidelity {
			nFidelity++
		}
	}
	t.Logf("期望字段使用情况：intent=%d ask_empty=%d ask_nonempty=%d corrected=%d fidelity=%d",
		nIntent, nAskEmpty, nAskNonempty, nCorrected, nFidelity)
	for name, n := range map[string]int{"intent": nIntent, "ask_empty": nAskEmpty, "ask_nonempty": nAskNonempty} {
		if n == 0 {
			t.Errorf("字段 expect.%s 无任何语料使用 —— 死字段（解析了但不参与判定）", name)
		}
	}
}
