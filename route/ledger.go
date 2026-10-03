// ledger.go —— 路由台账落盘（append-only JSONL）+ 可聚合（VHS-OUTPUT-001 对齐）。
//
// Peter：「大概什么时候用什么模型……要有记录。」
// 没有落盘，三个考察点就无法考察：L0 比例（低=太慢）/ 升级率（高=快模型选错）/
// 升级后是否真的变好（需要跨记录对比，见 Aggregate）。
package route

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Record 是一条台账记录（字段对齐 VHS-OUTPUT-001）。
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
	// WMCAP 留痕（否则"为什么这次抓了 N 个"无从回答）。
	Capacity    int     `json:"capacity,omitempty"`
	DemandFloor int     `json:"demand_floor,omitempty"`
	Familiarity float64 `json:"familiarity,omitempty"`
	Capped      bool    `json:"capped,omitempty"`
	DropCount   int     `json:"drop_count,omitempty"`
}

// Ledger 是 append-only 台账写入器。
type Ledger struct {
	Path string
	Now  func() time.Time
}

// Write 把一次决策（含其台账条目）落盘；每条决策写一行，含聚合所需的全部字段。
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

// Summary 是台账聚合结果（三个考察指标）。
type Summary struct {
	Total          int           `json:"total"`
	ByLevel        map[Level]int `json:"by_level"`
	L0Share        float64       `json:"l0_share"`        // 考察点①：低 = 太慢
	EscalationRate float64       `json:"escalation_rate"` // 考察点②：高 = 快模型选错
	DegradedRate   float64       `json:"degraded_rate"`
	AskedUserRate  float64       `json:"asked_user_rate"`
	KindFallbacks  int           `json:"kind_fallbacks"`
	RouteAmbiguous int           `json:"route_ambiguous"`
}

// Aggregate 读 JSONL 并算出考察指标（考察点③"升级后是否变好"需跨记录对比，留给上层分析）。
func Aggregate(path string) (Summary, error) {
	s := Summary{ByLevel: map[Level]int{}}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil // 没落盘过 = 空台账，不崩
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
			continue // 坏行跳过，不中断聚合
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
	}
	if s.Total > 0 {
		s.L0Share = float64(s.ByLevel[LevelL0]) / float64(s.Total)
		s.EscalationRate /= float64(s.Total)
		s.DegradedRate /= float64(s.Total)
		s.AskedUserRate /= float64(s.Total)
	}
	return s, sc.Err()
}
