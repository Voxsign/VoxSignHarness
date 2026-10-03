// select_criteria_test.go —— SK-4 口径：白名单 + 显式记录被过滤项。
package skill

import "testing"

func fixture() []Skill {
	return []Skill{
		{ID: "ai-native-architecture-design", Version: "v1.0.1", State: "active"},
		{ID: "arch-guardian", Version: "v0.1.0", State: "active"},
		{ID: "arch-review", Version: "v0.1.0", State: "active"},
		{ID: "deep-research", Version: "v0.3.0", State: "active", Kind: "research"},
		{ID: "arch-research", Version: "v0.2.0", State: "deprecated"},
		{ID: "probe-test", Version: "v1.0.0", State: "deprecated"},
		{ID: "bad-remote", Version: "v1.0.0", State: "active"},
		{ID: "dbg", Version: "v1.0.0", State: "active"},
		{ID: "slow", Version: "v1.0.0", State: "active"},
		{ID: "smoke-arch", Version: "v1.0.0", State: "active"},
	}
}

func allowlist() map[string]bool {
	return map[string]bool{
		"ai-native-architecture-design": true,
		"arch-guardian":                 true,
		"arch-review":                   true,
		"deep-research":                 true,
	}
}

// ① 只纳入白名单内且非 deprecated 的技能（4 个）
func TestSelectIncludesOnlyWhitelistedActive(t *testing.T) {
	sel := Select(fixture(), allowlist())
	if len(sel.Included) != 4 {
		t.Fatalf("[SK] 纳入数应为 4，实际 %d: %+v", len(sel.Included), sel.Included)
	}
	for _, s := range sel.Included {
		if s.State == "deprecated" || !allowlist()[s.ID] {
			t.Errorf("[SK] 纳入了不该纳入的: %+v", s)
		}
	}
}

// ② **被过滤项必须显式记录**，且理由分得清（deprecated vs not_in_whitelist）
func TestSelectRecordsFilteredWithReasons(t *testing.T) {
	sel := Select(fixture(), allowlist())
	reasons := map[string]int{}
	for _, f := range sel.Filtered {
		if f.Reason == "" {
			t.Errorf("[SK] 被过滤项无理由（静默丢弃）: %+v", f)
		}
		reasons[f.Reason]++
	}
	// SK-9 升版：下线态原因带前缀 `offline:<state>`（不止 deprecated）
	if reasons["offline:deprecated"] != 2 {
		t.Errorf("[SK] offline:deprecated 应记 2 条，实际 %d（reasons=%v）", reasons["offline:deprecated"], reasons)
	}
	if reasons["not_in_whitelist"] != 4 {
		t.Errorf("[SK] not_in_whitelist 应记 4 条，实际 %d", reasons["not_in_whitelist"])
	}
}

// ③ 守恒：没有任何技能被静默丢弃（纳入 + 过滤 == 全部）
func TestSelectIsConservativeNoSilentDrop(t *testing.T) {
	all := fixture()
	sel := Select(all, allowlist())
	if len(sel.Included)+len(sel.Filtered) != len(all) {
		t.Errorf("[SK] 有技能被静默丢弃: %d + %d != %d",
			len(sel.Included), len(sel.Filtered), len(all))
	}
}

// ④ 空白名单 ⇒ 一个都不纳入，但**仍逐条记录原因**（不是"什么都没有"）
func TestSelectEmptyWhitelistStillRecords(t *testing.T) {
	all := fixture()
	sel := Select(all, map[string]bool{})
	if len(sel.Included) != 0 {
		t.Errorf("[SK] 空白名单却纳入: %+v", sel.Included)
	}
	if len(sel.Filtered) != len(all) {
		t.Errorf("[SK] 空白名单时应逐条记录，实际 %d/%d", len(sel.Filtered), len(all))
	}
}
