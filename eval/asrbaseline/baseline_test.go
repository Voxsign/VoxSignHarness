//go:build asrharness

// 目标⑤：用 ASR 桩**反过来评测原 voice-sign harness** 的 ASR 容错。
//
// 分工说明（Peter 2026-10-03）：
//
//	DSH 出判据/语料/评测；voice-sign harness 出实现。
//
// 所以这个文件**不改 harness 的任何代码**——它只是把 harness **现有的**纠错能力
// （input/clean.go 的 Cleaner + memory/dictionary.go 的 Dictionary）包一层，
// 接到同一批真实语料上，把"现状"跑成数字与红灯。
//
// 它回答的问题很具体：
//
//	原 harness 那三块散落代码，面对**真实 ASR 失真**，到底兜住了多少？
//
// 运行：go test -tags asrharness ./eval/asrbaseline -v
package asrbaseline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"voicesign-harness/input"
)

type corpusCase struct {
	ID         string   `json:"id"`
	Raw        string   `json:"raw"`
	Provenance string   `json:"provenance"`
	Issue      string   `json:"issue"`
	Tags       []string `json:"tags"`
	Expect     struct {
		Corrected string `json:"corrected"`
		Fidelity  bool   `json:"fidelity"`
	} `json:"expect"`
}

func loadCorpus(t *testing.T) []corpusCase {
	t.Helper()
	// 单一权威源：asr/corpus/real-dialogue.jsonl
	// （曾同时存在 eval/asr-corpus 副本，已撤销——避免"这个版本那个版本"）
	for _, p := range []string{
		filepath.Join("..", "..", "asr", "corpus", "real-dialogue.jsonl"),
	} {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var out []corpusCase
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var c corpusCase
			if err := json.Unmarshal([]byte(line), &c); err != nil {
				t.Fatalf("语料 %s 非法行: %v", p, err)
			}
			out = append(out, c)
		}
		return out
	}
	t.Fatal("找不到语料 real-dialogue.jsonl")
	return nil
}

// punctNorm 把全角标点映射为半角（与 input/clean.go 的 fullwidthToHalf 同向）。
//
// 用途：把"只改了标点"与"改了正文"**分开计**。
// 二者性质完全不同：
//
//	· 只改标点 —— 是**产品定义问题**（中文正文里该不该归一化），取决于 Peter 的口径
//	· 改了正文 —— 是**实打实的过度纠正**，无争议
//
// 混在一起数会导致结论两头不讨好，所以本轮把它们拆成两个数字。
func punctNorm(s string) string {
	pairs := map[rune]rune{
		'，': ',', '。': '.', '：': ':', '；': ';', '！': '!', '？': '?',
		'（': '(', '）': ')', '、': ',', '“': '"', '”': '"', '‘': '\'', '’': '\'',
	}
	return strings.Map(func(r rune) rune {
		if q, ok := pairs[r]; ok {
			return q
		}
		return r
	}, s)
}

func hasTag(c corpusCase, tag string) bool {
	for _, t := range c.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// harnessCleaner 是**原 harness 现有纠错能力**的封装（只调用，不改动）。
type harnessCleaner struct{ c *input.Cleaner }

func newHarnessCleaner() harnessCleaner { return harnessCleaner{c: input.NewCleaner(nil)} }

func (h harnessCleaner) Correct(raw string) string { return h.c.Clean(raw) }

// ---------------------------------------------------------------------------
// 评测一：该纠的没纠（覆盖率）
//
// 语料里**明确写了期望纠正**的条目 —— 原 harness 兜住了几条？
// ---------------------------------------------------------------------------
func TestHarnessCorrectionCoverage(t *testing.T) {
	h := newHarnessCleaner()
	var total, fixed int
	var missed []string
	for _, c := range loadCorpus(t) {
		if c.Expect.Corrected == "" || c.Expect.Corrected == c.Raw {
			continue
		}
		total++
		got := h.Correct(c.Raw)
		if got == c.Expect.Corrected {
			fixed++
			continue
		}
		missed = append(missed, c.ID)
		t.Logf("未纠 [%s] %q\n     期望 %q\n     实际 %q\n     问题 %s",
			c.ID, c.Raw, c.Expect.Corrected, got, c.Issue)
	}
	t.Logf("原 harness 覆盖率：%d/%d 条（未纠：%v）", fixed, total, missed)
	if total == 0 {
		t.Fatal("语料里没有带期望纠正的条目 —— 评测没有对象")
	}
}

// ---------------------------------------------------------------------------
// 评测二：不该纠的纠了（安全侧）—— 与覆盖率同等重要
//
// 保真类条目上，原 harness **动了不该动的字**吗？
// 动一个字都可能改意思（尤其否定、条件）。
// ---------------------------------------------------------------------------
func TestHarnessOverCorrectionOnFidelity(t *testing.T) {
	h := newHarnessCleaner()
	var total int
	var over []string
	for _, c := range loadCorpus(t) {
		if !c.Expect.Fidelity {
			continue
		}
		total++
		if got := h.Correct(c.Raw); got != c.Raw {
			over = append(over, c.ID)
			t.Errorf("过度纠正 [%s]（%s）\n     原文 %q\n     改动 %q",
				c.ID, strings.Join(c.Tags, ","), c.Raw, got)
		}
	}
	t.Logf("保真类 %d 条，原 harness 改动其中 %d 条", total, len(over))
}

// ---------------------------------------------------------------------------
// 评测三：真实语料上的**逐条现状**（给"改动前"存档）
//
// 不判对错，只把现状打出来 —— 这是后续"改动后"要对比的基线。
// ---------------------------------------------------------------------------
func TestHarnessBaselineSnapshot(t *testing.T) {
	h := newHarnessCleaner()
	rows := loadCorpus(t)
	var changed, unchanged int
	for _, c := range rows {
		got := h.Correct(c.Raw)
		if got == c.Raw {
			unchanged++
			continue
		}
		changed++
	}
	t.Logf("原 harness 在 %d 条真实语料上：改动 %d 条 / 未动 %d 条", len(rows), changed, unchanged)

	// 分 provenance 看 —— 真实句与构造句的表现是否不同
	byProv := map[string][2]int{}
	for _, c := range rows {
		n := byProv[c.Provenance]
		if h.Correct(c.Raw) == c.Raw {
			n[1]++
		} else {
			n[0]++
		}
		byProv[c.Provenance] = n
	}
	for k, v := range byProv {
		t.Logf("  %s：改动 %d / 未动 %d", k, v[0], v[1])
	}
}

// ---------------------------------------------------------------------------
// 评测四：把"只改标点"与"改了正文"拆开计
//
// 这是为了让测量**不依赖产品口径**：不论中文标点归一化算不算问题，
// 两个数字都有各自的解释力。
// ---------------------------------------------------------------------------
func TestHarnessChangeBreakdown(t *testing.T) {
	h := newHarnessCleaner()
	var punctOnly, contentChange, total int
	var contentIDs []string
	for _, c := range loadCorpus(t) {
		if !c.Expect.Fidelity {
			continue
		}
		total++
		got := h.Correct(c.Raw)
		if got == c.Raw {
			continue
		}
		if punctNorm(got) == punctNorm(c.Raw) {
			punctOnly++ // 只有标点宽度差异
			continue
		}
		contentChange++ // 正文被改动 —— 无争议的过度纠正
		contentIDs = append(contentIDs, c.ID)
	}
	t.Logf("保真类 %d 条：未动 %d / **只改标点 %d** / **改了正文 %d**",
		total, total-punctOnly-contentChange, punctOnly, contentChange)
	if contentChange > 0 {
		t.Errorf("保真类里有 %d 条被改了正文（与标点口径无关，任何定义下都是过度纠正）：%v",
			contentChange, contentIDs)
	}
}
