// probe_auto.go —— 知己 · A4 detail survival 探针自动预埋（M1 v1.3 P0 规则版）。
//
// 定位：OnTaskEnd 在轨迹落盘后，从本任务文本里自动挑 2–3 个「低显著关键细节」
// 预埋成 SurvivalProbe。所谓低显著 = 本段文本里只出现 1 次的具体 token
// （数字/专名），而非反复强调的主题词——专门用来衡量压缩后细节是否还在（北极星）。
//
// 纯 stdlib、无模型、无外部依赖；与 compress.go 的 markProbeHits（召回侧）成对：
// 这里负责「预埋」，压缩器命中 KeyDetail 时调 ProbeRecall 标召回。
package zhiji

import (
	"regexp"
	"strings"
)

// probeTokenRe 单次交替扫描，避免把 "230SAR" 里的 "SAR" 当成独立大写专名重复抓。
//   - \d+[A-Za-z%./]*  数字/数量词（230SAR / 5G / 1204 / 429 / 30min / 3.5%）
//   - [A-Z][A-Za-z]+   大写英文专名（STC / Zain / Leap；≥2 个字母）
var probeTokenRe = regexp.MustCompile(`[0-9]+[A-Za-z%./]*|[A-Z][A-Za-z]+`)

// probeStopwords 通用高频停用词（归一化小写后比较）。P0 只收最常见的；
// 专名（STC/泰邦大厦）与数字（230SAR）天然不在表里。
var probeStopwords = map[string]bool{
	// 英文
	"the": true, "a": true, "an": true, "is": true, "are": true, "was": true,
	"this": true, "that": true, "and": true, "or": true, "to": true, "of": true,
	"in": true, "on": true, "for": true, "with": true, "task": true,
	"success": true, "failed": true, "retried": true, "judge": true,
	"cheap": true, "premium": true, "mid": true,
	// 中文通用词（非专名；bigram 粒度）
	"任务": true, "我们": true, "这个": true, "那个": true, "进行": true,
	"需要": true, "成功": true, "失败": true, "一个": true, "没有": true,
	"就是": true, "还是": true, "不是": true, "可以": true, "什么": true,
}

// extractLowSalienceDetails 从任务文本里挑 n 个「低显著关键细节」（P0 规则版）。
//
// 挑选规则（最终版 v1.3.1）：
//  1. 候选 token 三类：① 数字/数量词（起始为数字，可带字母/符号后缀：230SAR/5G/1204/429）
//     ② 大写英文专名（首字母大写且 ≥2 字母：STC/Zain/Leap）
//     ③ 中文滑窗 n-gram（2-gram + 3-gram：泰邦/大厦/泰邦大，不接 NER/分词库）。
//  2. 中文 n-gram 先过停用字过滤：含任一 cnStopRunes（的/了/是/在/我/你/他/们/…）直接丢。
//  3. 在本段文本内统计每个候选出现次数；只保留恰好出现 1 次的（低显著、非反复强调）。
//  4. 去停用词（英文通用高频词）；英文按小写归一键。
//  5. 按在原文首次出现先后顺序返回前 n 个；同形（归一键）去重。
//
// 空文本/无候选时返回 nil，不 panic。
func extractLowSalienceDetails(text string, n int) []string {
	if n <= 0 || strings.TrimSpace(text) == "" {
		return nil
	}
	type tally struct {
		freq    int
		surface string // 首现写法（展示用）
	}
	var order []string // 归一键首现顺序
	t := map[string]*tally{}
	add := func(surface string) {
		norm := strings.ToLower(surface)
		if _, ok := t[norm]; !ok {
			order = append(order, norm)
			t[norm] = &tally{surface: surface}
		}
		t[norm].freq++
	}

	// ① 数字词 + ② 大写英文专名（单次正则交替扫描，避免 230SAR 里的 SAR 被单独抓到）
	for _, m := range probeTokenRe.FindAllString(text, -1) {
		add(m)
	}
	// ③ 中文滑窗 2-gram + 3-gram；含停用字的 n-gram 直接丢。
	var cjk []rune
	flushCJK := func() {
		for size := 2; size <= 3; size++ {
			for i := 0; i+size <= len(cjk); i++ {
				g := string(cjk[i : i+size])
				if hasStopRune(g) {
					continue
				}
				add(g)
			}
		}
		cjk = nil
	}
	for _, r := range text {
		if isCJK(r) {
			cjk = append(cjk, r)
		} else {
			flushCJK()
		}
	}
	flushCJK()

	// 按首现顺序筛：频次==1 且非停用词。
	out := make([]string, 0, n)
	for _, norm := range order {
		if t[norm].freq != 1 || probeStopwords[norm] {
			continue
		}
		out = append(out, t[norm].surface)
		if len(out) >= n {
			break
		}
	}
	return out
}

// hasStopRune n-gram 是否含任一中文停用字（复用 edges_rules.go 的 cnStopRunes）。
func hasStopRune(g string) bool {
	for _, r := range g {
		if cnStopRunes[r] {
			return true
		}
	}
	return false
}

// taskEndCorpus 把一条 CallLog 拼成供探针抽取的语料（TaskProfile 为主，DetailProbe 为辅）。
func taskEndCorpus(log CallLog) string {
	return strings.Join([]string{log.TaskProfile, log.DetailProbe, log.Model, log.Outcome}, " ")
}
