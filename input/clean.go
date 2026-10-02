// Package input 是 ASR 输入容错管线（架构 §5）：
// ①raw 原样保留 → ②clean 清洗 → ③correct 词典纠错 → ④classify 意图 JSON → ⑤低置信回问。
// 本包只依赖 contract/config，不依赖 memory（纠错通过 Correcter 接口注入，memory.Dictionary 天然实现）。
package input

import "strings"

// defaultFillers 是默认填充词表（架构 §5.1）。
var defaultFillers = []string{"嗯", "那个", "请", "帮我", "麻烦", "的话", "一下"}

// Cleaner 做文本规范化：全角→半角、去句首句尾填充词、压缩空白、去句末标点。
type Cleaner struct {
	Fillers []string
}

// NewCleaner 构造清洗器。fillers 为 nil 时使用默认填充词表。
func NewCleaner(fillers []string) *Cleaner {
	if fillers == nil {
		fillers = defaultFillers
	}
	return &Cleaner{Fillers: fillers}
}

// Clean 执行清洗管线：
//  1. 全角字母/数字/标点 → 半角（含全角空格 U+3000 → 半角空格）；
//  2. 循环去除句首、句尾填充词；
//  3. 压缩连续空白为单个空格；
//  4. 去除句末句读标点（保留 /~._-、中文与字母数字等内部字符）。
func (c *Cleaner) Clean(raw string) string {
	s := fullwidthToHalf(raw)
	s = strings.TrimSpace(s)

	// 循环去句首填充词（每轮 TrimSpace 后再比对，处理「帮我 请 …」这类空格间隔）。
	for changed := true; changed; {
		changed = false
		t := strings.TrimSpace(s)
		for _, f := range c.Fillers {
			if f == "" {
				continue
			}
			if strings.HasPrefix(t, f) {
				s = strings.TrimSpace(t[len(f):])
				changed = true
				break
			}
		}
	}

	// 循环去句尾填充词。
	for changed := true; changed; {
		changed = false
		t := strings.TrimSpace(s)
		for _, f := range c.Fillers {
			if f == "" {
				continue
			}
			if strings.HasSuffix(t, f) {
				s = strings.TrimSpace(t[:len(t)-len(f)])
				changed = true
				break
			}
		}
	}

	// 压缩连续空白（含全角空格已在上一步转半角）。
	s = strings.Join(strings.Fields(s), " ")

	// 去句末句读标点（不动内部的 /~._-）。
	s = strings.TrimRight(s, "。．.！!？?，,、；;：:～~ ")
	return strings.TrimSpace(s)
}

// fullwidthToHalf 把全角 ASCII（U+FF01–U+FF5E）与全角空格（U+3000）转为半角。
func fullwidthToHalf(s string) string {
	r := []rune(s)
	for i, ch := range r {
		switch {
		case ch == 0x3000:
			r[i] = ' '
		case ch >= 0xFF01 && ch <= 0xFF5E:
			r[i] = ch - 0xFEE0
		}
	}
	return string(r)
}
