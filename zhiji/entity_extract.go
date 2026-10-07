package zhiji

import "strings"

// ExtractEntity 从"X叫Y""X是Y""X是个Y"模式里抽实体。
// 返回 (canonical, desc, aliases)。
func ExtractEntity(text string) (canonical, desc string, aliases []string) {
	runes := []rune(text)
	// 模式："X叫Y" → X 是代指，Y 是标准名
	for _, pattern := range []struct {
		sep string
	}{
		{"叫"},
		{"是"},
	} {
		idx := indexRuneSeq(runes, []rune(pattern.sep))
		if idx < 0 {
			continue
		}
		// sep 前面的 2-4 个汉字是代指
		before := runes[max(0, idx-4):idx]
		// 去掉常见前缀
		beforeStr := strings.TrimSpace(string(before))
		beforeStr = strings.TrimPrefix(beforeStr, "记一下")
		beforeStr = strings.TrimPrefix(beforeStr, "那个")
		if beforeStr == "" || len([]rune(beforeStr)) < 1 {
			continue
		}
		// sep 后面的内容是描述/标准名
		after := string(runes[idx+len([]rune(pattern.sep)):])
		after = strings.TrimSpace(after)
		// 截到逗号/句号
		if i := strings.IndexAny(after, "，,。."); i > 0 {
			after = after[:i]
		}
		if after == "" {
			continue
		}
		// 如果 after 像英文名（Amber/Peter），canonical=after，alias=before
		if isASCII(after) {
			return after, "", []string{beforeStr}
		}
		// 否则 canonical=before，desc=after
		return beforeStr, after, nil
	}
	return "", "", nil
}

func isASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return len(s) > 0
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var _ = strings.TrimSpace
