// model.go --    ·   cachenumdata type(   v1.0 §12 charseg  ly). 
//
//      type(objtgt/rule/ restrict/ as)+    obj(STM/LTM)+ calluseday . 
// charsegand   §12   : version/superseded_by/confidence/source_trajectory, 
//  because  importance/recency/relevance, domain domain(user|session|agent). 
package zhiji

import (
	"encoding/json"
	"time"
)

// Layer    type  (   §6.1). 
type Layer string

const (
	LayerGoal      Layer = "goal"      // objtgt : curbefore objtgt,  periodintent,  first 
	LayerRule      Layer = "rule"      // rule :    end,  value ,  boundary,   changein 
	LayerMechanism Layer = "mechanism" //  restrict : be  has    /  
	LayerBehavior  Layer = "behavior"  //  as : useuser  ,  as  ,    example
)

// Domain     domain(   §12: user/session/agent   ). 
type Domain string

const (
	DomainUser    Domain = "user"    //    useuser ( keep )
	DomainSession Domain = "session" //    in
	DomainAgent   Domain = "agent"   //       
)

// Status    objstatus(   §12: active|superseded|decayed). 
type Status string

const (
	StatusActive     Status = "active"
	StatusSuperseded Status = "superseded" // benew base  ,    overwrite
	StatusDecayed    Status = "decayed"    //     (   ,    )
)

// SelfItem      type obj(   §12 charseg ). 
type SelfItem struct {
	ID               string    `json:"id"`
	Layer            Layer     `json:"layer"`
	Text             string    `json:"text"`
	Version          int       `json:"version"`
	SupersededBy     string    `json:"superseded_by,omitempty"`
	Status           Status    `json:"status"` // active|superseded|decayed(and MemoryItem samelang ; new  objdefault active)
	UpdatedAt        time.Time `json:"updated_at"`
	SourceTrajectory string    `json:"source_trajectory,omitempty"` //   trace use(   )
	Confidence       float64   `json:"confidence"`                  // 0–1, out signalsplit 
}

// MemKind  class  classtype(   v1.1 §6.1: writesideheavy  split,    ). 
type MemKind string

const (
	MemKindEpisodic    MemKind = "episodic"    // casenode  :  timetime  event  
	MemKindSemantic    MemKind = "semantic"    // semantic  :   /  /  
	MemKindProcedural  MemKind = "procedural"  //     :    /  /  
	MemKindPreference  MemKind = "preference"  //     :    /   / value 
)

// Provenance close ize  chain(   v1.1 §6.1:  produceoccur/  /   type/   /on  ID). 
// and   Source string andstore: Source keep  reflect:stm   (SyncVault dependency), Prov ispatchfill. 
type Provenance struct {
	Origin      string   `json:"origin,omitempty"`       // user_input|tool_call|reflect|feedback|imported
	TurnID      string   `json:"turn_id,omitempty"`
	Model       string   `json:"model,omitempty"`
	Confidence  float64  `json:"confidence,omitempty"`    // 0–1
	DerivedFrom []string `json:"derived_from,omitempty"`  // on   / obj ID

}

// MemoryItem      obj(STM    / LTM    use). 
type MemoryItem struct {
	ID         string    `json:"id"`
	Text       string    `json:"text"`
	Layer      Layer     `json:"layer,omitempty"` //  restrict/ as   
	Domain     Domain    `json:"domain"`
	Status     Status    `json:"status"`
	Importance float64   `json:"importance"` // 1–10
	CreatedAt  time.Time `json:"created_at"`
	LastSeen   time.Time `json:"last_seen"`
	Hash       string    `json:"hash,omitempty"` // writebefore   heavyuse
	Source     string    `json:"source,omitempty"`
	// v1.1 newadd(safety  omitempty,   JSON  valuecompat;  modify Importance semanticand because  form)
	Kind      MemKind     `json:"kind,omitempty"`        //  class   split(tgtnotelist,  modify has  )
	KindScore float64     `json:"kind_score,omitempty"`  // 0–1,   back  Importance/10
	Entities  []string    `json:"entities,omitempty"`     //  body get( codeposthen, P1)
	Keywords  []string    `json:"keywords,omitempty"`     // word   in (P1)
	Embedding []float32   `json:"embedding,omitempty"`   // Phase1.5  , first empty
	Prov      *Provenance `json:"prov,omitempty"`        // close ize  chain
}

// Relevance   curbefore query   closeity(   §12:  because  split inof ). 
// by contract.go    sidecalluse;  place providecharseg default now(token heavy  + close word in). 
func (m *MemoryItem) Relevance(query string) float64 {
	if query == "" || m.Text == "" {
		return 0
	}
	//   heavy  : query in linkcontinueword  text inoutnow  example(0–1). 
	qWords := splitWords(query)
	if len(qWords) == 0 {
		return 0
	}
	hit := 0
	for _, w := range qWords {
		if containsFold(m.Text, w) {
			hit++
		}
	}
	return float64(hit) / float64(len(qWords))
}

// CallLog   calluseday (   §12: safety  pt,      orig ). 
type CallLog struct {
	ID          string    `json:"id"`
	At          time.Time `json:"at"`
	TaskProfile string    `json:"task_profile"`
	Model       string    `json:"model"`
	Outcome     string    `json:"outcome"` // success|failed|retried|judge
	Cost        float64   `json:"cost"`    //  tobecomebase(token   )
	Retry       int       `json:"retry"`
	DetailProbe string    `json:"detail_probe,omitempty"` // detail survival    base
}

// SurvivalProbe detail survival   (   §12:      close  node, compaction after  back). 
type SurvivalProbe struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	KeyDetail string    `json:"key_detail"` //       close  node
	Slot      string    `json:"slot"`       //    (e.g. "call:retry:reason")
	Recalled  bool      `json:"recalled"`   // compaction afteris    back
}

// Marshal    JSON  listizein (day /storestore use). 
func (c *CallLog) Marshal() []byte {
	b, _ := json.Marshal(c)
	return b
}

func splitWords(s string) []string {
	var out []string
	var cur []rune
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '，' || r == '。' || r == '、' || r == '？' || r == '！' {
			if len(cur) > 0 {
				out = append(out, string(cur))
				cur = nil
			}
			continue
		}
		cur = append(cur, r)
	}
	if len(cur) > 0 {
		out = append(out, string(cur))
	}
	return out
}

func containsFold(s, sub string) bool {
	return len(sub) > 0 && containsFoldImpl(s, sub)
}

// containsFoldImpl   write       disconnect(in  accept  ,   wordroot    ). 
func containsFoldImpl(s, sub string) bool {
	ls, lsub := toLower(s), toLower(sub)
	return len(ls) >= len(lsub) && containsSeq(ls, lsub)
}

func toLower(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		out = append(out, r)
	}
	return string(out)
}

func containsSeq(s, sub string) bool {
	rs, rsub := []rune(s), []rune(sub)
	for i := 0; i+len(rsub) <= len(rs); i++ {
		eq := true
		for j := range rsub {
			if rs[i+j] != rsub[j] {
				eq = false
				break
			}
		}
		if eq {
			return true
		}
	}
	return false
}

