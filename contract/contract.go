// Package contract define VoxSign harness safety module     numdata  : 
//  day  ,  type JSON     (ActionPlan),   back (Receipt), use (Usage). 
//   onlydependencytgtapprove , and         , keep  modulecharnode   . 
package contract

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Role tgt  day   send  . 
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool" // keep : V0   use OpenAI function-calling   
)

// Message is OpenAI       day  . 
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

// Usage    OpenAI usage  . 
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ToolSchema describe   tool contract in     . 
// safety  schema  start time  ity      showword,  after   requirebefore charnode   , 
//   keep prompt cache  endpoint(DeepSeek /  typein ) incache,     becomebase. 
type ToolSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"` // JSON Schema to 
}

// ActionPlan is type  JSON     (   dsh Code  form): 
//  type  returnback     list, harness      inby   and  back , 
// frombutpipe LLM↔harness     return  to  --langaudio scenario    ,  is  ity pt. 
type ActionPlan struct {
	Actions []Action `json:"actions"`
	Final   string   `json:"final"`
}

// Action is type require harness      . 
type Action struct {
	Seq       int            `json:"seq"`
	Tool      string         `json:"tool"`
	Args      map[string]any `json:"args"`
	TimeoutMs int            `json:"timeout_ms"`
	Confirm   bool           `json:"confirm"` //  formvoice     need  
}

// Receipt is       close . 
type Receipt struct {
	Seq        int    `json:"seq"`
	Tool       string `json:"tool"`
	OK         bool   `json:"ok"`
	Stdout     string `json:"stdout,omitempty"`
	Stderr     string `json:"stderr,omitempty"`
	Err        string `json:"err,omitempty"`
	Blocked    string `json:"blocked,omitempty"` // safesafetyblockorigbecause
	ConfirmAsk bool   `json:"confirm_ask,omitempty"` // distillation R6: receipt is a confirmation gate (render 待确认, not FAILED)
	DurationMs int64  `json:"duration_ms"`
}

// ParseActionPlan resolve  typeorigstart JSON  out(   ```json  code  ), 
// verify  and  ize   id(  by 1..n patch ). resolve   timeerror  keep origstart seg, thenattraceback . 
func ParseActionPlan(raw string) (ActionPlan, error) {
	s := strings.TrimSpace(raw)
	//    ```json / ```go / ```   
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		for _, tag := range []string{"json", "JSON", "go"} {
			if rest, ok := strings.CutPrefix(s, tag); ok {
				s = rest
				break
			}
		}
		if end := strings.LastIndex(s, "```"); end >= 0 {
			s = s[:end]
		}
	}
	s = strings.TrimSpace(s)

	var plan ActionPlan
	if err := json.Unmarshal([]byte(s), &plan); err != nil {
		return plan, fmt.Errorf("failed to parse action plan: %w; raw=%q", err, truncate(raw, 500))
	}
	if plan.Final == "" && len(plan.Actions) == 0 {
		return plan, fmt.Errorf("empty action plan: neither actions nor final; raw=%q", truncate(raw, 500))
	}
	for i := range plan.Actions {
		a := &plan.Actions[i]
		if a.Tool == "" {
			return plan, fmt.Errorf("action #%d missing tool field; raw=%q", i, truncate(raw, 500))
		}
		if a.Seq <= 0 {
			a.Seq = i + 1
		}
		if a.Args == nil {
			a.Args = map[string]any{}
		}
	}
	return plan, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// intentclassdiff  (input produceout, router/agent/trajectory   , safetymodule  ). 
// M1 classdiff( manageline): TIME/FILE_*/SHELL/APP_LAUNCH/INFO/UNKNOWN. 
const (
	IntentTime      = "TIME"
	IntentFileRead  = "FILE_READ"
	IntentFileWrite = "FILE_WRITE"
	IntentFileList  = "FILE_LIST"
	IntentShell     = "SHELL"
	IntentAppLaunch = "APP_LAUNCH"
	IntentInfo      = "INFO"
	IntentUnknown   = "UNKNOWN"
)

// M2 taskintentclassdiff(SPEC v2 §intent Schema  class + REGISTER_TOOL   intent). 
// and M1 classdiffandstore:  managelineuse M1 classdiff, M2 voice-drivenopensendmanagelineusetaskclassdiff. 
const (
	IntentNote         = "NOTE"          //    :   under/store/ 
	IntentQuery        = "QUERY"         //   code/status:  / / /on 
	IntentEdit         = "EDIT"          // modify code: modify/ become/pipe…modifybecome
	IntentDebug        = "DEBUG"         // fix bug: fix/  /as    
	IntentTest         = "TEST"          //    :    /  under
	IntentCommit       = "COMMIT"        //   :   / 
	IntentDeploy       = "DEPLOY"        //   /outsend/occurbecome table: send/online/  
	IntentAsk          = "ASK"           //    : as  /   /   
	IntentRegisterTool = "REGISTER_TOOL" // langaudionote new    (  fillsplit  , 8 classofout  define)
	IntentOrchestrate  = "ORCHESTRATE"   //   orchestrate(  erformrouteby): read    ->  ->occurbecomefile->
	IntentReminder     = "REMINDER"      // reminder/alarm: explicit not-implemented branch (no cron/scheduler), no longer misrouted to NOTE
	IntentBuildTest    = "BUILD_TEST"    // distillation R5 (2026-10-08): clone/download code -> build -> test real pipeline
	IntentContinue     = "CONTINUE"      // distillation R5: "开始干/立刻执行/继续" resumes the task slot from the previous turn
	IntentInstall      = "INSTALL"       // distillation R6 (2026-10-08): "安装 codex / claude code 到后台" -> real npm install on the host, gated by a confirm + task slot
)

// confirm  etc (Intent.Confirm, risk   decideafterbackfillauthoritative value). 
const (
	ConfirmAuto   = "auto"   //      + tgt   
	ConfirmLight  = "light"  //  show diff  need ->  confirm
	ConfirmStrong = "strong" // diff    +   split  ->  confirm
	ConfirmHuman  = "human"  //  reversible  ,   humanconfirm(  be   )
)

// risk  faceetc (RiskBaseline.Impact,   signal and,    type  ). 
const (
	ImpactSmall  = "small"
	ImpactMedium = "medium"
	ImpactHigh   = "high"
)

// Correction     correction(baselyword or type  correction). 
type Correction struct {
	From string `json:"from"`
	To   string `json:"to"`
	Rule string `json:"rule"` // dict | builtin | model
}

// M2 intent Schema   classtype(SPEC v2 §intent Schema)--and M1   Intent same  store. 

// Target istaskintentreferto  body(coreference resolutionproduceout). 
type Target struct {
	Entity  string `json:"entity,omitempty"`   //  resolveafter  body( name/ objname/filename/path)
	RefType string `json:"ref_type,omitempty"` // explicit | anaphora | inferred | space_scope
}

// Boundary is boundary  in   /  (  boundary     ). 
type Boundary struct {
	Scope   []string `json:"scope,omitempty"`   // overwrite   glob(  to/ topath)
	Exclude []string `json:"exclude,omitempty"` //  form  (.env*,   , out send )
}

// RiskBaseline isintent riskbaseline(risk   authoritative signalsplit ,  placeonlybaseline, Confidence 0 timeuse Intent.Confidence). 
type RiskBaseline struct {
	Reversible bool    `json:"reversible"`
	Impact     string  `json:"impact,omitempty"` // small | medium | high
	Confidence float64 `json:"confidence,omitempty"`
}

// intent    tgt (Intent.Conflict, input classify produceout, back giveuseuser). 
const (
	ConflictNone         = ""               // no  
	ConflictAskVsOp      = "ask_vs_op"      //   ity sentbe  as ASK
	ConflictNoteVsDeploy = "note_vs_deploy" // "send   "be  as NOTE
	ConflictDebugPlan    = "debug_plan"     // "fix bug   route"low-confidence -> clarification
	ConflictDelete       = "delete"         // delete word -> EDIT(action=delete)
	// ConflictNegation:   word connect     -> ASK confirm,     (   G1). 
	//  data SPEC-v2:49"Ask != '' ->     "and VS-HARNESS-001:314" clarification,  rejectalso pos ". 
	ConflictNegation = "negation"
	// ConflictMeta:  refer (openstart/continuecontinue/  )be cur      -> ASK   (   G3). 
	// "openstart  "is  to ,  is"  go test ./...". 
	ConflictMeta = "meta"
	// ConflictConditional:   sent(e.g. …then…)beno     -> ASK(   G6). 
	//     useuser   , also  pipe"hasbefore    "curno      . 
	ConflictConditional = "conditional"
	// ConflictMultiAction:  sent  has  byon   -> ASK  first   (   G5). 
	// produce  endis"   decision point",     useuser   , also     its   . 
	ConflictMultiAction = "multi_action"
	ConflictContinue    = "continue" // distillation R5: "开始干/立刻执行/继续" resumes the task slot
)

// Intent is in   produceout close izeintent(     §5.5): 
// intentclassdiff +    num +     + correctionafter base; Ask  emptytableshowneedneedclarification,     . 
// M2   charsegsafety  omitempty:  manageline(M1 classdiff)  erand has JSON trace accept  . 
type Intent struct {
	Intent        string            `json:"intent"`
	Slots         map[string]string `json:"slots"`
	Confidence    float64           `json:"confidence"`
	CorrectedText string            `json:"corrected_text"`
	Corrections   []Correction      `json:"corrections"`
	Ask           string            `json:"ask"`

	// M2   (voice-drivenopensendtaskintent; omitempty keepkeep M1 charnode compat). 
	RawText    string            `json:"raw_text,omitempty"`   // ASR orig (and input.Result.Raw samevalue, thenat  JSON heavy )
	Space      string            `json:"space,omitempty"`      // emptytime  (note name; empty =   refer/space_check  bot,   )
	Target     *Target           `json:"target,omitempty"`     // coreference resolutionclose 
	Params     map[string]string `json:"params,omitempty"`     //    num(action/object/value/path/test_kind/time_hint…)
	Boundary   *Boundary         `json:"boundary,omitempty"`   //   /  ( boundary  )
	Risk       *RiskBaseline     `json:"risk,omitempty"`       // riskbaseline( authoritative)
	Confirm    string            `json:"confirm,omitempty"`    // auto|light|strong|human(baseline, risk   decide)
	Acceptance string            `json:"acceptance,omitempty"` //  recv  (  , verify     )
	Context    []string          `json:"context,omitempty"`    //      use(project-map:xxx / decisions:xxx)
	Conflict   string            `json:"conflict,omitempty"`   // intent    tgt (back use)
}

// NeedsClarification    intentis needneedclarification. 
func (i *Intent) NeedsClarification() bool { return i.Ask != "" }

// ToolContract describe      (.contract,     + REGISTER_TOOL   ). 
// caps as    name ; risk as   riskbaseline( reversible  tgt irreversible). 
type ToolContract struct {
	Name          string            `json:"name"`
	Version       string            `json:"version"`
	Caps          []string          `json:"caps"`
	Params        map[string]string `json:"params"` //  numname -> classtypedescribe("string,required")
	SideEffects   []string          `json:"side_effects"`
	AllowedSpaces []string          `json:"allowed_spaces"`   // project | sandbox | vault-notes | vault-creds | external | global
	Risk          map[string]string `json:"risk"`             // cap -> low | medium | high | irreversible
	Source        string            `json:"source,omitempty"` // builtin | voice(REGISTER_TOOL note )
	RegisteredAt  string            `json:"registered_at,omitempty"`
}

// attribution  classdiff(   discuss stage errorclassify, write-backunder  onunder notein). 
const (
	AttrInput    = "input"     //  in/ASR error
	AttrContext  = "context"   // onunder /      
	AttrContract = "contract"  //   define  
	AttrModel    = "model"     //  type disconnecterror
	AttrExec     = "execution" //   /  error
	AttrExternal = "external"  // out dependency( typein /  /   )
)

// Attribution is  attribution  (  classify +  data +   , writetraceand discuss-log). 
type Attribution struct {
	RequestID  string `json:"request_id"`
	Stage      string `json:"stage"`                // record | discuss | control
	Class      string `json:"class"`                // attribution  of 
	Detail     string `json:"detail"`               // sendoccur  
	Evidence   string `json:"evidence"`             //  data(trace obj kind / file /  out seg)
	Suggestion string `json:"suggestion,omitempty"` // under     
	Ts         string `json:"ts"`
}

// ReceiptView is  four-line receipt  nownumdata(  /file/close /  , SPEC  recvuseexample 11). 
type ReceiptView struct {
	Action string `json:"action"` //    ( intentand  )
	Files  string `json:"files"`  //  andfile(nofile -> "—";  file -> "N  file")
	Result string `json:"result"` // close (OK/FAILED/BOUNDARY_VIOLATION/ confirm + close   need)
	Undo   string `json:"undo"`   //    form(git alsoorig /   path /     )
}

// RenderReceipt   four-line receipt base(mobile  in, SPEC  recvuseexample 11  form). 
func RenderReceipt(r ReceiptView) string {
	lines := []string{
		"Action: " + r.Action,
		"Files: " + r.Files,
		"Result: " + r.Result,
		"Undo: " + r.Undo,
	}
	return strings.Join(lines, "\n")
}
