//go:build asrharness

// ASR  diff harness -- C1–C4  data + ity  data. 
//
// **P1 stage: base    baseline Passthrough on,  cur  is  . **
//   =   also  ,  is"  write ".  datafirstat now(     §0). 
//
// lang : asr/corpus/real-dialogue.jsonl -- **safety    owner and DSH    to **, 
//     source outplace.     to    . 
//
//   : go test -tags asrharness ./asr -v
package asr

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Case is  lang . charsegand jsonl to . 
type Case struct {
	ID     string   `json:"id"`
	Raw    string   `json:"raw"`
	Issue  string   `json:"issue"`
	Source string   `json:"source"`
	Tags   []string `json:"tags"`
	Expect struct {
		Intent      string `json:"intent"`
		Corrected   string `json:"corrected"`
		Fidelity    bool   `json:"fidelity"`
		AskEmpty    bool   `json:"ask_empty"`
		AskNonempty bool   `json:"ask_nonempty"`
	} `json:"expect"`
}

func loadCorpus(t *testing.T) []Case {
	t.Helper()
	//   authoritative : asr/corpus/real-dialogue.jsonl(eval/asr-corpus  basealready  )
	for _, p := range []string{
		filepath.Join("corpus", "real-dialogue.jsonl"),
	} {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var out []Case
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var c Case
			if err := json.Unmarshal([]byte(line), &c); err != nil {
				t.Fatalf("语料 %s 有非法行: %v", p, err)
			}
			out = append(out, c)
		}
		return out
	}
	t.Fatal("找不到语料 real-dialogue.jsonl —— 语料是这套桩的地基，不能缺")
	return nil
}

// engine isbase     to (**connectlinept**,  is database ). 
// P2 raiseconnectoccurproduce  ; baseline Passthrough     TestBaselinePassthrough  to . 
func engine() Engine { return NewEngine() }

// hasTag   lang is   tgt . 
func hasTag(c Case, tag string) bool {
	for _, t := range c.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// C1         -- keep (safety side)
//
// revto    : correction**  **pipe  ,   ,  sentmodify . 
// baseline Passthrough dayhowever ed;  nowstage  ed  pos,     . 
// ---------------------------------------------------------------------------
func TestC1FidelityNeverOverCorrect(t *testing.T) {
	eng := engine()
	for _, c := range loadCorpus(t) {
		// keep class: correction   origkindetcat raw
		if c.Expect.Fidelity || hasTag(c, "negation") || hasTag(c, "safety") || hasTag(c, "long") {
			got := eng.Correct(CorrectRequest{Raw: c.Raw})
			if got.Text != c.Raw {
				t.Errorf("C1 保真被破坏 [%s] %q → %q\n    这是安全侧红线：纠错不得改动否定/条件/长句",
					c.ID, c.Raw, got.Text)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// C2 triggersendwordsameaudio   --       
//
// revto    :  chartriggersendwordbe  after   alsoorigintent. 
// baseline Passthrough    on however . 
// ---------------------------------------------------------------------------
func TestC2TriggerWordHomophone(t *testing.T) {
	eng := engine()
	for _, c := range loadCorpus(t) {
		if c.Expect.Corrected == "" || c.Expect.Corrected == c.Raw {
			continue
		}
		got := eng.Correct(CorrectRequest{Raw: c.Raw})
		if got.Text != c.Expect.Corrected {
			t.Errorf("C2 未纠正 [%s] %s\n    原文: %q\n    期望: %q\n    实际: %q\n    问题: %s",
				c.ID, c.Tags, c.Raw, c.Expect.Corrected, got.Text, c.Issue)
		}
	}
}

// ---------------------------------------------------------------------------
// C3  voice  patch --   voice  be become" raise   " sent 
//
//  to: ① voice  produceout  /modifywritebecome    base ②  voice  be as voice. 
// ---------------------------------------------------------------------------
func TestC3NoiseNeverHallucinated(t *testing.T) {
	eng := engine()
	for _, c := range loadCorpus(t) {
		if !hasTag(c, "noise") {
			continue
		}
		got := eng.Correct(CorrectRequest{Raw: c.Raw})
		//  voice posafter  change change"   "(  use     :    emptychange )
		if len([]rune(got.Text)) > len([]rune(c.Raw)) {
			t.Errorf("C3 脑补 [%s] %q → %q（凭空变长 = 在猜）", c.ID, c.Raw, got.Text)
		}
		// if  giveout  ,         ;     . 
		// but**  **giveoutuniqueand    modify    . 
		if got.Text != c.Raw && len(got.Candidates) == 0 {
			t.Errorf("C3 脑补 [%s] 噪声被唯一确定地改写为 %q，且无候选供人确认", c.ID, got.Text)
		}
	}
}

// ---------------------------------------------------------------------------
// C4     --   correction    data, raw    bot
//
//  to: ①modify     ② modify   empty  . 
// ---------------------------------------------------------------------------
func TestC4CorrectionsAreObservable(t *testing.T) {
	eng := engine()
	for _, c := range loadCorpus(t) {
		got := eng.Correct(CorrectRequest{Raw: c.Raw})

		//  modifythen  has  
		if got.Text == c.Raw && len(got.Corrections) > 0 {
			t.Errorf("C4 凭空记录 [%s] 文本未改却有 %d 条 Correction", c.ID, len(got.Corrections))
		}
		// modifythen  has  , and time back 
		if got.Text != c.Raw {
			if len(got.Corrections) == 0 {
				t.Errorf("C4 不可观测 [%s] 改成了 %q 却没有 Correction 记录", c.ID, got.Text)
				continue
			}
			for _, cor := range got.Corrections {
				if cor.Start < 0 || cor.End > len(c.Raw) || cor.Start >= cor.End {
					t.Errorf("C4 区间非法 [%s] start=%d end=%d (raw len=%d)", c.ID, cor.Start, cor.End, len(c.Raw))
				}
				if cor.Evidence == "" || cor.Kind == "" {
					t.Errorf("C4 缺证据 [%s] kind=%q evidence=%q", c.ID, cor.Kind, cor.Evidence)
				}
			}
		}
		// period has pos  obj,     has pos  
		if c.Expect.Corrected != "" && c.Expect.Corrected != c.Raw && len(got.Corrections) == 0 {
			t.Errorf("C4 未记录 [%s] 期望纠正为 %q 但一条 Correction 都没有", c.ID, c.Expect.Corrected)
		}
	}
}

// ---------------------------------------------------------------------------
// ity  data -- "fast"    
//
// baseline Passthrough   ed; but nowstage   in  safetytable  ,     . 
// ---------------------------------------------------------------------------
func TestPerfFastPathLatency(t *testing.T) {
	eng := engine()
	raw := strings.Repeat("把报价单改成中文", 6) //   48 char
	if n := len([]rune(raw)); n < 40 {
		t.Fatalf("测试前提：样本应为 50 字量级，实际 %d 字", n)
	}

	const N = 2000
	start := time.Now()
	for i := 0; i < N; i++ {
		eng.Correct(CorrectRequest{Raw: raw})
	}
	avg := time.Since(start) / N

	// objtgt:   at 1ms(fastroute  is sec )
	if avg > time.Millisecond {
		t.Errorf("快路过慢：平均 %v/次（目标 < 1ms，实为快路应达微秒级）", avg)
	}
	t.Logf("快路平均延迟 %v/次（%d 次）", avg, N)
}

// TestPerfOfflineCapable disconnect   : fastroute  dependencyout calluse. 
//
//  data form:  no      value Engine on     Correct(  panic,    ). 
func TestPerfOfflineCapable(t *testing.T) {
	eng := engine()
	done := make(chan struct{})
	go func() {
		defer close(done)
		eng.Correct(CorrectRequest{Raw: "查一下库存"})
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("快路阻塞超过 2s —— 疑似依赖了外部调用（违背「断网可跑」）")
	}
}

// ---------------------------------------------------------------------------
//   to: pipebaseline(  pos)   ,  as"changebefore"   . 
//
//     ** disconnectlang ed**-- onlypipebaseline     out , 
// and engine()  close and ,  become"  and   ". 
// ---------------------------------------------------------------------------
func TestBaselinePassthroughComparison(t *testing.T) {
	base := Passthrough{}
	prod := engine()
	var baseFixed, prodFixed, total int
	for _, c := range loadCorpus(t) {
		if c.Expect.Corrected == "" || c.Expect.Corrected == c.Raw {
			continue
		}
		total++
		if base.Correct(CorrectRequest{Raw: c.Raw}).Text == c.Expect.Corrected {
			baseFixed++
		}
		if prod.Correct(CorrectRequest{Raw: c.Raw}).Text == c.Expect.Corrected {
			prodFixed++
		}
	}
	t.Logf("自比对（%d 条期望纠正）：改动前 %d 条 / 改动后 %d 条", total, baseFixed, prodFixed)
	if prodFixed <= baseFixed {
		t.Errorf("改动后没有提升：基线 %d 条，现状 %d 条", baseFixed, prodFixed)
	}
}

// ---------------------------------------------------------------------------
// C3-bis  voice  sendout"   " signal(patch dataempty )
//
// diff   referout: orig C3 only " change  + has  ", but Passthrough(  posbaseline)
// also  100%  ed --    to now  **    **(     §9: nokindbasethen  ). 
// and Case    AskNonempty/AskEmpty/Intent   charseg**resolve butfromnodisconnectlang**. 
//
// base  patchondisconnectlang. by asr.go    : **Candidates  empty =    /  out **. 
// atis" voice  clarification"  ASR       formthenis:  voicekindbase  giveout empty Candidates. 
//
//  datafirstat now: base    now giveoutsignalbefore curis**  **. 
// ---------------------------------------------------------------------------
func TestC3BisNoiseMustSignalAsk(t *testing.T) {
	eng := engine()
	var checked int
	for _, c := range loadCorpus(t) {
		if !c.Expect.AskNonempty {
			continue
		}
		checked++
		got := eng.Correct(CorrectRequest{Raw: c.Raw})
		//  voice  bemodifywritebecome" raise   "  base
		if got.Text != c.Raw {
			t.Errorf("C3-bis [%s] 噪声被改写：%q → %q", c.ID, c.Raw, got.Text)
		}
		//   sendout"   " signal --  thenon nofrom   is voice. 
		//   ( now  2026-10-03 frozen):  voicereturnback**    **  , 
		// Text == Raw( givemodifywrite  ), Confidence == 0( has  modifywrite), 
		// Reason by "ask:noise" openhead(      before ). 
		if len(got.Candidates) == 0 {
			t.Errorf("C3-bis [%s] 噪声 %q 既未改写**也未给出候选** —— "+
				"上层拿不到任何「这是噪声、该回问」的信号（判据空洞，Passthrough 也能过）",
				c.ID, c.Raw)
			continue
		}
		cand := got.Candidates[0]
		if cand.Text != c.Raw {
			t.Errorf("C3-bis [%s] 噪声候选不得给出改写建议：Text=%q 应等于原文 %q", c.ID, cand.Text, c.Raw)
		}
		if cand.Confidence != 0 {
			t.Errorf("C3-bis [%s] 噪声候选置信度应为 0（无可信改写），实际 %v", c.ID, cand.Confidence)
		}
		if !strings.HasPrefix(cand.Reason, "ask:noise") {
			t.Errorf("C3-bis [%s] 噪声候选 Reason 应以 ask:noise 开头（稳定机器可判前缀），实际 %q",
				c.ID, cand.Reason)
		}
	}
	if checked == 0 {
		t.Fatal("语料里没有 AskNonempty 的条目 —— 该判据无样本，等于没测")
	}
	t.Logf("C3-bis 覆盖 %d 条噪声样本", checked)

	// ---- revto: has   sent **  **giveout  ( thenthenisno ly  useuser)----
	var reverse int
	for _, c := range loadCorpus(t) {
		if !c.Expect.AskEmpty {
			continue
		}
		reverse++
		if got := eng.Correct(CorrectRequest{Raw: c.Raw}); len(got.Candidates) != 0 {
			t.Errorf("C3-bis 反向 [%s] 有实义的句子 %q 不该给出候选，实际 %d 条",
				c.ID, c.Raw, len(got.Candidates))
		}
	}
	if reverse == 0 {
		t.Error("C3-bis 反向无样本 —— 只测「该问的问了」，没测「不该问的没问」，是单向判据")
	}
	t.Logf("C3-bis 反向覆盖 %d 条 ask_empty 样本", reverse)
}

// TestCaseExpectFieldsAllExercised preventstop"charsegresolve but  use"again sendoccur. 
func TestCaseExpectFieldsAllExercised(t *testing.T) {
	//    charseg  Case close  store ,      has  lang  use, 
	//  then   data   has" charseg"--beresolve but  and  . 
	var nIntent, nAskEmpty, nAskNonempty, nCorrected, nFidelity int
	for _, c := range loadCorpus(t) {
		if c.Expect.Intent != "" {
			nIntent++
		}
		if c.Expect.AskEmpty {
			nAskEmpty++
		}
		if c.Expect.AskNonempty {
			nAskNonempty++
		}
		if c.Expect.Corrected != "" {
			nCorrected++
		}
		if c.Expect.Fidelity {
			nFidelity++
		}
	}
	t.Logf("期望字段使用情况：intent=%d ask_empty=%d ask_nonempty=%d corrected=%d fidelity=%d",
		nIntent, nAskEmpty, nAskNonempty, nCorrected, nFidelity)
	for name, n := range map[string]int{"intent": nIntent, "ask_empty": nAskEmpty, "ask_nonempty": nAskNonempty} {
		if n == 0 {
			t.Errorf("字段 expect.%s 无任何语料使用 —— 死字段（解析了但不参与判定）", name)
		}
	}
}
