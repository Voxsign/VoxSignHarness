package selfheal

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"voicesign-harness/contract"
	"voicesign-harness/provider"
)

// MaxAutoRetries issame   refer    heavy   onlimit(prevent   /  ). 
const MaxAutoRetries = 2

// retryBackoff isrefernum   list(connect tgtapprove v1: network/transient classrefernum  ). 
//   1  heavy beforeetc 500ms,   2  etc 1s;  ed MaxAutoRetries i.e.stopstop. 
var retryBackoff = []time.Duration{500 * time.Millisecond, 1 * time.Second}

// ToolRunner heavy   read-only    (by pipeline notein o.run).  disconnect only   safesafetytimecalluse . 
type ToolRunner func(tool string, args map[string]any) contract.Receipt

// Attempt is     tool execution( disconnect + safesafetyheavy   in,  origstart args bythenheavy ). 
type Attempt struct {
	Tool      string
	Args      map[string]any
	Receipt   contract.Receipt
	RequestID string // P0-4b   :  chain request_id,  in disconnect trace(day /trace  use)
}

// Service is        time example:     +        type + (  )  heavy  . 
// safety   to" disconnect  use/  "  returnback nil,   to chain  error. 
type Service struct {
	KB     *KB               // error   (①  , 0  typecalluse)
	Diag   provider.Provider //      type(②  ); nil =    ,  ed type disconnect
	Runner ToolRunner        // read-only  heavy  ; nil = only disconnect heavy 

	counters map[string]int // fingerprint -> alreadyuse  heavy   
	last     *Diagnosis     //      disconnectclose (provideattribution   state  )
}

// NewService      . kb   ; diag/runner   nil(nil = to   ed). 
func NewService(kb *KB, diag provider.Provider, runner ToolRunner) *Service {
	if kb == nil {
		kb = OpenKB("") // empty  bot,    nil resolve use
	}
	return &Service{KB: kb, Diag: diag, Runner: runner, counters: map[string]int{}}
}

// LastDiagnosis returnback     disconnectclose (provideattribution); nothen nil. 
func (s *Service) LastDiagnosis() *Diagnosis { return s.last }

// ResetLast  empty   disconnect(   Run openheadcalluse,    task  ). 
func (s *Service) ResetLast() { s.last = nil }

// RetryCount returnback refer alreadyuse   heavy   (  disconnectlangonlimituse). 
func (s *Service) RetryCount(fp string) int { return s.counters[fp] }

// BackoffAfter returnback  round  heavy before   time (refernum  , limit MaxAutoRetries  ). 
// provide LLM   heavy path usesame    node . 
func BackoffAfter(round int) time.Duration {
	if round < 0 {
		round = 0
	}
	if round >= len(retryBackoff) {
		round = len(retryBackoff) - 1
	}
	return retryBackoff[round]
}

// Diagnose   ①->②  : first    ( ini.e. use, 0  typecalluse);   inand diag  useonlycall type. 
//     (no failures /  type    / Chat err /  time / JSON resolve   )-> returnback nil  ed. 
func (s *Service) Diagnose(ctx context.Context, task, intent string, traces []Trace) *Diagnosis {
	if len(traces) == 0 {
		return nil
	}
	primary := traces[0]
	fp := Fingerprint(intent, primary.Tool, primary.Raw)

	// ①     in. 
	if d, ok := s.KB.Lookup(fp); ok {
		d.Fingerprint = fp
		s.last = &d
		return &d
	}

	// ②  type    ->  ed( open ). 
	if s == nil || s.Diag == nil {
		return nil
	}

	user, err := encodeUser(task, intent, traces)
	if err != nil {
		return nil
	}
	resp, err := s.Diag.Chat(ctx, provider.ChatRequest{
		Messages: []contract.Message{
			{Role: "system", Content: SystemPrompt},
			{Role: "user", Content: user},
		},
		MaxTokens: 500,
	})
	if err != nil {
		// 404/5xx/ time/  :    . error  only statuscode+ disconnect  body(   alreadykeep    key). 
		log.Printf("[selfheal] rid=%s diag Chat 失败，跳过诊断（不阻断主链）: %v", primary.RequestID, err)
		return nil
	}

	d, err := parseDiagnosis(resp.Content)
	if err != nil {
		log.Printf("[selfheal] rid=%s diag JSON 解析失败，跳过诊断: %v", primary.RequestID, err)
		return nil
	}
	d.Source = "model"
	d.Fingerprint = fp
	d.normalizeAction()
	d.friendlySuggestion()
	s.last = &d
	return &d
}

// parseDiagnosis resolve  type JSON  out(  firsttailempty ).  close charseg/   JSON -> error. 
func parseDiagnosis(content string) (Diagnosis, error) {
	var d Diagnosis
	if err := json.Unmarshal([]byte(content), &d); err != nil {
		return Diagnosis{}, err
	}
	if d.Category == "" || d.Action == "" {
		return Diagnosis{}, errMissingFields
	}
	return d, nil
}

// errMissingFields is disconnect JSON    charseg   error. 
var errMissingFields = &diagError{"诊断 JSON 缺 category/action 字段"}

type diagError struct{ s string }

func (e *diagError) Error() string { return e.s }

// SafeRetry   ③  : to    tool execution  disconnect + safesafetyheavy . 
// returnback (newReceipt, diagnosis): 
//   - newReceipt  empty = safesafetyheavy become (alreadywrite-back   ), by chain and  Receipts; 
//   - diagnosis startendreturnback(  KB  in)provideattribution;    heavy / disconnect  timeas nil. 
//
// safesafetyinvariant: writeclass/ reversible  , recoverable=false,  ed 2  onlimit, no Runner ->   heavy . 
func (s *Service) SafeRetry(ctx context.Context, task, intent string, att Attempt) (*contract.Receipt, *Diagnosis) {
	if s == nil {
		return nil, nil
	}
	errText := att.Receipt.Err
	if errText == "" {
		errText = att.Receipt.Stderr
	}
	tr := NewTrace(att.Tool, "", att.Args, errText)
	tr.RequestID = att.RequestID // P0-4b:  disconnect trace   chain request_id(day /trace  )
	d := s.Diagnose(ctx, task, intent, []Trace{tr})
	if d == nil {
		return nil, nil
	}

	switch d.Action {
	case ActionRetry, ActionModify:
		// read-only only allow  heavy ;  thenclose only attribution. 
		if !IsReadOnlyTool(att.Tool, att.Args) {
			return nil, d
		}
		if s.Runner == nil {
			return nil, d
		}
		fp := d.Fingerprint
		//   onlimit:  ity 2  ; retry_params.max_retries  recv but   onlimit. 
		if s.counters[fp] >= d.effectiveMaxRetries() {
			return nil, d
		}
		//   :  first retry_params.backoff_seconds(refernum base*2^round)+ cooldown_seconds; 
		//   usein default  (500ms/1s).  heavy ctx cancel. 
		wait := waitForRound(d, s.counters[fp])
		select {
		case <-ctx.Done():
			return nil, d
		case <-time.After(wait):
		}
		args := att.Args
		if d.Action == ActionModify {
			args = applyRetryParams(att.Args, d.RetryParams)
		}
		s.counters[fp]++
		nr := s.Runner(att.Tool, args)
		if nr.OK {
			// fix become  ->   write-back   . 
			s.KB.Remember(*d)
			return &nr, d
		}
		// heavy   :  write-back KB(  pipe   close  ize). 
		return nil, d
	default:
		// fallback / ask / stop:  again  , close provideattribution. 
		return nil, d
	}
}

// waitForRound     round  heavy beforewaittime (v2 retry_params). 
func waitForRound(d *Diagnosis, round int) time.Duration {
	var w time.Duration
	if base := d.backoffSeconds(); base > 0 {
		mult := 1 << round // refernum:   round   = base * 2^round
		w = time.Duration(base * float64(mult) * float64(time.Second))
	} else {
		if round >= len(retryBackoff) {
			round = len(retryBackoff) - 1
		}
		w = retryBackoff[round]
	}
	if cd := d.cooldownSeconds(); cd > 0 {
		w += time.Duration(cd * float64(time.Second))
	}
	return w
}

// retryControlKeys is v2 retry_params    harness control ,  is   num, modify time  notein   args. 
var retryControlKeys = map[string]bool{
	"backoff_seconds": true, "max_retries": true, "cooldown_seconds": true,
}

// applyRetryParams pipe retry_params   tgt fixpos and  args(modify   ). 
// only in  tgt , and ed harness control , prevent typenotein  close ;    key   . 
func applyRetryParams(args map[string]any, params map[string]any) map[string]any {
	out := make(map[string]any, len(args)+len(params))
	for k, v := range args {
		out[k] = v
	}
	for k, v := range params {
		if retryControlKeys[k] {
			continue
		}
		switch v.(type) {
		case string, float64, bool:
			out[k] = v
		}
	}
	return out
}
