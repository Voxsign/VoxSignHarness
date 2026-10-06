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

// sediment.go --     **   **(SK-6 / SK-7 / SK-8). 
//
//  data `tasks/VHS-SKILL-001-   connect   .md:137-139`: 
//
//	SK-6  ⭐ **  calluse       **(skill_id/version/ scenario/ disconnect/ data/outcome)
//	SK-7  `verdict`  backfill ⇒     tgt as **pending**, **  curbecome"already  "**
//	SK-8  `verdict=wrong` ⇒ **    become   data**( thensame    heavy )
//	(:141"**SK-5 and SK-8 is     close ** --   pipe"  "from"    "changebecome"**  approve   **"")
//
// ⚠️ **SK-7 and"   !=  ed"issame  origthen**, onlyisto  same
// ( placemanage**     obj**; deliver   pending semantic   /   ,    file). 
//
// ⚠️ nowstatus(2026-10-03   ): `skill/`  ** occurproducecalluse**(  )⇒ this layer  after** nocalluse **
// (and `KnowhowFromRaw` same place ). ** is"approve  ",  is"useon". **

// Verdict is   obj **backfill decide**. 
//
// ⚠️  value `VerdictPending` = ** backfill** ⇒ by SK-7, **  curbecome"already  "**. 
type Verdict string

// ⚠️ getvalue**  rule **(`tasks/VHS-SKILL-001:96`): `correct|wrong|unverified`
//
//	  init  become `right|wrong|pending` -- **andrule   **(andpipe `pending`     to verdict on, 
//	but `pending` is **`outcome`**  getvalue). 
const (
	VerdictUnverified Verdict = "unverified" // ** backfill(defaultvalue)**
	VerdictCorrect    Verdict = "correct"
	VerdictWrong      Verdict = "wrong"
)

// Sediment is    (SK-6  charseg ). 
// ⚠️ **charsegnameandclasstype  rule **(`tasks/VHS-SKILL-001:93-99`   jsonl kindexample): 
//
//	{"at":…, "skill_id":…, "skill_version":…, "scenario":…, "judgement":…,
//	 "basis":[…], "outcome":…, "verdict":…, "evidence_ref":…}
//
//   init  become ts/version/evidence(string)/no evidence_ref ⇒ **5 place diff**, alreadybyrule modifyback. 
type Sediment struct {
	ID           string   `json:"id"` // base in   (rule kindexample  ; useatbackfill  )
	At           string   `json:"at"`
	SkillID      string   `json:"skill_id"`
	SkillVersion string   `json:"skill_version"`
	Scenario     string   `json:"scenario"`
	Judgement    string   `json:"judgement"`
	Basis        []string `json:"basis"` // **num **( datalisttable)
	Outcome      string   `json:"outcome"`
	Verdict      Verdict  `json:"verdict"`
	EvidenceRef  string   `json:"evidence_ref"`
}

// Outcome  getvalue(rule  `:95`): `adopted|revised|rejected|pending`. 
const (
	OutcomeAdopted  = "adopted"
	OutcomeRevised  = "revised"
	OutcomeRejected = "rejected"
	OutcomePending  = "pending"
)

// ValidOutcome    outcome is  rule   in. 
func ValidOutcome(o string) bool {
	switch o {
	case OutcomeAdopted, OutcomeRevised, OutcomeRejected, OutcomePending:
		return true
	}
	return false
}

// IsVerified     is ** curalready  **(SK-7:  backfill ⇒ false). 
func (s Sediment) IsVerified() bool { return s.Verdict == VerdictCorrect }

// SedimentStore is**append-only**    storestore(usage.jsonl). 
//
// ⚠️ append-only: backfill verdict ** modifywrite   **, butis**    backfill  **
// (and `NF-2 append-only` same   ). 
type SedimentStore struct {
	Path string
	mu   sync.Mutex
}

// NewSedimentStore   . Path as usage.jsonl  path. 
func NewSedimentStore(path string) *SedimentStore { return &SedimentStore{Path: path} }

// Record       (SK-6). ** backfill ⇒ Verdict as pending. **
func (s *SedimentStore) Record(e Sediment) (Sediment, error) {
	if strings.TrimSpace(e.SkillID) == "" {
		return Sediment{}, fmt.Errorf("skill: Record 需要 skill_id（SK-6：沉淀必须可归因）")
	}
	if e.Verdict == "" {
		e.Verdict = VerdictUnverified //  backfill(rule default)
	}
	if e.At == "" {
		e.At = time.Now().UTC().Format(time.RFC3339)
	}
	// ⚠️ outcome    rule   in(   by  ⇒    " has   "no   )
	if e.Outcome != "" && !ValidOutcome(e.Outcome) {
		return Sediment{}, fmt.Errorf("skill: outcome %q 不在规范枚举内（adopted|revised|rejected|pending）", e.Outcome)
	}
	if e.ID == "" {
		e.ID = fmt.Sprintf("sed-%d", time.Now().UnixNano())
	}
	if err := s.append(map[string]any{"kind": "sediment", "entry": e}); err != nil {
		return Sediment{}, err
	}
	return e, nil
}

// BackfillVerdict backfill decide(SK-7). **    backfill  **( modifywrite  ). 
//
// ⚠️ backfillvalue   ⇒ **  **(   curbecome right). 
func (s *SedimentStore) BackfillVerdict(id string, v Verdict) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("skill: BackfillVerdict 需要 id")
	}
	switch v {
	case VerdictCorrect, VerdictWrong:
	case VerdictUnverified:
		return fmt.Errorf("skill: 回填值不能是 unverified（那就是「未回填」）")
	default:
		return fmt.Errorf("skill: 未知裁决 %q ⇒ 判不了，**不当成 correct**", v)
	}
	return s.append(map[string]any{"kind": "verdict", "id": id, "verdict": v,
		"ts": time.Now().UTC().Format(time.RFC3339)})
}

// Load readbacksafety   ( usebackfill). returnbackbywrite  . 
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
			continue //       body(append-only  allowtail   )
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

// Pending returnback** backfill**   (SK-7:     curbecome"already  "). 
func (s *SedimentStore) Pending() ([]Sediment, error) {
	all, err := s.Load()
	if err != nil {
		return nil, err
	}
	var out []Sediment
	for _, e := range all {
		if e.Verdict == VerdictUnverified {
			out = append(out, e)
		}
	}
	return out, nil
}

// CriterionFromWrong pipe   **verdict=wrong**     become data(SK-8). 
//
// ⚠️   wrong ⇒ **  **(SK-8 onlyto wrong occur ;  allowpipe right also become data). 
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
		Version: e.SkillVersion,
		Field:   "judging",
		Text:    e.Scenario + " ⇒ " + e.Judgement,
		Check:   "manual",
		Manual:  true, // ⚠️ new datadefault**human**(   empty called   ize)
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
