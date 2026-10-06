// store_criteria_test.go -- inize(SK-2/3/4/9/10). 
package skill

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

// SK-2: inizeafter** line read**( send     requirei.e.  toin ). 
func TestSK2OfflineReadAfterInternalize(t *testing.T) {
	st := NewStore(t.TempDir(), time.Hour)
	if err := st.SaveManifest(Manifest{ID: "arch-guardian", Version: "v0.1.0", Source: "remote", Raw: json.RawMessage(`{"id":"arch-guardian"}`)}); err != nil {
		t.Fatal(err)
	}
	m, status, err := st.LoadManifest("arch-guardian")
	if err != nil || status != StatusOK || m.Version != "v0.1.0" {
		t.Fatalf("[SK-2] 离线读取失败: %v %s %+v", err, status, m)
	}
}

// SK-3:  end basechange/  TTL ⇒ tgt **stale**(   use  ). 
func TestSK3StaleWhenTTLExceeded(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir, time.Minute)
	base := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	st.Now = func() time.Time { return base }
	if err := st.SaveIndex([]Skill{{ID: "arch-guardian", Version: "v0.1.0", State: "active"}}); err != nil {
		t.Fatal(err)
	}
	if _, status, _ := st.LoadIndex(); status != StatusOK {
		t.Fatalf("[SK-3] 新建应 ok，实际 %s", status)
	}
	st.Now = func() time.Time { return base.Add(2 * time.Minute) }
	_, status, err := st.LoadIndex()
	if err != nil {
		t.Fatal(err)
	}
	if status != StatusStale {
		t.Errorf("[SK-3] 超 TTL 应 stale，实际 %s（静默用旧的）", status)
	}
}

// SK-4:  get/readget   ⇒ **unknown**,   cur" has". 
func TestSK4FailureIsUnknownNotAbsent(t *testing.T) {
	st := NewStore(filepath.Join(t.TempDir(), "nope"), time.Hour)
	_, status, err := st.LoadIndex()
	if err == nil {
		t.Error("[SK-4] 缺失应返回错误（可见）")
	}
	if status != StatusUnknown {
		t.Errorf("[SK-4] 失败应标 unknown，实际 %q", status)
	}
	if _, status2, _ := st.LoadManifest("missing"); status2 != StatusUnknown {
		t.Errorf("[SK-4] 单个 manifest 缺失也应 unknown，实际 %q", status2)
	}
}

// SK-9: underlinestateoverwrite +     state ⇒   inand   unknown_state
func TestSK9OfflineAndUnknownStates(t *testing.T) {
	for _, s := range []string{"deprecated", "disabled", "paused", "retired"} {
		if StateClass(s) != "offline" {
			t.Errorf("[SK-9] %q 应判为 offline", s)
		}
	}
	if StateClass("active") != "online" {
		t.Error("[SK-9] active 应为 online")
	}
	if StateClass("weird-new-state") != "unknown" {
		t.Error("[SK-9] 未登记 state 应为 unknown（不猜）")
	}
	all := []Skill{
		{ID: "arch-guardian", State: "active"},
		{ID: "x", State: "disabled"},
		{ID: "y", State: "weird-new-state"},
	}
	sel := Select(all, map[string]bool{"arch-guardian": true, "x": true, "y": true})
	if len(sel.Included) != 1 {
		t.Errorf("[SK-9] 只应纳入 1 个，实际 %d", len(sel.Included))
	}
	reasons := map[string]bool{}
	for _, f := range sel.Filtered {
		reasons[f.Reason] = true
	}
	if !reasons["offline:disabled"] || !reasons["unknown_state:weird-new-state"] {
		t.Errorf("[SK-9] 被过滤原因不全: %+v", sel.Filtered)
	}
}

// SK-10:  name   ** become  **(   ,  bycalluse    )
func TestSK10WhitelistIsDeclared(t *testing.T) {
	if len(DefaultWhitelist) != 4 {
		t.Errorf("[SK-10] 白名单应有 4 项，实际 %d", len(DefaultWhitelist))
	}
	for _, id := range []string{"ai-native-architecture-design", "arch-guardian", "arch-review", "deep-research"} {
		if !DefaultWhitelist[id] {
			t.Errorf("[SK-10] 白名单缺 %q", id)
		}
	}
}
