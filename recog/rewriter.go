// Package recog -- pipe word/diffnamecache**  connect correction out**(CACHE-001 K9). 
//
// as    become : `hotcache` dependency `asr`( use audiotable),  by `asr`   revtodependency `hotcache`; 
// this packagesametimedependency er,  " diff out"    . 
//
// semantic:  wordtable**modifychange out**only connecton; tableasempty/ emptytime out  backtoorigkind(K9    recv). 
package recog

import (
	"strings"

	"voicesign-harness/asr"
	"voicesign-harness/hotcache"
)

// Correction is placebecausecache( word/diffname/ audio)butsendoccur modifywrite. 
type Correction struct {
	Start, End int
	From, To   string
	Route      string
	Score      float64
}

// Rewriter   "baselinecorrection(  +word )"and"cachemodifywrite( word/diffname)". 
type Rewriter struct {
	Engine *asr.Personalized
	Dict   *asr.Dictionary
	Hot    *hotcache.Cache
}

// Rewrite  now asr.TextRewriter: only cachemodifywrite(  baselinecorrection), provide Pipeline in calluse. 
func (r *Rewriter) Rewrite(text string) (string, []asr.Correction) {
	if r.Hot == nil {
		return text, nil
	}
	out, corrs := rewriteByCache(text, r.Hot, 0.70)
	if len(corrs) == 0 {
		return text, nil
	}
	asrCorrs := make([]asr.Correction, 0, len(corrs))
	runes := []rune(text)
	offs := make([]int, 0, len(runes)+1)
	for i := range string(runes) {
		offs = append(offs, i)
	}
	offs = append(offs, len(string(runes)))
	for _, c := range corrs {
		if c.Start < 0 || c.End > len(runes) || c.Start >= c.End {
			continue
		}
		asrCorrs = append(asrCorrs, asr.Correction{
			Start: offs[c.Start], End: offs[c.End], From: c.From, To: c.To,
			Kind: "hotword", Confidence: c.Score,
			Evidence: "缓存关联度命中（route=" + c.Route + "，CACHE-001 K9）",
		})
	}
	if len(asrCorrs) == 0 {
		return text, nil
	}
	return out, asrCorrs
}

// Correct first baselinecorrection, againusecache close  modifywrite. 
func (r *Rewriter) Correct(raw string) (string, []Correction) {
	text := raw
	if r.Engine != nil {
		text = r.Engine.Correct(asr.CorrectRequest{Raw: raw}).Text
	}
	if r.Dict != nil {
		text, _ = r.Dict.Apply(text)
	}
	if r.Hot == nil {
		return text, nil
	}
	return rewriteByCache(text, r.Hot, 0.70)
}

// rewriteByCache     : 
//
//	①   ( -> )only **  /diffname/ audio** in --   allow     before (K9    ); 
//	② if ①  no  , againto**  word **     correction(e.g. voice-signn -> voice-sign). 
func rewriteByCache(text string, hot *hotcache.Cache, minScore float64) (string, []Correction) {
	out, corrs := pass1(text, hot, minScore)
	if len(corrs) > 0 {
		return out, corrs
	}
	return pass2(out, hot, minScore)
}

func pass1(text string, hot *hotcache.Cache, minScore float64) (string, []Correction) {
	runes := []rune(text)
	var b strings.Builder
	var corrs []Correction
	for i := 0; i < len(runes); {
		hit := false
		for l := 6; l >= 2; l-- {
			if i+l > len(runes) {
				continue
			}
			w := string(runes[i : i+l])
			res, ok := hot.LookupForRewrite(w)
			if !ok || res.Score < minScore || res.Canonical == w {
				continue
			}
			if res.Route == hotcache.RouteEdit {
				continue // ①       
			}
			corrs = append(corrs, Correction{Start: i, End: i + l, From: w, To: res.Canonical, Route: res.Route, Score: res.Score})
			b.WriteString(res.Canonical)
			i += l
			hit = true
			break
		}
		if !hit {
			b.WriteRune(runes[i])
			i++
		}
	}
	if len(corrs) == 0 {
		return text, nil
	}
	return b.String(), corrs
}

// pass2 to  word      correction( word,   allow word  char ). 
func pass2(text string, hot *hotcache.Cache, minScore float64) (string, []Correction) {
	var b strings.Builder
	var corrs []Correction
	runes := []rune(text)
	i := 0
	for i < len(runes) {
		if !isLatinWordRune(runes[i]) {
			b.WriteRune(runes[i])
			i++
			continue
		}
		j := i
		for j < len(runes) && isLatinWordRune(runes[j]) {
			j++
		}
		w := string(runes[i:j])
		res, ok := hot.LookupForRewrite(w)
		if ok && res.Score >= minScore && res.Canonical != w {
			corrs = append(corrs, Correction{Start: i, End: j, From: w, To: res.Canonical, Route: res.Route, Score: res.Score})
			b.WriteString(res.Canonical)
		} else {
			b.WriteString(w)
		}
		i = j
	}
	if len(corrs) == 0 {
		return text, nil
	}
	return b.String(), corrs
}

func isLatinWordRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.'
}
