// mixed.go —— 混排规范化（G1 第②步）：中文取拼音、拉丁保留（小写），整体归一。
//
// **只做整体归一，不做拆词** ⇒ 纯拉丁串（voice-sign）不受影响（防"治过头"）。
// 表外汉字一律放弃（宁可不匹配，不猜读音）。
package hotcache

import (
	"strings"

	"voicesign-harness/asr"
)

// MixedKey 返回混排归一化键；无法归一时 ok=false。
func MixedKey(s string) (string, bool) {
	if strings.TrimSpace(s) == "" {
		return "", false
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32)
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-':
			b.WriteRune(r)
		default:
			k, ok := asr.PinyinKey(string(r))
			if !ok {
				return "", false // 表外字符 ⇒ 放弃
			}
			b.WriteString(k)
		}
	}
	return b.String(), true
}
