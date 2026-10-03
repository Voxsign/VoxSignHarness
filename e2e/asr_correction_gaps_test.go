//go:build asrcorr

// 缺口桩：ASR 识别错误的**自动校正**（长期基本能力）
//
// 用户 2026-10-03 定调：
// 「你要自动能够纠正 ASR 可能带来的识别错误，要把这个做一个**长期基本能力**做好。
//
//	否则是不行的，要有自动校正的能力——至少你这个 harness 本身也要这个能力。」
//
// 与 peterzou.com《语音到意图的高保真转化》一致：**以文本修复为主、声学特征为辅**。
//
// 本桩覆盖四类真实 ASR 失真，每类都问同一个问题：
// **当失真发生在"触发词本身"上时，harness 还能不能理解用户？**
//
// 已有能力（不是从零开始）：
//   - input/clean.go     清洗：填充词、全角半角
//   - memory/dictionary.go 个人词典：**精确变体**纠错（如 季总→冀总，需预置词条）
//   - input/punctuate.go 标点恢复
//
// 缺口在于：**词典没有的词、以及触发词自身的同音/近音失真**，目前无人管。
//
// 运行：go test -tags asrcorr ./e2e -run TestASRCorrection -v
package e2e

import (
	"strings"
	"testing"

	"voicesign-harness/contract"
	"voicesign-harness/input"
)

// asrCase 是一条 ASR 失真样例。
type asrCase struct {
	id   string
	text string // ASR 原始输出（未纠错）
	want string // 期望意图
	why  string // 失真类型
}

// TestASRCorrectionFillerNoise 第一类：填充词/口水音。
// 这类已有 clean.go 覆盖，作为**基线**——它绿说明桩本身没坏。
func TestASRCorrectionFillerNoise(t *testing.T) {
	c := input.NewTaskClassifier(0.6, nil)
	for _, tc := range []asrCase{
		{"fn-01", "嗯那个呃记一下这个想法", contract.IntentNote, "句首填充词"},
		{"fn-02", "那个，帮我查一下库存还有多少", contract.IntentQuery, "插入停顿词"},
	} {
		got := c.ClassifyTask(tc.text)
		if got.Intent != tc.want {
			t.Errorf("基础能力回退: [%s] %s (%q) → %s，期望 %s",
				tc.id, tc.why, tc.text, got.Intent, tc.want)
		}
	}
}

// TestASRCorrectionTriggerWordDistortion 第二类（**核心缺口**）：触发词自身失真。
//
// 这是最致命的一类：用户说了正确的话，但 ASR 把**承载意图的那个词**听错了，
// 于是系统判 UNKNOWN 或判错 —— 用户会觉得"它听不懂我"。
//
// 同音/近音失真在中文 ASR 里极常见（吓/下、出/初、抱/报）。
func TestASRCorrectionTriggerWordDistortion(t *testing.T) {
	c := input.NewTaskClassifier(0.6, nil)
	for _, tc := range []asrCase{
		{"tw-01", "记一吓这个想法", contract.IntentNote, "记一下→记一吓"},
		{"tw-02", "查一吓库存还有多少", contract.IntentQuery, "查一下→查一吓"},
		{"tw-03", "跑一吓测试", contract.IntentTest, "跑一下→跑一吓"},
		{"tw-04", "帮我把这个文件出交一下", contract.IntentCommit, "提交→出交"},
	} {
		got := c.ClassifyTask(tc.text)
		if got.Intent != tc.want {
			t.Errorf("GAP ASR-①: [%s] %s（%q）→ %s，期望 %s —— 触发词失真后 harness 不再理解用户",
				tc.id, tc.why, tc.text, got.Intent, tc.want)
		}
	}
}

// TestASRCorrectionPhoneticObject 第三类：**专有名词/对象名**同音失真（词典外）。
//
// 词典只收预置词条；没预置的专名一旦被听错，槽位就填错，
// 而槽位错了比意图错了更隐蔽 —— 系统会自信地去操作一个不存在的东西。
func TestASRCorrectionPhoneticObject(t *testing.T) {
	c := input.NewTaskClassifier(0.6, nil)
	// 注意：这里只断言"意图仍正确"。槽位能否还原成正确实体需要真实词典+上下文，
	// 留待下一步（见文末"下一批判据"）。
	for _, tc := range []asrCase{
		{"po-01", "把抱价单改成中文", contract.IntentEdit, "报价单→抱价单"},
		{"po-02", "查一下阔茨目录里有多少个文件", contract.IntentQuery, "quotes→阔茨"},
	} {
		got := c.ClassifyTask(tc.text)
		if got.Intent != tc.want {
			t.Errorf("GAP ASR-②: [%s] %s（%q）→ %s，期望 %s",
				tc.id, tc.why, tc.text, got.Intent, tc.want)
		}
	}
}

// TestASRCorrectionNeverGuessesUnderGarbage 第四类：**纯噪声必须回问，不得猜测**。
//
// ASR 在嘈杂/没说清时会输出无意义音节。此时唯一正确的行为是回问，
// 绝不能"脑补"一个意图去执行 —— 这正是 peterzou.com
// 《警惕 AI 的脑补：代指消解失败如何演变为系统性风险》说的级联污染源头。
func TestASRCorrectionNeverGuessesUnderGarbage(t *testing.T) {
	c := input.NewTaskClassifier(0.6, nil)
	for _, tc := range []asrCase{
		{"gr-01", "呃呃呃", contract.IntentUnknown, "纯口水音"},
		{"gr-02", "那个那个那个", contract.IntentUnknown, "重复指代词"},
		{"gr-03", "……", contract.IntentUnknown, "纯标点"},
	} {
		got := c.ClassifyTask(tc.text)
		if got.Intent == contract.IntentEdit || got.Intent == contract.IntentCommit ||
			got.Intent == contract.IntentDeploy {
			t.Errorf("GAP ASR-③: [%s] %s（%q）被判成可执行意图 %s —— 噪声不得被脑补",
				tc.id, tc.why, tc.text, got.Intent)
		}
		if got.Ask == "" {
			t.Errorf("GAP ASR-③: [%s] %q 未回问（Ask 为空）—— 听不懂必须说听不懂",
				tc.id, tc.text)
		}
	}
}

// TestASRCorrectionIsFirstClassStage 第五类：**校正必须是显式可观测的阶段**。
//
// 长期基本能力的要求：不能"顺手纠一点"，必须是一个**可单独观测、可单独评测**的阶段，
// 否则无法回答"它到底纠对了吗、纠错了多少"。
//
// 判据：清洗/校正后的文本必须能在产物里被读到（corrected_text），
// 且**原始 ASR 文本必须留底**（否则无法回溯是谁的错）。
func TestASRCorrectionIsFirstClassStage(t *testing.T) {
	opts := simOpts(t)
	out := simRun(t, opts, "嗯那个呃记一下这个想法")

	if strings.TrimSpace(out.Intent.CorrectedText) == "" {
		t.Error("GAP ASR-④: corrected_text 为空 —— 校正不是一个可观测的阶段")
	}
	// 原始文本必须留底（ASR 原文留底是既有设计约定）
	if out.Intent.RawText == "" {
		t.Error("GAP ASR-④: raw_text 未留底 —— 原始 ASR 输出丢失，无法事后归因是 ASR 的错还是系统的错")
	}
	t.Logf("raw=%q → corrected=%q → intent=%s",
		out.Intent.RawText, out.Intent.CorrectedText, out.Intent.Intent)
}
