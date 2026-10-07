package input

// taskintent.go -- M2 voice-drivenopensendtaskintentclassify . 
//
// [pseudocode logic layer](review gateartifact; SPEC v2 §pseudocode logic layer  to  nodeasauthoritative , basenote as now   baseline. 
//  intent disconnectrule authoritative definition  SPEC v2/VSL define , this layeronlydescribe modulecontrol flow/branch/errorpath,  heavy rulebody. )
//
// module  : pipecorrectionafter baseclassifyas M2 taskintent( class + REGISTER_TOOL), 
//    get emptytime   /    num / timetime pt /  boundarybaseline /  recv   / riskandconfirmbaseline. 
//  in: corrected text; emptytime  table(note name+diffname, pipeline notein);     value. 
//  out: contract.Intent(M2 charseg fill; low-confidence Ask  empty,     ). 
//
// control flow(branch  ,  first from to ): 
//   0. empty base/ empty  -> UNKNOWN + Ask. 
//   1.     (firstat classtriggersend): 
//      a. delete word( /delete/  /  / empty)-> EDIT + params.action=delete
//         (risk by reversiblehandle ->   humanconfirm,   be   ). 
//      b. note  word(     /note   /newadd  …)-> REGISTER_TOOL. 
//      c.   ity sent(   /   by/is  by/   )-> ASK,  is QUERY. 
//      d.    restrict  ( "  /  "and beemptytimename  )-> NOTE("send   "-> NOTE,  is DEPLOY; 
//         emptytimename   classwordtime  --"   "is bodyname is   triggersend, 20 kindexample to   surfaced afterfix ). 
//      e. status sent(  /modify modify/modify )-> QUERY,  is EDIT/DEBUG. 
//      f. DEBUG triggersend + " route/   /  /  "-> low-confidence -> Ask(    intent). 
//   2.  classtriggersend(SPEC v1 §2 triggersendwordtable,  word first;  use M1   substring     , CJK noword boundary): 
//      NOTE  :   under/ under /  /   under/  /store /store /storeto
//        ( use " "--20 kindexample to   surfaced:    in"  /  /  " become QUERY    NOTE)
//      QUERY  :   under/ /  under/ /on /  under/ /  / 
//      EDIT modify: pipe…modifybecome/modifybecome/ become/modify under/modify/  /modify
//      DEBUG fix:   /as    /  /  /out /bug/fix under/fix  /fix  /fix fix/fix
//      TEST  :    /  under/  under/    /  ( "ity "-> test_kind=bench)
//      COMMIT   :   / on / to
//      DEPLOY send:   /online/occurbecome table/sendto/send 
//      ASK  : as  /   /   /is    /   /e.g. 
//   3.    get(byintent  ): 
//      - path/object:  idin  -> ~//openhead token -> obj /filebeforeword. 
//      - EDIT: "pipe X modifybecome Y"-> object=before  body( "pipe"), value=after value. 
//      - TEST: test_kind(bench|go test ./...|refer path). 
//      - time: timetime ptorig +resolve dayperiod(timeanchor.go). 
//   4. emptytime  : note emptytimename/diffname  basein     -> Space + Target{ref_type:explicit}; 
//      no in -> Space=""(  ,  give refer/space_check  bot,   global read-onlyorclarification). 
//   5. objtgt body: only form use( id body/emptytime  /"  X  "  X)  Target;  name/coreferenceword  refer  . 
//   6.  boundarybaseline: Scope = emptytime  nameor formpath; Exclude default [".env*","node_modules"]. 
//   7. riskbaseline( authoritative, risk  use  signalheavy ): NOTE/QUERY/TEST/ASK -> reversible small; 
//      EDIT -> reversible medium; DEBUG -> reversible high; COMMIT/DEPLOY ->  reversible; REGISTER_TOOL -> reversible medium. 
//   8. Confirm baseline: COMMIT/DEPLOY -> human; DEBUG/EDIT/REGISTER_TOOL -> light; its  auto. 
//   9. Acceptance   (verify     ): EDIT -> "changelimit   {scope} in, noout-of-scopechange"; 
//      TEST -> "  safety , noback "; DEBUG -> " now    , back    ed"; COMMIT -> "  become ,    end"; 
//      DEPLOY -> "  become andobjtgt   "; QUERY/NOTE/ASK -> " outfull   ". 
//  10.    :    inand    -> 0.9;  class inand    -> 0.85;  close    -0.2;      -> 0.5. 
//  11. low-confidence(<  valueand  NOTE/ASK/REGISTER_TOOL)-> Ask clarification; UNKNOWN  clarification. 
//
// errorhandle: emptytimename  in ->   diffname out,     (clarification);  base > 512 char  disconnectagainclassify; 
//   Ask   byintent restrict(referout   ), UNKNOWN ofout  allow     as. 

import (
	"strings"
	"time"

	"voicesign-harness/contract"
)

// SpaceHint isemptytime   show(pipeline noteinnote emptytimename/diffname, provideclassify      ). 
type SpaceHint struct {
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
}

// TaskClassifier pipecorrectionafter baseclassifyas M2 taskintent( class + REGISTER_TOOL). 
type TaskClassifier struct {
	ConfThreshold float64
	Spaces        []SpaceHint
}

// NewTaskClassifier   task classification . conf<=0 timeget 0.6(and M1   ). 
func NewTaskClassifier(conf float64, spaces []SpaceHint) *TaskClassifier {
	if conf <= 0 {
		conf = 0.6
	}
	return &TaskClassifier{ConfThreshold: conf, Spaces: spaces}
}

const taskAskTemplate = "你是想让我做什么？请再说清楚一点（记想法/查代码/改代码/修 bug/跑测试/提交/部署/问问题）"

var (
	// registerTriggers is  triggersendwordtable: keep give actionWords(  /     safety )
	// and close ize bot use. new      registerToolRequest  close ity data --
	// seeunder "note   intent(     A1)". 
	registerTriggers = []string{"加一个工具", "加个工具", "注册工具", "新增工具", "加个新工具", "加一个新工具",
		// 2026-10-04     R11:  langnote "add     day    "--
		// "add   /newadd  /    "+"  " lang  in("   "    in). 
		"增加一个", "新增一个", "添加一个"}
	// writeTriggers writefilerefer (2026-10-04     R6/R7  out): 
	// "pipe XX writeto /path"  beforebe  TEST( "  "char)or UNKNOWN(notriggersendword). 
	//   table  before, firstat TEST   . 
	writeTriggers  = []string{"写到", "写入", "保存到", "保存为", "创建文件", "写一个", "写个"}
	deleteTriggers = []string{"删掉", "删除", "去掉", "移除", "清空"}
	feasibleAsk    = []string{"能不能", "可不可以", "是否可以", "行不行"}
	thoughtWords   = []string{"想法", "备忘"}
	statusQuestion = []string{"好了吗", "弄好了吗", "搞定了吗", "改好了吗", "改没改", "改了没", "改了吗", "弄了吗"}
	debugPlanWords = []string{"思路", "怎么做", "方案", "打算"}
	noteTriggers   = []string{"记一下", "记下来", "记下", "记个", "记住", "记录一下", "记录", "存档", "存个", "存到",
		// 2026-10-08 (distillation R2): "记一条：明天上午9点开会" fell to UNKNOWN because
		// "记一条" was missing from the note table. Added for NOTE capability baseline (U3).
		"记一条", "记个条", "记个想法", "记条"}
	// reminderTriggers: reminder/alarm/timed prompts. Must be matched BEFORE noteTriggers,
	// otherwise "记个提醒：明天八点开会" trips "记个" and the whole sentence is misrouted to
	// NOTE (appended verbatim into notes.md) — the repo has no cron/scheduler at all.
	reminderTriggers = []string{"提醒我", "提醒", "闹钟", "几点叫我", "到点提醒", "定时提醒", "设个时间", "定时叫"}
	queryTriggers  = []string{"查一下", "查", "找一下", "找", "上次", "搜一下", "搜", "看看", "看",
		"几点", "几点钟", "什么时间", "几号", "星期几", "周几",
		// 2026-10-04   control  seg:   formtriggersendword empty , only"   ls/   cat"class in, 
		//    "    pos  "(noempty   in). 
		"运行 ", "执行 ", "帮我跑 ", "截个图", "截图",
		// 2026-10-08 (distillation R2): arithmetic / email drafting / translation are content
		// generation requests; route them to QUERY instead of UNKNOWN (U5/U6/A5 baseline).
		"算一下", "等于多少", "多少", "等于", "邮件", "写一封", "翻译", "translate", "翻译成"}
	editTriggers    = []string{"改成", "换成", "改一下", "修改", "替换", "改"}
	debugTriggers   = []string{"报错", "为什么失败", "崩溃", "闪退", "出错", "bug", "修一下", "修这个", "修那个", "修一修", "修"}
	testTriggers    = []string{"跑测试", "跑一下", "测一下", "跑个测试", "测试"}
	commitTriggers  = []string{"提交", "推上去", "推到"}
	deployTriggers  = []string{"部署", "上线", "生成报表", "发到", "发布"}
	askTriggers     = []string{"为什么", "怎么办", "你觉得", "是什么意思", "怎么弄", "如何",
		// 2026-10-08 (distillation R2): "那个东西怎么样了" is an ambiguous referent question;
		// route to ask so the harness asks which item instead of guessing (U4 baseline).
		"怎么样了", "怎么样"}
	defaultExcludes = []string{".env*", "node_modules"}
)

// ---------------------------------------------------------------------------
// note   intent(     A1 · decide  #7"  note  restrict belangaudiocalluse")
//
//   : "note     , use     "be  UNKNOWN --   chain fromin thendisconnect. 
// rootbecause is" recv  word", butispipe"note intent" nowbecometo   lang wordtable  
// (     /note   /newadd  …). useuser   then   wordtable: same  intent
//  by become"note     ""newadd    ""     ""     base". 
//
//    (close ity): **note  word** connect  **  nameword** --  eroftimeonly allow
//  ize/ word(  / /  /new …),       becomesplit. data : 
//
//	"note     "      note  +    +      -> ✅ REGISTER_TOOL
//	"  new  "          +  new +        -> ✅ REGISTER_TOOL
//	"  undernote table"      note  afterfaceis"table",        nameword -> ❌ keepkeep QUERY
//	"  underalreadynote    " note  and    oftime ing" ", isdescribe isnote    -> ❌
//
// note :  data  ** ** allow" "etc  becomesplit ed wordandnamewordoftime,  then
// "  underalreadynote    " class sent be  becomenote refer (back protectsee
// input/registertool_regression_test.go). 
// ---------------------------------------------------------------------------

// registerVerbs is"pipe  new       "  word. 
// " / / "is langin  ize word,    "    nameword"only  , thus    use. 
var registerVerbs = []string{"注册", "新增", "添加", "增加", "创建", "新建", "加", "做", "搞", "上架", "接入"}

// registerCapabilityNouns is benote    classdiffnameword(need    word/ wordofafter). 
var registerCapabilityNouns = []string{"工具", "小工具", "工具链", "命令", "子命令", "指令", "技能", "能力", "插件", "功能", "脚本"}

// registerGapFillers isnote  wordand  namewordoftime allowoutnow  ize/ word. 
// **  by    **: registerGapThenNoun getfirst before  in,  word firstonly  be
// "  "firstpipe"  new " disconnect( then"  new   " become"new   "after   ). 
var registerGapFillers = []string{
	"一个全新的",
	"一个新的", "一种新的", "一款新的",
	"个新的",
	"一个", "一条", "一款", "一种", "一支", "一项", "个新", "新的",
	"新", "个", "条", "款", "种", "支", "项",
}

// registerVerbWordPart    char word v   pos placeis onlyis  word   split, but    word. 
// "        "  " " at"  "(     ),  is"     "--
//      pipe   becomenote intent. onlyhas char ize wordneedneed  word split disconnect. 
func registerVerbWordPart(text string, pos int, v string) bool {
	if v != "加" || pos == 0 {
		return false
	}
	runes := []rune(text[:pos])
	return runes[len(runes)-1] == '参'
}

// registerToolRequest    baseis as"note   new  " langaudiorefer (close ity data). 
func registerToolRequest(text string) bool {
	lower := strings.ToLower(text)
	for _, v := range registerVerbs {
		from := 0
		for {
			i := strings.Index(lower[from:], v)
			if i < 0 {
				break
			}
			pos := from + i
			if registerVerbWordPart(lower, pos, v) {
				from = pos + len(v)
				continue
			}
			if registerGapThenNoun(lower[pos+len(v):]) {
				return true
			}
			from = pos + len(v)
		}
	}
	return false
}

// registerGapThenNoun    after is by"( ize word)*   nameword"openhead. 
func registerGapThenNoun(after string) bool {
	rest := after
	for {
		matched := false
		for _, g := range registerGapFillers {
			if strings.HasPrefix(rest, g) {
				rest = rest[len(g):]
				matched = true
				break
			}
		}
		if !matched {
			break
		}
	}
	//    scenario:  word+ word after connectis  nameword("      ""note      "). 
	for _, n := range registerCapabilityNouns {
		if strings.HasPrefix(rest, n) {
			return true
		}
	}
	// describe scenario(2026-10-04     "   add      control      "): 
	//  word+ word after    describe, by  namewordrecvtail(…   /  /  /  /  ). 
	//     /timestatelang : "  underalreadynote    "(mid=" "), "alreadynote    "(mid="alreadynote  ")
	// -- mid by word/timestatewordopenheador note  word ->  is"note new  "refer . 
	for _, n := range registerCapabilityNouns {
		if strings.HasSuffix(rest, n) {
			mid := strings.TrimSuffix(rest, n)
			if mid == "" {
				continue // namewordbase :    scenarioalreadyoverwrite
			}
			if strings.HasPrefix(mid, "的") || strings.HasPrefix(mid, "了") ||
				strings.HasPrefix(mid, "吧") || strings.HasPrefix(mid, "呢") ||
				strings.HasPrefix(mid, "已") || strings.HasPrefix(mid, "在") ||
				strings.HasPrefix(mid, "过") {
				return false
			}
			for _, v := range registerVerbs {
				if strings.Contains(mid, v) {
					return false // describe note  word ->   /  lang 
				}
			}
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
//     (   G1)
//
//   : ` needdelete  file` be deleteTriggers  first become EDIT(action=delete), 
//   wordfinishsafety be see -- objtgt   resolve then    . 
//  data: SPEC-v2:49"Ask != '' ->     "; 
//
//	VS-HARNESS-001:314" clarification,  rejectalso pos ,    JSON     ". 
//
//   :   word** connect    **time   as ASK confirm,     . 
// ---------------------------------------------------------------------------

// negationMarkers is char  word( word before,   " needagain"be" need" first). 
//   **  **"  ":  is"   "   split, recv pipe  ity sent  . 
//    P1 patch :  /  /  /noneed/ again/  -- origfirst recv,    
// "  delete""noneed  "" again  " be become     . 
var negationMarkers = []string{
	"不需要", "不要再", "请勿", "切勿", "无需", "不再",
	"不要", "不用", "先别", "别再", "免了",
}

// markerIsNegation   "    ,     " onunder (   P2). 
//
//	need needdelete  file   -> " need"onlyis"need need"   split,  is  
//	 need ,   delete    -> " need "=  close ,  sentis"  delete"
func markerIsNegation(text string, pos int, marker string) bool {
	if marker != "不要" {
		return true
	}
	if pos > 0 && strings.HasSuffix(text[:pos], "要") {
		return false // need need…
	}
	if strings.HasPrefix(text[pos+len("不要"):], "紧") {
		return false //  need 
	}
	return true
}

// bieFollowingVerbs is char"diff"afterface  timeonly  as      word. 
//    P1 patch:  /manage/ / ("diff delete…""diffmanage  delete  "). 
var bieFollowingVerbs = []rune("删发改动碰关停做执提交部署记看查修跑送去管乱忘")

// bieBlockPrefixes is "diff"beforefaceoutnowtime    atword   split( is  ) char: 
//  / /split/ /diff/class/ / /ity/ / / /  -- e.g." diffclosenote"" diff"  "diff". 
var bieBlockPrefixes = []rune("特告分个差类级区性离作识辨")

// hasNegation    base is has  , and     is **base then    **. 
//
// returnbackvalue: ( in    form, is     , is  in)
//   -  char  word( need/ use/…)->     =false, calluse needconfirmsentin has  word, 
//     by pipe" use  " class  changebecomedecision point; 
//   -  "diff"+   word(diff /diffsend/…)->     =true, base thenis"  +  "  data. 
func hasNegation(text string) (string, bool, bool) {
	for _, m := range negationMarkers {
		from := 0
		for {
			i := strings.Index(text[from:], m)
			if i < 0 {
				break
			}
			pos := from + i
			if markerIsNegation(text, pos, m) {
				return m, false, true
			}
			from = pos + len(m)
		}
	}
	runes := []rune(text)
	for i, r := range runes {
		//  char  before : diff /  .     **     word**only   , 
		//  then  in" diff/ diff/  " classword   split(   P1/P7). 
		if r != '别' && r != '勿' {
			continue
		}
		if r == '别' && bieIsWordPart(runes, i) {
			continue
		}
		if i+1 >= len(runes) {
			continue
		}
		for _, v := range bieFollowingVerbs {
			if runes[i+1] == v {
				return string(r) + string(runes[i+1]), true, true
			}
		}
	}
	return "", false, false
}

// bieIsWordPart    runes[i]('diff')is onlyis  word   split(e.g." diff"). 
func bieIsWordPart(runes []rune, i int) bool {
	if i == 0 {
		return false
	}
	for _, p := range bieBlockPrefixes {
		if runes[i-1] == p {
			return true
		}
	}
	return false
}

// containsAnyReturn and containsAny same , butreturnback in close word(useatclarification  ). 
func containsAnyReturn(text string, keywords []string) (string, bool) {
	lower := strings.ToLower(text)
	for _, k := range keywords {
		if strings.Contains(lower, strings.ToLower(k)) {
			return k, true
		}
	}
	return "", false
}

// actionWords is"  wordneed     "safety . onlyhascur  and  sametimeoutnowtimeonlyclarification, 
//   pipe" use  " classno     alsochangebecomedecision point. 
var actionWords = func() []string {
	var all []string
	for _, group := range [][]string{
		registerTriggers, writeTriggers, deleteTriggers, noteTriggers, queryTriggers,
		debugTriggers, testTriggers, commitTriggers, deployTriggers,
		askTriggers, editTriggers,
	} {
		all = append(all, group...)
	}
	return all
}()

// ---------------------------------------------------------------------------
//  refer   (   G3)
//
//   : "openstart  ""continuecontinue  "be  TEST 0.85 and test_kind="go test ./..." --       . 
// but  sent to    is" in  stage / continuecontinue  ", is**to control**,  is    . 
//
//     :  ini.e.clarification  ,      , also     . 
// ---------------------------------------------------------------------------

// metaPrefixes isto controlclassbefore . 
var metaPrefixes = []string{"开始", "继续", "接着", "下一步", "接下来", "推进", "开工", "先这样", "暂停", "停一下"}

// metaInstruction    baseis as" refer  +   "  state. 
//
//   before sametimebecome only  refer : 
//  1.  refer outnow **sentfirst  **(beforeface  ed 2  char ),  then onlyissent    split; 
//  2. itsafter   ing    word--  refer (e.g."openstart") has      , 
//      back UNKNOWN handlei.e. ,      sent; 
//  3.  wordofafter**   base  hasdiff   in **(   P1:  usecharnumcur   data). 
//      sent  "openstart/  "is  (M7     , see
//     pipeline.TestCodexNineRegressions#8:  sent refer   keepkeep  Ask)--
//      sentdayhowever has    in ,  bebase     . 

// ---------------------------------------------------------------------------
//   sent  (   G6)
//
//   : "e.g.    edthen  "be  TEST 0.85  connect   -- **before befinishsafety  **. 
//     useuser   , also  pipe"hasbefore    "curno      . 
// ---------------------------------------------------------------------------

// conditionalMarkers isbefore   tgt . 
// "then"is type  linkconnectwordbut lang freq(thenas  / then  )--2026-10-04 useuser    , 
// by conditionalClause in conditionalJiPrefixOK   (only"donestatebefore +then+  "only    ). 
var conditionalMarkers = []string{"如果", "只要", "除非", "一旦", "要是", "假如", "就"}

// conditionalWindow is  tgt ofafter allowoutnow  word charnum  . 
//
// as    limit  : M7    sent
// " now    under,       kind, e.g.      ,   thencontinuecontinue  …"
// sametime "e.g. ""then"and  word, but is seg lang  , **  keepkeep Ask asempty**
// ( hasback  pipeline.TestColloquialQuestionNoReferAsk). 
//   sent   is"after     "(e.g.    ed**then  **), but issent    allhas. 
const conditionalWindow = 12

// conditionalClause    baseis  "   +   itsafter   ", returnback in tgt . 
func conditionalClause(text string) (string, bool) {
	for _, m := range conditionalMarkers {
		from := 0
		for {
			i := strings.Index(text[from:], m)
			if i < 0 {
				break
			}
			pos := from + i
			if m == "就" {
				// 2026-10-04 useuser  : "thenas  / then  " lang  . onlycur"then"beforehasdonestatebefore 
				// (tailchar /ed/finish/  etc)onlyis   ("  edthen  ");  lang" then…" ed. 
				if !conditionalJiPrefixOK(text[:pos]) {
					from = pos + len(m)
					continue
				}
			}
			after := text[pos+len(m):]
			if r := []rune(after); len(r) > conditionalWindow {
				after = string(r[:conditionalWindow])
			}
			if containsAny(after, actionWords) || registerToolRequest(after) {
				return m, true
			}
			from = pos + len(m)
		}
	}
	return "", false
}

// conditionalJiPrefixOK   "then"before langsegis  donestatebefore ("  edthen  "->"  ed"). 
//  lang" then  …"(tailchar" "), sentfirst"thenas  …"->    ,  ed,    . 
func conditionalJiPrefixOK(prefix string) bool {
	r := []rune(strings.TrimSpace(prefix))
	if len(r) == 0 {
		return false
	}
	switch r[len(r)-1] {
	case '了', '过', '完', '好', '成', '绿':
		return true
	}
	return false
}

// metaMaxPrefixRunes is refer wordofbefore allow before charnum("  , ""  first"" , "). 
const metaMaxPrefixRunes = 4

// metaTrailingNoise is refer senttail lang /  becomesplit,   after  "  in ". 
var metaTrailingNoise = []string{
	"好不好", "行吗", "可以吗", "一下吧", "一下",
	"吧", "呢", "啊", "呀", "了", "嗯", "那", "好", "不",
	"。", "，", "、", "！", "？", "!", "?", " ",
}

func metaInstruction(text string) (string, bool) {
	for _, m := range metaPrefixes {
		i := strings.Index(text, m)
		if i < 0 {
			continue
		}
		if len([]rune(text[:i])) > metaMaxPrefixRunes {
			continue
		}
		rest := text[i+len(m):]
		if rest == "" || !containsAny(rest, actionWords) {
			continue
		}
		//     wordandsenttaillang becomesplit;   all   = useuser giveto  =  refer . 
		residual := rest
		for _, w := range append(append([]string{}, actionWords...), metaTrailingNoise...) {
			residual = strings.ReplaceAll(residual, w, "")
		}
		if strings.TrimSpace(residual) == "" {
			return m, true
		}
	}
	return "", false
}

// metaActionText giveoutand  refer word called   (   P4: " stop  "  be become"is  continuecontinue  "). 
func metaActionText(meta string) string {
	switch meta {
	case "暂停", "停一下", "先这样":
		return "先停一下 / 收尾"
	default:
		return "继续推进"
	}
}

// ---------------------------------------------------------------------------
//      (   G5)
//
//   : "  under store, howeverafter  underclose ,  after  "only      in intent, 
// its   **    also  show**, novoice   -- useuserbyas   all . 
//
//  data  use**  linkconnectword**but is"sent  outnow    word": 
// M7    sent" now    under,       kind, e.g.      ,   thencontinuecontinue  …"
// sametime  TEST and QUERY word, butis seg lang  ,   keepkeep  Ask
// ( hasback  pipeline.TestColloquialQuestionNoReferAsk).   has  linkconnectword. 
// ---------------------------------------------------------------------------

// sequenceConnectors isin    tableshow"under  " linkconnectword. 
var sequenceConnectors = []string{"然后", "接着", "之后", "随后", "最后", "再"}

// actionIntentGroups is and    num intentsplit (ASK     , thus   askTriggers). 
var actionIntentGroups = []struct {
	intent   string
	triggers []string
}{
	{contract.IntentRegisterTool, registerTriggers},
	{contract.IntentEdit, writeTriggers},
	{contract.IntentNote, noteTriggers},
	{contract.IntentQuery, queryTriggers},
	{contract.IntentDebug, debugTriggers},
	{contract.IntentTest, testTriggers},
	{contract.IntentCommit, commitTriggers},
	{contract.IntentDeploy, deployTriggers},
	{contract.IntentEdit, editTriggers},
}

// clauseActions returnback  splitsent in **safety **  intent. 
//
//    G5-P1-4: orig nowbywordtable  onlyget    in(wordtable  Query   Debug/Edit ofbefore, 
// and queryTriggers   char" / / "),  pipe"  under    " become  QUERY, 
// pipe"modify under then  under" become  QUERY --   clarification    approve, also**    num**. 
func clauseActions(clause string) []string {
	var out []string
	for _, g := range actionIntentGroups {
		// REGISTER_TOOL useclose ity data( word    nameword),  againuse   langwordtable --
		//  then"note     , howeverafter   " be become 1    but     . 
		hit := containsAny(clause, g.triggers)
		if g.intent == contract.IntentRegisterTool {
			hit = registerToolRequest(clause)
		}
		if hit {
			out = append(out, g.intent)
		}
	}
	return out
}

// multiActionClauses by  linkconnectword split base, returnback: 
//   -     **splitsentnum**(   G5-P0-2: bysplitsentnum ,  by" sameintentnum"  --
//     "first  A again  B"is    , ifby sameintent heavy  become 1,     be    )
//   -   splitsent outnowed   intent( heavy, onlyuseatclarification  )
func multiActionClauses(text string) (int, []string, bool) {
	//    G5-P2-6: first   idin  -- "pipe showlangmodifybecome"howeverafter  ""  "howeverafter"
	// is**be use  base**,  is  linkconnectword,         . 
	clean := stripQuotedSpans(text)
	hadConnector := false
	for _, c := range sequenceConnectors {
		if strings.Contains(clean, c) {
			hadConnector = true
			break
		}
	}

	parts := []string{clean}
	// firstby  linkconnectword , againbysplitsenttgtpt (   G5-P2-5: ASR  pipelinkconnectword  , 
	// "  under store,   underclose ,   "only  idsplit ). 
	for _, sep := range append(append([]string{}, sequenceConnectors...), clauseSeparators...) {
		var next []string
		for _, p := range parts {
			next = append(next, strings.Split(p, sep)...)
		}
		parts = next
	}

	n := 0
	seen := map[string]bool{}
	var labels []string
	for _, p := range parts {
		acts := clauseActions(p)
		if len(acts) == 0 {
			continue
		}
		n++
		for _, a := range acts {
			if !seen[a] {
				seen[a] = true
				labels = append(labels, a)
			}
		}
	}
	return n, labels, hadConnector
}

// multiActionTrips is**decide **(andonface   split ): 
//   - has  linkconnectword -> 2 seg   i.e.    ; 
//   -  tgtptsplit  -> needrequire >=3 seg. 
//
// aftererisaskeep  M7  lang sent:  use idsplit , butonlyhas 2 seg   , andis seg   --
//   keepkeep Ask asempty( hasback  pipeline.TestColloquialQuestionNoReferAsk). 
func multiActionTrips(n int, hadConnector bool) bool {
	if hadConnector {
		return n >= 2
	}
	return n >= 3
}

// clauseSeparators issplitsenttgtpt. 
var clauseSeparators = []string{"，", "；", "。", ",", ";"}

// stripQuotedSpans   be id   in ( idbase also  ), 
//   pipe"be use  base"curbecome  refer . 
func stripQuotedSpans(text string) string {
	pairs := [][2]string{{"「", "」"}, {"“", "”"}, {"\"", "\""}}
	for _, q := range pairs {
		for {
			i := strings.Index(text, q[0])
			if i < 0 {
				break
			}
			j := strings.Index(text[i+len(q[0]):], q[1])
			if j < 0 {
				break
			}
			text = text[:i] + text[i+len(q[0])+j+len(q[1]):]
		}
	}
	return text
}

// joinIntentLabels pipe  intentlisttable become" ,  ,   " kind in  . 
func joinIntentLabels(kinds []string) string {
	labels := make([]string, 0, len(kinds))
	for _, k := range kinds {
		labels = append(labels, intentLabel(k))
	}
	return strings.Join(labels, "、")
}

// intentLabel giveuseuser  in   name. 
func intentLabel(kind string) string {
	switch kind {
	case contract.IntentQuery:
		return "查"
	case contract.IntentNote:
		return "记"
	case contract.IntentEdit:
		return "改"
	case contract.IntentDebug:
		return "修"
	case contract.IntentTest:
		return "跑测试"
	case contract.IntentCommit:
		return "提交"
	case contract.IntentDeploy:
		return "部署"
	case contract.IntentRegisterTool:
		return "注册工具"
	default:
		return kind
	}
}

//   / task(ORCHESTRATE) linksignalwordtable--  erformrouteby  in  : 
//
//	organizeWords ∩ docWords ∩ (saveWords ∪ commitTriggers) sametimebecome . 
//
//   get (   B):     (read->summarize->write->commit)by pipeline orchestrate    ityproduceout, 
//   classify   function-calling; classify onlyresponsible" is    orchestratetask"  diff +  outobjtgt  name. 
var (
	orchOrganizeWords = []string{"整理成", "整理", "汇总成", "汇总", "汇编"}
	orchDocWords      = []string{"沟通记录", "设计文档", "文档", "记录"}
	orchSaveWords     = []string{"保存提交", "保存", "生成", "落成", "写成"}
)


//  nowclass  task( now/  serveserviceclass)close ity diffwordtable -- fix  2  1+2(2026-10-03). 
//
//  formto  REGISTER_TOOL   toback  : close  =  now word before,   nameword after( word  nameword), 
// middle allowfix becomesplit(  word/ lang);  sentbecomesplit(  /e.g. /as  …)outnowi.e.   --
// "    is   now ""e.g.  now  cache"    . 
//
//  inafter:   nowclose or  …    use -> ORCHESTRATE(kind=implement), 
//    nowtaskbyorchestrate    resolve, classify only  diff(to fix  2  2). 
var (
	implementVerbs   = []string{"实现", "搭建", "开发", "构建", "重构", "编码", "写一个", "造一个", "做一个", "写一套", "落地一个", "建一个"} // F1 fix :      "    OT   numdata  …  rule "-> ORCHESTRATE
	// fix class word(  2, 2026-10-04 nohuman    : fix /patch /finish table   in implement -> task connect done 0  noartifact). 
	//  infix class wordtime outneedrequire nowdomain signal(implDomainWords), prevent"fix   "  . 
	reviseVerbs      = []string{"修订", "补齐", "完善", "修正", "修复", "整改", "接着做", "继续做", "继续改", "按反馈", "根据反馈", "按验收", "根据验收"}
	implDomainWords  = []string{"服务", "后台", "系统", "程序", "代码", "端点", "bug", "编译", "跑通", "上线", "实现", "接口", "引擎", "网关"}
	implementNouns   = []string{"服务", "后台", "系统", "模块", "平台", "程序", "工具", "组件", "引擎", "网关", "中间件", "需求说明书"}
	questionExcludes = []string{"怎么", "如何", "为什么", "哪能", "能否", "怎么弄", "怎么做", "怎么样"}

	// --   calluse(line C fix   2026-10-03)--
	skillInvokeVerbs = []string{"调用", "使用", "用", "按", "按照", "依据", "根据", "让"}
	skillNames       = []string{"校验与对齐", "校准与对齐", "validate-align", "对齐", "校验", "校准",
		"plain-explainer", "解释", "检索", "写作", "翻译", "合规", "审阅", "报价", "文案"}
	skillActionWords = []string{"校准", "对齐", "校验", "检查", "审阅", "分析", "检索", "总结", "翻译", "编写", "生成", "输出"}
)

// detectSkillInvocation    baseis as"calluse/ use      "(line C fix  ). 
//  inreturnback ORCHESTRATE + params{kind=skill, skill_name?, action?}. 
// triggersend:   "  " and(calluse word ∪   name connectoutnow)->   calluseintent. 
//  sent  ("  use  …"is   iscalluse). 
func detectSkillInvocation(text string) (string, map[string]string, bool) {
	if containsAny(text, questionExcludes) {
		return "", nil, false
	}
	if !containsAny(text, []string{"技能"}) {
		return "", nil, false
	}
	params := map[string]string{"kind": "skill"}
	invoked := false
	// ①  form  nameoutnow("verifyandto   "/"validate-align   ")->  signal
	for _, n := range skillNames {
		if strings.Contains(text, n) {
			if !strings.Contains(text, "怎么") && !strings.Contains(text, "如何") {
				params["skill_name"] = n
				invoked = true
				break
			}
		}
	}
	// ② calluse word + "  " +   domainword("calluse    approve/ out approve  ")
	//   recv (2026-10-03   ):  " use   X"(X    domain)  --  
	//   " now   use       "be     block(   nowclass). 
	if !invoked {
		hasInvokeVerb := false
		for _, v := range skillInvokeVerbs {
			if strings.Contains(text, v) {
				hasInvokeVerb = true
				break
			}
		}
		if hasInvokeVerb {
			for _, a := range skillActionWords {
				if strings.Contains(text, a) {
					params["action"] = a
					invoked = true
					break
				}
			}
		}
	}
	if !invoked {
		return "", nil, false
	}
	return contract.IntentOrchestrate, params, true
}

// detectImplementOrchestrate    baseis as" now/       "    nowtask. 
//  inreturnback ORCHESTRATE + params{kind=implement, target_doc?}. 
func detectImplementOrchestrate(text string) (string, map[string]string, bool) {
	//  sent  :     /e.g. /as  … =    now form,  is nowtask. 
	if containsAny(text, questionExcludes) {
		return "", nil, false
	}
	vi, ni := -1, -1
	//  use now word( now/  /opensend…) orig  ; fix class word(fix /patch /finish …)
	//   sametime in nowdomain signal(implDomainWords),  then is nowtask(prevent"fix   "  ). 
	for _, w := range implementVerbs {
		if idx := strings.Index(text, w); idx >= 0 && (vi < 0 || idx < vi) {
			// 2026-10-03   sendnow:  word in  **   nameid … in**--
			// "  become safetyscenarioopensend   andkeepstore  " "opensend"is  namebecomesplit,  cur now word
			// ⇒   classbe   implement(L-01  recv  , see /tmp/l01_confirm.txt). 
			if insideBookTitle(text, idx, idx+len(w)) {
				continue
			}
			// 2026-10-04  2:  wordafter  "  /  /  /  /  "timeisnameword lang
			//(e.g." now  "),  is now   ->  ed("pipe now  fix  under…"). 
			if strings.HasPrefix(text[idx+len(w):], "计划") || strings.HasPrefix(text[idx+len(w):], "方案") ||
				strings.HasPrefix(text[idx+len(w):], "文档") || strings.HasPrefix(text[idx+len(w):], "报告") ||
				strings.HasPrefix(text[idx+len(w):], "步骤") {
				continue
			}
			vi = idx
		}
	}
	reviseHit := false

	for _, w := range reviseVerbs {
		if idx := strings.Index(text, w); idx >= 0 && (vi < 0 || idx < vi) {
			if insideBookTitle(text, idx, idx+len(w)) {
				continue
			}
			if !containsAny(text, implDomainWords) {
				continue // fix classno nowdomainsignal(e.g."fix     ")->  is nowtask
			}
			vi = idx
			reviseHit = true
		}
	}
	// nameword   ** wordofafter**  (text[vi:]): 
	// "  ityizeASRafter needrequire   v2  now  …after serveservice"  nameidin "after "is langbecomesplit, 
	// ifsafetysent   nameword  to ni<vi    ity(2026-10-03   sendnow, see zz_debug   ). 
	if vi >= 0 {
		for _, w := range implementNouns {
			if idx := strings.Index(text[vi:], w); idx >= 0 && (ni < 0 || idx < ni) {
				ni = idx + vi // alsoorigas to  
			}
		}
		// fix class word namewordsignal  to nowdomainword(endpoint/ code/connect /bug/  /  …): 
		// "pipe    /v1/blacklist endpointpatch "no implementNouns  bodyword,   nowdomainword bot. 
		if reviseHit {
			for _, w := range implDomainWords {
				if idx := strings.Index(text[vi:], w); idx >= 0 && (ni < 0 || idx < ni) {
					ni = idx + vi
				}
			}
			if ni < 0 && containsAny(text, implDomainWords) {
				// fix  lang   wordbefore("pipe   endpointpatch "): safety  nowdomainword ini.e. namewordsignalbecome . 
				ni = vi + 1
			}
		}
		// coreference  (2026-10-03 onunder  ):  word inand**safety ** coreferenceword(  /   )
		// also asnamewordsignal--"by  needrequire   continuecontinue nowand  "no bodynameword,   resolve objtgt. 
		//  safety but  text[vi:]: coreferenceword   wordbefore("by  needrequire   **continuecontinue now**"). 
		if ni < 0 {
			for _, d := range []string{"这份", "该文档", "此文档", "这个", "这些", "它", "那"} {
				if strings.Contains(text, d) {
					ni = vi + 1
					break
				}
			}
		}
	}
	//  word  nameword:  word  store and wordofafterhasnameword. 
	if vi < 0 || ni < 0 || ni <= vi {
		return "", nil, false
	}
	params := map[string]string{"kind": "implement"}
	if title := extractBookTitle(text); title != "" {
		params["target_doc"] = title
	}
	return contract.IntentOrchestrate, params, true
}


// detectOrchestrate    baseis as"      ->occurbecomefile->keepstore/  "    task. 
//  intimereturnback (IntentOrchestrate, params{target_doc, source_hint}, true). 
// target_doc from …  nameid  ;   tothenbyorchestrate   defaultfilename. 
func detectOrchestrate(text string) (string, map[string]string, bool) {
	if !containsAny(text, orchOrganizeWords) {
		return "", nil, false
	}
	if !containsAny(text, orchDocWords) {
		return "", nil, false
	}
	if !(containsAny(text, orchSaveWords) || containsAny(text, commitTriggers)) {
		return "", nil, false
	}
	params := map[string]string{"commit": "1"}
	if title := extractBookTitle(text); title != "" {
		params["target_doc"] = title
	}
	return contract.IntentOrchestrate, params, true
}

// extractBookTitle  get …  nameidin   name(  nameid, keep   namept). 
func extractBookTitle(text string) string {
	i := strings.Index(text, "《")
	j := strings.Index(text, "》")
	if i >= 0 && j > i {
		return strings.TrimSpace(text[i+len("《") : j])
	}
	return ""
}

// nowFn istimetime (timeanchor.go dependency,      ). 
var nowFn = func() time.Time { return time.Now() }

// ClassifyTask   pseudocode logic layer branch  , produceout M2 taskintent JSON. 
func (c *TaskClassifier) ClassifyTask(text string) contract.Intent {
	ti := contract.Intent{
		Intent:        contract.IntentUnknown,
		CorrectedText: text,
		Confidence:    0.2,
		Ask:           taskAskTemplate,
		Boundary:      &contract.Boundary{Exclude: defaultExcludes},
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return ti
	}
	if runes := []rune(text); len(runes) > 512 {
		text = string(runes[:512])
		ti.CorrectedText = text
	}

	// 0.     (   G1,     delete  **ofbefore**). 
	//    ` needdelete  file` iffirst on deleteTriggers, then be become     EDIT(action=delete); 
	//      word connect    time    ASK confirm -- SPEC-v2:49"Ask != '' ->     ". 
	if neg, actionBound, ok := hasNegation(text); ok && (actionBound ||
		containsAny(text, actionWords) || registerToolRequest(text)) {
		got := c.fill(ti, contract.IntentAsk, 0.9, nil)
		got.Conflict = contract.ConflictNegation
		got.Ask = "我听到的是「" + neg + "」——确认不执行这个动作吗？" +
			"确认不做请说「取消」；确实要做请重新说一遍完整指令"
		return got
	}

	// 0.5  refer   (   G3): "openstart  "is  to ,  is"  go test ./...". 
	//       has  wordonly ( "openstart" ed  ),  ini.e.clarification  . 
	if meta, ok := metaInstruction(text); ok {
		got := c.fill(ti, contract.IntentAsk, 0.9, nil)
		got.Conflict = contract.ConflictMeta
		got.Ask = "「" + meta + "」是让我" + metaActionText(meta) + "，还是要我现在就执行后面的动作？" +
			"要执行请直接说完整指令（例如「跑一下测试」）"
		return got
	}

	// 0.6   sent  (   G6): "e.g.    edthen  " before   be  . 
	//         useuser    --     ,  useuserfirstdonebefore againunderrefer . 
	if cond, ok := conditionalClause(text); ok {
		got := c.fill(ti, contract.IntentAsk, 0.9, nil)
		got.Conflict = contract.ConflictConditional
		got.Ask = "「" + cond + "」是带前提的动作。我不会替你守着条件——" +
			"请先完成前提（例如先把测试跑完），再直接说指令"
		return got
	}

	// 1.     ( first   )
	switch {
	case registerToolRequest(text):
		return c.fill(ti, contract.IntentRegisterTool, 0.95, nil)
	case containsAny(text, deleteTriggers):
		ti.Conflict = contract.ConflictDelete
		return c.fill(ti, contract.IntentEdit, 0.9, map[string]string{"action": "delete"})
	case containsAny(text, feasibleAsk):
		ti.Conflict = contract.ConflictAskVsOp
		return c.fill(ti, contract.IntentAsk, 0.9, nil)
	case containsAny(text, thoughtWords) && !c.spaceShadowsThought(text):
		ti.Conflict = contract.ConflictNoteVsDeploy
		return c.fill(ti, contract.IntentNote, 0.9, nil)
	case containsAny(text, statusQuestion):
		return c.fill(ti, contract.IntentQuery, 0.85, c.queryParams(text))
	case containsAny(text, debugTriggers) && containsAny(text, debugPlanWords):
		ti.Intent = contract.IntentAsk
		ti.Confidence = 0.5
		ti.Ask = "你是想让我给修 bug 的思路，还是直接动手修？"
		ti.Conflict = contract.ConflictDebugPlan
		return ti
	}

	// 2a-pre.      (   G5): "  under store, howeverafter  underclose ,  after  "
	// ifonly      in intent, its    **novoice  **, useuserbyasall . 
	//
	//      2a(extractReplace  beforereturnback)**ofbefore** --  then
	// "pipe    modifybecomenew    head, howeverafter  under  " first in EDIT  connectreturnback. 
	//
	//   only   isproduce   has end(   decision point),  by  **    **, 
	//  connectpipe    open useuser first   . 
	//    G5-P1-3: orchestratetaskbase    , by    , **    **. 
	// orig now "   has  linkconnectword"only be   -- protectis however ,  isclose ity : 
	// "…  become safetyscenarioopensend   , howeverafteragainkeepstore  " be become NOTE+COMMIT but    orchestrate. 
	//    G5-P2-6   : **  fixposchain is   **. 
	// "pipetgt modifybecomein ,  to, modifybecome  ,   , modifybecome   lang"by id  out 3 seg EDIT, 
	// but issame  intentberev fixpos,   give G7  fixpos  handle. 
	corrections := 0
	if cut, _ := lastCorrection(text); cut >= 0 {
		corrections = 1
	}
	_, _, isOrch := detectOrchestrate(text)
	_, _, isImpl := detectImplementOrchestrate(text)
	if !isOrch && !isImpl && corrections == 0 {
		if n, labels, hadConn := multiActionClauses(text); multiActionTrips(n, hadConn) {
			got := c.fill(ti, contract.IntentAsk, 0.9, nil)
			got.Conflict = contract.ConflictMultiAction
			// 2026-10-04 useuserorig " manage   ,  first resolve    ":  again  clarification"first   ", 
			//   ASK   type   resolveandback (   face    decision point, butto   useuser). 
			got.Ask = "你说了几件事（" + joinIntentLabels(labels) + "）。" +
				"我先整体理解一下，你接着补一句最要紧的，我一件一件来"
			return got
		}
	}

	// 2a.  form"pipe X modifybecome/ become Y"-> EDIT( firstat COMMIT/DEPLOY etctriggersendword, e.g."pipe  by modifybecomein ")
	if _, _, ok := extractReplace(text); ok {
		return c.fill(ti, contract.IntentEdit, 0.9, c.editParams(text))
	}

	// 2a-bis.   / task(  erformrouteby):   /   +   /   +(keepstore/  ) linksignal
	// -> ORCHESTRATE,  again become   NOTE/COMMIT. 
	//   firstat 2b  classtriggersend: "    " "  "  in noteTriggers, "  "  in commitTriggers--
	//  task"pipesafety     and      become … andkeepstore  " beforebe  as   NOTE  seg append. 
	if kind, params, ok := detectOrchestrate(text); ok {
		return c.fill(ti, kind, 0.9, params)
	}

	// 2a-ter.  nowclass task(ORCHESTRATE kind=implement):  now/  /fix      . 
	//  2 fix (2026-10-04): splitsend   formcalluse detectImplementOrchestrate--
	//  then"pipeneedrequire   nowout …and  to  "be 2b   commitTriggers  become    COMMIT, 
	//  nowclass task    inorchestrate  ( beforeonly  num connectinsplitsend,  data   triggersend). 
	if kind, params, ok := detectImplementOrchestrate(text); ok {
		params["kind"] = "implement"
		return c.fill(ti, kind, 0.9, params)
	}

	// [pseudocode logic layer](M5-1 triggersendword    : NOTE lang word vs TEST triggersendword)
	//
	// control flow(  i.e. first ,  word before; base switch  onbutunderfirst  iner out): 
	//
	//	if containsAny(text, noteTriggers):  return NOTE   //  under/  under/  /  …
	//	elif containsAny(text, queryTriggers): return QUERY
	//	elif containsAny(text, debugTriggers): return DEBUG
	//	elif containsAny(text, testTriggers): return TEST  //    /  under/  …
	//	…
	//
	//  first rule(M5-1   , fix "     under M4   "be"  "   TEST): 
	//   - NOTE lang word( under/  under/  /  ,  "  /  "lang )**firstat** TEST word  ; 
	//     thus" under X   ""  under X   ""      under"   NOTE--senttail"  "is
	//     be   to ,  isneed     . 
	//   - **exampleout(   back )**: sent      NOTE word, only   TEST triggersend
	//     (  under  /    /     num)->    TEST. 
	//   - intent disconnectrulebody(    NOTE/TEST semantic)  VSL,  placeonlywritecontrol flowand    . 
	//
	//    : input.TestTriggerCollisionNoteVsTest(posrevexample + QUERY  back ). 
	//
	// 2b.  classtriggersend( word before, SPEC v1 §2 table)
	switch {
	case containsAny(text, writeTriggers):
		// 2026-10-04     R6/R7: writefilerefer (pipe XX writeto/write/keepstoreto /path)
		// firstat NOTE/TEST   --"pipe        file writeto /tmp" againbe"  " keep. 
		return c.fill(ti, contract.IntentEdit, 0.85, nil)
	case containsAny(text, reminderTriggers):
		// Reminder/alarm must win over noteTriggers: "记个提醒：…" must not be appended to
		// notes.md. The executor reports REMINDER as explicitly not implemented.
		return c.fill(ti, contract.IntentReminder, 0.9, nil)
	case containsAny(text, noteTriggers):
		return c.fill(ti, contract.IntentNote, 0.85, nil)
	case containsAny(text, queryTriggers):
		return c.fill(ti, contract.IntentQuery, 0.85, c.queryParams(text))
	case containsAny(text, debugTriggers):
		return c.fill(ti, contract.IntentDebug, 0.85, map[string]string{"object": "debug"})
	case containsAny(text, testTriggers):
		kind := "go test ./..."
		if containsAny(text, []string{"性能", "bench", "基准"}) {
			kind = "bench"
		}
		return c.fill(ti, contract.IntentTest, 0.85, map[string]string{"test_kind": kind})
	case containsAny(text, commitTriggers):
		return c.fill(ti, contract.IntentCommit, 0.85, nil)
	case containsAny(text, deployTriggers):
		return c.fill(ti, contract.IntentDeploy, 0.85, nil)
	case containsAny(text, askTriggers):
		return c.fill(ti, contract.IntentAsk, 0.85, nil)
	case containsAny(text, editTriggers): // its  EDIT  state(modify under/modify/  /modify)
		return c.fill(ti, contract.IntentEdit, 0.85, c.editParams(text))
	}

	// 0.8  rev /    (2026-10-04 useuser    ptname" is      "clarification): 
	//   /  / rev   (" diff   ""after has has  ""   ")  in UNKNOWN clarification--
	//   ASK   type connect  back . ASK class  triggersendclarification(pseudocode logic layer 11: low-confidence  NOTE/ASK onlyclarification). 
	// note :    classtriggersendofafter--"   under"first in TEST,   be    . 
	if containsAny(text, chatTriggers) {
		return c.fill(ti, contract.IntentAsk, 0.8, nil)
	}

	// 3. no  triggersendword -> UNKNOWN, clarification
	return ti
}

// fill byintent  patch  emptytime  /timetime pt/ boundary/riskbaseline/confirmbaseline/ recv/   , and low-confidenceclarification. 
func (c *TaskClassifier) fill(ti contract.Intent, kind string, conf float64, params map[string]string) contract.Intent {
	ti.Intent = kind
	ti.Confidence = clamp(conf)
	ti.Ask = "" //   UNKNOWN    emptyinitstartclarification; applyCommon onlytolow-confidenceheavy 
	if params != nil {
		ti.Params = params
	}
	c.applyCommon(&ti, kind, conf)

	//    G2:    sent i.e.then   also  clarification. 
	// orig as:  classtriggersend  0.85,   atdefault value 0.6, atis applyCommon    
	// "low-confidenceclarification" default  under      -- "modify under""fix"" " connect     . 
	if ti.Ask == "" && slotGateTrips(kind, ti.CorrectedText) {
		ti.Ask = askForKind(kind)
	}
	return ti
}

// slotGateFillers is  time and    word/ fillword. 
var slotGateFillers = []string{
	"一下", "一遍", "这个", "那个", "它", "帮我", "麻烦", "请", "吧", "呢", "啊", "呀",
	"了", "的", "，", "。", "、", "！", "？", ",", ".", "!", "?", " ",
}

// slotGateTrips    intent sent  triggersendwordbase ofoutis       . 
//
// onlyto EDIT / DEBUG / QUERY occur :   class "to "is need  ,  thennofrom  . 
// TEST/COMMIT/DEPLOY/NOTE/ASK hasdefaultvalueorbase thennoneedto ,   base   in. 
//
//  nowneedpt: **  **pipetriggersendwordfrom base   again    -- triggersendword  thenis  base . 
// examplee.g. DEBUG  triggersendwordtable has "bug", "fix under   bug" finishonly empty,  be  as   . 
// because modifyas:    wordafter,  **    is   edsentin in   triggersendword**. 
//
//	"modify under"            ->   "modify"(1) <=   triggersendword"modify under"(3) -> clarification
//	"fix under   bug"     ->   "fixbug"(4) >   triggersendword"bug"(3)  ->  clarification
//	" "                ->   " "(1) <= " "(1)              -> clarification
func slotGateTrips(kind, text string) bool {
	triggers := slotGateTriggers(kind)
	if triggers == nil {
		return false
	}
	residual := text
	for _, f := range slotGateFillers {
		residual = strings.ReplaceAll(residual, f, "")
	}
	residual = strings.TrimSpace(residual)

	longest := 0
	for _, t := range triggers {
		if strings.Contains(text, t) {
			if n := len([]rune(t)); n > longest {
				longest = n
			}
		}
	}
	if longest == 0 {
		return false
	}
	return len([]rune(residual)) <= longest
}

// slotGateTriggers returnback intent and    triggersendwordtable;    inreturnback nil. 
func slotGateTriggers(kind string) []string {
	switch kind {
	case contract.IntentEdit:
		return editTriggers
	case contract.IntentDebug:
		return debugTriggers
	case contract.IntentQuery:
		return queryTriggers
	default:
		return nil
	}
}

// applyCommon  fill emptytime  /objtgt/timetime pt/ boundarybaseline/riskbaseline/confirmbaseline/ recv  /low-confidenceclarification. 
func (c *TaskClassifier) applyCommon(ti *contract.Intent, kind string, conf float64) {
	text := ti.CorrectedText

	// 4. emptytime  (note emptytimename/diffname    ,     ;  note  bodyonly  Target,    Space)
	if name, ok := c.matchSpace(text); ok {
		ti.Space = name
		ti.Target = &contract.Target{Entity: name, RefType: "explicit"}
		ti.Context = []string{"project-map:" + name}
		if ti.Boundary != nil {
			ti.Boundary.Scope = []string{name + "/**"}
		}
	} else if p, ok := extractInClause(text); ok {
		ti.Target = &contract.Target{Entity: p, RefType: "explicit"}
	}

	// timetime pt(safety intent   )
	if hint, date, _ := ResolveTimeAnchor(text, nowFn()); hint != "" {
		if ti.Params == nil {
			ti.Params = map[string]string{}
		}
		ti.Params["time_hint"] = hint
		ti.Params["time_date"] = date
	}

	// 7. riskbaseline + 8. confirmbaseline( authoritative; risk   decideafterbackfillauthoritative value)
	switch kind {
	case contract.IntentCommit, contract.IntentDeploy, contract.IntentOrchestrate:
		ti.Risk = &contract.RiskBaseline{Reversible: false, Impact: contract.ImpactHigh}
		ti.Confirm = contract.ConfirmHuman
	case contract.IntentDebug:
		ti.Risk = &contract.RiskBaseline{Reversible: true, Impact: contract.ImpactHigh}
		ti.Confirm = contract.ConfirmLight
	case contract.IntentEdit, contract.IntentRegisterTool:
		ti.Risk = &contract.RiskBaseline{Reversible: true, Impact: contract.ImpactMedium}
		ti.Confirm = contract.ConfirmLight
	default: // NOTE/QUERY/TEST/ASK read-onlyor   
		ti.Risk = &contract.RiskBaseline{Reversible: true, Impact: contract.ImpactSmall}
		ti.Confirm = contract.ConfirmAuto
	}

	// 9.  recv  
	ti.Acceptance = acceptanceTemplate(kind, ti.Space, ti.Boundary)

	// 11. low-confidenceclarification(NOTE/ASK/REGISTER_TOOL no need  ,  because    clarification)
	if kind != contract.IntentUnknown && kind != contract.IntentNote &&
		kind != contract.IntentAsk && kind != contract.IntentRegisterTool &&
		conf < c.ConfThreshold {
		ti.Ask = askForKind(kind)
	}
}

// spaceShadowsThought    base in note emptytimename/diffnameis  "  /  "classword. 
// 20 kindexample to   surfaced: emptytimename"   "   "  ", be thoughtWords    first  NOTE--
// emptytimenameis bodyname is"   "triggersend,  ini.e.      ( after      , SPEC v2 §1#10 already  ). 
func (c *TaskClassifier) spaceShadowsThought(text string) bool {
	low := strings.ToLower(text)
	for _, h := range c.Spaces {
		for _, alias := range append([]string{h.Name}, h.Aliases...) {
			if alias == "" {
				continue
			}
			a := strings.ToLower(alias)
			if strings.Contains(low, a) {
				for _, w := range thoughtWords {
					if strings.Contains(a, w) {
						return true
					}
				}
			}
		}
	}
	return false
}

// matchSpace   basein note emptytimename/diffname    ;   (same    in)->   (ok=false). 
func (c *TaskClassifier) matchSpace(text string) (string, bool) {
	best, bestLen := "", 0
	low := strings.ToLower(text)
	for _, h := range c.Spaces {
		for _, alias := range append([]string{h.Name}, h.Aliases...) {
			if alias == "" {
				continue
			}
			a := strings.ToLower(alias)
			if strings.Contains(low, a) && len(a) > bestLen {
				best, bestLen = h.Name, len(a)
			} else if strings.Contains(low, a) && len(a) == bestLen && h.Name != best && best != "" {
				bestLen = -1 //    -> tgt   
			}
		}
	}
	if bestLen <= 0 {
		return "", false
	}
	return best, true
}

// editParams  get EDIT   : "pipe X modifybecome Y"-> action/object/value;  then action=replace + object=path. 
func (c *TaskClassifier) editParams(text string) map[string]string {
	p := map[string]string{}
	if object, value, ok := extractReplace(text); ok {
		p["action"] = "replace"
		p["object"] = object
		p["value"] = value
		return p
	}
	p["action"] = "replace"
	if obj, ok := extractPath(text); ok {
		p["object"] = obj
	}
	return p
}

// queryParams  get QUERY   : object and on  semantic. 
func (c *TaskClassifier) queryParams(text string) map[string]string {
	p := map[string]string{}
	if obj, ok := extractPath(text); ok {
		p["object"] = obj
	}
	if containsAny(text, []string{"上次", "之前"}) {
		p["scope"] = "recent"
	}
	return p
}

// extractReplace  get"pipe X modifybecome Y / pipe X  become Y"   : 
//   - before segfirstget"pipe"ofafter  split(object), again  "  X  /in"before  sent(its  emptytime  ). 
func extractReplace(text string) (object, value string, ok bool) {
	//    G7:     fixpos. useuser " to/  "ofbefore table   , onlyhasitsafteronlyis  intent. 
	//
	// orig as: `  underA,  to, modifybecome B`  pipe"  underA,  to" segcurbecome object. 
	//
	// fixposafter   to (`pipetgt modifybecomein ,  to, modifybecome  `), thus connectfixposbefore to . 
	carry := ""
	if cut, end := lastCorrection(text); cut >= 0 {
		if obj, _, okPre := extractReplaceRaw(text[:cut]); okPre {
			carry = obj
		}
		text = strings.TrimLeft(text[end:], "，。、, ")
	}
	object, value, ok = extractReplaceRaw(text)
	if !ok {
		return "", "", false
	}
	if object == "" {
		object = carry
	}
	if object == "" || value == "" {
		return "", "", false
	}
	return object, value, true
}

// correctionMarkers is  fixpossignal(   G7). getits** after  **outnow,  keeplinkcontinuefixpos. 
var correctionMarkers = []string{"不对", "说错了", "打错了", "重新说", "我是说", "应该是"}

// lastCorrection returnback after    fixpostgt  raisestartandcloseend**charnode**undertgt; nothen (-1, -1). 
func lastCorrection(s string) (int, int) {
	bestStart, bestEnd := -1, -1
	for _, m := range correctionMarkers {
		if i := strings.LastIndex(s, m); i >= 0 && i > bestStart {
			bestStart, bestEnd = i, i+len(m)
		}
	}
	return bestStart, bestEnd
}

// extractReplaceRaw is    fixposhandle origstart now. 
//
// and  unique semanticdiffdiff: **to asempty again connect   **(onlyneedrequire value  empty), 
// bythen extractReplace  "modify after  to "time connectfixposbefore to . 
// is  endbecome by extractReplace decide . 
func extractReplaceRaw(text string) (object, value string, ok bool) {
	low := strings.ToLower(text)
	for _, sep := range []string{"改成", "换成"} {
		if i := strings.Index(low, sep); i >= 0 {
			before := strings.TrimSpace(text[:i])
			value = strings.TrimSpace(text[i+len(sep):])
			if j := strings.LastIndex(before, "把"); j >= 0 {
				before = before[j+len("把"):]
			}
			if stripped, found := stripInClause(before); found {
				before = stripped
			}
			object = strings.Trim(before, "，。、  ")
			if value == "" {
				return "", "", false
			}
			return object, value, true
		}
	}
	return "", "", false
}

// stripInClause   "  X  /in/ face" sent, returnback   splitandis   . 
func stripInClause(s string) (string, bool) {
	low := strings.ToLower(s)
	for _, open := range []string{"在", "从"} {
		if i := strings.Index(low, open); i >= 0 {
			for _, close := range []string{"里面", "里", "中"} {
				rest := low[i+len(open):]
				if j := strings.Index(rest, close); j >= 0 {
					remain := s[i+len(open)+j+len(close):]
					return strings.TrimSpace(remain), true
				}
			}
		}
	}
	return s, false
}

// extractInClause  get"  X  /in/ face"  obj/path body. 
func extractInClause(text string) (string, bool) {
	low := strings.ToLower(text)
	for _, open := range []string{"在", "从"} {
		if i := strings.Index(low, open); i >= 0 {
			for _, close := range []string{"里面", "里", "中"} {
				rest := text[i+len(open):]
				if j := strings.Index(rest, close); j >= 0 {
					entity := strings.TrimSpace(rest[:j])
					entity = strings.Trim(entity, "，。、 ")
					if entity != "" {
						return entity, true
					}
				}
			}
		}
	}
	return "", false
}

// acceptanceTemplate byintentreturnback recv  (verify       howeverlanglang data). 
func acceptanceTemplate(kind, space string, b *contract.Boundary) string {
	scope := "*"
	if space != "" {
		scope = space + "/**"
	} else if b != nil && len(b.Scope) > 0 {
		scope = b.Scope[0]
	}
	switch kind {
	case contract.IntentEdit:
		return "改动限定在 " + scope + " 内，无越界改动"
	case contract.IntentTest:
		return "测试全绿，无回归"
	case contract.IntentDebug:
		return "复现问题消失，回归测试通过"
	case contract.IntentCommit:
		return "提交成功，未推远端"
	case contract.IntentDeploy:
		return "部署成功且目标可访问"
	default:
		return "输出满足提问"
	}
}

// askForKind byintent restrictclarification  (referout   ,  allow  ). 
func askForKind(kind string) string {
	switch kind {
	case contract.IntentEdit:
		return "要改哪些文件/哪里？请再说一遍"
	case contract.IntentQuery:
		return "要查哪个项目/文件？请再说一遍"
	case contract.IntentDebug:
		return "要修的是哪个报错？请贴一下报错内容"
	case contract.IntentTest:
		return "要跑哪里的测试？请再说一遍"
	case contract.IntentCommit:
		return "要提交哪些改动？请再说一遍"
	case contract.IntentDeploy:
		return "要部署到哪里？目标是什么？"
	default:
		return taskAskTemplate
	}
}

// insideBookTitle  disconnect [start,end)  timeis finishsafety   nameid … in. 
// useway:  word/nameword      namebecomesplit(e.g. safetyscenarioopensend     "opensend" is now word). 
func insideBookTitle(text string, start, end int) bool {
	i := strings.Index(text, "《")
	j := strings.Index(text, "》")
	if i < 0 || j <= i {
		return false
	}
	return start >= i && end <= j
}
