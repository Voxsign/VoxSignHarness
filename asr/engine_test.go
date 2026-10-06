// engine_test.go --  now patchfill   (   asrharness tgt ,   `go test ./...`   ). 
//
//   : harness_test.go is** data**(rule ,  allowmodify); basefileis** now  safesafety **, 
// pipesame approvelang  safesafetysideinvariant  default forbid, andoverwrite data write  boundary: 
//   ,  time  ,    route,  line  , andsend  numity. 
//
//  datafile  tgt time  and  ,  by   lang   anddisconnectlangis   now,  is  data. 
package asr

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

type corpusCase struct {
	ID     string   `json:"id"`
	Raw    string   `json:"raw"`
	Tags   []string `json:"tags"`
	Expect struct {
		Corrected   string `json:"corrected"`
		Fidelity    bool   `json:"fidelity"`
		AskEmpty    bool   `json:"ask_empty"`
		AskNonempty bool   `json:"ask_nonempty"`
	} `json:"expect"`
}

func loadCorpusFile(t *testing.T) []corpusCase {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("corpus", "real-dialogue.jsonl"))
	if err != nil {
		t.Fatalf("读语料失败: %v", err)
	}
	var out []corpusCase
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var c corpusCase
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			t.Fatalf("语料非法行: %v", err)
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		t.Fatal("语料为空")
	}
	return out
}

func tagged(c corpusCase, tag string) bool {
	for _, t := range c.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
//  fillword  :  pt as
// ---------------------------------------------------------------------------

func TestFillerGuardTable(t *testing.T) {
	cases := []struct {
		name, raw, want string
	}{
		{"句首填充串全删", "嗯那个呃记一下这个想法", "记一下这个想法"},
		{"句首感叹词连同从句标点", "嗯，有个这样的重要的问题", "有个这样的重要的问题"},
		{"句中填充词保留（就是不是填充词）", "就是这里时候要不要用deept的", "就是这里时候要不要用deept的"},
		{"句首指示代词单独出现不删", "这个哈尼斯做一个AR识别的", "这个哈尼斯做一个AR识别的"},
		{"句中指示代词不删", "报价页那个别字", "报价页那个错别字"},
		{"纯指示代词噪声不动", "那个那个那个", "那个那个那个"},
		{"纯感叹词噪声不动（空串守卫）", "呃呃呃", "呃呃呃"},
		{"句中标点后的感叹词删掉", "有那个。嗯开始吧", "有那个。开始吧"},
		{"指示代词被夹住时随串删", "嗯那个呃记一下", "记一下"},
	}
	eng := NewEngine()
	for _, tc := range cases {
		got := eng.Correct(CorrectRequest{Raw: tc.raw})
		if got.Text != tc.want {
			t.Errorf("%s: %q → %q，期望 %q", tc.name, tc.raw, got.Text, tc.want)
		}
	}
}

// TestTruncationGuard preventstop disconnectalsoorig  : " diffchar"  againbealsoorig  . 
func TestTruncationGuard(t *testing.T) {
	eng := NewEngine()
	for _, raw := range []string{"错别字", "这个是错别字", "别字"} {
		got := eng.Correct(CorrectRequest{Raw: raw})
		if raw == "别字" {
			if got.Text != "错别字" {
				t.Errorf("别字 → %q，期望 错别字", got.Text)
			}
			continue
		}
		if strings.Contains(got.Text, "错错别字") {
			t.Errorf("套娃：%q → %q", raw, got.Text)
		}
	}
}

// ---------------------------------------------------------------------------
//    :  time      orig  charnode timeon
// ---------------------------------------------------------------------------

func TestCorrectionSpansExact(t *testing.T) {
	eng := NewEngine()
	got := eng.Correct(CorrectRequest{Raw: "嗯，有个这样的重要的问题"})
	if got.Text != "有个这样的重要的问题" {
		t.Fatalf("文本: %q", got.Text)
	}
	if len(got.Corrections) != 1 {
		t.Fatalf("期望 1 条 Correction，实际 %d", len(got.Corrections))
	}
	c := got.Corrections[0]
	if c.Start != 0 || c.End != len("嗯，") || c.From != "嗯，" || c.To != "" {
		t.Fatalf("区间/片段不符: %+v（「嗯，」字节长 %d）", c, len("嗯，"))
	}
	if c.Kind == "" || c.Evidence == "" {
		t.Fatalf("缺 kind/evidence: %+v", c)
	}
	if c.Confidence <= 0 || c.Confidence > 1 {
		t.Fatalf("置信度越界: %v", c.Confidence)
	}
}

func TestCorrectionSpansNeverEscapeRaw(t *testing.T) {
	eng := NewEngine()
	for _, c := range loadCorpusFile(t) {
		got := eng.Correct(CorrectRequest{Raw: c.Raw})
		if got.Text == c.Raw && len(got.Corrections) > 0 {
			t.Errorf("[%s] 未改却有 Correction", c.ID)
		}
		for _, cor := range got.Corrections {
			if cor.Start < 0 || cor.End > len(c.Raw) || cor.Start >= cor.End {
				t.Errorf("[%s] 区间非法 %+v (raw len=%d)", c.ID, cor, len(c.Raw))
				continue
			}
			if c.Raw[cor.Start:cor.End] != cor.From {
				t.Errorf("[%s] From 与原文区间不一致: %q vs %q", c.ID, c.Raw[cor.Start:cor.End], cor.From)
			}
		}
	}
}

// ---------------------------------------------------------------------------
//    route: only  ,   modify base
// ---------------------------------------------------------------------------

func TestPinyinCandidateSurfaces(t *testing.T) {
	eng := NewEngine()
	got := eng.Correct(CorrectRequest{Raw: "提胶一下"})
	if got.Text != "提胶一下" {
		t.Fatalf("候选支路不得改文本，实际 %q", got.Text)
	}
	if !hasCandidate(got, "提交") {
		t.Fatalf("拼音近音未召回 提交: %+v", got.Candidates)
	}
	// onunder  in oncall   
	plain := eng.Correct(CorrectRequest{Raw: "提胶一下"})
	boosted := eng.Correct(CorrectRequest{Raw: "提胶一下", Context: []string{"提交这个文件"}})
	if confOf(boosted, "提交") <= confOf(plain, "提交") {
		t.Fatalf("上下文未上调候选置信度: plain=%v boosted=%v",
			confOf(plain, "提交"), confOf(boosted, "提交"))
	}
}

func TestProperNounsOnlyCandidate(t *testing.T) {
	eng := NewEngine()
	cases := map[string]string{
		"就是这里时候要不要用deept的":            "DeepSeek",
		"我这里一个核心思想不是让deep sick直接去改代码": "DeepSeek",
		"这个哈尼斯做一个AR识别的汉尼斯":            "harness",
	}
	for raw, want := range cases {
		got := eng.Correct(CorrectRequest{Raw: raw})
		if got.Text != raw {
			t.Errorf("专名不得自动改写：%q → %q", raw, got.Text)
		}
		if !hasCandidate(got, want) {
			t.Errorf("专名未给候选 %q: %+v", want, got.Candidates)
		}
	}
	// AAAR    AR    in(  word boundary  )
	got := eng.Correct(CorrectRequest{Raw: "不是让deep sick去改那个AAAR的代码"})
	for _, c := range got.Candidates {
		if c.Text == "ASR" {
			t.Errorf("AAAR 中的子串不应命中 AR→ASR: %+v", got.Candidates)
		}
	}
}

// ---------------------------------------------------------------------------
// Observe only  data,  writeback(ASR-MODEL-02 L2); writebackonly  learn
// ---------------------------------------------------------------------------

func TestObserveRecordsEvidenceOnly(t *testing.T) {
	eng := NewEngine()
	before := eng.Lexicon("dev").Version

	if err := eng.Observe(Feedback{Raw: "哈牛斯", Corrected: "harness", Accepted: true, Source: "user_edit"}); err != nil {
		t.Fatalf("Observe: %v", err)
	}
	after := eng.Lexicon("dev").Version
	if after == before {
		t.Fatal("Observe 未产生新版本（热词审计应更新）")
	}

	//  data  be under( is learn   in)
	ev := eng.Evidence()
	if len(ev) != 1 || ev[0].Raw != "哈牛斯" || ev[0].Corrected != "harness" || !ev[0].Accepted || ev[0].Source != "user_edit" || ev[0].At == "" {
		t.Fatalf("证据记录不完整: %+v", ev)
	}

	// but**  writeback  **:   modify base, also produceoccur  
	got := eng.Correct(CorrectRequest{Raw: "把哈牛斯接上"})
	if got.Text != "把哈牛斯接上" {
		t.Fatalf("Observe 不得自动改写: %q", got.Text)
	}
	if hasCandidate(got, "harness") {
		t.Fatalf("Observe 不得写回候选（写回只走 learn）: %+v", got.Candidates)
	}

	lex := eng.Lexicon("dev")
	if lex.Domain != "dev" {
		t.Errorf("Domain = %q", lex.Domain)
	}
	if !hasHotword(lex, "harness") {
		t.Errorf("Lexicon 未记录审计热词 harness: %+v", lex.Hotwords)
	}
}

func TestObserveEvidenceKeepsBothOutcomes(t *testing.T) {
	eng := NewEngine()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(eng.Observe(Feedback{Raw: "哈牛斯", Corrected: "harness", Accepted: true, Source: "user_edit"}))
	must(eng.Observe(Feedback{Raw: "哈牛斯", Corrected: "harness", Accepted: false, Source: "user_edit"}))

	ev := eng.Evidence()
	if len(ev) != 2 || !ev[0].Accepted || ev[1].Accepted {
		t.Fatalf("证据应保留接受/否认两次记录: %+v", ev)
	}
	// no connectacceptand , all writeback  
	got := eng.Correct(CorrectRequest{Raw: "把哈牛斯接上"})
	if hasCandidate(got, "harness") {
		t.Fatalf("Observe 不得写回候选: %+v", got.Candidates)
	}
}

func TestObserveRejectsEmptyFeedback(t *testing.T) {
	eng := NewEngine()
	if err := eng.Observe(Feedback{}); err == nil {
		t.Fatal("空反馈应当报错")
	}
}

// ---------------------------------------------------------------------------
// Correct    numity:  andsend,  back 
// ---------------------------------------------------------------------------

func TestCorrectConcurrentAndReplayable(t *testing.T) {
	eng := NewEngine()
	inputs := []string{
		"嗯那个呃记一下这个想法",
		"就是这里时候要不要用deept的",
		"帮我把这个文件题交一下",
		"那个那个那个",
	}
	want := make([]string, len(inputs))
	for i, in := range inputs {
		want[i] = eng.Correct(CorrectRequest{Raw: in}).Text
	}

	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := 0; k < 200; k++ {
				i := k % len(inputs)
				if got := eng.Correct(CorrectRequest{Raw: inputs[i]}).Text; got != want[i] {
					t.Errorf("并发下不一致: %q → %q，期望 %q", inputs[i], got, want[i])
					return
				}
			}
		}()
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// lang  invariant(and harness_test.go same lang , but   tgt also )
// ---------------------------------------------------------------------------

func TestCorpusSafetyInvariants(t *testing.T) {
	eng := NewEngine()
	fixed := 0
	for _, c := range loadCorpusFile(t) {
		got := eng.Correct(CorrectRequest{Raw: c.Raw})
		if c.Expect.Fidelity || tagged(c, "negation") || tagged(c, "safety") || tagged(c, "long") {
			if got.Text != c.Raw {
				t.Errorf("保真红线被破 [%s]: %q → %q", c.ID, c.Raw, got.Text)
			}
		}
		if tagged(c, "noise") {
			if len([]rune(got.Text)) > len([]rune(c.Raw)) {
				t.Errorf("噪声被脑补变长 [%s]: %q → %q", c.ID, c.Raw, got.Text)
			}
			if got.Text != c.Raw && len(got.Candidates) == 0 {
				t.Errorf("噪声被唯一改写且无候选 [%s]: %q", c.ID, got.Text)
			}
		}
		// C3  " clarification"     :  voice -> Candidates  emptyand  ask:noise before . 
		if c.Expect.AskNonempty && len(got.Candidates) == 0 {
			t.Errorf("ask_nonempty 未交付 [%s]: 噪声没有发出任何回问信号", c.ID)
		}
		// revto: has   sent   becurbecome voice(   clarification). 
		if c.Expect.AskEmpty && len(got.Candidates) != 0 {
			t.Errorf("ask_empty 被破 [%s]: 正常句却给了候选 %+v", c.ID, got.Candidates)
		}
		if c.Expect.Corrected != "" && c.Expect.Corrected != c.Raw {
			if got.Text != c.Expect.Corrected {
				t.Errorf("未还原 [%s]: 期望 %q 实际 %q", c.ID, c.Expect.Corrected, got.Text)
				continue
			}
			if len(got.Corrections) == 0 {
				t.Errorf("还原无记录 [%s]", c.ID)
			}
			fixed++
		}
	}
	if fixed == 0 {
		t.Fatal("语料里一条都没还原——判据形同虚设")
	}
	t.Logf("语料级还原 %d 条", fixed)
}

// ---------------------------------------------------------------------------
// ity :  dataonlydisconnectlang**  ** < 1ms,  recvtgtapprovewrite is p99,   patchon
// ---------------------------------------------------------------------------

func TestPerformanceP99(t *testing.T) {
	eng := NewEngine()
	raw := strings.Repeat("把报价单改成中文", 6)
	const n = 2000
	durs := make([]time.Duration, 0, n)
	for i := 0; i < n; i++ {
		durs = append(durs, eng.Correct(CorrectRequest{Raw: raw}).Latency)
	}
	sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
	p50, p99 := durs[n/2], durs[n*99/100]
	if p99 > time.Millisecond {
		t.Fatalf("p99 = %v，超过 1ms 目标", p99)
	}
	t.Logf("快路 p50=%v p99=%v（%d 次，含 Latency 计时开销）", p50, p99, n)
}

// TestMemoryFootprint  code recvtgtapprove"  instore < 50 MB". 
//    form    :      + 100  fastroutecalluseafter  add (   time voice). 
func TestMemoryFootprint(t *testing.T) {
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	eng := NewEngine()
	var sink string
	for i := 0; i < 100; i++ {
		sink = eng.Correct(CorrectRequest{Raw: "嗯那个呃把报价单改成中文，提胶一下"}).Text
	}
	runtime.ReadMemStats(&after)
	if sink == "" {
		t.Fatal("sink 为空")
	}
	used := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	if used > 50<<20 {
		t.Fatalf("堆增量 %d KB，超过 50 MB 目标", used/1024)
	}
	t.Logf("引擎堆增量 %d KB（含一次构造 + 100 次快路）", used/1024)
}

// TestPinyinTableDeterministic   " back ":  audiotable  dependency Go map     . 
func TestPinyinTableDeterministic(t *testing.T) {
	a, b := buildPinyinTable(), buildPinyinTable()
	if len(a) != len(b) {
		t.Fatalf("两次构建长度不同: %d vs %d", len(a), len(b))
	}
	for r, syl := range a {
		if b[r] != syl {
			t.Fatalf("两次构建不一致: %q → %q vs %q", r, syl, b[r])
		}
	}
	// tablestore andclose char readaudio   period(prevent ). 
	for r, want := range map[rune]string{'题': "ti", '提': "ti", '交': "jiao", '报': "bao", '存': "cun"} {
		if got := a[r]; got != want {
			t.Errorf("%q → %q，期望 %q", r, got, want)
		}
	}
}

// TestPinyinTableHasNoCrossGroupDuplicate   " audiochar intable": 
// samechar audionode    buildPinyinTable  "firsttofirst "dependency   , 
// is P1   referout     . need   heavy , need  formdecide readaudio. 
func TestPinyinTableHasNoCrossGroupDuplicate(t *testing.T) {
	seen := make(map[rune]string)
	for _, g := range pinyinGroups {
		for _, r := range g.chars {
			if prev, ok := seen[r]; ok {
				t.Errorf("多音字跨组重复：%q 同时在 %q 与 %q（请删掉一个或显式决定）", r, prev, g.syllable)
				continue
			}
			seen[r] = g.syllable
		}
	}
}

// TestNoiseEmitsAskSignal is P2     connect data:  voice  sendout    clarificationsignal. 
//   : Candidates  has  , Text==Raw, Confidence==0, Reason by ask:noise openhead. 
func TestNoiseEmitsAskSignal(t *testing.T) {
	eng := NewEngine()
	for _, raw := range []string{"呃呃呃", "那个那个那个", "嗯，那个呃，嗯", "这个"} {
		got := eng.Correct(CorrectRequest{Raw: raw})
		if got.Text != raw {
			t.Errorf("噪声不得改文本 [%q]: %q", raw, got.Text)
		}
		if len(got.Corrections) != 0 {
			t.Errorf("噪声不改文本却记录了 Correction [%q]: %+v", raw, got.Corrections)
		}
		if len(got.Candidates) != 1 {
			t.Fatalf("噪声回问信号应恰有 1 条候选 [%q]: %+v", raw, got.Candidates)
		}
		c := got.Candidates[0]
		if c.Text != raw || c.Confidence != 0 || !strings.HasPrefix(c.Reason, askNoiseReasonPrefix) {
			t.Errorf("回问信号约定不符 [%q]: %+v", raw, c)
		}
	}
	// revto: has   sent   triggersendclarificationsignal. 
	for _, raw := range []string{"查一下库存", "把报价单改成中文", "开始测试"} {
		got := eng.Correct(CorrectRequest{Raw: raw})
		for _, c := range got.Candidates {
			if strings.HasPrefix(c.Reason, askNoiseReasonPrefix) {
				t.Errorf("正常句被误判为噪声 [%q]: %+v", raw, c)
			}
		}
	}
}

// TestConcurrentCorrectAndObserve is P4   needrequire   andsend: 
// reader(Correct/Lexicon)andwriteer(Observe)sametime  ,    `go test -race`  use. 
func TestConcurrentCorrectAndObserve(t *testing.T) {
	eng := NewEngine()
	raw := "嗯那个呃记一下这个想法"
	want := eng.Correct(CorrectRequest{Raw: raw}).Text

	done := make(chan struct{})
	var readers, writers sync.WaitGroup

	for r := 0; r < 4; r++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				got := eng.Correct(CorrectRequest{Raw: raw})
				if got.Text != want {
					t.Errorf("并发下文本漂移: %q，期望 %q", got.Text, want)
					return
				}
				for _, cor := range got.Corrections {
					if cor.Start < 0 || cor.End > len(raw) || cor.Start >= cor.End {
						t.Errorf("并发下区间非法: %+v", cor)
						return
					}
				}
				_ = eng.Lexicon("dev")
			}
		}()
	}

	for w := 0; w < 2; w++ {
		writers.Add(1)
		go func(id int) {
			defer writers.Done()
			target := "harness-" + string(rune('a'+id))
			for i := 0; i < 300; i++ {
				_ = eng.Observe(Feedback{Raw: "哈牛斯", Corrected: target, Accepted: true, Source: "user_edit"})
				_ = eng.Observe(Feedback{Raw: "哈牛斯", Corrected: target, Accepted: false, Source: "user_edit"})
			}
		}(w)
	}

	writers.Wait()
	close(done)
	readers.Wait()
}

// ---------------------------------------------------------------------------
//    
// ---------------------------------------------------------------------------

func hasCandidate(r CorrectResult, text string) bool {
	for _, c := range r.Candidates {
		if c.Text == text {
			return true
		}
	}
	return false
}

func confOf(r CorrectResult, text string) float64 {
	for _, c := range r.Candidates {
		if c.Text == text {
			return c.Confidence
		}
	}
	return -1
}

func hasHotword(l Lexicon, term string) bool {
	for _, h := range l.Hotwords {
		if h.Term == term {
			return true
		}
	}
	return false
}
