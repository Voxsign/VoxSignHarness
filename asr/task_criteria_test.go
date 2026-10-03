//go:build vhsui

// task_criteria_test.go —— /v1/task：只规划、不执行；域门禁在规划期生效；document 不落盘。
package asr

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func taskCall(t *testing.T, base, body string) map[string]any {
	t.Helper()
	resp, err := http.Post(base+"/v1/task", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

// ① 只规划不执行：可规划的任务返回步骤，且 execute=false。
func TestTaskPlansWithoutExecuting(t *testing.T) {
	base, _ := newUIServer(t)
	got := taskCall(t, base, `{"task":"把这个项目里所有 TODO 整理成一份文档","document":"TODO: x"}`)
	if got["execute"] != false {
		t.Fatalf("[task] execute 必须为 false（只规划）: %v", got["execute"])
	}
	p, _ := got["plan"].(map[string]any)
	if p == nil {
		t.Fatal("[task] 缺 plan")
	}
	steps, _ := p["steps"].([]any)
	if len(steps) == 0 {
		t.Errorf("[task] 可规划任务应给出步骤: %+v", p)
	}
	if p["source"] == nil || p["source"] == "" {
		t.Errorf("[task] 必须回传 Source（可解释）: %+v", p)
	}
}

// ② 域门禁在**规划期**生效：能力缺口 ⇒ 拒绝 + 该找谁（owner 前缀）。
func TestTaskRefusesOutOfScopeAtPlanningTime(t *testing.T) {
	base, _ := newUIServer(t)
	got := taskCall(t, base, `{"task":"帮我部署到生产服务器"}`)
	p, _ := got["plan"].(map[string]any)
	if p == nil {
		t.Fatal("[task] 缺 plan")
	}
	if p["refused"] != true {
		t.Errorf("[task] 越域/缺口计划应在规划期被拒: %+v", p)
	}
	missing, _ := p["missing"].([]any)
	if len(missing) == 0 {
		t.Fatal("[task] 拒绝必须给出原因与 owner")
	}
	joined := ""
	for _, m := range missing {
		joined += m.(string) + "|"
	}
	if !strings.Contains(joined, "人：") && !strings.Contains(joined, "网关：") {
		t.Errorf("[task] 卡点必须带 owner 前缀（找谁）: %v", missing)
	}
}

// ③ document **不落盘**（除 reallog 外，dataDir 不得多文件）。
func TestTaskDocumentNotPersisted(t *testing.T) {
	base, logPath := newUIServer(t)
	dataDir := filepath.Dir(logPath)
	before := map[string]bool{}
	_ = filepath.Walk(dataDir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			before[p] = true
		}
		return nil
	})
	big := strings.Repeat("机密文档内容。", 200)
	taskCall(t, base, `{"task":"总结这个文档","document":"`+big+`"}`)
	err := filepath.Walk(dataDir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !before[p] {
			t.Errorf("[task] document 被落盘: %s", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// ④ 页面：有任务入口 + **明确标注只规划不执行**。
func TestTaskPageHasUploadAndPlanOnlyLabel(t *testing.T) {
	page := fetchPage(t)
	for _, want := range []string{`id="doc"`, `id="task"`, "planTask()", "只规划，不执行", `id="file"`} {
		if !strings.Contains(page, want) {
			t.Errorf("[task] 页面缺 %q", want)
		}
	}
}

// #2-b：**文档必须真的影响规划结果**（同一任务：空文档 vs 有内容 ⇒ 必须不同）。
func TestTaskDocumentActuallyAffectsPlan(t *testing.T) {
	base, _ := newUIServer(t)
	empty := taskCall(t, base, `{"task":"把文档里的待办整理成计划","document":""}`)
	full := taskCall(t, base, `{"task":"把文档里的待办整理成计划","document":"# 报价文档\n- TODO: 改报价单\n- TODO: 发邮件\n库存：120 件\n客户：ABC"}`)

	if empty["document_read"] != false || empty["document_note"] == "" {
		t.Errorf("[文档] 空文档必须显式说明（document_read=false + note）: %v/%v", empty["document_read"], empty["document_note"])
	}
	if full["document_read"] != true {
		t.Fatalf("[文档] 有内容却未读: %v", full)
	}
	pe, _ := empty["plan"].(map[string]any)
	pf, _ := full["plan"].(map[string]any)
	ce, _ := pe["considered"].([]any)
	cf, _ := pf["considered"].([]any)
	if len(cf) <= len(ce) {
		t.Fatalf("[文档] 规划结果未因文档而变：considered %d → %d", len(ce), len(cf))
	}
	// ② 文档关键词必须出现在 considered 里
	joined := ""
	for _, c := range cf {
		m, _ := c.(map[string]any)
		joined += m["element"].(string) + "|"
	}
	if !strings.Contains(joined, "document:") || !strings.Contains(joined, "TODO") {
		t.Errorf("[文档] considered 未见文档结构: %s", joined)
	}
	// 文档实体进活跃实体口径（w_used 或 considered 可见）
	if pf["wm"] == nil || pe["wm"] == nil {
		t.Errorf("[文档] 缺 wm")
	}
}

// ④ 超大文档 ⇒ 降级并说明（不许静默当没有）。
func TestTaskTooLargeDocumentDegrades(t *testing.T) {
	base, _ := newUIServer(t)
	huge := strings.Repeat("这是一段很长的文档内容。", 30000) // > 200k runes
	got := taskCall(t, base, `{"task":"总结文档","document":"`+huge+`"}`)
	p, _ := got["plan"].(map[string]any)
	if p["degraded"] != true || p["degraded_reason"] == "" {
		t.Errorf("[文档] 超大文档应降级并说明: %+v", p["degraded_reason"])
	}
	if got["document_read"] != true {
		t.Errorf("[文档] 超大文档仍应（截断后）参与规划")
	}
}

// #2-c：**文档必须影响 steps**。
func TestTaskDocumentAffectsSteps(t *testing.T) {
	base, _ := newUIServer(t)
	// ① 空文档 vs 34 条目文档 ⇒ steps 必须不同
	empty := taskCall(t, base, `{"task":"把这个项目里所有 TODO 整理成一份文档","document":""}`)
	var b strings.Builder
	for i := 0; i < 34; i++ {
		b.WriteString("- TODO: 条目 " + strconv.Itoa(i) + "\n")
	}
	big := taskCall(t, base, `{"task":"把这个项目里所有 TODO 整理成一份文档","document":"`+strings.ReplaceAll(b.String(), "\n", "\\n")+`"}`)
	es, _ := empty["plan"].(map[string]any)["steps"].([]any)
	bs, _ := big["plan"].(map[string]any)["steps"].([]any)
	if len(es) == len(bs) {
		t.Fatalf("[#2-c] 34 条目文档未改变步骤数量: 空=%d 有=%d", len(es), len(bs))
	}

	// ② **防乱变**：内容等价、措辞不同的两个文档 ⇒ 步骤数量必须相同
	var c strings.Builder
	for i := 0; i < 34; i++ {
		c.WriteString("- 待办事项 " + strconv.Itoa(i) + "\n")
	}
	other := taskCall(t, base, `{"task":"把这个项目里所有 TODO 整理成一份文档","document":"`+strings.ReplaceAll(c.String(), "\n", "\\n")+`"}`)
	os, _ := other["plan"].(map[string]any)["steps"].([]any)
	if len(os) != len(bs) {
		t.Errorf("[#2-c 防乱变] 等价内容的两个文档得到不同步数: %d vs %d", len(bs), len(os))
	}

	// ③ 未读文档（空）⇒ 与"完全不带 document 字段"一致
	none := taskCall(t, base, `{"task":"把这个项目里所有 TODO 整理成一份文档"}`)
	ns, _ := none["plan"].(map[string]any)["steps"].([]any)
	if len(ns) != len(es) {
		t.Errorf("[#2-c] 未读文档时步骤应与无文档一致: %d vs %d", len(es), len(ns))
	}
}

// #2-c-① 条目格式覆盖：每种格式都要认；普通段落不得被算成条目。
func TestTaskItemFormatCoverage(t *testing.T) {
	base, _ := newUIServer(t)
	cases := []struct {
		name string
		doc  string
		want int
	}{
		{"短横线", "- a\n- b\n", 2},
		{"星号", "* a\n* b\n", 2},
		{"加号", "+ a\n+ b\n", 2},
		{"数字点", "1. a\n2. b\n", 2},
		{"数字括号", "1) a\n2) b\n", 2},
		{"任务未完成", "- [ ] a\n- [ ] b\n", 2},
		{"任务已完成", "- [x] a\n- [x] b\n", 2},
		{"TODO 冒号", "TODO: a\nTODO: b\n", 2},
		{"FIXME", "FIXME: a\nFIXME: b\n", 2},
		{"普通段落", "这是一段普通文字。\n还有第二行。\n", 0},
	}
	for _, c := range cases {
		got := taskCall(t, base, `{"task":"整理文档","document":"`+strings.ReplaceAll(c.doc, "\n", "\\n")+`"}`)
		doc, _ := got["document"].(map[string]any)
		if doc == nil {
			t.Fatalf("%s: 缺 document", c.name)
		}
		items, _ := doc["items"].(float64)
		if int(items) != c.want {
			t.Errorf("[格式] %s：条目数=%v，期望 %d", c.name, items, c.want)
		}
	}
}

// #2-c-② 驱动量与任务语义一致：34 个 TODO 行也必须分批。
func TestTaskTodoCountDrivesBatching(t *testing.T) {
	base, _ := newUIServer(t)
	var b strings.Builder
	for i := 0; i < 34; i++ {
		b.WriteString("TODO: 待办 " + strconv.Itoa(i) + "\n")
	}
	got := taskCall(t, base, `{"task":"把这个项目里所有 TODO 整理成一份文档","document":"`+strings.ReplaceAll(b.String(), "\n", "\\n")+`"}`)
	p, _ := got["plan"].(map[string]any)
	steps, _ := p["steps"].([]any)
	if len(steps) <= 2 {
		t.Fatalf("[驱动量] 34 个 TODO 行未触发分批（steps=%d）—— 任务说整理 TODO 却不用 TODO 数决定", len(steps))
	}
	// ③ chain_len 必须等于展开后的步数
	wm, _ := p["wm"].(map[string]any)
	if wm != nil {
		if cl, _ := wm["chain_len"].(float64); int(cl) != len(steps) {
			t.Errorf("[chain_len] %v != steps %d（数字是假的）", wm["chain_len"], len(steps))
		}
	}
	// ④ 数字可解释：considered 必须说明驱动量与批数
	found := false
	for _, c := range p["considered"].([]any) {
		m, _ := c.(map[string]any)
		if el, _ := m["element"].(string); strings.Contains(el, "document_batches:") && strings.Contains(el, "TODO") {
			found = true
		}
	}
	if !found {
		t.Errorf("[驱动量] considered 未说明驱动量（应含 TODO）: %v", p["considered"])
	}
}
