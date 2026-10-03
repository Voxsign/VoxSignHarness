// Package recog —— 把热词/别名缓存**真的接进纠错输出**（CACHE-001 K9）。
//
// 为什么单独成包：`hotcache` 依赖 `asr`（复用拼音表），所以 `asr` 不能反向依赖 `hotcache`；
// 本包同时依赖两者，做"识别输出"的组合层。
//
// 语义：热词表**改变输出**才算接上；表为空/清空时输出必须回到原样（K9 三条验收）。
package recog

import (
	"strings"

	"voicesign-harness/asr"
	"voicesign-harness/hotcache"
)

// Correction 是一处因缓存（热词/别名/近音）而发生的改写。
type Correction struct {
	Start, End int
	From, To   string
	Route      string
	Score      float64
}

// Rewriter 组合"基线纠错（引擎+词典）"与"缓存改写（热词/别名）"。
type Rewriter struct {
	Engine *asr.Personalized
	Dict   *asr.Dictionary
	Hot    *hotcache.Cache
}

// Rewrite 实现 asr.TextRewriter：只做缓存改写（不做基线纠错），供 Pipeline 内联调用。
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

// Correct 先跑基线纠错，再用缓存做关联度改写。
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

// rewriteByCache 两遍扫描：
//
//	① 滑窗（长→短）只认**精确/别名/拼音**命中 —— 不允许编辑距离吃前缀（K9 实测坑）；
//	② 若 ① 一无所获，再对**拉丁词元**做编辑距离纠错（如 voice-signn → voice-sign）。
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
			res, ok := hot.Lookup(w)
			if !ok || res.Score < minScore || res.Canonical == w {
				continue
			}
			if res.Route == hotcache.RouteEdit {
				continue // ① 不认编辑距离
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

// pass2 对拉丁词元做编辑距离纠错（整词，不允许跨词元吃字符）。
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
		res, ok := hot.Lookup(w)
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
