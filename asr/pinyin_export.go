// pinyin_export.go -- toout  nocallaudionode list(provide hotcache   audiorouteby usesame  table). 
package asr

import "strings"

// PinyinKey returnback base nocallaudionode list("|" linkconnect). 
//   char  (  )tableini.e. ok=false --      , also  readaudio. 
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
