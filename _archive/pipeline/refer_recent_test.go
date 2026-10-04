package pipeline

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"voicesign-harness/refer"
)

// TestExtractRecentEntities（2026-10-04 多轮指代接线）：文本核心实体提取——
// 拉丁专名（OT-ODP/SPoG/DMZ）+ 书名号 + 引号短语，去重。
func TestExtractRecentEntities(t *testing.T) {
	ents := extractRecentEntities("查一下 OT-ODP 六层目标架构，SPoG 和 DMZ 的机制，参考《国家电网南非方案》和“企业发布”")
	got := map[string]string{}
	for _, e := range ents {
		got[e.Entity] = e.Kind
	}
	for want, kind := range map[string]string{
		"OT-ODP": "project", "SPoG": "project", "DMZ": "project",
		"国家电网南非方案": "file", "企业发布": "project",
	} {
		if got[want] != kind {
			t.Errorf("实体 %q 缺失或 kind 错误: got %v, want %q", want, got[want], kind)
		}
	}
	if len(ents) > 8 {
		t.Errorf("去重失败：%d 个实体", len(ents))
	}
}

// TestRecentSlotRoundtrip：write→load 闭环（append-only 槽）。
func TestRecentSlotRoundtrip(t *testing.T) {
	logDir := t.TempDir()
	convID := "sess-roundtrip"
	e1 := refer.RecentEntity{Space: "default", Entity: "OT-ODP", Kind: "project", Ts: "2026-10-04T00:00:01Z"}
	e2 := refer.RecentEntity{Space: "default", Entity: "SPoG", Kind: "project", Ts: "2026-10-04T00:00:02Z"}
	writeRecentEntities(logDir, convID, []refer.RecentEntity{e1, e2})
	got := loadRecentEntities(logDir, convID)
	if len(got) != 2 {
		t.Fatalf("load 到 %d 条, want 2", len(got))
	}
	if got[0].Entity != "SPoG" { // ts 倒序：新的在前
		t.Errorf("排序错误: 第一条 %q, want SPoG", got[0].Entity)
	}
	if got[1].Entity != "OT-ODP" {
		t.Errorf("排序错误: 第二条 %q, want OT-ODP", got[1].Entity)
	}
}

// TestMultiTurnReferThroughPipeline：多轮指代接线端到端——
// 轮 1 记 OT-ODP（执行成功 → 写会话槽）；轮 2"这个方案…查一下"（非问句 QUERY，
// 有 Recent 上下文 → refer 解析"这个"→ Target=OT-ODP，不再回问）。
func TestMultiTurnReferThroughPipeline(t *testing.T) {
	o := testOptions(t, nil)
	out1, err := Run(context.Background(), o, "记一下：OT-ODP 六层目标架构")
	if err != nil {
		t.Fatal(err)
	}
	if out1.Intent.Intent != "NOTE" {
		t.Fatalf("轮1 意图 %s, want NOTE", out1.Intent.Intent)
	}

	out2, err := Run(context.Background(), o, "这个方案里 DMZ 发布机制查一下")
	if err != nil {
		t.Fatal(err)
	}
	if out2.Ask != "" {
		t.Errorf("轮2 仍回问 %q（多轮指代未生效）", out2.Ask)
	}
	if out2.Intent.Target == nil || !strings.Contains(out2.Intent.Target.Entity, "OT-ODP") {
		t.Errorf("轮2 指代未解析到 OT-ODP: target=%+v", out2.Intent.Target)
	}
}

// TestRecentSlotSessionIsolation：会话隔离——不同 ConvID 槽互不读取，不跨会话猜。
func TestRecentSlotSessionIsolation(t *testing.T) {
	o1 := testOptions(t, nil)
	o1.ConvID = "sess-A"
	if _, err := Run(context.Background(), o1, "记一下：OT-ODP 六层目标架构"); err != nil {
		t.Fatal(err)
	}
	o2 := testOptions(t, nil)
	o2.ConvID = "sess-B"
	out2, err := Run(context.Background(), o2, "这个方案里 DMZ 发布机制查一下")
	if err != nil {
		t.Fatal(err)
	}
	// sess-B 无任何 Recent 上下文 → 高置信非裸指代不解析（M7 原行为：不回写指代 Ask）。
	ents := loadRecentEntities(filepath.Join(o2.Cfg.Global.LogDir), "sess-B")
	for _, e := range ents {
		if e.Entity == "OT-ODP" {
			t.Errorf("会话隔离失败：sess-B 读到 sess-A 的实体 %q", e.Entity)
		}
	}
	_ = out2
}
