// breakdown.go -- 从用户的"X的Y"解释里反推正确字。
//
// 真实场景（截图）：
//   用户说"我叫张大山，用户说"A的B"模式解释每个字"
//   ASR转写错字，但用户自己解释了正确字。
//
// 规则：
//   "A的B" → B 是要确认的那个字（"A的B"→B是正确字）
//   这些字比 ASR 转写的字置信度高，应该覆盖 ASR 错字。
package zhiji

import "strings"

// extractBreakdownChars 从"X的Y，A的B"模式里抽出确认字列表。
// 返回顺序就是名字位置：第一个抽到的是姓，第二个是中间，第三个是尾。
func extractBreakdownChars(text string) []string {
	runes := []rune(text)
	var chars []string
	// 找所有"的X"模式：的后面跟一个汉字，且这个字不是虚词
	i := 0
	for i < len(runes) {
		if runes[i] == '的' {
			// 的后面第一个汉字
			if i+1 < len(runes) {
				next := runes[i+1]
				if isCJKHan(next) && !isFunctionWord(next) {
					chars = append(chars, string(next))
				}
			}
			i += 2
			continue
		}
		i++
	}
	return chars
}

func isCJKHan(r rune) bool {
	return r >= 0x4e00 && r <= 0x9fff
}

func isFunctionWord(r rune) bool {
	switch r {
	case '的', '了', '是', '在', '有', '和', '就', '都', '也', '还', '又', '很', '太', '最':
		return true
	}
	return false
}

// extractSpellingLetters 从"Z O U"或"Z-O-U"里抽出大写字母序列。
// 用户拼字母是强信号，直接锁定。
func extractSpellingLetters(text string) []string {
	var letters []string
	for _, r := range text {
		if r >= 'A' && r <= 'Z' {
			letters = append(letters, string(r))
		}
	}
	return letters
}

// reconstructName 融合 ASR 抽的名字和 breakdown 确认字。
// asrName: ASR 转写抽到的名字（可能有错字）
// breakdown: 用户自己解释的确认字列表（高置信度）
// 返回：用 breakdown 字覆盖 ASR 对应位置后的修正名字。
func reconstructName(asrName string, breakdown []string) string {
	if len(breakdown) == 0 {
		return asrName
	}
	runes := []rune(asrName)
	// breakdown 字按顺序覆盖：breakdown[0]→runes[0], breakdown[1]→runes[1]...
	for i, ch := range breakdown {
		if i < len(runes) {
			runes[i] = []rune(ch)[0]
		}
	}
	return string(runes)
}

// NameCorrection 返回从一句用户输入里重建的正确名字。
// 融合：ASR 抽名 + breakdown 确认字 + 同音字纠错。
// 返回 (correctedName, confidence, question)
//   correctedName: 重建后的名字（空表示没抽到）
//   confidence: 0-1，越高越可信
//   question: 低置信度时应该问用户的确认问题（空表示不用问）
func NameCorrection(text string) (string, float64, string) {
	asrName := extractIdentityName(text)
	breakdown := extractBreakdownChars(text)
	spelling := extractSpellingLetters(text)

	if asrName == "" && len(breakdown) == 0 && len(spelling) == 0 {
		return "", 0, ""
	}

	// 有 breakdown 字 → 重建名字，置信度高
	if asrName != "" && len(breakdown) > 0 {
		corrected := reconstructName(asrName, breakdown)
		conf := 0.9
		if len(spelling) > 0 {
			conf = 0.99 // 用户还拼了字母，最高置信度
		}
		return corrected, conf, ""
	}

	// 只有 ASR 名字，没有 breakdown
	if asrName != "" {
		// 检查是不是和已锁定名字同音字（这个在 OnInput 里处理）
		return asrName, 0.5, ""
	}

	return "", 0, ""
}

var _ = strings.TrimSpace // keep strings import
