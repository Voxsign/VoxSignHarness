package skill

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// sediment.go —— 技能层的**沉淀层**（SK-6 / SK-7 / SK-8）。
//
// 依据 `tasks/VHS-SKILL-001-技能层接口设计.md:137-139`：
//
//	SK-6  ⭐ **每次调用必须留一条沉淀**（skill_id/version/场景/判断/依据/outcome）
//	SK-7  `verdict` 未回填 ⇒ 该条沉淀标记为 **pending**，**不得当成"已验证"**
//	SK-8  `verdict=wrong` ⇒ **必须能转成一条判据**（否则同一个坑会重犯）
//	（:141「**SK-5 和 SK-8 是这套设计的关键** —— 它们把"技能"从"参考资料"变成"**可校准的能力**"」）
//
// ⚠️ **SK-7 与「未测 ≠ 通过」是同一条原则**，只是对象不同
// （此处管**技能沉淀条目**；交付物的 pending 语义在台账/审计里，不在此文件）。
//
// ⚠️ 现状（2026-10-03 实测）：`skill/` 包**零生产调用**（孤岛）⇒ 本层做好后**仍无调用方**
// （与 `KnowhowFromRaw` 同一处境）。**这是"准备好了"，不是"用上了"。**

// Verdict 是沉淀条目的**回填裁决**。
//
// ⚠️ 零值 `VerdictPending` = **未回填** ⇒ 按 SK-7，**不得当成"已验证"**。
type Verdict string

const (
	VerdictPending Verdict = "pending" // 未回填（**默认值**）
	VerdictRight   Verdict = "right"
	VerdictWrong   Verdict = "wrong"
)

// Sediment 是一条沉淀（SK-6 的字段集）。
type Sediment struct {
	ID        string  `json:"id"`
	SkillID   string  `json:"skill_id"`
	Version   string  `json:"version"`
	Scenario  string  `json:"scenario"`  // 场景
	Judgement string  `json:"judgement"` // 判断
	Evidence  string  `json:"evidence"`  // 依据
	Outcome   string  `json:"outcome"`
	Verdict   Verdict `json:"verdict"`
	TS        string  `json:"ts"`
}

// IsVerified 报告该条是否**可当已验证**（SK-7：未回填 ⇒ false）。
func (s Sediment) IsVerified() bool { return s.Verdict == VerdictRight }

// SedimentStore 是**append-only** 的沉淀存储（usage.jsonl）。
//
// ⚠️ append-only：回填 verdict **不改写历史行**，而是**追加一条回填记录**
// （与 `NF-2 append-only` 同一纪律）。
type SedimentStore struct {
	Path string
	mu   sync.Mutex
}

// NewSedimentStore 构造。Path 为 usage.jsonl 的路径。
func NewSedimentStore(path string) *SedimentStore { return &SedimentStore{Path: path} }

// Record 追加一条沉淀（SK-6）。**未回填 ⇒ Verdict 为 pending。**
func (s *SedimentStore) Record(e Sediment) (Sediment, error) {
	if strings.TrimSpace(e.SkillID) == "" {
		return Sediment{}, fmt.Errorf("skill: Record 需要 skill_id（SK-6：沉淀必须可归因）")
	}
	if e.Verdict == "" {
		e.Verdict = VerdictPending
	}
	if e.TS == "" {
		e.TS = time.Now().UTC().Format(time.RFC3339)
	}
	if e.ID == "" {
		e.ID = fmt.Sprintf("sed-%d", time.Now().UnixNano())
	}
	if err := s.append(map[string]any{"kind": "sediment", "entry": e}); err != nil {
		return Sediment{}, err
	}
	return e, nil
}

// BackfillVerdict 回填裁决（SK-7）。**追加一条回填记录**（不改写历史）。
//
// ⚠️ 回填值非法 ⇒ **报错**（不静默当成 right）。
func (s *SedimentStore) BackfillVerdict(id string, v Verdict) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("skill: BackfillVerdict 需要 id")
	}
	switch v {
	case VerdictRight, VerdictWrong:
	case VerdictPending:
		return fmt.Errorf("skill: 回填值不能是 pending（那就是「未回填」）")
	default:
		return fmt.Errorf("skill: 未知裁决 %q ⇒ 判不了，**不当成 right**", v)
	}
	return s.append(map[string]any{"kind": "verdict", "id": id, "verdict": v,
		"ts": time.Now().UTC().Format(time.RFC3339)})
}

// Load 读回全部沉淀（应用回填）。返回按写入顺序。
func (s *SedimentStore) Load() ([]Sediment, error) {
	f, err := os.Open(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	byID := map[string]int{}
	var out []Sediment
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec struct {
			Kind    string          `json:"kind"`
			Entry   Sediment        `json:"entry"`
			ID      string          `json:"id"`
			Verdict Verdict         `json:"verdict"`
			Raw     json.RawMessage `json:"-"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue // 坏行不拖垮整体（append-only 允许尾部半行）
		}
		switch rec.Kind {
		case "sediment":
			out = append(out, rec.Entry)
			byID[rec.Entry.ID] = len(out) - 1
		case "verdict":
			if i, ok := byID[rec.ID]; ok {
				out[i].Verdict = rec.Verdict
			}
		}
	}
	return out, sc.Err()
}

// Pending 返回**未回填**的沉淀（SK-7：它们不得当成"已验证"）。
func (s *SedimentStore) Pending() ([]Sediment, error) {
	all, err := s.Load()
	if err != nil {
		return nil, err
	}
	var out []Sediment
	for _, e := range all {
		if e.Verdict == VerdictPending {
			out = append(out, e)
		}
	}
	return out, nil
}

// CriterionFromWrong 把一条 **verdict=wrong** 的沉淀转成判据（SK-8）。
//
// ⚠️ 非 wrong ⇒ **报错**（SK-8 只对 wrong 生效；不许把 right 也转成判据）。
func CriterionFromWrong(e Sediment) (Criterion, error) {
	if e.Verdict != VerdictWrong {
		return Criterion{}, fmt.Errorf("skill: SK-8 只对 verdict=wrong 生效（实际 %q）", e.Verdict)
	}
	if strings.TrimSpace(e.Scenario) == "" || strings.TrimSpace(e.Judgement) == "" {
		return Criterion{}, fmt.Errorf("skill: 从 wrong 转判据需要 scenario 与 judgement（否则重犯时无法识别）")
	}
	return Criterion{
		ID:      "from-wrong-" + e.ID,
		Skill:   e.SkillID,
		Version: e.Version,
		Field:   "judging",
		Text:    e.Scenario + " ⇒ " + e.Judgement,
		Check:   "manual",
		Manual:  true, // ⚠️ 新判据默认**人工**（不得凭空宣称可机械化）
	}, nil
}

func (s *SedimentStore) append(v any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(s.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return err
}
