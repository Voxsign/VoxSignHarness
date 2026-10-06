// Package risk isconfirm    (   v2 §4): use signal(reversibleity/  face/   )
// pipe  intent decideas auto|light|strong|human   confirm  . 
//   face   signal(filebe use num/heat/  overwrite),    type  ; 
//  reversibleis rule,   humanconfirm, and  be   cache   . 
// this packageonlydependencytgtapprove  + contract. 
package risk

// [pseudocode logic layer](review gateartifact;  disconnectsemantic authoritative definition    v2 §4 / VSL  disconnectsemantic , 
//  this layeronlydescribe modulecontrol flow/branch/errorpath,  heavy rulebody--rule semanticstgtnote"  VSL". )
//
// module  :  in=intent(   : is reversible)+   signal ImpactInput; 
//    out=Decision.Level(auto|light|strong|human)+    Reason. 
//
// StaticImpact(in ImpactInput) -> string(  VSL:   face valuedefine): 
//   base = small
//   if in.RefCount >= 8:            base = high
//   else if in.RefCount >= 3:       base = medium
//   else if in.Heat >= 10:          base = medium   // heat but use ,  byin  handle(v2  ize)
//   return base
//
// Evaluate(intent, imp) -> Decision: 
//   1.  reversible  forbid( first   , no its signal;   VSL:  reversibleintentlist): 
//      if intent == COMMIT or intent == DEPLOY:            return human
//      if intent == EDIT and intent.Params["action"]=="delete": return human
//      if intent.Space == "vault-creds":                   return human  //      
//   2. reversibleitygetvalue(  VSL): 
//      reversible = true(default); intent.Risk  emptytimeget intent.Risk.Reversible
//      if reversible == false: return human   // baselinealreadytgt reversible, samekind  forbid
//   3.   face = StaticImpact(imp)(   ,  read intent.Risk.Impact   type  )
//   4.     = intent.Risk.Confidence   0 useof,  then intent.Confidence;  value 0.7
//   5. decide   (  VSL    v2 §4 table): 
//      switch impact:
//        case small:
//           if conf >= 0.7: level=auto  reason="reversible+ + :     +tgt   "
//           else:           level=auto  reason="reversible+ + :     +   +back   "
//        case medium: level=light reason="reversible+in:  show diff  need-> confirm"
//        case high:   level=strong reason="reversible+ : diff   +  split -> confirm"
//   error: intent.Risk as nil -> reversible=true,    back  intent.Confidence,    (   ). 
//
// Guard(confirm  preventprotect,   VSL    v2 §4): 
//   ShouldDowngrade(path) -> bool: 
//      // uniquein :    base  confirmpath, again decideis   . 
//      if path == g.lastPath: g.counts[path]++ else g.counts[path]=1
//      g.lastPath = path
//      return g.counts[path] > 3   // samepathlinkcontinue >3   confirm ->  as"     "

import (
	"voicesign-harness/contract"
)

// ImpactInput is  face  signal(   type  ;    v2 §4). 
type ImpactInput struct {
	RefCount int  // filebe use num
	HasTest  bool // is has  overwrite
	Heat     int  //   changeheat(  N  change num)
}

// Signals is and decide  signal   state(reversibleity rule +     face +     signal). 
type Signals struct {
	Revertible bool
	Impact     string
	Confidence float64
}

// Decision is decideclose : Level ∈ auto|light|strong|human, Reason   resolve (back /  use). 
type Decision struct {
	Level  string
	Reason string
}

// confHigh is"   " value( signal, onlydecide  auto inis   ,     diff). 
const confHigh = 0.7

// StaticImpact by  signal   face: RefCount>=8->high; >=3->medium; heat>=10 patch to medium;  then small. 
func StaticImpact(in ImpactInput) string {
	switch {
	case in.RefCount >= 8:
		return contract.ImpactHigh
	case in.RefCount >= 3:
		return contract.ImpactMedium
	case in.Heat >= 10:
		return contract.ImpactMedium
	default:
		return contract.ImpactSmall
	}
}

// irreversible    intentis  in reversible  forbid(COMMIT/DEPLOY/delete/   ). 
//   VSL:  reversibleintentlist(COMMIT/DEPLOY/DELETE/outsend/  ),   be   . 
func irreversible(it *contract.Intent) bool {
	switch it.Intent {
	case contract.IntentCommit, contract.IntentDeploy:
		return true
	case contract.IntentEdit:
		if it.Params != nil && it.Params["action"] == "delete" {
			return true
		}
	}
	if it.Space == "vault-creds" {
		return true
	}
	return false
}

// Evaluate by signaldecide    decideconfirmetc .  reversible -> human(  forbid, firstat  ). 
func Evaluate(it contract.Intent, imp ImpactInput) Decision {
	// 1.  reversible  forbid
	if irreversible(&it) {
		return Decision{Level: contract.ConfirmHuman, Reason: "不可逆动作（提交/部署/删除/外发/凭证）→ 永远人工确认"}
	}
	// 2. reversibleity(baselinetgt reversiblesamekind  forbid)
	reversible := true
	conf := it.Confidence
	if it.Risk != nil {
		reversible = it.Risk.Reversible
		if it.Risk.Confidence != 0 {
			conf = it.Risk.Confidence
		}
	}
	if !reversible {
		return Decision{Level: contract.ConfirmHuman, Reason: "意图基线标不可逆 → 永远人工确认"}
	}
	// 3.     face + 5. decide   
	switch StaticImpact(imp) {
	case contract.ImpactMedium:
		return Decision{Level: contract.ConfirmLight, Reason: "可逆+中影响：展示 diff 摘要 → 轻确认"}
	case contract.ImpactHigh:
		return Decision{Level: contract.ConfirmStrong, Reason: "可逆+高影响：diff 预览+影响分析 → 强确认"}
	default: // small
		if conf >= confHigh {
			return Decision{Level: contract.ConfirmAuto, Reason: "可逆+小+高置信：自动执行 + 标待抽查"}
		}
		return Decision{Level: contract.ConfirmAuto, Reason: "可逆+小+低置信：自动执行 + 待抽查 + 回执高亮"}
	}
}

// Guard isconfirm  preventprotect :   samepathlinkcontinue confirm num,  limiti.e.  as     . 
type Guard struct {
	lastPath string
	counts   map[string]int
}

// NewGuard   empty Guard. 
func NewGuard() *Guard {
	return &Guard{counts: map[string]int{}}
}

// ShouldDowngrade     to path   confirm, and decidesamepathlinkcontinue confirmis already >3  . 
// returnback true tableshowbase   as"     ",  again time disconnect. 
func (g *Guard) ShouldDowngrade(path string) bool {
	if path == "" {
		path = "<empty>"
	}
	if path == g.lastPath {
		g.counts[path]++
	} else {
		g.counts[path] = 1
	}
	g.lastPath = path
	return g.counts[path] > 3
}
