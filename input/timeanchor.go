package input

// timeanchor.go —— 时间锚点解析（SPEC v2 缺口 36 落地基座）。
//
// 【伪代码逻辑层】
// 模块职责：从文本中解析口语时间表述（今天/明天/后天/昨天/前天/周X/下周X）为具体日期。
// 输入：文本；参考时间 now（解析时刻，本地时区）。
// 输出：hint（原文表述）、date（YYYY-MM-DD）、ambiguous（是否建议回问）。
// 控制流：
//   1. 绝对偏移：今天/明天/后天/昨天/前天（不歧义），取文本中最早命中。
//   2. 星期锚点（组合模式字符串匹配，避免 CJK 字节切片问题）：
//      对每个星期名构造：下下周X(+2周) / 下个星期X、下星期X、下周X(+1周) / 本周X、这周X(+0) / 裸X(+0)；
//      命中取「位置最早，同位置取模式最长」；裸X 且目标早于今天 → 顺延下周 + ambiguous=true。
//   3. 绝对偏移 vs 星期锚点 → 取位置更前者。
// 异常处理：无命中 → 全空；「下个月」「几号」不解析（SPEC v2 可扩展）；跨月/跨年由 AddDate 进位；
//   星期名归一：星期天/周天/星期日 → 周日。

import (
	"strings"
	"time"
)

var weekdayNames = map[string]time.Weekday{
	"周一":  time.Monday,
	"周二":  time.Tuesday,
	"周三":  time.Wednesday,
	"周四":  time.Thursday,
	"周五":  time.Friday,
	"周六":  time.Saturday,
	"周日":  time.Sunday,
	"星期天": time.Sunday,
	"周天":  time.Sunday,
	"星期日": time.Sunday,
}

// absoluteAnchors 绝对偏移锚点（按优先级排列；首命中即返回）。
type absoluteAnchor struct {
	hint string
	days int // 相对 now 的天数偏移
}

var absoluteAnchors = []absoluteAnchor{
	{"今天", 0},
	{"明天", 1},
	{"后天", 2},
	{"昨天", -1},
	{"前天", -2},
}

// weekdayPat 是一个星期锚点模式（pat 为完整匹配串）。
type weekdayPat struct {
	pat  string
	off  int  // 周偏移：0=本周 1=下周 2=下下周
	bare bool // 裸周X（无前缀修饰）
}

// weekdayPatterns 为星期名构造组合模式。
// 注意：不能用「下周」+「周一」拼接（会得到四字「下周周一」）——
// 自然表述中「周」字合并（「下周一」= 下+周+一），因此按「天后缀」构造（"下周"+日 → 下周一）。
func weekdayPatterns(name string) []weekdayPat {
	runes := []rune(name)
	day := string(runes[len(runes)-1])
	return []weekdayPat{
		{"下下周" + day, 2, false},
		{"下个星期" + day, 1, false},
		{"下星期" + day, 1, false},
		{"下周" + day, 1, false},
		{"本周" + day, 0, false},
		{"这周" + day, 0, false},
		{name, 0, true}, // 裸周X（周一/周二/…/周日/星期天/周天/星期日）
	}
}

// ResolveTimeAnchor 解析文本中的首个时间锚点。
// 返回值：hint 原文、date（YYYY-MM-DD，无命中为 ""）、ambiguous（是否建议回问）。
func ResolveTimeAnchor(text string, now time.Time) (hint, date string, ambiguous bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", "", false
	}

	// 1. 绝对偏移（最早命中）
	absIdx, absHint, absDays := -1, "", 0
	for _, a := range absoluteAnchors {
		if i := strings.Index(text, a.hint); i >= 0 && (absIdx < 0 || i < absIdx) {
			absIdx, absHint, absDays = i, a.hint, a.days
		}
	}

	// 2. 星期锚点（组合模式；位置最早，同位置长模式优先）
	wdIdx, wdHint, wdOffset, wdBare, wdName := -1, "", 0, false, ""
	for name := range weekdayNames {
		for _, p := range weekdayPatterns(name) {
			if i := strings.Index(text, p.pat); i >= 0 {
				if wdIdx < 0 || i < wdIdx || (i == wdIdx && len(p.pat) > len(wdHint)) {
					wdIdx, wdHint, wdOffset, wdBare, wdName = i, p.pat, p.off, p.bare, name
				}
			}
		}
	}

	// 3. 取更靠前者
	switch {
	case wdIdx >= 0 && (absIdx < 0 || wdIdx < absIdx):
		d, _ := resolveWeekday(now, weekdayNames[wdName], wdOffset)
		// 归一化到当日零点再比较（AddDate 保留时刻，否则「周五」当天 10:00 不触发顺延）
		dMid := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location())
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		if wdBare {
			switch {
			case dMid.Before(today):
				d, ambiguous = d.AddDate(0, 0, 7), true // 本周该日已过 → 顺延下周 + 歧义
			case dMid.Equal(today):
				ambiguous = true // 恰为今天（如周五说「周五」）→ 今天 vs 下周 歧义，回问候选
			}
		}
		return wdHint, d.Format("2006-01-02"), ambiguous

	case absIdx >= 0:
		d := now.AddDate(0, 0, absDays)
		return absHint, d.Format("2006-01-02"), false
	}

	return "", "", false
}

// resolveWeekday 返回以 now 所在周为基准、第 weekOffset 周（0=本周,1=下周,2=下下周）的 wd 日期。
func resolveWeekday(now time.Time, wd time.Weekday, weekOffset int) (time.Time, bool) {
	mon := mondayOf(now)
	target := mon.AddDate(0, 0, (int(wd)+6)%7+7*weekOffset) // 周一起始：周一=0 … 周日=6
	return target, false
}

// mondayOf 返回 now 所在周的周一（周一起始的 ISO 语义）。
func mondayOf(now time.Time) time.Time {
	offset := (int(now.Weekday()) + 6) % 7 // 周一=0
	return now.AddDate(0, 0, -offset)
}
