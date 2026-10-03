// pinyin_export.go —— 对外暴露无调音节序列（供 hotcache 的近音路由复用同一张表）。
package asr

import "strings"

// PinyinKey 返回文本的无调音节序列（"|" 连接）。
// 任一字不在（稀疏）表内即 ok=false —— 宁可不匹配，也不猜读音。
func PinyinKey(text string) (string, bool) {
	table := buildPinyinTable()
	rs := []rune(text)
	if len(rs) == 0 {
		return "", false
	}
	parts := make([]string, 0, len(rs))
	for _, r := range rs {
		s, ok := table[r]
		if !ok {
			return "", false
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "|"), true
}
