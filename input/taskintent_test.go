package input

import (
	"testing"
	"time"

	"voicesign-harness/contract"
)

// testSpaces 模拟 pipeline 注入的注册空间候选（VoxBuyBot 沙特女装项目 + 医疗耗材项目）。
var testSpaces = []SpaceHint{
	{Name: "voxbuybot", Aliases: []string{"VoxBuyBot", "女装项目", "voxbuy"}},
	{Name: "medsupply", Aliases: []string{"医疗耗材", "耗材"}},
}

func TestTaskClassifyNoteWithTimeAnchor(t *testing.T) {
	// SPEC 验收用例 1：NOTE + 时间锚点
	c := NewTaskClassifier(0.6, testSpaces)
	got := c.ClassifyTask("记一下冀总那个厂房下周一出报价")
	if got.Intent != contract.IntentNote {
		t.Fatalf("意图 = %q, 期望 NOTE", got.Intent)
	}
	if got.Ask != "" {
		t.Fatalf("NOTE 不应回问: %q", got.Ask)
	}
	if got.Confidence < 0.7 {
		t.Fatalf("置信度 = %.2f, 期望 >= 0.7", got.Confidence)
	}
	if got.Params["time_hint"] != "下周一" {
		t.Fatalf("time_hint = %q, 期望 下周一", got.Params["time_hint"])
	}
	if got.Params["time_date"] == "" {
		t.Fatal("time_date 应为解析后的具体日期")
	}
}

func TestTaskClassifyEditWithSpace(t *testing.T) {
	// SPEC v1 §2 示例：在 VoxBuyBot 里把错误提示改中文
	c := NewTaskClassifier(0.6, testSpaces)
	got := c.ClassifyTask("在 voxbuybot 里把错误提示改成中文")
	if got.Intent != contract.IntentEdit {
		t.Fatalf("意图 = %q, 期望 EDIT", got.Intent)
	}
	if got.Space != "voxbuybot" {
		t.Fatalf("空间 = %q, 期望 voxbuybot", got.Space)
	}
	if got.Target == nil || got.Target.Entity != "voxbuybot" || got.Target.RefType != "explicit" {
		t.Fatalf("Target 未正确填充: %+v", got.Target)
	}
	if got.Params["action"] != "replace" || got.Params["object"] != "错误提示" || got.Params["value"] != "中文" {
		t.Fatalf("EDIT 槽位不正确: %+v", got.Params)
	}
	if got.Confirm != contract.ConfirmLight {
		t.Fatalf("EDIT 基线确认应为 light, 实际 %q", got.Confirm)
	}
}

func TestTaskClassifyConflicts(t *testing.T) {
	c := NewTaskClassifier(0.6, testSpaces)
	cases := []struct {
		text   string
		want   string
		conf   string // 期望 conflict 标记
		params map[string]string
	}{
		{"发个想法", contract.IntentNote, contract.ConflictNoteVsDeploy, nil},                               // 想法压制部署
		{"查一下能不能跑测试", contract.IntentAsk, contract.ConflictAskVsOp, nil},                                // 可行性问句
		{"删掉那条记录", contract.IntentEdit, contract.ConflictDelete, map[string]string{"action": "delete"}}, // 删除动词
		{"修一下这个 bug 的思路", contract.IntentAsk, contract.ConflictDebugPlan, nil},                          // 修 bug 的思路
		{"那个文件改好了吗", contract.IntentQuery, "", nil},                                                     // 状态问句 → QUERY
	}
	for _, tc := range cases {
		got := c.ClassifyTask(tc.text)
		if got.Intent != tc.want {
			t.Errorf("%q: 意图 = %q, 期望 %q", tc.text, got.Intent, tc.want)
		}
		if got.Conflict != tc.conf {
			t.Errorf("%q: conflict = %q, 期望 %q", tc.text, got.Conflict, tc.conf)
		}
		if tc.params != nil {
			for k, v := range tc.params {
				if got.Params[k] != v {
					t.Errorf("%q: params[%s] = %q, 期望 %q", tc.text, k, got.Params[k], v)
				}
			}
		}
	}
}

func TestTaskClassifyEightClasses(t *testing.T) {
	c := NewTaskClassifier(0.6, testSpaces)
	cases := []struct {
		text string
		want string
	}{
		{"记一下这个想法", contract.IntentNote},
		{"上次那个客户的项目聊到哪了", contract.IntentQuery},
		{"把报价模块改成中文", contract.IntentEdit},
		{"这个函数为什么报错", contract.IntentDebug},
		{"跑一下测试", contract.IntentTest},
		{"提交这批改动", contract.IntentCommit},
		{"部署到服务器", contract.IntentDeploy},
		{"你觉得这个方案怎么样", contract.IntentAsk},
		{"加一个工具把 markdown 转成 pdf", contract.IntentRegisterTool},
	}
	for _, tc := range cases {
		got := c.ClassifyTask(tc.text)
		if got.Intent != tc.want {
			t.Errorf("%q: 意图 = %q, 期望 %q", tc.text, got.Intent, tc.want)
		}
	}
}

func TestTaskClassifyIrreversibleConfirm(t *testing.T) {
	c := NewTaskClassifier(0.6, testSpaces)
	for _, text := range []string{"提交这批改动", "部署到服务器"} {
		got := c.ClassifyTask(text)
		if got.Confirm != contract.ConfirmHuman {
			t.Errorf("%q: 不可逆意图基线应为 human, 实际 %q", text, got.Confirm)
		}
		if got.Risk == nil || got.Risk.Reversible {
			t.Errorf("%q: 风险基线应为不可逆", text)
		}
	}
}

func TestTaskClassifyUnknownAsks(t *testing.T) {
	c := NewTaskClassifier(0.6, testSpaces)
	got := c.ClassifyTask("")
	if got.Intent != contract.IntentUnknown || got.Ask == "" {
		t.Fatalf("空文本应 UNKNOWN + Ask: %+v", got)
	}
	got = c.ClassifyTask("今天天气不错")
	if got.Intent != contract.IntentUnknown || got.Ask == "" {
		t.Fatalf("无触发词文本应 UNKNOWN + Ask: %+v", got)
	}
}

func TestTaskClassifyLowConfAsks(t *testing.T) {
	// 低置信阈值测试：低于阈值且非 NOTE/ASK/REGISTER_TOOL → 回问
	c := NewTaskClassifier(0.95, testSpaces) // 阈值抬高，让 0.9 的「把…改成」命中触发回问
	got := c.ClassifyTask("把那个改成这个")
	if got.Intent != contract.IntentEdit {
		t.Fatalf("意图 = %q, 期望 EDIT", got.Intent)
	}
	if got.Ask == "" {
		t.Fatal("低置信 EDIT 应回问")
	}
	if got.Ask == taskAskTemplate {
		t.Fatalf("回问应针对意图定制, 实际为通用模板")
	}
}

func TestTaskClassifySpaceTieNoGuess(t *testing.T) {
	// 平局不猜：两个同长度别名同时命中 → 不选
	spaces := []SpaceHint{
		{Name: "aa", Aliases: []string{"XY"}},
		{Name: "bb", Aliases: []string{"XY"}},
	}
	c := NewTaskClassifier(0.6, spaces)
	got := c.ClassifyTask("在 XY 里把错误提示改成中文")
	if got.Space != "" {
		t.Fatalf("平局应不猜空间, 实际 = %q", got.Space)
	}
}

func TestTaskClassifyLongTextTruncated(t *testing.T) {
	c := NewTaskClassifier(0.6, testSpaces)
	long := "记一下 " + string(make([]rune, 600))
	got := c.ClassifyTask(long)
	if got.Intent != contract.IntentNote {
		t.Fatalf("长文本应正常分类为 NOTE, 实际 %q", got.Intent)
	}
	if len([]rune(got.CorrectedText)) > 512 {
		t.Fatal("超长文本未截断到 512 字符")
	}
}

func TestResolveTimeAnchor(t *testing.T) {
	now := time.Date(2026, 10, 2, 10, 0, 0, 0, time.Local) // 2026-10-02 是周五
	cases := []struct {
		text      string
		wantHint  string
		wantDate  string
		wantAmbig bool
	}{
		{"今天", "今天", "2026-10-02", false},
		{"明天", "明天", "2026-10-03", false},
		{"后天", "后天", "2026-10-04", false},
		{"昨天", "昨天", "2026-10-01", false},
		{"下周一", "下周一", "2026-10-05", false},
		{"下周五", "下周五", "2026-10-09", false},
		{"下下周一", "下下周一", "2026-10-12", false},
		{"本周六", "本周六", "2026-10-03", false}, // 本周六（今天周五）
		{"周五", "周五", "2026-10-02", true},    // 裸周X 恰为今天 → 今天 vs 下周 歧义，回问候选
		{"没有时间词", "", "", false},
	}
	for _, tc := range cases {
		hint, date, ambig := ResolveTimeAnchor(tc.text, now)
		if hint != tc.wantHint || date != tc.wantDate || ambig != tc.wantAmbig {
			t.Errorf("%q: got (%q,%q,%v), want (%q,%q,%v)", tc.text, hint, date, ambig,
				tc.wantHint, tc.wantDate, tc.wantAmbig)
		}
	}
}
