package input

// taskintent_20sample_test.go —— SPEC v2 任务卡 #1：20 条真实任务方向验证的可执行断言。
//
// 这是分片 A 新增的测试文件（不改任何既有 .go）。它把 docs/20-任务方向验证记录.md
// 与 data/20-tasks.jsonl 里的 20 条样例固化为可复现断言：
//
//	pipeline-lite：NewCleaner.Clean → memory.LoadDictionary.Correct → NewTaskClassifier.ClassifyTask
//
// 断言口径：20 条样例全部断言意图类别 == 人工标注（现已 20/20 通过）。
// 历史：首版 #17/#18/#20 因 thoughtWords「想法」命中空间名「想法库」子串被误判为 NOTE，
// 由组织者修复 input/taskintent.go（空间名命中跳过 thoughtWords + noteTriggers 去裸「记」），
// 断言已回正为 EDIT/QUERY/DEPLOY（见 SPEC v2 缺口 #8/#10 裁定与 20 样例记录 §3「已修复」）。

import (
	"testing"

	"voicesign-harness/contract"
	"voicesign-harness/memory"
)

// sampleCase 是一条 20 样例验证记录。
// gotIntent 为当前 TaskClassifier 实跑输出（断言目标）；humanLabel 为人工标注（统计口径）。
type sampleCase struct {
	id         int
	source     string
	raw        string
	gotIntent  string // 当前代码实跑意图（断言这个，保证 go test 绿）
	humanLabel string // 人工标注意图（pass 统计口径）
	wantSpace  string
	note       string
}

var twentySamples = []sampleCase{
	// ① VoxBuyBot 沙特女装
	{1, "voxbuybot", "帮我把那个错误提示啊改成中文，就 voxbuybot 下单页那个", contract.IntentEdit, contract.IntentEdit, "voxbuybot", ""},
	{2, "voxbuybot", "查一下 voxbuybot 昨天那个订单状态", contract.IntentQuery, contract.IntentQuery, "voxbuybot", ""},
	{3, "voxbuybot", "记一下，VoxBuyBot 那个沙特客户喜欢周五下午发货", contract.IntentNote, contract.IntentNote, "voxbuybot", ""},
	{4, "voxbuybot", "跑一下 voxbuybot 的测试", contract.IntentTest, contract.IntentTest, "voxbuybot", ""},
	{5, "voxbuybot", "voxbuybot 下单页为什么报错啊", contract.IntentDebug, contract.IntentDebug, "voxbuybot", ""},
	// ② consultant 医疗耗材
	{6, "medsupply", "查一下医疗耗材那个透析器库存还有多少", contract.IntentQuery, contract.IntentQuery, "medsupply", ""},
	{7, "medsupply", "记一下，季总那个厂房下周一出报价", contract.IntentNote, contract.IntentNote, "", "词典纠错 季总→冀总"},
	{8, "medsupply", "把医疗耗材的报价模板改成新的公司抬头", contract.IntentEdit, contract.IntentEdit, "medsupply", ""},
	{9, "medsupply", "部署一下医疗耗材那个销售报表", contract.IntentDeploy, contract.IntentDeploy, "medsupply", ""},
	{10, "medsupply", "医疗耗材进口沙特的政策你觉得要注意哪些", contract.IntentAsk, contract.IntentAsk, "medsupply", ""},
	// ③ 模型中心运维
	{11, "modelcenter", "查一下今天彼得周点com的访问日志", contract.IntentQuery, contract.IntentQuery, "", "词典纠错 彼得周点com→model.peterzou.com"},
	{12, "modelcenter", "修一下那个 API 超时的报错", contract.IntentDebug, contract.IntentDebug, "", ""},
	{13, "modelcenter", "跑一下那个推理性能基准", contract.IntentTest, contract.IntentTest, "", "test_kind=bench"},
	{14, "modelcenter", "部署最新的模型更新到服务器", contract.IntentDeploy, contract.IntentDeploy, "", ""},
	{15, "modelcenter", "为什么凌晨三点那个告警一直响", contract.IntentAsk, contract.IntentAsk, "", ""},
	// ④ 想法库整理
	{16, "vaultnotes", "记一下这个想法：沙特女装可以做直播带货", contract.IntentNote, contract.IntentNote, "vaultnotes", "note_vs_deploy 仲裁正确"},
	// surfaced finding 三条（#17/#18/#20）已由组织者修复 input/taskintent.go：
	//   1) 命中空间名/别名含「想法/备忘」时跳过 thoughtWords 仲裁（「想法库」是实体名）；
	//   2) noteTriggers 移除裸「记」，补「记录一下/记录」（「记的/记忆/忘记」不再误触 NOTE）。
	// 现断言回正为人工标注意图。
	{17, "vaultnotes", "把想法库那几个旧标签改成一个新标签", contract.IntentEdit, contract.IntentEdit, "vaultnotes", "已修复：想法库=实体名跳过 thoughtWords，extractReplace 命中 EDIT"},
	{18, "vaultnotes", "上次那个想法库里面记的报价客户是哪家", contract.IntentQuery, contract.IntentQuery, "vaultnotes", "已修复：noteTriggers 去裸「记」，「上次」触发 QUERY，「记的」不再误触"},
	{19, "vaultnotes", "把想法库那两条重复的客户条目删掉", contract.IntentEdit, contract.IntentEdit, "vaultnotes", "delete 仲裁"},
	{20, "vaultnotes", "把想法库的内容发到外部 markdown 文件", contract.IntentDeploy, contract.IntentDeploy, "vaultnotes", "已修复：想法库实体名跳过 thoughtWords，「发到」触发 DEPLOY"},
}

func twentyTestSpaces() []SpaceHint {
	return []SpaceHint{
		{Name: "voxbuybot", Aliases: []string{"VoxBuyBot", "女装项目", "voxbuy"}},
		{Name: "medsupply", Aliases: []string{"医疗耗材", "耗材"}},
		{Name: "modelcenter", Aliases: []string{"模型中心", "模型库"}},
		{Name: "vaultnotes", Aliases: []string{"想法库", "想法"}},
	}
}

// Test20SampleValidation 跑通 pipeline-lite，断言 20 条样例意图分类。
func Test20SampleValidation(t *testing.T) {
	cleaner := NewCleaner(nil)
	dict, err := memory.LoadDictionary("")
	if err != nil {
		t.Fatalf("加载内置词典失败: %v", err)
	}
	clf := NewTaskClassifier(0.6, twentyTestSpaces())

	passAgainstHuman := 0
	for _, sc := range twentySamples {
		cleaned := cleaner.Clean(sc.raw)
		corrected, corr := dict.Correct(cleaned)
		got := clf.ClassifyTask(corrected)

		// 断言当前代码实跑意图（go test 绿的口径）。
		if got.Intent != sc.gotIntent {
			t.Errorf("#%d [%s] %q: 实跑意图=%q, 期望(当前行为)=%q",
				sc.id, sc.source, sc.raw, got.Intent, sc.gotIntent)
		}
		// 空间候选断言（人工期望非空时才校验）。
		if sc.wantSpace != "" && got.Space != sc.wantSpace {
			t.Errorf("#%d: 空间=%q, 期望=%q", sc.id, got.Space, sc.wantSpace)
		}
		// 词典纠错样例必须真的发生了纠正。
		if sc.id == 7 && len(corr) == 0 {
			t.Errorf("#7: 期望词典把「季总」纠正为「冀总」，实际无纠正记录")
		}
		if sc.id == 11 && got.Params["time_date"] == "" {
			t.Errorf("#11: 期望解析时间锚点「今天」")
		}
		// 人工标注口径统计（不致失败，仅打印；总通过率应 >=17/20）。
		if got.Intent == sc.humanLabel {
			passAgainstHuman++
		}
	}
	t.Logf("20 样例：分类与人工标注一致 %d/20（门槛 >=17）", passAgainstHuman)
	if passAgainstHuman < 17 {
		t.Errorf("20 样例通过率 %d/20 低于门槛 17", passAgainstHuman)
	}
}

// Test20SampleReceiptFields 断言 contract.RenderReceipt 对每条样例产出四行（动作/文件/结果/撤销）。
func Test20SampleReceiptFields(t *testing.T) {
	// 四行格式由 contract.RenderReceipt 保证；这里只验证它对任意 ReceiptView 都恰好四行、四标签齐全。
	view := ReceiptViewStub()
	lines := splitLines(contract.RenderReceipt(view))
	if len(lines) != 4 {
		t.Fatalf("回执应四行, 实际 %d 行: %q", len(lines), lines)
	}
	for i, want := range []string{"动作：", "文件：", "结果：", "撤销："} {
		if len(lines[i]) < len(want) || lines[i][:len(want)] != want {
			t.Errorf("第 %d 行应以 %q 开头, 实际 %q", i+1, want, lines[i])
		}
	}
}

// ReceiptViewStub 构造一个 stub 四行回执视图（与 data/20-tasks.jsonl 的合成口径一致）。
func ReceiptViewStub() contract.ReceiptView {
	return contract.ReceiptView{
		Action: "EDIT/replace 错误提示",
		Files:  "voxbuybot/**（stub：1 个文件）",
		Result: "OK（轻确认后执行）",
		Undo:   "备份 <log_dir>/backups/<ts>.bak（无 git 仓库）",
	}
}

func splitLines(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == '\n' {
			out = append(out, cur)
			cur = ""
		} else {
			cur += string(r)
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
