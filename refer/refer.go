// Package refer iscoreference resolution (   v2 §14): pipe lang    table (" /  file/on ")
//    resolveas body body: word  (100%) -> onunder rule  -> langlang (  ) -> low-confidenceclarification. 
// rule low-confidence      : need   ModelFn, need  form Ask;  domain    clarification"  domain ". 
// this packageonlydependencytgtapprove  + contract + memory. 
package refer

// [pseudocode logic layer](review gateartifact; split  resolveauthoritative definition    v2 §14.1, 
//  this layeronlydescribe modulecontrol flow/branch/rejectpath/errorhandle, rule semanticstgtnote"  VSL". )
//
// Resolve(intent, spaceID) -> *Intent: 
//   0. if intent.Target.Entity already form(explicit): only word  rule ize,  connectreturnback( clarification). 
//   1. word  (  VSL:  bodyword  100%): 
//        CorrectedText in inword  Term/Variant -> Target.Entity=Term, RefType="dict", returnback. 
//   2. coreferencetriggersend  (  VSL:  /  file/  /on /ofbefore  ): 
//      ifnocoreferenceword -> noneed resolve, origkindreturnback. 
//   3. onunder rule (  VSL:  usedomainin   body): 
//      cands = r.Recent by kind ed ("  file"->file; " "-> first file,  bot  )
//         : (space==spaceID  first, Ts   =newer first)
//      if cands empty ->   4(langlang /Ask)
//      if   unique(spaceID  in or new Ts    first)-> getofas Target, RefType="anaphora", returnback
//      if cands     space andno spaceID  in( domain  )-> Ask"   is  domain "
//      if top   (same space same Ts /    andlist)-> Ask,   
//   4. langlang (  ,   VSL:   +   ): 
//      if r.ModelFn != nil:
//         entity, conf = r.ModelFn(CorrectedText,    bodyname)
//         if conf >=  value: Target=entity, RefType="model", returnback
//      // rule low-confidence allow    :  to     Ask
//      Ask"   "< word>"refer is   "
//   error: Dict as nil -> word   ed(   ); Recent asempty ->  connect langlang /Ask. 
//
// Solidify(entity, variant) -> error: 
//   confirmafter diffname ize   word (memory.Dictionary.AddTerm, source=model). 
//
// ResolveOptions(intent, spaceID) -> (*Intent, []Option, error)[M4   by ize]: 
//   // and Resolve same  split ;   clarificationtime outproduceoutclose ize  (2-4  , providemobile clientpt continue ). 
//        first (  VSL:   ->  ): 
//     1) word  in :  base in word  Term -> Option{ID:"dict:"+term, Label:term+"(word )"}
//     2) onunder    body: r.Recent by (spaceID  first, Ts new)   aftergetbefore N
//        -> Option{ID:"rec:"+entity, Label:kind before + body+( domaintimetgtnote domain:xxx)}
//   num onlimit 4; by ID  heavy; word    before,    body after. 
//   only [  clarificationbranch]produceout  :  domain   / top   . 
//   objtgt store orno  (Recent empty + word no in)-> Options=[], only Ask  base(     ). 
//   Resolve keep frozen signature, in  callbase numand   Options. 

import (
	"sort"
	"strings"

	"voicesign-harness/contract"
	"voicesign-harness/memory"
)

// RecentEntity is pipeline notein    body(bytimetime  ; coreference resolution onunder   ). 
type RecentEntity struct {
	Space  string //   domain
	Entity string //  body body(filename/ objname/ name)
	Kind   string // file | project | person | space
	Ts     string // ISO timetime (char    i.e."changenew", needrequirecalluse give char      form)
}

// Resolver iscoreference resolution : word   + onunder rule  +   langlang . 
type Resolver struct {
	Dict    *memory.Dictionary
	Recent  []RecentEntity
	ModelFn func(q string, cands []string) (entity string, conf float64)
}

// Option is   pt    objtgt(M4   by ize; D   as AskOption{ID,Label}). 
// ID     ("dict:"+term / "rec:"+entity); Label in  read. 
type Option struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// New    resolve (dict  as nil, word     ed). 
func New(dict *memory.Dictionary) *Resolver {
	return &Resolver{Dict: dict}
}

// anaphoraTriggers isin  langcoreferenceword( word first, CJK noword boundary,     ). 
var anaphoraTriggers = []string{
	"那个文件", "那个客户", "那个项目", "上次那个", "之前那个",
	"上次", "之前", "那个", "这个文件", "这个", "它", "他",
}

// fileTriggers tableshowcoreferencereferto"file"class body. 
var fileTriggers = []string{"那个文件", "这个文件", "它"}

// modelConfThreshold islanglang    value. 
const modelConfThreshold = 0.6

func hasAny(text string, words []string) (string, bool) {
	for _, w := range words {
		if strings.Contains(text, w) {
			return w, true
		}
	}
	return "", false
}

// dictLookup   basein word  in(returnbackrule word;   inreturnbackempty). 
func (r *Resolver) dictLookup(text string) string {
	if r.Dict == nil {
		return ""
	}
	low := strings.ToLower(text)
	for _, t := range r.Dict.Terms {
		if strings.Contains(low, strings.ToLower(t.Term)) {
			return t.Term
		}
		for _, v := range t.Variants {
			if v != "" && strings.Contains(low, strings.ToLower(v)) {
				return t.Term
			}
		}
	}
	return ""
}

// rankCands by (spaceID  first, Ts   )     , returnback      . 
func rankCands(cands []RecentEntity, spaceID string) []RecentEntity {
	out := make([]RecentEntity, len(cands))
	copy(out, cands)
	sort.SliceStable(out, func(i, j int) bool {
		si := out[i].Space == spaceID
		sj := out[j].Space == spaceID
		if si != sj {
			return si // space  iner first
		}
		return out[i].Ts > out[j].Ts // Ts   (newer first; ISO char  char  =timetime )
	})
	return out
}

// Resolve   split  resolve, backfill intent.Target;   timewrite intent.Ask(    ). 
// keep  M2 frozen signature; close ize  see ResolveOptions. 
func (r *Resolver) Resolve(it *contract.Intent, spaceID string) (*contract.Intent, error) {
	got, _, err := r.ResolveOptions(it, spaceID)
	return got, err
}

// ResolveOptions and Resolve same  split ;   clarificationtime outreturnback 2-4  close ize  objtgt(M4). 
func (r *Resolver) ResolveOptions(it *contract.Intent, spaceID string) (*contract.Intent, []Option, error) {
	if it == nil {
		return nil, nil, nil
	}
	var opts []Option

	// 0. already formobjtgt: onlyword rule ize,  clarification
	if it.Target != nil && it.Target.Entity != "" {
		if canon := r.dictLookup(it.CorrectedText); canon != "" {
			it.Target.Entity = canon
		}
		return it, opts, nil
	}

	text := it.CorrectedText

	//    G4: UNKNOWN    origbecause**  **becoreferenceclarification write. 
	//
	// classify   UNKNOWN time write" is       "-- isuseuser needneed     . 
	// if refer pipe modifywritebecome"   "  "refer is   ", useuser be   **error   **. 
	//
	// but     ( hasback  pipeline.TestCodexNineRegressions#1 needrequire
	// " now    openstart …pipe   …  raise …"  sent**  **keep  refer  coreferenceclarification). 
	//  splittgtapprove: is is**  coreference**. 
	//   - "pipe   …"->   coreference, refer    has value ->   ; 
	//   - "         under"-> lang word,  is  to  -> keep classify    origbecause. 
	if it.Intent == contract.IntentUnknown && it.Ask != "" && !anyOperationAnaphora(text) {
		return it, opts, nil
	}

	//    G9: NOTE sent  **in coreference** is  coreference. 
	//
	//   is by base: "  under:   needfix is      diffchar"  "  "
	// isin    split,   "refer is  " no  (useuserthenis    )again disconnect . 
	//
	//  boundary( hasback  pipeline.TestCodexNineRegressions#7): `  under   `
	//  coreference**thenissafety in **,  time         --   keepkeep  . 
	if it.Intent == contract.IntentNote {
		if trigger, ok := hasAny(text, anaphoraTriggers); ok && !notePayloadIsJustPronoun(text, trigger) {
			return it, opts, nil
		}
	}

	// 1. word  (100%)
	if canon := r.dictLookup(text); canon != "" {
		it.Target = &contract.Target{Entity: canon, RefType: "dict"}
		return it, opts, nil
	}

	// 2. coreferencetriggersend  
	trigger, ok := hasAny(text, anaphoraTriggers)
	if !ok {
		return it, opts, nil // nocoreference, noneed resolve
	}

	// 3. onunder rule : by kind ed 
	wantKind := ""
	if _, isFile := hasAny(text, fileTriggers); isFile {
		wantKind = "file"
	}
	var pool []RecentEntity
	for _, e := range r.Recent {
		if wantKind == "" || e.Kind == wantKind {
			pool = append(pool, e)
		}
	}
	if len(pool) == 0 && wantKind != "" { // fileclassno in ->  bot  
		for _, e := range r.Recent {
			pool = append(pool, e)
		}
	}

	if len(pool) > 0 {
		ranked := rankCands(pool, spaceID)
		top := ranked[0]
		//  domain  :  has     in spaceID, and  split   >1  domain
		spaces := map[string]bool{}
		for _, e := range ranked {
			spaces[e.Space] = true
		}
		matchedSpace := top.Space == spaceID
		// top   :   and  same space same Ts
		tie := len(ranked) > 1 &&
			ranked[1].Space == top.Space && ranked[1].Ts == top.Ts
		switch {
		case matchedSpace:
			it.Target = &contract.Target{Entity: top.Entity, RefType: "anaphora"}
			return it, opts, nil
		case !matchedSpace && len(spaces) > 1:
			it.Ask = "Which domain did you mean by \"" + trigger + "\"? I see recent activity in several domains"
			opts = r.buildOptions(text, ranked)
			return it, opts, nil
		case tie:
			it.Ask = "Which one did you mean by \"" + trigger + "\"? Several similar objects exist"
			opts = r.buildOptions(text, ranked)
			return it, opts, nil
		default:
			//    (  domainbutunique)->  connect resolve
			it.Target = &contract.Target{Entity: top.Entity, RefType: "anaphora"}
			return it, opts, nil
		}
	}

	// 4. langlang (  )
	if r.ModelFn != nil {
		candNames := make([]string, 0, len(r.Recent))
		for _, e := range r.Recent {
			candNames = append(candNames, e.Entity)
		}
		if entity, conf := r.ModelFn(text, candNames); conf >= modelConfThreshold && entity != "" {
			it.Target = &contract.Target{Entity: entity, RefType: "model"}
			return it, opts, nil
		}
	}

	// rule low-confidence allow     ->  formclarification(no  then Options asempty)
	it.Ask = "Which did you mean by \"" + trigger + "\"? Please clarify"
	return it, opts, nil
}

// kindPrefix by bodyclasstypegivein  readbefore . 
func kindPrefix(kind string) string {
	switch kind {
	case "file":
		return "that file: "
	case "project":
		return "project: "
	case "person":
		return "person: "
	case "space":
		return "domain: "
	default:
		return ""
	}
}

// buildOptions fromword  in +   after    body   2-4  close ize  ( heavy, onlimit 4). 
func (r *Resolver) buildOptions(text string, ranked []RecentEntity) []Option {
	seen := map[string]bool{}
	var opts []Option
	// 1) word  in  before
	if r.Dict != nil {
		low := strings.ToLower(text)
		for _, t := range r.Dict.Terms {
			if !strings.Contains(low, strings.ToLower(t.Term)) {
				continue
			}
			id := "dict:" + t.Term
			if !seen[id] {
				seen[id] = true
				opts = append(opts, Option{ID: id, Label: t.Term + " (dictionary)"})
			}
		}
	}
	// 2) onunder    body
	for _, e := range ranked {
		id := "rec:" + e.Entity
		if seen[id] {
			continue
		}
		seen[id] = true
		label := kindPrefix(e.Kind) + e.Entity
		if e.Space != "" {
			label += " (domain:" + e.Space + ")"
		}
		opts = append(opts, Option{ID: id, Label: label})
	}
	if len(opts) > 4 {
		opts = opts[:4]
	}
	return opts
}

// Solidify pipeconfirmafter diffname ize   word (AddTerm, source=voice). 
func (r *Resolver) Solidify(entity, variant string) error {
	if r.Dict == nil {
		return nil
	}
	term := memory.Term{Term: entity, Category: "voice", Source: "voice"}
	if variant != "" && variant != entity {
		term.Variants = []string{variant}
	}
	if err := r.Dict.AddTerm(term); err != nil {
		return err
	}
	return nil
}

// operationVerbs iscoreferencewordafter  time   is"  to "  word. 
var operationVerbs = []rune("发删改查看开关跑修记提部打建写读")

// operationAnaphora     coreferencewordis  become"  coreference". 
//
//	pipe    modify under     -> before charis"pipe"          -> is  coreference
//	  file modify under     -> after charis word"modify"       -> is  coreference
//	         under   -> beforeafterall is  lang       -> onlyislang word,  is  to 
func operationAnaphora(text, trigger string) bool {
	i := strings.Index(text, trigger)
	if i < 0 {
		return false
	}
	if i > 0 {
		prev := []rune(text[:i])
		if len(prev) > 0 {
			p := prev[len(prev)-1]
			switch p {
			case '把', '将', '对', '给':
				return true
			}
			//  word coreferencewordofbefore: " open ""delete  "--  is  to . 
			for _, v := range operationVerbs {
				if p == v {
					return true
				}
			}
		}
	}
	rest := []rune(text[i+len(trigger):])
	if len(rest) > 0 {
		for _, v := range operationVerbs {
			if rest[0] == v {
				return true
			}
		}
	}
	return false
}

// anyOperationAnaphora    baseinis store **  **    coreference. 
//
// note     safety   : hasAny bywordtable  returnbackfirst  in, butwordtable  andoutnow  noclose, 
//  sent   first inlang word"  ", but  change outnow   coreference"pipe  ". 
func anyOperationAnaphora(text string) bool {
	for _, t := range anaphoraTriggers {
		if strings.Contains(text, t) && operationAnaphora(text, t) {
			return true
		}
	}
	return false
}

// noteTriggersForRefer is NOTE intent triggersendword(and input  keepkeep  ; refer   import input, 
// thus place  listout, onlyuseat disconnect"  afteris   all  "). 
var noteTriggersForRefer = []string{
	"记一下", "记下来", "记下", "记个", "记住", "记录一下", "记录", "存档", "存个", "存到",
}

// notePayloadIsJustPronoun    NOTE sent   coreferenceandtriggersendwordafteris **  all  **. 
//
//	  under                           ->  empty -> true (coreferencethenissafety in ,     )
//	  under:   needfix is      diffchar   ->  ":   needfix is    diffchar" -> false(isin ,    )
func notePayloadIsJustPronoun(text, trigger string) bool {
	rest := strings.Replace(text, trigger, "", 1)
	for _, w := range noteTriggersForRefer {
		rest = strings.ReplaceAll(rest, w, "")
	}
	rest = strings.Trim(rest, "：:，。、！？!? 　")
	return strings.TrimSpace(rest) == ""
}
