package skill

import (
	"os"
	"path/filepath"
	"testing"
)

// SK-6 · **每次调用必须留一条沉淀**（`tasks/VHS-SKILL-001:137`）。
//
// 本判据只核"沉淀**能被记下来且可归因**"（层本身）；**"每次调用都记"需接线后才能真验**
// （`skill/` 目前零生产调用 —— 已如实标注在 `sediment.go` 顶部）。
func TestSK6RecordProducesAttributableSediment(t *testing.T) {
	st := NewSedimentStore(filepath.Join(t.TempDir(), "usage.jsonl"))
	e, err := st.Record(Sediment{
		SkillID: "arch-guardian", SkillVersion: "v0.1.0",
		Scenario: "长程任务：搭建计费服务", Judgement: "先确认域", Basis: []string{"space_check"},
		Outcome: OutcomeRejected,
	})
	if err != nil {
		t.Fatalf("[SK-6] Record 失败: %v", err)
	}
	// ① 默认必须是 pending（不得凭空"已验证"）
	if e.Verdict != VerdictUnverified {
		t.Errorf("[SK-6] 新沉淀的裁决应为 pending，实际 %q ⇒ **未回填就等于已验证**（违反 SK-7）", e.Verdict)
	}
	// ② 必须可归因
	if e.SkillID == "" || e.ID == "" || e.At == "" {
		t.Errorf("[SK-6] 沉淀缺归因字段: %+v", e)
	}
	// ③ 缺 skill_id ⇒ **报错**（不静默记一条无法归因的）
	if _, err := st.Record(Sediment{Judgement: "x"}); err == nil {
		t.Errorf("[SK-6] 无 skill_id 竟然记成功 ⇒ 沉淀不可归因")
	}
	t.Logf("[SK-6] 沉淀已记: id=%s verdict=%s ✅", e.ID, e.Verdict)
}

// SK-7 · `verdict` 未回填 ⇒ **pending**，**不得当成"已验证"**。
//
// ⚠️ 这与「**未测 ≠ 通过**」是**同一条原则**（对象不同：技能沉淀 vs 交付物）。
func TestSK7UnbackfilledStaysPending(t *testing.T) {
	st := NewSedimentStore(filepath.Join(t.TempDir(), "usage.jsonl"))
	a, _ := st.Record(Sediment{SkillID: "s1", Scenario: "sc", Judgement: "j"})
	b, _ := st.Record(Sediment{SkillID: "s2", Scenario: "sc", Judgement: "j"})

	// 回填 b 为 right，a 留 pending
	if err := st.BackfillVerdict(b.ID, VerdictCorrect); err != nil {
		t.Fatalf("[SK-7] 回填失败: %v", err)
	}
	all, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	var gotA, gotB *Sediment
	for i := range all {
		switch all[i].ID {
		case a.ID:
			gotA = &all[i]
		case b.ID:
			gotB = &all[i]
		}
	}
	if gotA == nil || gotB == nil {
		t.Fatalf("[SK-7] 读回缺条目")
	}
	if gotA.Verdict != VerdictUnverified {
		t.Errorf("[SK-7] 未回填的条目裁决=%q ⇒ 应为 pending", gotA.Verdict)
	}
	if gotA.IsVerified() {
		t.Errorf("[SK-7] **未回填却被当成已验证** ⇒ 违反 SK-7（≡「未测 != 通过」）")
	}
	if !gotB.IsVerified() {
		t.Errorf("[SK-7] 已回填 right 的条目应算已验证，实际 %q", gotB.Verdict)
	}
	// 非法回填值 ⇒ 报错（不静默当 right）
	if err := st.BackfillVerdict(b.ID, Verdict("???")); err == nil {
		t.Errorf("[SK-7] 非法裁决竟然回填成功 ⇒ 会把「判不了」当成「已验证」")
	}
	if err := st.BackfillVerdict(b.ID, VerdictUnverified); err == nil {
		t.Errorf("[SK-7] 用 pending 回填竟然成功 ⇒ 语义上就是「未回填」")
	}
	pend, err := st.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pend) != 1 || pend[0].ID != a.ID {
		t.Errorf("[SK-7] Pending() 应恰好返回未回填那一条，实际 %d 条", len(pend))
	}
	t.Logf("[SK-7] 未回填 %d 条 · 已回填 1 条 ✅", len(pend))
}

// SK-8 · `verdict=wrong` ⇒ **必须能转成一条判据**（否则同一个坑会重犯）。
func TestSK8WrongBecomesCriterion(t *testing.T) {
	st := NewSedimentStore(filepath.Join(t.TempDir(), "usage.jsonl"))
	e, _ := st.Record(Sediment{
		SkillID: "arch-guardian", SkillVersion: "v0.1.0",
		Scenario: "对只读域发起写操作", Judgement: "应先消歧域，不得回落默认域",
	})
	// ① **非 wrong ⇒ 报错**（不许把 right 也转成判据）
	if _, err := CriterionFromWrong(e); err == nil {
		t.Errorf("[SK-8] verdict=pending 竟然转成了判据 ⇒ 只有 wrong 才该转")
	}
	if err := st.BackfillVerdict(e.ID, VerdictWrong); err != nil {
		t.Fatal(err)
	}
	all, _ := st.Load()
	var wrong Sediment
	for _, x := range all {
		if x.ID == e.ID {
			wrong = x
		}
	}
	c, err := CriterionFromWrong(wrong)
	if err != nil {
		t.Fatalf("[SK-8] 从 wrong 转判据失败: %v", err)
	}
	// ② 判据必须带可识别的场景与判断（否则重犯时认不出）
	if c.Text == "" || c.Skill == "" || c.ID == "" {
		t.Errorf("[SK-8] 转出的判据不完整: %+v", c)
	}
	// ③ **不得凭空宣称可机械化**（新判据默认人工）
	if !c.Manual || c.Check != "manual" {
		t.Errorf("[SK-8] 新判据默认应为人工（不得凭空说可机械化）: %+v", c)
	}
	// ④ 缺 scenario/judgement ⇒ 报错
	if _, err := CriterionFromWrong(Sediment{ID: "x", Verdict: VerdictWrong, SkillID: "s"}); err == nil {
		t.Errorf("[SK-8] 缺 scenario/judgement 竟然转成功 ⇒ 重犯时认不出")
	}
	t.Logf("[SK-8] 已转判据: %s · %s ✅", c.ID, c.Text)
}

// SK-7 补充 · 沉淀必须 **append-only**（回填不改写历史行）。
func TestSK7SedimentIsAppendOnly(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "usage.jsonl")
	st := NewSedimentStore(p)
	e, _ := st.Record(Sediment{SkillID: "s", Scenario: "sc", Judgement: "j"})
	before, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.BackfillVerdict(e.ID, VerdictCorrect); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) <= len(before) {
		t.Errorf("[SK-7] 回填后文件没有变长 ⇒ 不是 append-only（可能改写了历史行）")
	}
	if string(after[:len(before)]) != string(before) {
		t.Errorf("[SK-7] 回填**改写了已有内容** ⇒ 违反 append-only")
	}
	t.Logf("[SK-7] append-only 成立（%d → %d 字节，前缀未变）✅", len(before), len(after))
}
