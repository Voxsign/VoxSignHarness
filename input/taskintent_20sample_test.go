package input

// taskintent_20sample_test.go -- SPEC v2 task  #1: 20    task to      disconnectlang. 
//
//  issplit  A newadd   file( modify   has .go).  pipe docs/20-task to    .md
// and data/20-tasks.jsonl    20  kindexample izeas  nowdisconnectlang: 
//
//	pipeline-lite: NewCleaner.Clean -> memory.LoadDictionary.Correct -> NewTaskClassifier.ClassifyTask
//
// disconnectlang path: 20  kindexamplesafety disconnectlangintentclassdiff == humantgtnote(nowalready 20/20  ed). 
//   : first  #17/#18/#20 because thoughtWords"  " inemptytimename"   "  be  as NOTE, 
// by  erfix  input/taskintent.go(emptytimename in ed thoughtWords + noteTriggers   " "), 
// disconnectlangalreadybackposas EDIT/QUERY/DEPLOY(see SPEC v2    #8/#10   and 20 kindexample   §3"alreadyfix "). 

import (
	"testing"

	"voicesign-harness/contract"
	"voicesign-harness/memory"
)

// sampleCase is   20 kindexample    . 
// gotIntent ascurbefore TaskClassifier    out(disconnectlangobjtgt); humanLabel ashumantgtnote(   path). 
type sampleCase struct {
	id         int
	source     string
	raw        string
	gotIntent  string // curbefore code  intent(disconnectlang  , keep  go test  )
	humanLabel string // humantgtnoteintent(pass    path)
	wantSpace  string
	note       string
}

var twentySamples = []sampleCase{
	// ① VoxBuyBot     
	{1, "voxbuybot", "帮我把那个错误提示啊改成中文，就 voxbuybot 下单页那个", contract.IntentEdit, contract.IntentEdit, "voxbuybot", ""},
	{2, "voxbuybot", "查一下 voxbuybot 昨天那个订单状态", contract.IntentQuery, contract.IntentQuery, "voxbuybot", ""},
	{3, "voxbuybot", "记一下，VoxBuyBot 那个沙特客户喜欢周五下午发货", contract.IntentNote, contract.IntentNote, "voxbuybot", ""},
	{4, "voxbuybot", "跑一下 voxbuybot 的测试", contract.IntentTest, contract.IntentTest, "voxbuybot", ""},
	{5, "voxbuybot", "voxbuybot 下单页为什么报错啊", contract.IntentDebug, contract.IntentDebug, "voxbuybot", ""},
	// ② consultant     
	{6, "medsupply", "查一下医疗耗材那个透析器库存还有多少", contract.IntentQuery, contract.IntentQuery, "medsupply", ""},
	{7, "medsupply", "记一下，网总那个厂房下周一出报价", contract.IntentNote, contract.IntentNote, "", "词典纠错 网总→王总"},
	{8, "medsupply", "把医疗耗材的报价模板改成新的公司抬头", contract.IntentEdit, contract.IntentEdit, "medsupply", ""},
	{9, "medsupply", "部署一下医疗耗材那个销售报表", contract.IntentDeploy, contract.IntentDeploy, "medsupply", ""},
	{10, "medsupply", "医疗耗材进口沙特的政策你觉得要注意哪些", contract.IntentAsk, contract.IntentAsk, "medsupply", ""},
	// ③  typein   
	{11, "modelcenter", "查一下今天彼得周点com的访问日志", contract.IntentQuery, contract.IntentQuery, "", "词典纠错 彼得周点com→model.example.com"},
	{12, "modelcenter", "修一下那个 API 超时的报错", contract.IntentDebug, contract.IntentDebug, "", ""},
	{13, "modelcenter", "跑一下那个推理性能基准", contract.IntentTest, contract.IntentTest, "", "test_kind=bench"},
	{14, "modelcenter", "部署最新的模型更新到服务器", contract.IntentDeploy, contract.IntentDeploy, "", ""},
	{15, "modelcenter", "为什么凌晨三点那个告警一直响", contract.IntentAsk, contract.IntentAsk, "", ""},
	// ④      
	{16, "vaultnotes", "记一下这个想法：沙特女装可以做直播带货", contract.IntentNote, contract.IntentNote, "vaultnotes", "note_vs_deploy 仲裁正确"},
	// surfaced finding   (#17/#18/#20)alreadyby  erfix  input/taskintent.go: 
	//   1)  inemptytimename/diffname "  /  "time ed thoughtWords   ("   "is bodyname); 
	//   2) noteTriggers    " ", patch"   under/  "("  /  /  " again trigger NOTE). 
	// nowdisconnectlangbackposashumantgtnoteintent. 
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

// Test20SampleValidation    pipeline-lite, disconnectlang 20  kindexampleintentclassify. 
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
		corrected, _ := dict.Correct(cleaned)
		got := clf.ClassifyTask(corrected)

		// disconnectlangcurbefore code  intent(go test    path). 
		if got.Intent != sc.gotIntent {
			t.Errorf("#%d [%s] %q: 实跑意图=%q, 期望(当前行为)=%q",
				sc.id, sc.source, sc.raw, got.Intent, sc.gotIntent)
		}
		// emptytime  disconnectlang(humanperiod  emptytimeonlyverify). 
		if sc.wantSpace != "" && got.Space != sc.wantSpace {
			t.Errorf("#%d: 空间=%q, 期望=%q", sc.id, got.Space, sc.wantSpace)
		}
			if sc.id == 11 && got.Params["time_date"] == "" {
			t.Errorf("#11: 期望解析时间锚点「今天」")
		}
		// humantgtnote path  (    , only  ;   edrate  >=17/20). 
		if got.Intent == sc.humanLabel {
			passAgainstHuman++
		}
	}
	t.Logf("20 样例：分类与人工标注一致 %d/20（门槛 >=17）", passAgainstHuman)
	if passAgainstHuman < 17 {
		t.Errorf("20 样例通过率 %d/20 低于门槛 17", passAgainstHuman)
	}
}

// Test20SampleReceiptFields disconnectlang contract.RenderReceipt to  kindexampleproduceout  (  /file/close /  ). 
func Test20SampleReceiptFields(t *testing.T) {
	//    formby contract.RenderReceipt keep ;   only   to   ReceiptView all    ,  tgt  safety. 
	view := ReceiptViewStub()
	lines := splitLines(contract.RenderReceipt(view))
	if len(lines) != 4 {
		t.Fatalf("回执应四行, 实际 %d 行: %q", len(lines), lines)
	}
	for i, want := range []string{"Action: ", "Files: ", "Result: ", "Undo: "} {
		if len(lines[i]) < len(want) || lines[i][:len(want)] != want {
			t.Errorf("第 %d 行应以 %q 开头, 实际 %q", i+1, want, lines[i])
		}
	}
}

// ReceiptViewStub      stub four-line receipt  (and data/20-tasks.jsonl   become path  ). 
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
