// systemone.go --    · System One control   (   v1.1 §6.3 / G4  ly). 
//
// pipe reflect.go    code   /classify/routeby/stopstop/writebefore   becomeconnect ; 
// DefaultSystemOne useandnowhas  samevalue default now, Tick/Verify  as   change(32/32 disconnectlangsafety before ). 
// P2   Qwen3-4B+LoRA:  has type out  AskNoul/AskChoice hasboundaryverify,   back basedefault now. 
//
// close   : Interval/MaxIdle/Threshold onlystore  (  DefaultSystemOne  ), 
// Reflector  ed name in *DefaultSystemOne     (r.Interval == r.DefaultSystemOne.Interval), 
//   modify r.Threshold / r.MaxIdle / SetInterval time r.Gate readto issame  value,     . 
package zhiji

import (
	"math"
	"strings"
	"time"
)

// SystemOne slow   fast control (System One). 
type SystemOne interface {
	// Gate  in   seg: returnback(skip, idleForced, deep)
	Gate(idle time.Duration, importance float64) (skip, idleForced, deep bool)
	// Classify writeside classheavy  split(episodic/semantic/procedural/preference)
	Classify(item MemoryItem) map[MemKind]float64
	// Route   routeby:       rate +     
	Route(query string) (views map[EdgeRel]float64, hops int)
	// Assess  data  : returnback(enough, lowValue) stopstopdecide 
	Assess(evidence []MemoryItem) (enough, lowValue bool)
	// VerifyMemory/VerifySelf writebefore  (connect izenowhas reflect.go startsendform)
	VerifyMemory(m MemoryItem) bool
	VerifySelf(s SelfItem) bool
}

// DefaultSystemOne rule defaultcontrol . 
// Interval/MaxIdle/Threshold/DeepMinImp and reflect.go nowhas  samevalue--
// Reflector in baseclose after, Tick  asand day    . 
type DefaultSystemOne struct {
	Threshold  float64       //  rev   heavyneedity value(=DefaultThreshold 30)
	MaxIdle    time.Duration //   empty keepbot(=DefaultMaxIdle 3600s)
	Interval   time.Duration //   node (=DefaultInterval 60s;  call 60–600s)
	DeepMinImp float64       //  rev   underlimit(=DeepReflectMinImp 6.0)
}

// Gate  segsemantic(and reflect.go Tick nowhas switch   to ): 
//
//	idle > MaxIdle   -> idleForced(keepbot  ,   rev )
//	idle >= Interval -> skip( ed  node no in)
//	 then             -> shallow + importance>=Threshold -> deep
func (d *DefaultSystemOne) Gate(idle time.Duration, importance float64) (skip, idleForced, deep bool) {
	if idle > d.MaxIdle {
		return false, true, false
	}
	if idle >= d.Interval {
		return true, false, false
	}
	return false, false, importance >= d.Threshold
}

// timetimestatuslangwordtable( its  -> episodic / temporal    in). 
var timeWords = []string{"昨天", "今天", "上周", "刚才", "刚"}

// Classify rule  class split(P0 frozenrule, P2   Noul  type): 
//
//	 "   /   /  "           -> preference=0.8
//	 "  /e.g. /  /first...again/   " -> procedural=0.8
//	 timetimeword( day/ day/on / only/ ) -> episodic=0.7
//	 then                             -> semantic=0.6
//	its class 0.1  bot. 
func (d *DefaultSystemOne) Classify(item MemoryItem) map[MemKind]float64 {
	text := item.Text
	out := map[MemKind]float64{
		MemKindEpisodic:   0.1,
		MemKindSemantic:   0.1,
		MemKindProcedural:  0.1,
		MemKindPreference: 0.1,
	}
	fired := false
	if hasAny(text, []string{"我喜欢", "我讨厌", "偏好"}) {
		out[MemKindPreference] = 0.8
		fired = true
	}
	// procedural:   /e.g. /  /   ; orsametime "first"and"again"(  posthen first.*again)
	if hasAny(text, []string{"怎么", "如何", "步骤", "第一步"}) ||
		(strings.Contains(text, "先") && strings.Contains(text, "再")) {
		out[MemKindProcedural] = 0.8
		fired = true
	}
	if hasAny(text, timeWords) {
		out[MemKindEpisodic] = 0.7
		fired = true
	}
	if !fired {
		out[MemKindSemantic] = 0.6
	}
	return out
}

// Route rule routeby(P0 frozenrule): 
//
//	 timetimeword                       -> temporal=0.7
//	 "as  /  /becauseas/ by"      -> causal=0.7
//	  body bodyname( write  word or in  name  >=2)-> entity=0.6
//	 then                           -> semantic=0.6
//	hops=3.     sametime  . 
func (d *DefaultSystemOne) Route(query string) (views map[EdgeRel]float64, hops int) {
	views = map[EdgeRel]float64{}
	fired := false
	if hasAny(query, timeWords) {
		views[EdgeRelTemporal] = 0.7
		fired = true
	}
	if hasAny(query, []string{"为什么", "导致", "因为", "所以"}) {
		views[EdgeRelCausal] = 0.7
		fired = true
	}
	if hasEntityName(query) {
		views[EdgeRelEntity] = 0.6
		fired = true
	}
	if !fired {
		views[EdgeRelSemantic] = 0.6
	}
	return views, 3
}

// Assess rule  data  (P0  ize ): 
//
//	evidence asempty              -> lowValue=true(again also use)
//	evidence >=3             -> enough=true( data )
//	its                       -> continuecontinue 
func (d *DefaultSystemOne) Assess(evidence []MemoryItem) (enough, lowValue bool) {
	if len(evidence) == 0 {
		return false, true
	}
	if len(evidence) >= 3 {
		return true, false
	}
	return false, false
}

// VerifyMemory writebefore  ( startsendform split: empty basereject; Store  close heavy   Reflector.verifyMemory). 
func (d *DefaultSystemOne) VerifyMemory(m MemoryItem) bool {
	return strings.TrimSpace(m.Text) != ""
}

// VerifySelf writebefore  ( startsendform split: empty basereject; same        Reflector.verifySelf). 
func (d *DefaultSystemOne) VerifySelf(s SelfItem) bool {
	return strings.TrimSpace(s.Text) != ""
}

// ---- hasboundary outverify(P2 use; verify   -> back  DefaultSystemOne + write decision JSONL fallback)----

// AskNoul verify     rate ∈[0,1] and  NaN;    i.e. false(calluse back rule). 
func AskNoul(p float64) bool {
	return !math.IsNaN(p) && p >= 0 && p <= 1
}

// AskChoice verifymutexclassifysplit :       NaN, and  and~=1( diff 0.05). 
// note : base numonlyuseat P2  type out  Choice split ; Route()      heavyand  as 1. 
func AskChoice(dist map[EdgeRel]float64) bool {
	sum := 0.0
	for _, v := range dist {
		if v < 0 || math.IsNaN(v) {
			return false
		}
		sum += v
	}
	return math.Abs(sum-1.0) < 0.05
}

// hasAny   write   ly disconnect s is    subs in   seg(in  accept  ). 
func hasAny(s string, subs []string) bool {
	for _, sub := range subs {
		if containsFold(s, sub) {
			return true
		}
	}
	return false
}

// hasEntityName    body: onlycur query   write  word(   namesignal, e.g. OpenAI/VHS/GPT)time  body. 
// P0   startsendform; P1  posthen/NER  name body get. 
// note :  againpipe"   rune   >=2  in  seg" as body--in noempty splitword,  kind  semantic botbranchto hasin       
// (and Route"defaultnosignal->semantic"    ). 
func hasEntityName(query string) bool {
	for _, r := range query {
		if r >= 'A' && r <= 'Z' {
			return true
		}
	}
	return false
}
