package input

import (
	"strings"
	"testing"

	"voicesign-harness/config"
	"voicesign-harness/contract"
)

func TestClean(t *testing.T) {
	c := NewCleaner(nil)
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"句首填充词", "帮我打开文件", "打开文件"},
		// M7(Codex 2026-10-02): "  "is  coreferencewordalreadyfrom fillwordtable  ("fix  " "  "is  to ), 
		//   useexamplemodifyuse   fillword         . 
		{"多重句首填充词循环", "嗯请帮我列出目录", "列出目录"},
		{"句尾填充词", "打开文件一下", "打开文件"},
		{"全角转半角", "打开ＶｏｘＳｉｇｎ", "打开VoxSign"},
		{"全角空格与连续空白", "打开　  文件", "打开 文件"},
		{"去句末标点", "列出目录。", "列出目录"},
		{"保留路径字符", "读 /home/a_b/c.txt", "读 /home/a_b/c.txt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := c.Clean(tc.in); got != tc.want {
				t.Errorf("Clean(%q) = %q, 期望 %q", tc.in, got, tc.want)
			}
		})
	}
}

// stubCorrecter    memory.Dictionary: pipe"  "->"Mansour". 
type stubCorrecter struct{}

func (stubCorrecter) Correct(text string) (string, []contract.Correction) {
	if !strings.Contains(text, "美墅") {
		return text, nil
	}
	return strings.ReplaceAll(text, "美墅", "Mansour"),
		[]contract.Correction{{From: "美墅", To: "Mansour", Rule: "dict"}}
}

func TestClassifyAllSix(t *testing.T) {
	c := NewClassifier(0.6)

	cases := []struct {
		name        string
		text        string
		wantIntent  string
		wantConfMin float64
		wantAsk     bool
	}{
		{"TIME", "现在几点了", contract.IntentTime, 0.95, false},
		{"FILE_LIST", "列出 ~/project 有什么", contract.IntentFileList, 0.8, false},
		{"FILE_READ", "读一下 ~/note.txt", contract.IntentFileRead, 0.8, false},
		{"FILE_READ缺槽位", "读一下内容", contract.IntentFileRead, 0.6, false}, // 0.6  <0.6
		{"SHELL", "运行 ls -la", contract.IntentShell, 0.7, false},
		{"SHELL缺cmd低置信回问", "运行", contract.IntentShell, 0.5, true}, // 0.5<0.6
		{"INFO", "翻译这句话", contract.IntentInfo, 0.4, false},
		{"UNKNOWN无触发词", "随便来点什么", contract.IntentUnknown, 0.2, true},
		{"UNKNOWN空文本", "", contract.IntentUnknown, 0.2, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := c.Classify(tc.text)
			if got.Intent != tc.wantIntent {
				t.Fatalf("intent = %q, 期望 %q", got.Intent, tc.wantIntent)
			}
			if got.Confidence < tc.wantConfMin-1e-9 || got.Confidence > tc.wantConfMin+1e-9 {
				t.Errorf("confidence = %.3f, 期望 %.3f", got.Confidence, tc.wantConfMin)
			}
			if got.NeedsClarification() != tc.wantAsk {
				t.Errorf("Ask = %q, needs=%v, 期望 needs=%v", got.Ask, got.NeedsClarification(), tc.wantAsk)
			}
		})
	}
}

// TIME/INFO   clarification; UNKNOWN  clarification. 
func TestAskNeverForTimeInfo(t *testing.T) {
	c := NewClassifier(0.0) //  value 0:   low-confidenceall clarification,   TIME/INFO
	if got := c.Classify("几点"); got.Ask != "" {
		t.Errorf("TIME 即使低阈值也不应回问: %q", got.Ask)
	}
	// INFO by  triggersendwordtriggersend,   clarification
	if got := c.Classify("帮我翻译一下"); got.Intent != contract.IntentInfo || got.Ask != "" {
		t.Errorf("INFO 不应回问: intent=%q ask=%q", got.Intent, got.Ask)
	}
	// no  triggersendword -> UNKNOWN,  clarification
	if got := c.Classify("随便说点啥"); got.Intent != contract.IntentUnknown || got.Ask == "" {
		t.Errorf("无触发词应 UNKNOWN 且回问: intent=%q ask=%q", got.Intent, got.Ask)
	}
}

//   1back : "file "before  word    wordbefore ( openMansour -> Mansour). 
func TestExtractPathStripsVerbPrefix(t *testing.T) {
	cfg := config.Default()
	p := NewPipeline(&cfg, stubCorrecter{})

	raw := "帮我打开美墅的文件夹看看有什么"
	res, err := p.Process(raw)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if res.Intent.Intent != contract.IntentFileList {
		t.Fatalf("intent = %q, 期望 FILE_LIST", res.Intent.Intent)
	}
	got := res.Intent.Slots["path"]
	if got != "Mansour" {
		t.Errorf("path = %q, 期望剥离动词前缀后为 %q", got, "Mansour")
	}
}

// Pipeline safety   : ①-④. 
func TestPipelineProcess(t *testing.T) {
	cfg := config.Default()
	p := NewPipeline(&cfg, stubCorrecter{})

	raw := "帮我打开美墅的文件夹看看有什么"
	res, err := p.Process(raw)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}

	if res.Raw != raw {
		t.Errorf("Raw 必须原样保留, 实际 %q", res.Raw)
	}
	if !strings.Contains(res.Cleaned, "美墅") {
		t.Errorf("Cleaned 应保留美墅, 实际 %q", res.Cleaned)
	}
	if !strings.Contains(res.Corrected, "Mansour") {
		t.Errorf("Corrected 应含 Mansour, 实际 %q", res.Corrected)
	}
	if len(res.Corrections) != 1 || res.Corrections[0].To != "Mansour" {
		t.Errorf("Corrections 记录不符: %+v", res.Corrections)
	}
	if res.Intent.Intent != contract.IntentFileList {
		t.Errorf("Intent = %q, 期望 FILE_LIST", res.Intent.Intent)
	}
	if res.Intent.CorrectedText != res.Corrected {
		t.Errorf("Intent.CorrectedText 未填充: %q", res.Intent.CorrectedText)
	}
}
