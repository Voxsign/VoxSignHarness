// phonetic.go -- ASR 同音字纠错层。
//
// 问题：ASR 把"<USER_NAME>"识别成"周勇明"/"周永明"是高频错误（同音字）。
// 如果盲信新 ASR 输出，名字会在同音字之间横跳（截图里的 bug）。
//
// 架构：
//   1. 用户第一次自我介绍时如果带 breakdown（"山东邹县的邹"），字就被"确认锁定"。
//   2. 后续 ASR 抽到新名字时，和已锁定的名字逐字比：
//      - 每对字是同音字组里的（邹≈周，勇≈永）→ 是 ASR 误识，保留已锁定字，不更新。
//      - 用户明确说"不对/改/不姓X" → 才覆盖。
//   3. 没有已锁定名字时，新名字正常写入。
package zhiji

// homophoneGroups 常见中文姓氏/名字同音字组。扩展时往里加。
var homophoneGroups = [][]string{
	{"邹", "周"},       // zōu / zhōu
	{"勇", "永", "勇"}, // yǒng
	{"明", "民", "铭"}, // míng
	{"张", "章"},       // zhāng
	{"李", "里"},       // lǐ
	{"王", "汪"},       // wáng / wāng
	{"刘", "柳"},       // liú
	{"陈", "晨"},       // chén
	{"杨", "阳"},       // yáng
	{"赵", "照"},       // zhào
}

// isHomophone 判断两个汉字是否同音字（在同一组里）。
func isHomophone(a, b rune) bool {
	if a == b {
		return true
	}
	sa, sb := string(a), string(b)
	for _, g := range homophoneGroups {
		inA, inB := false, false
		for _, ch := range g {
			if ch == sa {
				inA = true
			}
			if ch == sb {
				inB = true
			}
		}
		if inA && inB {
			return true
		}
	}
	return false
}

// samePersonName 判断新 ASR 抽到的名字是否和已锁定名字是"同一个人"（逐字同音）。
// 长度不同不算（可能是新名字）。
func samePersonName(locked, freshly string) bool {
	lr, fr := []rune(locked), []rune(freshly)
	if len(lr) != len(fr) || len(lr) < 2 {
		return false
	}
	homophones := 0
	for i := range lr {
		if isHomophone(lr[i], fr[i]) {
			homophones++
		}
	}
	// 3字名字允许错1字（2/3同音），2字名字必须全同
	required := len(lr)
	if len(lr) >= 3 {
		required = len(lr) - 1
	}
	return homophones >= required
}

// explicitCorrection 判断用户这句话是不是在明确纠正（"不对/改/不姓X/是X不是Y"）。
// 这种情况下即使同音字也要覆盖。
func explicitCorrection(text string) bool {
	for _, kw := range []string{"不对", "不是", "改一下", "我不姓", "我不是", "应该是", "叫错了"} {
		if containsRune(text, kw) {
			return true
		}
	}
	return false
}

func containsRune(text, sub string) bool {
	return indexRuneSeq([]rune(text), []rune(sub)) >= 0
}
