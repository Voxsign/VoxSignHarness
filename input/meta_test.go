// 元指令仲裁的边界测试（缺口 G3）。
//
// G3 的真实危害：「开始测试」被判 TEST 0.85 且 test_kind="go test ./..." —— **真的去跑测试**。
// 但这句话在对话里说的是"进入测试阶段"，是对话控制，不是执行命令。
//
// 边界的关键是**长句豁免**：M7 真机教训「我想开始认真测一下，接下来把项目推进起来」
// 曾被系统追问，项目已把它钉成回归用例（pipeline.TestCodexNineRegressions 第 8 条，
// 断言 Ask 为空）。所以元指令只对**短句**生效 —— 长句里的「开始/推进」是叙述。
package input

import (
	"testing"

	"voicesign-harness/contract"
)

// TestMetaInstructionAsksForDisambiguation G3 主线：短句元指令要消歧，且不得判 TEST。
func TestMetaInstructionAsksForDisambiguation(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	for _, text := range []string{"开始测试", "继续测试", "开始跑测试", "继续部署"} {
		got := c.ClassifyTask(text)
		if got.Intent == contract.IntentTest {
			t.Errorf("G3: %q 被判 TEST —— 元指令被当作可执行命令（test_kind=%q）",
				text, got.Params["test_kind"])
		}
		if got.Intent != contract.IntentAsk {
			t.Errorf("%q: 意图 = %q，期望 ASK（歧义不猜）", text, got.Intent)
		}
		if got.Conflict != contract.ConflictMeta {
			t.Errorf("%q: conflict = %q，期望 %q", text, got.Conflict, contract.ConflictMeta)
		}
		if got.Ask == "" {
			t.Errorf("%q: 元指令消歧必须带非空 ask", text)
		}
	}
}

// TestMetaLongSentenceExempt 长句豁免：长句里的「开始/推进」是叙述，不是控制指令。
func TestMetaLongSentenceExempt(t *testing.T) {
	long := "我想开始认真测一下，接下来把项目推进起来"
	if len([]rune(long)) <= metaMaxRunes {
		t.Fatal("测试前提：该句应长于 metaMaxRunes")
	}
	c := NewTaskClassifier(0.6, nil)
	got := c.ClassifyTask(long)
	if got.Conflict == contract.ConflictMeta {
		t.Errorf("长句被误判为元指令：conflict=%q intent=%s", got.Conflict, got.Intent)
	}
	if got.Ask != "" {
		t.Errorf("长句元指令不得回问（M7 真机回归，TestCodexNineRegressions#8）：Ask=%q", got.Ask)
	}
}

// TestMetaRequiresActionWord 纯元指令无动作可误执行，不必多问一句。
func TestMetaRequiresActionWord(t *testing.T) {
	for _, text := range []string{"开始", "继续", "暂停"} {
		if _, ok := metaInstruction(text); ok {
			t.Errorf("%q 无动作词，不应判元指令", text)
		}
	}
	// 有动作词才算
	for _, text := range []string{"开始测试", "继续部署"} {
		if _, ok := metaInstruction(text); !ok {
			t.Errorf("%q 有动作词，应判元指令", text)
		}
	}
}

// TestMetaDoesNotBreakNormalCommands 正常指令不受影响。
func TestMetaDoesNotBreakNormalCommands(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	cases := []struct{ text, wantIntent string }{
		{"跑一下测试", contract.IntentTest},
		{"部署到服务器", contract.IntentDeploy},
		{"记一下这个想法", contract.IntentNote},
		{"提交这批改动", contract.IntentCommit},
	}
	for _, tc := range cases {
		got := c.ClassifyTask(tc.text)
		if got.Intent != tc.wantIntent {
			t.Errorf("%q: 意图 = %q，期望 %q（元指令仲裁误伤）", tc.text, got.Intent, tc.wantIntent)
		}
		if got.Conflict == contract.ConflictMeta {
			t.Errorf("%q 被误判为元指令", tc.text)
		}
	}
}

// TestNegationBeatsMeta 否定优先于元指令：「不要开始测试」应先判否定。
func TestNegationBeatsMeta(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	got := c.ClassifyTask("不要开始测试")
	if got.Conflict != contract.ConflictNegation {
		t.Errorf("conflict = %q，期望 %q（否定应优先于元指令）", got.Conflict, contract.ConflictNegation)
	}
}
