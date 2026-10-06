// Package selfheal  now VoxSign"  error  " (docs/error    -     type.md /
// docs/errorhandleconnect tgtapprove-v1.md): 
//
//	① error    in(0  typecalluse): errorrefer  -> alreadyhasrootbecause/fix ,  connect use  ; 
//	②      type(JEV,  typein  OpenAI compat  ):   inonlycall,  outclose ize JSON  disconnect; 
//	③      : by action   , limit 2  , onlytoread-only     heavy , fix become write-back   . 
//
//     end(  pathall   rev): 
//   -  disconnect [  ]: diag provider     / no key / 4xx / 5xx /  time / JSON resolve    ->
//     returnback nil( ed),   to chain  error, nowhas  path char change; 
//   -        reversible  (git commit / deploy / note append / filewrite); 
//   -    word name : retry|modify|fallback|ask|stop; 
//   - API key     code / day  / commit / error  . 
//
// this package out dependency(onlytgtapprove  + in  config/contract/provider). 
package selfheal

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// SystemPrompt issendgive     type  system  base(connect tgtapprove v1 §2,  char). 
const SystemPrompt = "你是 VoxSign 异常诊断模型。根据用户任务与执行失败轨迹，输出结构化 JSON 诊断。只输出 JSON，不要多余文字。"

// errorclassify  (connect tgtapprove v1 §3, budget  first). 
const (
	CatBudget      = "budget"       //   /  use  ->     ,    as  
	CatNetwork     = "network"      //      / time -> refernum  heavy 
	CatAuth        = "auth"         // key no / limit -> stop,  show   key
	CatParam       = "param"        //  num be keep(e.g. max_tokens 400)-> modify fixpos num
	CatToolMissing = "tool_missing" //   /     -> ask/fallback
	CatPermission  = "permission"   //   bereject/no limit -> stop/ask
	CatTransient   = "transient"    //  time   -> retry
	CatUnknown     = "unknown"      // no  disconnect -> fallback keepkeepnowhas  
)

//      word name . 
const (
	ActionRetry    = "retry"    // by  heavy (refernum  , limit 2  )
	ActionModify   = "modify"   // by retry_params fixpos num/  typeafterheavy   
	ActionFallback = "fallback" // safesafety  (e.g. QUERY     )
	ActionAsk      = "ask"      //  clarificationuseuser
	ActionStop     = "stop"     // endstopandattribution
)

// validCategory / validAction   name verify( type   outout-of-scopevalue,     as unknown/stop). 
var validCategory = map[string]bool{
	CatBudget: true, CatNetwork: true, CatAuth: true, CatParam: true,
	CatToolMissing: true, CatPermission: true, CatTransient: true, CatUnknown: true,
}

var validAction = map[string]bool{
	ActionRetry: true, ActionModify: true, ActionFallback: true, ActionAsk: true, ActionStop: true,
}

// Trace is v2      trace(user content   traces num   , connect tgtapprove v2 §2). 
//    v1   {tool,args,err,stdout,model}: 
//   - params: orig args origkind map( again becomechar   need); 
//   - error: {code,type}--code  firstget resolve   HTTP statuscode,  then    ; type aserrorclassdiff; 
//   - stdout: v2 no charseg,   (e.g.need andin params); 
//   - model:  typeclass  (fast/jev calluse) becall typename,   class   empty. 
type Trace struct {
	Tool      string         `json:"tool"`
	Params    map[string]any `json:"params,omitempty"`
	Error     *TraceError    `json:"error,omitempty"`
	Model     string         `json:"model,omitempty"`
	Raw       string         `json:"-"` // origstarterror base, onlybaselyrefer use,  on  type
	RequestID string         `json:"-"` // P0-4a: baselyclose  request_id(day /trace  use,  on  type)
}

// TraceError is v2 trace  errorclose . 
type TraceError struct {
	Code string `json:"code"` // HTTP statuscode(e.g. "503"/"429");   resolve then "ERR_UNKNOWN"
	Type string `json:"type"` // errorclassdiff(timeout/rate_limit/auth/not_found/overload…)
}

// NewTrace fromorigstarterror base     v2 trace(   class error.code/type). 
func NewTrace(tool, model string, params map[string]any, errText string) Trace {
	code, typ := classifyError(errText)
	return Trace{Tool: tool, Params: params, Model: model, Error: &TraceError{Code: code, Type: typ}, Raw: errText}
}

// classifyError pipe openaiClient/   error base classas v2 error{code,type}. 
// openaiClient error stateas "HTTP 404: …" / "HTTP 500: …",  firstget  statuscode. 
func classifyError(s string) (code, typ string) {
	code = "ERR_UNKNOWN"
	typ = "unknown"
	if i := strings.Index(s, "HTTP "); i >= 0 {
		rest := s[i+len("HTTP "):]
		digits := ""
		for _, ch := range rest {
			if ch >= '0' && ch <= '9' {
				digits += string(ch)
			} else {
				break
			}
		}
		if len(digits) == 3 {
			code = digits
			switch digits {
			case "400", "422":
				typ = "invalid_request"
			case "401", "403":
				typ = "auth"
			case "404":
				typ = "not_found"
			case "429":
				typ = "rate_limit"
			default:
				if digits[0] == '5' {
					typ = "overload"
				}
			}
			return
		}
	}
	lower := strings.ToLower(s)
	switch {
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline"):
		code, typ = "ERR_TIMEOUT", "timeout"
	case strings.Contains(lower, "connection") || strings.Contains(lower, "dial") || strings.Contains(lower, "no such host"):
		code, typ = "ERR_NETWORK", "connection"
	}
	return
}

// Diagnosis is   disconnectclose (    inor type out  izeafter  status). 
type Diagnosis struct {
	Category    string         `json:"category"`
	RootCause   string         `json:"root_cause"`
	Confidence  float64        `json:"confidence"`
	Recoverable bool           `json:"recoverable"`
	Suggestion  string         `json:"suggestion"`
	Action      string         `json:"action"`
	RetryParams map[string]any `json:"retry_params,omitempty"`
	Source      string         `json:"source"` // "kb" | "model"
	Fingerprint string         `json:"fingerprint"`
}

// normalizeAction     rule: recoverable=false time   action all  stop; 
// action out-of-scope  as stop; category out-of-scope  as unknown. 
func (d *Diagnosis) normalizeAction() {
	if !validCategory[d.Category] {
		d.Category = CatUnknown
	}
	if !validAction[d.Action] {
		d.Action = ActionStop
	}
	if !d.Recoverable {
		d.Action = ActionStop
	}
}

// friendlySuggestion byclassifypatch    (budget  "  use "but     ; 
// param  showby retry_params fixpos num). no disconnecttime chainkeepkeeporig  ,  accept  . 
func (d *Diagnosis) friendlySuggestion() {
	switch d.Category {
	case CatBudget:
		if !strings.Contains(d.Suggestion, "预算") {
			d.Suggestion = "Model budget/quota exhausted: " + d.Suggestion
		}
	case CatParam:
		if len(d.RetryParams) > 0 && !strings.Contains(d.Suggestion, "参数") {
			d.Suggestion = "Retry with corrected params: " + d.Suggestion
		}
	}
}

// errFirstSegment geterrorfirstseg: first  empty ;  ed 80 char  disconnectto 80. 
// (refer  in   ity  : same classerror         in. )
func errFirstSegment(s string) string {
	line := strings.TrimSpace(s)
	if i := strings.IndexAny(line, "\n\r"); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	if len(line) > 80 {
		line = line[:80]
	}
	return line
}

// Fingerprint   errorrefer  = sha256(intent + "|" + tool + "|" + errfirstseg). 
//    byrefer  in,  ini.e. 0  typecalluse useclose . 
func Fingerprint(intent, tool, errText string) string {
	seg := errFirstSegment(errText)
	sum := sha256.Sum256([]byte(intent + "|" + tool + "|" + seg))
	return fmt.Sprintf("%x", sum)
}

// diagnoseRequest issendgive     type  user   (v2: traces close izenum ). 
type diagnoseRequest struct {
	Task   string  `json:"task"`
	Intent string  `json:"intent"`
	Traces []Trace `json:"traces"`
}

// encodeUser pipe disconnect in codeas user content JSON. 
func encodeUser(task, intent string, traces []Trace) (string, error) {
	if len(traces) == 0 {
		traces = []Trace{{}}
	}
	b, err := json.Marshal(diagnoseRequest{Task: task, Intent: intent, Traces: traces})
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// retry_params readget  (v2 charsegname: backoff_seconds / max_retries / cooldown_seconds). 

// backoffSeconds get retry_params.backoff_seconds(refernum  basenum, sec);   returnback 0(usein default). 
func (d *Diagnosis) backoffSeconds() float64 {
	if d == nil || d.RetryParams == nil {
		return 0
	}
	if v, ok := toFloat(d.RetryParams["backoff_seconds"]); ok && v > 0 {
		return v
	}
	return 0
}

// cooldownSeconds get retry_params.cooldown_seconds(heavy before  ity but, sec). 
func (d *Diagnosis) cooldownSeconds() float64 {
	if d == nil || d.RetryParams == nil {
		return 0
	}
	if v, ok := toFloat(d.RetryParams["cooldown_seconds"]); ok && v > 0 {
		return v
	}
	return 0
}

// effectiveMaxRetries get retry_params.max_retries, but ity  ed MaxAutoRetries(2  ). 
func (d *Diagnosis) effectiveMaxRetries() int {
	limit := MaxAutoRetries
	if d != nil && d.RetryParams != nil {
		if v, ok := toFloat(d.RetryParams["max_retries"]); ok {
			m := int(v)
			if m >= 0 && m < limit {
				limit = m
			}
		}
	}
	return limit
}

// toFloat pipe JSON numchar(float64)/ numdisconnectlangas float64. 
func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	}
	return 0, false
}

// MaxRetries returnbackbase  disconnect allow heavy   onlimit(retry_params.max_retries,  ity  ed 2). 
func (d *Diagnosis) MaxRetries() int { return d.effectiveMaxRetries() }

// Wait returnback  round  heavy beforewaittime (v2 retry_params.backoff_seconds refernum + cooldown). 
func (d *Diagnosis) Wait(round int) time.Duration { return waitForRound(d, round) }
