// ledger.go -- routeby    (append-only JSONL)+    (VHS-OUTPUT-001 to ). 
//
// owner: "    time use   type……needhas  . "
//  has  ,     ptthenno   : L0  example( = slow)/   rate( =fast type  )/
//   afteris   change (needneed   to , see Aggregate). 
package route

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Record is      (charsegto  VHS-OUTPUT-001). 
type Record struct {
	At             string `json:"at"`
	Task           string `json:"task,omitempty"`
	Level          Level  `json:"level"`
	ModelID        string `json:"model_id,omitempty"`
	Reason         string `json:"reason"`
	Escalated      bool   `json:"escalated"`
	Degraded       bool   `json:"degraded"`
	Outcome        string `json:"outcome"` // answered | escalated | degraded | asked_user
	Kind           Kind   `json:"kind,omitempty"`
	KindFallback   bool   `json:"kind_fallback"`
	RouteAmbiguous bool   `json:"route_ambiguous"`
	// WMCAP   ( then"as      N  "nofromanswer). 
	Capacity    int     `json:"capacity,omitempty"`
	DemandFloor int     `json:"demand_floor,omitempty"`
	Familiarity float64 `json:"familiarity,omitempty"`
	Capped      bool    `json:"capped,omitempty"`
	DropCount   int     `json:"drop_count,omitempty"`
	//   pt③(  afteris change ) needcharseg. 
	// **toaftercompat**:   obj has  charseg ⇒ refer as nil ⇒   tgt unknown, **  cur 0**. 
	TaskID        string   `json:"task_id,omitempty"`
	QualityBefore *float64 `json:"quality_before,omitempty"`
	QualityAfter  *float64 `json:"quality_after,omitempty"`
	OutcomeAfter  string   `json:"outcome_after,omitempty"`
}

// Ledger is append-only   write . 
type Ledger struct {
	Path string
	Now  func() time.Time
}

// Write pipe  decide ( its   obj)  ;   decide write  ,     need safety charseg. 
func (l *Ledger) Write(d Decision, task string, kind Kind) error {
	if l.Path == "" {
		return nil
	}
	now := time.Now
	if l.Now != nil {
		now = l.Now
	}
	kindFallback, routeAmbiguous := false, false
	for _, e := range d.Ledger {
		if e.Reason == "kind_fallback=true" {
			kindFallback = true
		}
		if e.Reason == "route_ambiguous=true" {
			routeAmbiguous = true
		}
	}
	outcome := "answered"
	switch {
	case d.Degraded:
		outcome = "degraded"
	case d.Action == ActionAskUser:
		outcome = "asked_user"
	case d.Action == ActionEscalate:
		outcome = "escalated"
	}
	escalated := false
	for _, e := range d.Ledger {
		if e.Escalated {
			escalated = true
		}
	}
	rec := Record{
		At: now().UTC().Format(time.RFC3339Nano), Task: task, Level: d.Level,
		ModelID: d.ModelID, Reason: d.Reason, Escalated: escalated, Degraded: d.Degraded,
		Outcome: outcome, Kind: kind, KindFallback: kindFallback, RouteAmbiguous: routeAmbiguous,
		Capacity: d.Capacity, DemandFloor: d.DemandFloor, Familiarity: d.Familiarity,
		Capped: d.Capped, DropCount: d.DropCount,
	}
	if err := os.MkdirAll(filepath.Dir(l.Path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(l.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

// Summary is    close (    refertgt). 
type Summary struct {
	Total          int           `json:"total"`
	ByLevel        map[Level]int `json:"by_level"`
	L0Share        float64       `json:"l0_share"`        //   pt①:   =  slow
	EscalationRate float64       `json:"escalation_rate"` //   pt②:   = fast type  
	DegradedRate   float64       `json:"degraded_rate"`
	AskedUserRate  float64       `json:"asked_user_rate"`
	KindFallbacks  int           `json:"kind_fallbacks"`
	RouteAmbiguous int           `json:"route_ambiguous"`
	//   pt③: only  **sametime  beforeafter  **  obj;  charseg  UnknownQuality( cur 0). 
	ComparableQuality int `json:"comparable_quality"`
	UnknownQuality    int `json:"unknown_quality"`
	Improved          int `json:"improved"`
	Worsened          int `json:"worsened"`
}

// Aggregate read JSONL and out  refertgt(  pt③"  afteris change "need   to ,  giveon split ). 
func Aggregate(path string) (Summary, error) {
	s := Summary{ByLevel: map[Level]int{}}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil //    ed = empty  ,   
		}
		return s, err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec Record
		if err := json.Unmarshal(line, &rec); err != nil {
			continue //    ed,  interrupt  
		}
		s.Total++
		s.ByLevel[rec.Level]++
		if rec.Escalated {
			s.EscalationRate++
		}
		if rec.Degraded {
			s.DegradedRate++
		}
		if rec.Outcome == "asked_user" {
			s.AskedUserRate++
		}
		if rec.KindFallback {
			s.KindFallbacks++
		}
		if rec.RouteAmbiguous {
			s.RouteAmbiguous++
		}
		if rec.QualityBefore != nil && rec.QualityAfter != nil {
			s.ComparableQuality++
			switch {
			case *rec.QualityAfter > *rec.QualityBefore:
				s.Improved++
			case *rec.QualityAfter < *rec.QualityBefore:
				s.Worsened++
			}
		} else {
			s.UnknownQuality++ //   obj/ charseg ⇒ unknown,  cur 0
		}
	}
	if s.Total > 0 {
		s.L0Share = float64(s.ByLevel[LevelL0]) / float64(s.Total)
		s.EscalationRate /= float64(s.Total)
		s.DegradedRate /= float64(s.Total)
		s.AskedUserRate /= float64(s.Total)
	}
	return s, sc.Err()
}
