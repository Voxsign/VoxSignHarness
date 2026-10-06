// evidence.go -- `basis`  data ( data first )+ `verified`  **  define**. 
//
// ⭐  decide(Lead 2026-10-03): **`verified` only by"already    check"produceoccur; calluse  headgive    `hearsay`. **
//  now   usebase obj has    : **classtypeonwrite out ** -- `kind` charseg**  out**, 
// out  only  ed     produceoccur Claim, **no    verified**. 
package skill

import "fmt"

// provenance is data  etc (**  out** ⇒ out only     produceoccur). 
type provenance int

const (
	provClaimed  provenance = iota //  headcalled( calluse  called"already  ")
	provExecuted                   // **already    check** produceoccur
)

// Claim is   data  . 
type Claim struct {
	kind      provenance //   out: out write out verified
	Executed  bool       // by   decide 
	CheckID   string     // already   check  tgt (Executed=true time empty)
	Traceable bool       // is      (URL/file/  )
	// paradigm **  out**: by Lead  decide, `Paradigm` and `verified` same path --
	// **only by"already    check"produceoccur**;  head  " isnew form"    . 
	paradigm bool
	Detail   string
}

// ClaimedByCaller only produceoccur hearsay(**no calluse    called**). 
func ClaimedByCaller(detail string) Claim {
	return Claim{kind: provClaimed, Executed: false, Detail: detail}
}

// ExecutedCheck is**unique** produceoccur verified  path(needgiveout check tgt ). 
func ExecutedCheck(checkID string, passed bool, detail string) (Claim, error) {
	if checkID == "" {
		return Claim{}, fmt.Errorf("skill: ExecutedCheck 需要 checkID（verified 必须有来源）")
	}
	c := Claim{kind: provExecuted, Executed: true, CheckID: checkID, Detail: detail}
	if !passed {
		c.kind = provClaimed //   ed ⇒   already  
		c.Executed = false
	}
	return c, nil
}

// IsVerified      is is**already   check produceoccur **. 
func (c Claim) IsVerified() bool { return c.kind == provExecuted && c.Executed }

// WithTraceable only  "   "tgt (  and verified/paradigm  produceoccurrule). 
func (c Claim) WithTraceable(v bool) Claim { c.Traceable = v; return c }

// RankByEvidence  now basis  data (**   now,  placeoccur **): 
//
//	①       >     
//	② already   check >  headcalled
//	③ already   >   called
//	④  form first(    defaultpos ity, only its   timeoccur )
//
// returnback    ; `degraded=true` tableshow** has  already   data**(only   head/ form). 
func RankByEvidence(claims []Claim) (Claim, bool) {
	if len(claims) == 0 {
		return Claim{}, true
	}
	best := claims[0]
	for _, c := range claims[1:] {
		if better(c, best) {
			best = c
		}
	}
	return best, !best.IsVerified()
}

// IsParadigm      is **  check   **as form. 
func (c Claim) IsParadigm() bool { return c.paradigm }

// ExecutedParadigmCheck and ExecutedCheck same path: ** form  also    already    check**. 
func ExecutedParadigmCheck(checkID string, passed bool, detail string) (Claim, error) {
	c, err := ExecutedCheck(checkID, passed, detail)
	if err != nil {
		return Claim{}, err
	}
	if passed {
		c.paradigm = true
	}
	return c, nil
}

// ruleAppliers pipe** rulename**  to**       num**( thenis"  "): 
//   tablegiveoutrulename ⇒      hasto  now, ruleonly  modifychange  . 
var ruleAppliers = map[string]func(a, b Claim) int{
	RuleExecuted: func(a, b Claim) int { // ② already   >  head
		if a.IsVerified() == b.IsVerified() {
			return 0
		}
		if a.IsVerified() {
			return 1
		}
		return -1
	},
	RuleVerified: func(a, b Claim) int { // ③ already   >   called(same ②, semanticdiffname)
		if a.IsVerified() == b.IsVerified() {
			return 0
		}
		if a.IsVerified() {
			return 1
		}
		return -1
	},
	RuleTraceable: func(a, b Claim) int { // ①     >     
		if a.Traceable == b.Traceable {
			return 0
		}
		if a.Traceable {
			return 1
		}
		return -1
	},
	RuleParadigm: func(a, b Claim) int { // ④  form first(only  check   )
		if a.paradigm == b.paradigm {
			return 0
		}
		if a.paradigm {
			return 1
		}
		return -1
	},
}

// RulesFromMapping onlyreturnback**  become ** rulename(manual  **  and**   --  allow     ). 
func RulesFromMapping(ms []BasisMapping) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range ms {
		for _, mp := range m.Mapped {
			if !seen[mp.Rule] {
				seen[mp.Rule] = true
				out = append(out, mp.Rule)
			}
		}
	}
	return out
}

// RankByEvidenceWith use**give rule **  (rules asempty ⇒   use  rule, keepkeeporig ). 
//  is"   ⇒   "   pt:   out  rule,    then  use  . 
func RankByEvidenceWith(rules []string, claims []Claim) (Claim, bool) {
	if len(claims) == 0 {
		return Claim{}, true
	}
	best := claims[0]
	for _, c := range claims[1:] {
		for _, r := range rules {
			fn, ok := ruleAppliers[r]
			if !ok {
				continue //   now rulename    ( allow  occur )
			}
			if fn(c, best) > 0 {
				best = c
				break
			}
			if fn(c, best) < 0 {
				break
			}
		}
	}
	return best, !best.IsVerified() // defaultsafetyrule(and RankByEvidenceWith safetyruleetc )
}

func better(a, b Claim) bool {
	// ②③ already  /already   first(  )
	if a.IsVerified() != b.IsVerified() {
		return a.IsVerified()
	}
	// ①     first
	if a.Traceable != b.Traceable {
		return a.Traceable
	}
	// ④  form first(**onlyhas  check     form**only and;  head called  )
	if a.paradigm != b.paradigm {
		return a.paradigm
	}
	return false
}
