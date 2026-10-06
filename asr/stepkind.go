// stepkind.go -- trace   ** nameclasstype**(ASR-EXEC-05 v2 ①). 
//
//  bychar     **  periodthenwrite out **: "hotcache"  class timenamechar    , 
//  data  again protect"    name name "( name  however :   hotcache ->   teach ->   learn…). 
package asr

// TraceStepKind istrace   classtype(  ). 
type TraceStepKind string

const (
	StepRetain    TraceStepKind = "retain"    // origstart in store
	StepCorrect   TraceStepKind = "correct"   // pos correction(Correct   num)
	StepLexicon   TraceStepKind = "lexicon"   // word /wordtablecorrection
	StepPunctuate TraceStepKind = "punctuate" // tgtpt  
	StepCache     TraceStepKind = "cache"     // cachemodifywrite( word/diffname/ audio/  word)
)

// kindForStep pipe  name  as nameclasstype(  namechar ⇒ empty,  datadata   ). 
func kindForStep(name string) TraceStepKind {
	switch name {
	case "retain":
		return StepRetain
	case "clean":
		return StepCorrect
	case "dict":
		return StepLexicon
	case "hotcache":
		return StepCache
	case "punctuate":
		return StepPunctuate
	default:
		return ""
	}
}
