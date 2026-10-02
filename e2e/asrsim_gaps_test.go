//go:build asrsimgap

// 手机 App ASR 模拟 · 已知缺口套件（红 —— M8 驱动）。
//
// 用构建标签隔离，保证 `./build.sh`（内部跑 go test ./...）不受影响：
//
//	go test -tags asrsimgap ./e2e -run TestASRSimGap -v   # 缺口看板
//
// 这个文件里的用例断言的是**产品应该做到、当前没做到**的行为，所以现在是红的。
// 它们的存在本身就是测试产物：每条失败信息都指明"哪句话、应该怎样、现在怎样"。
package e2e

import (
	"strings"
	"testing"

	"voicesign-harness/contract"
	"voicesign-harness/input"
)

// TestASRSimGapNegationIgnored 缺口 G1：否定不被理解。
//
// 用户说"不要删除"，系统判成可执行的删除。今天之所以没删成，靠的是域门兜底
// （BOUNDARY_VIOLATION）和指代回问——不是意图层看懂了"不要"。
// 一旦目标可解析（词典命中/上下文唯一），这句就会真的删。
func TestASRSimGapNegationIgnored(t *testing.T) {
	c := input.NewTaskClassifier(0.6, nil)
	for _, text := range []string{"不要删除那个文件", "别删掉这条记录", "不用提交这批改动"} {
		got := c.ClassifyTask(text)
		if got.Intent == contract.IntentEdit && got.Params["action"] == "delete" {
			t.Errorf("GAP G1: %q 被判为可执行删除（intent=%s action=%s conf=%.2f）；"+
				"否定词未被识别，目标一旦可解析就会真的删",
				text, got.Intent, got.Params["action"], got.Confidence)
		}
	}
	// 端到端：否定句不得以"无回问 + 放行 + 已执行"收场
	opts := buildOpts(t, func(string, string) (bool, error) { return true, nil })
	out := simRun(t, opts, "不要删除那个文件")
	if out.Ask == "" && out.Confirmed {
		t.Errorf("GAP G1: 端到端 %q 未回问且已确认执行（intent=%s result=%q）",
			"不要删除那个文件", out.Intent.Intent, out.View.Result)
	}
}

// TestASRSimGapMissingSlotDoesNotAsk 缺口 G2：关键槽位缺失却不追问。
//
// 单类触发命中一律 0.85 置信，而默认阈值 0.6 —— askForKind 那条"低置信回问"分支
// 在默认配置下几乎不可达。于是"改一下""修"这种零信息的句子直接进执行通道。
func TestASRSimGapMissingSlotDoesNotAsk(t *testing.T) {
	c := input.NewTaskClassifier(0.6, nil)
	cases := []struct{ text, wantIntent string }{
		{"改一下", contract.IntentEdit},
		{"修", contract.IntentDebug},
		{"查", contract.IntentQuery},
	}
	for _, tc := range cases {
		got := c.ClassifyTask(tc.text)
		if got.Ask == "" {
			t.Errorf("GAP G2: %q 被判 %s conf=%.2f 但 Ask 为空（槽位全缺却不追问），已进入执行通道",
				tc.text, got.Intent, got.Confidence)
		}
	}
}

// TestASRSimGapMetaInstructionMisclassified 缺口 G3：元指令被当成可执行任务。
//
// "开始测试"是对话层面的元指令（开始这轮测试/进入测试阶段），
// 不是"跑 go test ./..."；现在判 TEST 0.85 并真的去执行。
func TestASRSimGapMetaInstructionMisclassified(t *testing.T) {
	c := input.NewTaskClassifier(0.6, nil)
	for _, text := range []string{"开始测试", "继续测试", "推进项目"} {
		got := c.ClassifyTask(text)
		if got.Intent == contract.IntentTest {
			t.Errorf("GAP G3: %q 被判 TEST conf=%.2f test_kind=%q —— 元指令被当作可执行命令",
				text, got.Confidence, got.Params["test_kind"])
		}
	}
}

// TestASRSimGapAskReasonOverwritten 缺口 G4：澄清原因被下游覆写。
//
// 分类器说"我不知道你想干嘛"（UNKNOWN），refer 层随后把 Ask 改写成
// "你说的「那个」指的是哪个？"——用户被问了一个错误的问题。
func TestASRSimGapAskReasonOverwritten(t *testing.T) {
	opts := simOpts(t)
	out := simRun(t, opts, "嗯那个呃记一下")
	if out.Intent.Intent == contract.IntentUnknown && out.Ask != "" {
		if !strings.Contains(out.Ask, "你是想让我做什么") {
			t.Errorf("GAP G4: UNKNOWN 的澄清原因被覆写：Ask=%q（期望保留"+
				"「你是想让我做什么」），用户会答非所问", out.Ask)
		}
	}
}

// TestASRSimGapMultiIntentSilentlyDropped 缺口 G5：复杂口述多动作被静默截断。
//
// contract.Intent 只能承载一个意图，且管线按"首个命中意图"执行；
// 其余动作既不执行、也不提示。实测"查一下库存，然后记一下结果，最后提交"
// 只落了 NOTE（真的写了 notes.md），查询和提交无声消失。
// 契约底线：一次只能做一个动作时，必须回问/拆分，而不是静默丢掉其余的。
func TestASRSimGapMultiIntentSilentlyDropped(t *testing.T) {
	opts := simOpts(t)
	cases := []struct {
		id      string
		text    string
		dropped []string
	}{
		{"cx-01", "查一下库存，然后记一下结果，最后提交", []string{"QUERY", "COMMIT"}},
		{"cx-02", "把报价模板改成新的公司抬头，然后跑一下测试", []string{"TEST"}},
		{"cx-03", "跑一下测试然后提交这批改动", []string{"COMMIT"}},
		{"cx-04", "把主栈改成中文然后部署到服务器", []string{"DEPLOY"}},
		{"cx-05", "记一下明天开会然后查一下上次的报价", []string{"QUERY"}},
	}
	for _, tc := range cases {
		out := simRun(t, opts, tc.text)
		if out.Ask != "" || out.Intent.NeedsClarification() {
			continue // 回问了就不算静默丢弃
		}
		t.Errorf("GAP G5: %s %q —— 系统只执行了 %s，静默丢弃 %v（result=%q）",
			tc.id, tc.text, out.Intent.Intent, tc.dropped, out.View.Result)
	}
}

// TestASRSimGapConditionalTreatedAsUnconditional 缺口 G6：条件句被无条件执行。
//
// "如果测试通过就提交"里有一条前置条件；系统判 TEST 0.85 直接跑，
// contract.Intent 里没有任何字段能承载条件。
func TestASRSimGapConditionalTreatedAsUnconditional(t *testing.T) {
	opts := simOpts(t)
	for _, text := range []string{"如果测试通过就提交", "测试过了就部署"} {
		out := simRun(t, opts, text)
		if out.Ask == "" && !out.Intent.NeedsClarification() {
			t.Errorf("GAP G6: %q 含前置条件却被无条件执行（intent=%s，无 Ask）",
				text, out.Intent.Intent)
		}
	}
}

// TestASRSimGapSelfCorrectionNotUnderstood 缺口 G7：口述自我修正不被理解。
//
// "记一下A，不对，改成记B"——"不对/改成"是修正信号，应当丢弃前一句。
// 现在 EDIT 把修正前的整段当成 object（object="记一下A,不对,"）。
func TestASRSimGapSelfCorrectionNotUnderstood(t *testing.T) {
	c := input.NewTaskClassifier(0.6, nil)
	got := c.ClassifyTask("记一下A，不对，改成记B")
	if got.Intent == contract.IntentEdit {
		obj := got.Params["object"]
		if strings.Contains(obj, "不对") || strings.Contains(obj, "记一下") {
			t.Errorf("GAP G7: 自我修正未被理解：object=%q 把修正前的内容吞进了目标槽位", obj)
		}
	}
}

// TestASRSimGapNoteContentAnaphoraNotAsk 缺口 G9：NOTE 句里的**内容指代**被当成操作指代。
//
// 发现于 LHT-0001 结算（step1）：`记一下：这次要修的是报价页那个错别字`
// → intent=NOTE，却触发了「你说的「那个」指的是哪个？」。
//
// "那个"在这里是**笔记内容的一部分**，不是要解析的操作对象 —— 笔记是自由文本，
// 系统没有理由追问它指的是哪个。追问会打断用户，且用户无法回答"哪个"（他就是这么说的）。
//
// 边界（既有回归 pipeline.TestCodexNineRegressions#7）：
// `记一下 这个` 的"这个"**就是全部内容**，此时追问是合理的 —— 必须保持 Ask。
func TestASRSimGapNoteContentAnaphoraNotAsk(t *testing.T) {
	opts := simOpts(t)
	out := simRun(t, opts, "记一下：这次要修的是报价页那个错别字")
	if out.Intent.Intent != contract.IntentNote {
		t.Skipf("意图 = %s，未走 NOTE 路径，本缺口不适用", out.Intent.Intent)
	}
	if strings.Contains(out.Ask, "指的是哪个") {
		t.Errorf("GAP G9: NOTE 句里的内容指代被当成操作指代：Ask=%q\n"+
			"「那个」是笔记内容的一部分，追问会打断用户且用户无法回答", out.Ask)
	}
}
