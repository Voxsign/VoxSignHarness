package input

import (
	"strings"
	"unicode"
)

// punctuate.go -- M7 server endtgtptafterhandle bot( path,  useuser  ). 
//
//  scenario: ASR tgtptsafety  , iOS side addsPunctuation=true as path(and   in). 
// basefile now server end bot: if  openclose/locale      difftgtpt,  base intentclassifybefore
//   patchsentendtgtpt, useuserfinishsafetyno . **curbefore connectline pipeline**-- path, by  er  iOS
// side  close out afterdecide is   Clean->Correct->Classify oftime inbase num. 
//
//   : 
//   - rule first( in tgtpt, noout dependency); 
//   - LLM  fixonly  connect (LMRefiner,    key  as nil ->  rule); 
//   -  etc: totgtptalreadyfinish   base  modify, heavy calluseclose   . 

// questionWords isin   triggersendword( ini.e. sentend " "). 
var questionWords = []string{"谁", "什么", "为什么", "怎么", "哪", "吗", "呢", "能不能", "可不可以", "是否", "多少", "几"}

// sentenceEndPunct is as"sentendalreadyhastgtpt" char (in  sentread). 
var sentenceEndPunct = map[rune]bool{
	'。': true, '？': true, '！': true, '；': true, '…': true,
	'.': true, '?': true, '!': true, ';': true,
}

// LMRefiner is   LLM  fixconnect   . 
//
// [  , basetask   ]: provider    key timeglobal LMRefiner  as nil, Punctuate  rule; 
// will   key afterby  ernotein   now, Punctuate  rule tgtptafter  calluse  fixin stop . 
// connect signaturefrozen:  in fix base, returnback fixafter base(out timereturnbackorig ,    disconnect flow). 
type LMRefiner interface {
	RefinePunctuation(text string) (string, error)
}

// lmRefiner is LLM  fix globalnoteinpt; nil =  rule(default). 
var lmRefiner LMRefiner = nil

// SetLMRefiner notein LLM  fix (  ;   nil backto rule). 
// by  er connectline M7 timecalluse;  callusebefore Punctuate finishsafetyisrule num. 
func SetLMRefiner(r LMRefiner) { lmRefiner = r }

// Punctuate to ASR  base sentendtgtpt bot(in as ): 
//   - alreadyhassentendtgtpt(.   .!? etc)-> origkindreturnback( etc); 
//   -   code/numchar/  /URL/path(noin char )-> origkindreturnback,   tgtpt; 
//   -   sentend -> patch". ";  in  word( /  /as  / / /   /   by…)-> patch" ". 
//
// in stop /listtable  id in**  keepkeepkeep **(base      split),   will connectlineafter  
// under triggersendword    ; in stop  by LMRefiner   connect or iOS  pathhandle. 
func Punctuate(text string) string {
	s := strings.TrimSpace(text)
	if s == "" {
		return ""
	}

	//  etc: sentendalreadyissentreadtgtpt ->   . 
	runes := []rune(s)
	if sentenceEndPunct[runes[len(runes)-1]] {
		return s
	}

	// noin char  ->  as code/numchar/  /URL/path,   tgtpt. 
	if !hasChinese(runes) {
		return s
	}

	// rule:   word ->  ,  then -> . 
	var end string
	if containsAny(s, questionWords) {
		end = "？"
	} else {
		end = "。"
	}
	out := s + end

	// LLM  fix  (   key time ed; out back ruleclose ). 
	if lmRefiner != nil {
		if refined, err := lmRefiner.RefinePunctuation(out); err == nil && strings.TrimSpace(refined) != "" {
			return strings.TrimSpace(refined)
		}
	}
	return out
}

// hasChinese    rune  listis       CJK   table  char. 
func hasChinese(runes []rune) bool {
	for _, r := range runes {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}
