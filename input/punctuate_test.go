package input

// punctuate_test.go -- M7 server endtgtptafterhandle bot    ( rule,  path connectline). 

import "testing"

// TestPunctuateSentenceEnd   sentend   ". ". 
func TestPunctuateSentenceEnd(t *testing.T) {
	in := "把那个错误提示改成中文"
	got := Punctuate(in)
	if want := "把那个错误提示改成中文。"; got != want {
		t.Errorf("陈述句落句号: got %q, want %q", got, want)
	}
}

// TestPunctuateQuestion   sent(  word in)sentend " ". 
func TestPunctuateQuestion(t *testing.T) {
	cases := map[string]string{
		"这个报价客户是哪家":  "这个报价客户是哪家？",
		"为什么凌晨告警一直响": "为什么凌晨告警一直响？",
		"能不能帮我看一下库存": "能不能帮我看一下库存？",
		"今天订单状态怎么样":  "今天订单状态怎么样？",
	}
	for in, want := range cases {
		if got := Punctuate(in); got != want {
			t.Errorf("疑问句落问号: got %q, want %q", got, want)
		}
	}
}

// TestPunctuateIdempotent already sentendtgtpt  base heavy ; heavy calluseclose   . 
func TestPunctuateIdempotent(t *testing.T) {
	already := "把那个错误提示改成中文。"
	if got := Punctuate(already); got != already {
		t.Errorf("已带句号不应改动: got %q", got)
	}
	question := "这个客户是哪家？"
	if got := Punctuate(question); got != question {
		t.Errorf("已带问号不应改动: got %q", got)
	}
	//  etc: toclose againcalluse   etcatclose . 
	once := Punctuate("查一下昨天订单")
	twice := Punctuate(once)
	if once != twice {
		t.Errorf("重复调用不一致: once=%q twice=%q", once, twice)
	}
}

// TestPunctuateNoPunctuationForCode numchar/  /URL/ codepath  tgtpt. 
func TestPunctuateNoPunctuationForCode(t *testing.T) {
	cases := []string{
		"go test ./...",
		"https://example.com/order?id=123",
		"2026-10-02 14:30",
		"npm install voicesign",
		"",
	}
	for _, in := range cases {
		if got := Punctuate(in); got != in {
			t.Errorf("纯代码/数字/URL 不应加标点: input %q -> got %q", in, got)
		}
	}
}
