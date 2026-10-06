// mixed.go --   rule ize(G1  ② ): in get audio,   keep ( write),  body  . 
//
// **only  body  ,    word** ⇒     (voice-sign) accept  (prevent" edhead"). 
// tableout char    (     ,   readaudio). 
package hotcache

import (
	"strings"

	"voicesign-harness/asr"
)

// MixedKey returnback    ize ; no   time ok=false. 
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
				return "", false // tableoutchar  ⇒   
			}
			b.WriteString(k)
		}
	}
	return b.String(), true
}
