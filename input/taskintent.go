package input

// taskintent.go —— M2 语音驱动开发任务意图分类器。
//
// 【伪代码逻辑层】（评审关卡产物；SPEC v2 §伪代码逻辑层 的对应小节为权威版，本注释为实现的评审基线。
//  意图判断规则的权威定义在 SPEC v2/VSL 定义层，本层只描述单模块控制流/分支/异常路径，不重复规则本体。）
//
// 模块职责：把纠错后文本分类为 M2 任务意图（八类 + REGISTER_TOOL），
//   抽取 空间候选 / 动作参数 / 时间锚点 / 边界基线 / 验收模板 / 风险与确认基线。
// 输入：corrected text；空间候选表（注册名+别名，pipeline 注入）；置信度阈值。
// 输出：contract.Intent（M2 字段填充；低置信 Ask 非空，绝不执行）。
//
// 控制流（分支判定，优先级从高到低）：
//   0. 空文本/纯空白 → UNKNOWN + Ask。
//   1. 冲突仲裁（先于单类触发）：
//      a. 删除动词（删/删除/去掉/移除/清空）→ EDIT + params.action=delete
//         （风险包按不可逆处理 → 永远人工确认，不可被学习掉）。
//      b. 注册动词（加一个工具/注册工具/新增工具…）→ REGISTER_TOOL。
//      c. 可行性问句（能不能/可不可以/是否可以/行不行）→ ASK，不是 QUERY。
//      d. 想法压制部署（含「想法/备忘」且未被空间名遮蔽）→ NOTE（「发个想法」→ NOTE，不是 DEPLOY；
//         空间名含想法类词时遮蔽——「想法库」是实体名不是记想法触发，20 样例方向验证 surfaced 后修复）。
//      e. 状态问句（好了吗/改没改/改了吗）→ QUERY，不是 EDIT/DEBUG。
//      f. DEBUG 触发 + 「思路/怎么做/方案/打算」→ 低置信 → Ask（不猜执行意图）。
//   2. 单类触发（SPEC v1 §2 触发词表，长词优先；沿用 M1 的 substring 匹配约定，CJK 无词边界）：
//      NOTE 记：记一下/记下来/记住/记录一下/记录/存档/存个/存到
//        （不用裸「记」——20 样例方向验证 surfaced：子串命中「记的/记忆/忘记」造成 QUERY 误判 NOTE）
//      QUERY 查：查一下/查/找一下/找/上次/搜一下/搜/看看/看
//      EDIT 改：把…改成/改成/换成/改一下/修改/替换/改
//      DEBUG 修：报错/为什么失败/崩溃/闪退/出错/bug/修一下/修这个/修那个/修一修/修
//      TEST 跑：跑测试/跑一下/测一下/跑个测试/测试（含「性能」→ test_kind=bench）
//      COMMIT 提交：提交/推上去/推到
//      DEPLOY 发：部署/上线/生成报表/发到/发布
//      ASK 问：为什么/怎么办/你觉得/是什么意思/怎么弄/如何
//   3. 槽位抽取（按意图模板）：
//      - path/object：引号内容 → ~//开头 token → 目录/文件前词。
//      - EDIT：「把 X 改成 Y」→ object=前置实体（剥「把」），value=后置值。
//      - TEST：test_kind（bench|go test ./...|指定路径）。
//      - time：时间锚点原文+解析日期（timeanchor.go）。
//   4. 空间候选：注册空间名/别名在文本中最长匹配 → Space + Target{ref_type:explicit}；
//      无命中 → Space=""（不猜，交给 refer/space_check 兜底，走 global 只读或回问）。
//   5. 目标实体：仅显式引用（引号实体/空间候选/「在 X 里」的 X）填 Target；人名/指代词交 refer 层。
//   6. 边界基线：Scope = 空间候选名或显式路径；Exclude 默认 [".env*","node_modules"]。
//   7. 风险基线（非权威，risk 包用机械信号重算）：NOTE/QUERY/TEST/ASK → 可逆 small；
//      EDIT → 可逆 medium；DEBUG → 可逆 high；COMMIT/DEPLOY → 不可逆；REGISTER_TOOL → 可逆 medium。
//   8. Confirm 基线：COMMIT/DEPLOY → human；DEBUG/EDIT/REGISTER_TOOL → light；其余 auto。
//   9. Acceptance 模板（verify 契约消费）：EDIT → "改动限定在 {scope} 内，无越界改动"；
//      TEST → "测试全绿，无回归"；DEBUG → "复现问题消失，回归测试通过"；COMMIT → "提交成功，未推远端"；
//      DEPLOY → "部署成功且目标可访问"；QUERY/NOTE/ASK → "输出满足提问"。
//  10. 置信度：仲裁命中且槽位齐 → 0.9；单类命中且槽位齐 → 0.85；缺关键槽位 -0.2；模棱两可 → 0.5。
//  11. 低置信（< 阈值且非 NOTE/ASK/REGISTER_TOOL）→ Ask 回问；UNKNOWN 恒回问。
//
// 异常处理：空间名多命中 → 最长别名胜出，平局不猜（回问）；文本 > 512 字符截断再分类；
//   Ask 文案按意图定制（指出缺什么），UNKNOWN 之外不允许静默降级行为。

import (
	"strings"
	"time"

	"voicesign-harness/contract"
)

// SpaceHint 是空间候选提示（pipeline 注入注册空间名/别名，供分类器做最长匹配）。
type SpaceHint struct {
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
}

// TaskClassifier 把纠错后文本分类为 M2 任务意图（八类 + REGISTER_TOOL）。
type TaskClassifier struct {
	ConfThreshold float64
	Spaces        []SpaceHint
}

// NewTaskClassifier 构造任务分类器。conf<=0 时取 0.6（与 M1 一致）。
func NewTaskClassifier(conf float64, spaces []SpaceHint) *TaskClassifier {
	if conf <= 0 {
		conf = 0.6
	}
	return &TaskClassifier{ConfThreshold: conf, Spaces: spaces}
}

const taskAskTemplate = "你是想让我做什么？请再说清楚一点（记想法/查代码/改代码/修 bug/跑测试/提交/部署/问问题）"

var (
	// registerTriggers 是历史触发词表：保留给 actionWords（否定/条件的动作全集）
	// 与非结构化兜底使用。新判定一律走 registerToolRequest 的结构性判据 ——
	// 见下方"注册工具意图（架构缺口 A1）"。
	registerTriggers = []string{"加一个工具", "加个工具", "注册工具", "新增工具", "加个新工具", "加一个新工具",
		// 2026-10-04 验证器 R11：口语注册"增加一个查看天气的能力"——
		// "增加一个/新增一个/添加一个"+"能力" 语境命中（"加一个"太宽不加入）。
		"增加一个", "新增一个", "添加一个"}
	// writeTriggers 写文件指令（2026-10-04 验证器 R6/R7 逼出）：
	// "把 XX 写到 /path" 此前被判 TEST（含"测试"字）或 UNKNOWN（无触发词）。
	// 独立表放最前，先于 TEST 判定。
	// 2026-10-08 用户实测：说「写一封英文邮件」被判 UNKNOWN → 死循环回问。
	// 根因：触发词只有「写一个/写个」，「写一封/写邮件/写信」不命中且「邮件」无词表。
	// 补全写作/邮件触发词（WRITE → LLM 生成内容返回，不回问）。
	writeTriggers  = []string{"写到", "写入", "保存到", "保存为", "创建文件", "写一个", "写个",
		"写一封", "写封", "写邮件", "写信", "写个邮件", "起草一封", "写一篇", "写一段"}
	deleteTriggers = []string{"删掉", "删除", "去掉", "移除", "清空"}
	feasibleAsk    = []string{"能不能", "可不可以", "是否可以", "行不行"}
	thoughtWords   = []string{"想法", "备忘"}
	statusQuestion = []string{"好了吗", "弄好了吗", "搞定了吗", "改好了吗", "改没改", "改了没", "改了吗", "弄了吗"}
	debugPlanWords = []string{"思路", "怎么做", "方案", "打算"}
	noteTriggers   = []string{"记一下", "记下来", "记下", "记个", "记住", "记录一下", "记录", "存档", "存个", "存到"}
	queryTriggers  = []string{"查一下", "查", "找一下", "找", "上次", "搜一下", "搜", "看看", "看",
		"几点", "几点钟", "什么时间", "几号", "星期几", "周几",
		// 2026-10-04 远程控制第二段：命令式触发词带空格，仅"运行 ls/执行 cat"类命中，
		// 不误伤"程序运行正常吗"（无空格不命中）。
		"运行 ", "执行 ", "帮我跑 ", "截个图", "截图"}
	editTriggers    = []string{"改成", "换成", "改一下", "修改", "替换", "改"}
	debugTriggers   = []string{"报错", "为什么失败", "崩溃", "闪退", "出错", "bug", "修一下", "修这个", "修那个", "修一修", "修"}
	testTriggers    = []string{"跑测试", "跑一下", "测一下", "跑个测试", "测试"}
	commitTriggers  = []string{"提交", "推上去", "推到"}
	deployTriggers  = []string{"部署", "上线", "生成报表", "发到", "发布"}
	askTriggers     = []string{"为什么", "怎么办", "你觉得", "是什么意思", "怎么弄", "如何"}
	defaultExcludes = []string{".env*", "node_modules"}
)

// ---------------------------------------------------------------------------
// 注册工具意图（架构缺口 A1 · 决策 #7「契约注册机制可被语音调用」）
//
// 问题：「注册一个命令，用来压缩图片」被判 UNKNOWN —— 自举链条从入口就断了。
// 根因不是"少收了几个词"，而是把"注册意图"实现成了对固定短语的词表匹配
// （加一个工具/注册工具/新增工具…）。用户不会迁就系统的词表：同一个意图
// 可以说成「注册一个命令」「新增一个技能」「添加个插件」「创建一个脚本」。
//
// 真特征（结构性）：**注册动词**直接支配**能力名词** —— 两者之间只允许
// 量化/虚词（一个/个/一条/新的…），不得夹带实义成分。据此：
//
//	「注册一个命令」      注册 + 一个 + 命令   → ✅ REGISTER_TOOL
//	「加个新工具」        加 + 个新 + 工具     → ✅ REGISTER_TOOL
//	「查一下注册表」      注册 后面是"表"，不支配任何能力名词 → ❌ 保持 QUERY
//	「查一下已注册的工具」 注册 与 工具 之间夹着"的"，是描述不是注册动作 → ❌
//
// 注意：判据刻意**不**允许"的"等实义成分跨过动词与名词之间，否则
// 「查一下已注册的工具」这类问句会被误判成注册指令（回归保护见
// input/registertool_regression_test.go）。
// ---------------------------------------------------------------------------

// registerVerbs 是"把一项新能力登记进系统"的动词。
// "加/做/搞"是口语中的泛化动词，必须靠"紧邻能力名词"才判定，故不能单独用。
var registerVerbs = []string{"注册", "新增", "添加", "增加", "创建", "新建", "加", "做", "搞", "上架", "接入"}

// registerCapabilityNouns 是可被注册的能力类别名词（需紧邻在动词/量词之后）。
var registerCapabilityNouns = []string{"工具", "小工具", "工具链", "命令", "子命令", "指令", "技能", "能力", "插件", "功能", "脚本"}

// registerGapFillers 是注册动词与能力名词之间允许出现的量化/虚词。
// **必须按长度降序**：registerGapThenNoun 取首个前缀命中，长词优先才不会被
// "一个"先把"一个新的"截断（否则"一个新的工具"剥成"新的工具"后判失败）。
var registerGapFillers = []string{
	"一个全新的",
	"一个新的", "一种新的", "一款新的",
	"个新的",
	"一个", "一条", "一款", "一种", "一支", "一项", "个新", "新的",
	"新", "个", "条", "款", "种", "支", "项",
}

// registerVerbWordPart 报告单字动词 v 在 pos 处是否只是某个词的一部分，而非独立动词。
// 「参加一个工具培训」里的"加"属于"参加"（去参加培训），不是"加一个工具"——
// 不加此闸会把它误判成注册意图。只有单字泛化动词需要这个词部分判断。
func registerVerbWordPart(text string, pos int, v string) bool {
	if v != "加" || pos == 0 {
		return false
	}
	runes := []rune(text[:pos])
	return runes[len(runes)-1] == '参'
}

// registerToolRequest 报告文本是否为"注册一项新能力"的语音指令（结构性判据）。
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

// registerGapThenNoun 报告 after 是否以「(量化虚词)* 能力名词」开头。
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
	// 紧邻场景：动词+虚词 后直接是能力名词（「加一个 工具」「注册一个 命令」）。
	for _, n := range registerCapabilityNouns {
		if strings.HasPrefix(rest, n) {
			return true
		}
	}
	// 描述场景（2026-10-04 能力自举「你必须增加一个 远程控制电脑 的能力」）：
	// 动词+虚词 后跟 能力描述，以能力名词收尾（…的能力/工具/功能/技能/插件）。
	// 排除查询/时态语境：「查一下已注册的工具」（mid="的"）、「已注册的能力」（mid="已注册的"）
	// —— mid 以虚词/时态词开头或含注册动词 → 不是"注册新能力"指令。
	for _, n := range registerCapabilityNouns {
		if strings.HasSuffix(rest, n) {
			mid := strings.TrimSuffix(rest, n)
			if mid == "" {
				continue // 名词本身：紧邻场景已覆盖
			}
			if strings.HasPrefix(mid, "的") || strings.HasPrefix(mid, "了") ||
				strings.HasPrefix(mid, "吧") || strings.HasPrefix(mid, "呢") ||
				strings.HasPrefix(mid, "已") || strings.HasPrefix(mid, "在") ||
				strings.HasPrefix(mid, "过") {
				return false
			}
			for _, v := range registerVerbs {
				if strings.Contains(mid, v) {
					return false // 描述含注册动词 → 查询/说明语境
				}
			}
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// 否定仲裁（缺口 G1）
//
// 问题：`不要删除那个文件` 被 deleteTriggers 抢先判成 EDIT(action=delete)，
// 否定词完全没被看见 —— 目标一旦可解析就会真的删。
// 依据：SPEC-v2:49「Ask != '' → 绝不执行」；
//
//	VS-HARNESS-001:314「该回问、该拒绝也算正确，单纯 JSON 合法不算」。
//
// 设计：否定词**直接支配动作**时一律转为 ASK 确认，绝不执行。
// ---------------------------------------------------------------------------

// negationMarkers 是多字否定词（长词在前，避免"不要再"被"不要"抢先）。
// 刻意**不含**"不能"：它是"能不能"的一部分，收了会把可行性问句判错。
// 评审 P1 补齐：勿/请勿/切勿/无需/不再/免了 —— 原先漏收，会导致
// 「请勿删除」「无需提交」「不再部署」仍被判成可执行动作。
var negationMarkers = []string{
	"不需要", "不要再", "请勿", "切勿", "无需", "不再",
	"不要", "不用", "先别", "别再", "免了",
}

// markerIsNegation 排除"形似否定、实非否定"的上下文（评审 P2）。
//
//	要不要删除那个文件   → "不要"只是"要不要"的一部分，不是否定
//	不要紧，帮我删除它   → "不要紧"= 没关系，整句是"帮我删除"
func markerIsNegation(text string, pos int, marker string) bool {
	if marker != "不要" {
		return true
	}
	if pos > 0 && strings.HasSuffix(text[:pos], "要") {
		return false // 要不要…
	}
	if strings.HasPrefix(text[pos+len("不要"):], "紧") {
		return false // 不要紧
	}
	return true
}

// bieFollowingVerbs 是单字"别"后面紧跟时才认定为否定的动作动词。
// 评审 P1 补：去/管/乱/忘（「别去删除…」「别管那个删除操作」）。
var bieFollowingVerbs = []rune("删发改动碰关停做执提交部署记看查修跑送去管乱忘")

// bieBlockPrefixes 是裸"别"前面出现时说明它属于词的一部分（不是否定）的字：
// 特/告/分/个/差/类/级/区/性/离/作/识/辨 —— 如"特别关注""识别"里的"别"。
var bieBlockPrefixes = []rune("特告分个差类级区性离作识辨")

// hasNegation 报告文本里是否有否定，并说明该否定是否**本身就绑定了动作**。
//
// 返回值：(命中的否定形式, 是否自带动作, 是否命中)
//   - 多字否定词（不要/不用/…）→ 自带动作=false，调用方需确认句中另有动作词，
//     以免把「不用担心」这类寒暄变成决策点；
//   - 裸"别"+动作动词（别删/别发/…）→ 自带动作=true，本身就是"否定+动作"的证据。
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
		// 单字否定前缀：别 / 勿。它们必须**紧邻动作动词**才算否定，
		// 否则会命中"特别/识别/勿忘"这类词的一部分（评审 P1/P7）。
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

// bieIsWordPart 报告 runes[i]（'别'）是否只是某个词的一部分（如"特别"）。
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

// containsAnyReturn 与 containsAny 同义，但返回命中的关键词（用于回问文案）。
func containsAnyReturn(text string, keywords []string) (string, bool) {
	lower := strings.ToLower(text)
	for _, k := range keywords {
		if strings.Contains(lower, strings.ToLower(k)) {
			return k, true
		}
	}
	return "", false
}

// actionWords 是"否定词要支配的动作"全集。只有当否定与动作同时出现时才回问，
// 避免把「不用担心」这类无动作的寒暄也变成决策点。
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
// 元指令仲裁（缺口 G3）
//
// 问题：「开始测试」「继续测试」被判 TEST 0.85 且 test_kind="go test ./..." —— 真的去跑测试。
// 但这两句在对话里说的是「进入测试阶段 / 继续推进」，是**对话控制**，不是执行命令。
//
// 歧义不猜：命中即回问消歧，既不误执行，也不假装听懂了。
// ---------------------------------------------------------------------------

// metaPrefixes 是对话控制类前缀。
var metaPrefixes = []string{"开始", "继续", "接着", "下一步", "接下来", "推进", "开工", "先这样", "暂停", "停一下"}

// metaInstruction 报告文本是否为"元指令 + 动作"的形态。
//
// 三个前提同时成立才判元指令：
//  1. 元指令出现在**句首附近**（前面不超过 2 个字符），否则它只是句子的一部分；
//  2. 其后确实跟着一个动作词——纯元指令（如「开始」）没有动作可误执行，
//     交回 UNKNOWN 处理即可，不必多问一句；
//  3. 元词之后**除了动作本身没有别的实质内容**（评审 P1：不用字数当代理判据）。
//     长句里的「开始/推进」是叙述（M7 真机教训，见
//     pipeline.TestCodexNineRegressions#8：长句元指令必须保持不 Ask）——
//     长句天然带有大量实质内容，会被本条自动排除。

// ---------------------------------------------------------------------------
// 条件句仲裁（缺口 G6）
//
// 问题：「如果测试通过就提交」被判 TEST 0.85 直接执行 —— **前提被完全忽略**。
// 系统不替用户守条件，也不该把"有前提的动作"当无条件命令做掉。
// ---------------------------------------------------------------------------

// conditionalMarkers 是前置条件标记。
// "就"是典型条件连接词但口语高频（就为什么/我就想让）——2026-10-04 用户实测误伤，
// 由 conditionalClause 内 conditionalJiPrefixOK 豁免（仅"完成态前提+就+动作"才算真条件）。
var conditionalMarkers = []string{"如果", "只要", "除非", "一旦", "要是", "假如", "就"}

// conditionalWindow 是条件标记之后允许出现动作词的字数窗口。
//
// 为什么必须限窗口：M7 真机长句
// 「我现在测试一下，看看效果怎么样，如果这个效果好，我们就继续推进…」
// 同时含"如果""就"和动作词，但它是一段口语陈述，**必须保持 Ask 为空**
// （既有回归 pipeline.TestColloquialQuestionNoReferAsk）。
// 条件句的特征是"后果紧跟条件"（如果测试通过**就提交**），而不是句子里恰好都有。
const conditionalWindow = 12

// conditionalClause 报告文本是否含"条件 + 紧跟其后的动作"，返回命中的标记。
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
				// 2026-10-04 用户实测：「就为什么/我就想让」口语误伤。仅当"就"前有完成态前提
				// （尾字 了/过/完/好 等）才是真条件（"测试过了就部署"）；口语"我就…"跳过。
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

// conditionalJiPrefixOK 判定"就"前的语段是否像完成态前提（"测试过了就部署"→"测试过了"）。
// 口语"我就想让…"（尾字"我"）、句首"就为什么…"→ 非条件，跳过，不误伤。
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

// metaMaxPrefixRunes 是元指令词之前允许的前置字数（"好的，""我们先""嗯，"）。
const metaMaxPrefixRunes = 4

// metaTrailingNoise 是元指令句尾的语气/征询成分，剥掉后不算"实质内容"。
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
		// 剥掉动作词与句尾语气成分；什么都不剩 = 用户没给对象 = 元指令。
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

// metaActionText 给出与该元指令词相称的文案（评审 P4：「暂停测试」不该被说成"是让我继续推进"）。
func metaActionText(meta string) string {
	switch meta {
	case "暂停", "停一下", "先这样":
		return "先停一下 / 收尾"
	default:
		return "继续推进"
	}
}

// ---------------------------------------------------------------------------
// 多动作检测（缺口 G5）
//
// 问题：「查一下库存，然后记一下结果，最后提交」只执行第一个命中的意图，
// 其余动作**既不执行也不提示**，无声消失 —— 用户以为三件事都做了。
//
// 判据刻意用**顺序连接词**而不是"句子里出现两个动作词"：
// M7 真机长句「我现在测试一下，看看效果怎么样，如果这个效果好，我们就继续推进…」
// 同时含 TEST 与 QUERY 词，却是一段口语独白，必须保持不 Ask
// （既有回归 pipeline.TestColloquialQuestionNoReferAsk）。它没有顺序连接词。
// ---------------------------------------------------------------------------

// sequenceConnectors 是中文口述里表示"下一步"的连接词。
var sequenceConnectors = []string{"然后", "接着", "之后", "随后", "最后", "再"}

// actionIntentGroups 是参与多动作计数的意图分组（ASK 不算动作，故不含 askTriggers）。
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

// clauseActions 返回单个分句命中的**全部**动作意图。
//
// 评审 G5-P1-4：原实现按词表顺序只取第一个命中（词表里 Query 在 Debug/Edit 之前，
// 且 queryTriggers 含单字"看/查/找"），会把「查一下这个报错」判成纯 QUERY、
// 把「改一下顺便查一下」判成纯 QUERY —— 既让回问文案说不准，也**低估动作数**。
func clauseActions(clause string) []string {
	var out []string
	for _, g := range actionIntentGroups {
		// REGISTER_TOOL 用结构性判据（动词支配能力名词），不再用固定短语词表 ——
		// 否则「注册一个命令，然后跑测试」会被算成 1 个动作而漏报多动作。
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

// multiActionClauses 按顺序连接词切分文本，返回：
//   - 含动作的**分句数**（评审 G5-P0-2：按分句数计，不按"不同意图数"计 ——
//     「先查 A 再查 B」是两个动作，若按不同意图去重会算成 1，第二个仍被静默丢弃）
//   - 这些分句里出现过的动作意图（去重，仅用于回问文案）
func multiActionClauses(text string) (int, []string, bool) {
	// 评审 G5-P2-6：先剥掉引号内容 —— 「把提示语改成「然后提交」」里的"然后"
	// 是**被引用的文本**，不是顺序连接词，切它会误报多动作。
	clean := stripQuotedSpans(text)
	hadConnector := false
	for _, c := range sequenceConnectors {
		if strings.Contains(clean, c) {
			hadConnector = true
			break
		}
	}

	parts := []string{clean}
	// 先按顺序连接词切，再按分句标点切（评审 G5-P2-5：ASR 常把连接词吞掉，
	// 「查一下库存，记一下结果，提交」只靠逗号分层）。
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

// multiActionTrips 是**决策**（与上面的测量分离）：
//   - 有顺序连接词 → 2 段带动作即算多动作；
//   - 纯标点分层 → 要求 ≥3 段。
//
// 后者是为了保住 M7 口语长句：它用逗号分层，但只有 2 段带动作，且是一段独白 ——
// 必须保持 Ask 为空（既有回归 pipeline.TestColloquialQuestionNoReferAsk）。
func multiActionTrips(n int, hadConnector bool) bool {
	if hadConnector {
		return n >= 2
	}
	return n >= 3
}

// clauseSeparators 是分句标点。
var clauseSeparators = []string{"，", "；", "。", ",", ";"}

// stripQuotedSpans 去掉被引号包裹的内容（引号本身也去掉），
// 避免把"被引用的文本"当成真实指令。
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

// joinIntentLabels 把动作意图列表拼成"查、记、提交"这样的中文串。
func joinIntentLabels(kinds []string) string {
	labels := make([]string, 0, len(kinds))
	for _, k := range kinds {
		labels = append(labels, intentLabel(k))
	}
	return strings.Join(labels, "、")
}

// intentLabel 给用户看的中文动作名。
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

// 复合/长任务（ORCHESTRATE）三连信号词表——组织者式路由的命中条件：
//
//	organizeWords ∩ docWords ∩ (saveWords ∪ commitTriggers) 同时成立。
//
// 设计取舍（方案 B）：动作计划（read→summarize→write→commit）由 pipeline 编排引擎确定性产出，
// 不在分类层做 function-calling；分类层只负责"这是一个多步编排任务"的识别 + 抽出目标文档名。
var (
	orchOrganizeWords = []string{"整理成", "整理", "汇总成", "汇总", "汇编"}
	orchDocWords      = []string{"沟通记录", "设计文档", "文档", "记录"}
	orchSaveWords     = []string{"保存提交", "保存", "生成", "落成", "写成"}
)


// 实现类长程任务（实现/搭建服务类）结构性识别词表 —— 修订卡2 层1+2（2026-10-03）。
//
// 范式对齐 REGISTER_TOOL 的双向回归法：结构 = 实现动词在前、能力名词在后（动词支配名词），
// 中间允许修饰成分（形容词/定语）；问句成分（怎么/如何/为什么…）出现即排除 ——
// 「这个系统是怎么实现的」「如何实现一个缓存」不得误判。
//
// 命中后：纯实现结构或带《…》文档引用 → ORCHESTRATE(kind=implement)，
// 长程实现任务由编排引擎去拆解，分类层只做识别（对齐修订卡2 层2）。
var (
	implementVerbs   = []string{"实现", "搭建", "开发", "构建", "重构", "编码", "写一个", "造一个", "做一个", "写一套", "落地一个", "建一个"} // F1 修复：真实长叙述「建一个 OT 运营数据平台…帮我规划」→ ORCHESTRATE
	// 修订类动词（洞 2，2026-10-04 无人工干预测试：修订/补齐/完善表述未命中 implement → 任务直接 done 0 轮无产物）。
	// 命中修订类动词时额外要求实现域强信号（implDomainWords），防"修订文档"误判。
	reviseVerbs      = []string{"修订", "补齐", "完善", "修正", "修复", "整改", "接着做", "继续做", "继续改", "按反馈", "根据反馈", "按验收", "根据验收"}
	implDomainWords  = []string{"服务", "后台", "系统", "程序", "代码", "端点", "bug", "编译", "跑通", "上线", "实现", "接口", "引擎", "网关"}
	implementNouns   = []string{"服务", "后台", "系统", "模块", "平台", "程序", "工具", "组件", "引擎", "网关", "中间件", "需求说明书"}
	questionExcludes = []string{"怎么", "如何", "为什么", "哪能", "能否", "怎么弄", "怎么做", "怎么样"}

	// —— 技能调用（线 C 修订卡 2026-10-03）——
	skillInvokeVerbs = []string{"调用", "使用", "用", "按", "按照", "依据", "根据", "让"}
	skillNames       = []string{"校验与对齐", "校准与对齐", "validate-align", "对齐", "校验", "校准",
		"plain-explainer", "解释", "检索", "写作", "翻译", "合规", "审阅", "报价", "文案"}
	skillActionWords = []string{"校准", "对齐", "校验", "检查", "审阅", "分析", "检索", "总结", "翻译", "编写", "生成", "输出"}
)

// detectSkillInvocation 判定文本是否为"调用/使用某技能做某事"（线 C 修订卡）。
// 命中返回 ORCHESTRATE + params{kind=skill, skill_name?, action?}。
// 触发：含 "技能" 且（调用动词 ∪ 技能名直接出现）→ 技能调用意图。
// 问句排除（"怎么用技能…"是询问不是调用）。
func detectSkillInvocation(text string) (string, map[string]string, bool) {
	if containsAny(text, questionExcludes) {
		return "", nil, false
	}
	if !containsAny(text, []string{"技能"}) {
		return "", nil, false
	}
	params := map[string]string{"kind": "skill"}
	invoked := false
	// ① 显式技能名出现（"校验与对齐技能"/"validate-align 技能"）→ 强信号
	for _, n := range skillNames {
		if strings.Contains(text, n) {
			if !strings.Contains(text, "怎么") && !strings.Contains(text, "如何") {
				params["skill_name"] = n
				invoked = true
				break
			}
		}
	}
	// ② 调用动词 + "技能" + 动作域词（"调用技能做校准/输出校准报告"）
	//   收紧（2026-10-03 评审）：纯"使用技能做X"（X 非动作域）不判——避免
	//   "实现一个使用技能的推荐系统"被技能检测器拦截（应走实现类）。
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

// detectImplementOrchestrate 判定文本是否为"实现/搭建某能力系统"的长程实现任务。
// 命中返回 ORCHESTRATE + params{kind=implement, target_doc?}。
func detectImplementOrchestrate(text string) (string, map[string]string, bool) {
	// 问句排除：含 怎么/如何/为什么… = 询问实现方式，不是实现任务。
	if containsAny(text, questionExcludes) {
		return "", nil, false
	}
	vi, ni := -1, -1
	// 通用实现动词（实现/搭建/开发…）照原逻辑；修订类动词（修订/补齐/完善…）
	// 必须同时命中实现域强信号（implDomainWords），否则不是实现任务（防"修订文档"误判）。
	for _, w := range implementVerbs {
		if idx := strings.Index(text, w); idx >= 0 && (vi < 0 || idx < vi) {
			// 2026-10-03 真跑发现：动词命中必须**不在书名号《…》内**——
			// 「整理成《全景开发文档》并保存提交」的"开发"是文档名成分，误当实现动词
			// ⇒ 整理类被误判 implement（L-01 验收真跑，见 /tmp/l01_confirm.txt）。
			if insideBookTitle(text, idx, idx+len(w)) {
				continue
			}
			// 2026-10-04 洞2：动词后紧跟"计划/方案/文档/报告/步骤"时是名词短语
			//（如"实现计划"），不是实现动作 → 跳过（"把实现计划修订一下…"）。
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
				continue // 修订类无实现域信号（如"修订这份文档"）→ 不是实现任务
			}
			vi = idx
			reviseHit = true
		}
	}
	// 名词必须在**动词之后**查找（text[vi:]）：
	// 「《个性化ASR后台需求说明书v2》实现一个…后台服务」里书名号中的"后台"是定语成分，
	// 若全句找最早名词会得到 ni<vi 的假阴性（2026-10-03 真跑发现，见 zz_debug 探针）。
	if vi >= 0 {
		for _, w := range implementNouns {
			if idx := strings.Index(text[vi:], w); idx >= 0 && (ni < 0 || idx < ni) {
				ni = idx + vi // 还原为绝对位置
			}
		}
		// 修订类动词的名词信号放宽到实现域词（端点/代码/接口/bug/编译/跑通…）：
		// 「把缺失的 /v1/blacklist 端点补齐」无 implementNouns 实体词，靠实现域词兜底。
		if reviseHit {
			for _, w := range implDomainWords {
				if idx := strings.Index(text[vi:], w); idx >= 0 && (ni < 0 || idx < ni) {
					ni = idx + vi
				}
			}
			if ni < 0 && containsAny(text, implDomainWords) {
				// 修订宾语常在动词前（"把缺失的端点补齐"）：全文实现域词命中即视名词信号成立。
				ni = vi + 1
			}
		}
		// 指代放宽（2026-10-03 上下文槽）：动词命中且**全文**含指代词（这份/该文档）
		// 也视为名词信号——「按这份需求说明书继续实现并提交」无实体名词，靠槽解析目标。
		// 查全文而非 text[vi:]：指代词常居动词前（"按这份需求说明书**继续实现**"）。
		if ni < 0 {
			for _, d := range []string{"这份", "该文档", "此文档", "这个", "这些", "它", "那"} {
				if strings.Contains(text, d) {
					ni = vi + 1
					break
				}
			}
		}
	}
	// 动词支配名词：动词必须存在且动词之后有名词。
	if vi < 0 || ni < 0 || ni <= vi {
		return "", nil, false
	}
	params := map[string]string{"kind": "implement"}
	if title := extractBookTitle(text); title != "" {
		params["target_doc"] = title
	}
	return contract.IntentOrchestrate, params, true
}


// detectOrchestrate 判定文本是否为"整理多份文档→生成文件→保存/提交"的复合长任务。
// 命中时返回 (IntentOrchestrate, params{target_doc, source_hint}, true)。
// target_doc 从《…》书名号里抽；抽不到则由编排引擎落默认文件名。
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

// extractBookTitle 抽取《…》书名号内的文档名（去书名号，保留扩展名点）。
func extractBookTitle(text string) string {
	i := strings.Index(text, "《")
	j := strings.Index(text, "》")
	if i >= 0 && j > i {
		return strings.TrimSpace(text[i+len("《") : j])
	}
	return ""
}

// nowFn 是时间源（timeanchor.go 依赖，测试可替换）。
var nowFn = func() time.Time { return time.Now() }

// ClassifyTask 执行伪代码逻辑层的分支判定，产出 M2 任务意图 JSON。
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

	// 0. 否定仲裁（缺口 G1，必须排在删除仲裁**之前**）。
	//    `不要删除那个文件` 若先撞上 deleteTriggers，就会被判成可执行的 EDIT(action=delete)；
	//    否定词直接支配动作时一律转 ASK 确认 —— SPEC-v2:49「Ask != '' → 绝不执行」。
	if neg, actionBound, ok := hasNegation(text); ok && (actionBound ||
		containsAny(text, actionWords) || registerToolRequest(text)) {
		got := c.fill(ti, contract.IntentAsk, 0.9, nil)
		got.Conflict = contract.ConflictNegation
		got.Ask = "我听到的是「" + neg + "」——确认不执行这个动作吗？" +
			"确认不做请说「取消」；确实要做请重新说一遍完整指令"
		return got
	}

	// 0.5 元指令仲裁（缺口 G3）：「开始测试」是推进对话，不是"跑 go test ./..."。
	//     必须有动作词才算（纯「开始」不过这里），命中即回问消歧。
	if meta, ok := metaInstruction(text); ok {
		got := c.fill(ti, contract.IntentAsk, 0.9, nil)
		got.Conflict = contract.ConflictMeta
		got.Ask = "「" + meta + "」是让我" + metaActionText(meta) + "，还是要我现在就执行后面的动作？" +
			"要执行请直接说完整指令（例如「跑一下测试」）"
		return got
	}

	// 0.6 条件句仲裁（缺口 G6）：「如果测试通过就提交」的前提不能被忽略。
	//     系统不替用户守条件 —— 明确告知，请用户先完成前提再下指令。
	if cond, ok := conditionalClause(text); ok {
		got := c.fill(ti, contract.IntentAsk, 0.9, nil)
		got.Conflict = contract.ConflictConditional
		got.Ask = "「" + cond + "」是带前提的动作。我不会替你守着条件——" +
			"请先完成前提（例如先把测试跑完），再直接说指令"
		return got
	}

	// 1. 冲突仲裁（优先级最高）
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

	// 2a-pre. 多动作检测（缺口 G5）：「查一下库存，然后记一下结果，最后提交」
	// 若只执行第一个命中的意图，其余动作会**无声消失**，用户以为都做了。
	//
	// 必须排在 2a（extractReplace 提前返回）**之前** —— 否则
	// 「把报价模板改成新的公司抬头，然后跑一下测试」会先命中 EDIT 直接返回。
	//
	// 一次只做一件是产品的既有约束（一屏一决策点），所以这里**不猜顺序**，
	// 直接把几件事摊开让用户选先做哪个。
	// 评审 G5-P1-3：编排任务本身多动作、由计划承载，**必须豁免**。
	// 原实现靠"恰好没有顺序连接词"才没被抢走 —— 保护是偶然的，不是结构性的：
	// 「…整理成《全景开发文档》，然后再保存提交」会被截成 NOTE+COMMIT 而永远不走编排。
	// 评审 G5-P2-6 延伸：**自我修正链不是多动作**。
	// 「把标题改成中文，不对，改成英文，说错了，改成阿拉伯语」按逗号会切出 3 段 EDIT，
	// 但它是同一个意图被反复修正，应交给 G7 的修正逻辑处理。
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
			// 2026-10-04 用户原话"不管多少件，你先理解我的意思"：不再机械回问"先做哪个"，
			// 归 ASK 交模型人话理解并回应（执行层面仍一屏一决策点，但对话不卡用户）。
			got.Ask = "你说了几件事（" + joinIntentLabels(labels) + "）。" +
				"我先整体理解一下，你接着补一句最要紧的，我一件一件来"
			return got
		}
	}

	// 2a. 显式「把 X 改成/换成 Y」→ EDIT（优先于 COMMIT/DEPLOY 等触发词，如「把提交按钮改成中文」）
	if _, _, ok := extractReplace(text); ok {
		return c.fill(ti, contract.IntentEdit, 0.9, c.editParams(text))
	}

	// 2a-bis. 复合/长任务（组织者式路由）：整理/汇总 + 文档/记录 +（保存/提交）三连信号
	// → ORCHESTRATE，不再压成单条 NOTE/COMMIT。
	// 必须先于 2b 单类触发：「沟通记录」含「记录」会命中 noteTriggers，「提交」会命中 commitTriggers——
	// 长任务「把全部沟通记录和设计文档整理成《…》并保存提交」此前被降级为单条 NOTE 整段 append。
	if kind, params, ok := detectOrchestrate(text); ok {
		return c.fill(ti, kind, 0.9, params)
	}

	// 2a-ter. 实现类长任务（ORCHESTRATE kind=implement）：实现/搭建/修订某能力系统。
	// 洞2 修复（2026-10-04）：分发必须显式调用 detectImplementOrchestrate——
	// 否则「把需求文档实现出来…并提交到仓库」被 2b 的 commitTriggers 截成单动作 COMMIT，
	// 实现类长任务永远不进入编排引擎（此前只加了函数未接入分发，证据门永不触发）。
	if kind, params, ok := detectImplementOrchestrate(text); ok {
		params["kind"] = "implement"
		return c.fill(ti, kind, 0.9, params)
	}

	// 【伪代码逻辑层】（M5-1 触发词碰撞仲裁：NOTE 语境词 vs TEST 触发词）
	//
	// 控制流（顺序即优先级，长词在前；本 switch 自上而下首个命中者胜出）：
	//
	//	if containsAny(text, noteTriggers):  return NOTE   // 记下/记一下/记个/记录…
	//	elif containsAny(text, queryTriggers): return QUERY
	//	elif containsAny(text, debugTriggers): return DEBUG
	//	elif containsAny(text, testTriggers): return TEST  // 跑测试/跑一下/测试…
	//	…
	//
	// 优先级规则（M5-1 定稿，修复「在笔记里记下 M4 测试」被「测试」误判 TEST）：
	//   - NOTE 语境词（记下/记一下/记个/记录，含「笔记/想法」语境）**先于** TEST 词判定；
	//     故「记下 X 测试」「记一下 X 测试」「记个想法测试下」一律 NOTE——句尾「测试」是
	//     被记录的对象，不是要执行的动作。
	//   - **例外（必须不回归）**：句子不含任何 NOTE 词、仅独立 TEST 触发
	//     （跑一下测试/执行测试/测试这个函数）→ 仍判 TEST。
	//   - 意图判断规则本体（什么算 NOTE/TEST 语义）搬 VSL，此处只写控制流与仲裁顺序。
	//
	// 验证器：input.TestTriggerCollisionNoteVsTest（正反例 + QUERY 不回归）。
	//
	// 2b. 单类触发（长词在前，SPEC v1 §2 表）
	switch {
	case containsAny(text, writeTriggers):
		// 2026-10-04 验证器 R6/R7：写文件指令（把 XX 写到/写入/保存到 /path）
		// 先于 NOTE/TEST 判定——"把 自动验证器测试文件 写到 /tmp"不再被"测试"劫持。
		return c.fill(ti, contract.IntentEdit, 0.85, nil)
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
	case containsAny(text, editTriggers): // 其余 EDIT 形态（改一下/修改/替换/改）
		return c.fill(ti, contract.IntentEdit, 0.85, c.editParams(text))
	}

	// 0.8 元反馈/闲聊仲裁（2026-10-04 用户多轮截图点名"你是想让我做什么"回问）：
	// 抱怨/试探/元反馈话术（"识别太糟糕""后台有没有干活""懂我吗"）不落入 UNKNOWN 回问——
	// 归 ASK 交模型直接人话回应。ASK 类永不触发回问（伪代码逻辑层 11：低置信非 NOTE/ASK 才回问）。
	// 注意：排在单类触发之后——「测试一下」先命中 TEST，不会被这里抢走。
	if containsAny(text, chatTriggers) {
		return c.fill(ti, contract.IntentAsk, 0.8, nil)
	}

	// 3. 无任何触发词 → UNKNOWN，回问
	return ti
}

// fill 按意图模板补齐 空间候选/时间锚点/边界/风险基线/确认基线/验收/置信度，并做低置信回问。
func (c *TaskClassifier) fill(ti contract.Intent, kind string, conf float64, params map[string]string) contract.Intent {
	ti.Intent = kind
	ti.Confidence = clamp(conf)
	ti.Ask = "" // 非 UNKNOWN 一律清空初始回问；applyCommon 仅对低置信重设
	if params != nil {
		ti.Params = params
	}
	c.applyCommon(&ti, kind, conf)

	// 缺口 G2：零信息句子即便高置信也必须回问。
	// 原行为：单类触发恒 0.85，恒高于默认阈值 0.6，于是 applyCommon 里那条
	// "低置信回问"在默认配置下几乎不可达 —— 「改一下」「修」「查」直接进执行通道。
	if ti.Ask == "" && slotGateTrips(kind, ti.CorrectedText) {
		ti.Ask = askForKind(kind)
	}
	return ti
}

// slotGateFillers 是剥除时一并去掉的虚词/填充词。
var slotGateFillers = []string{
	"一下", "一遍", "这个", "那个", "它", "帮我", "麻烦", "请", "吧", "呢", "啊", "呀",
	"了", "的", "，", "。", "、", "！", "？", ",", ".", "!", "?", " ",
}

// slotGateTrips 报告该意图的句子除了触发词本身之外是否几乎不剩信息。
//
// 只对 EDIT / DEBUG / QUERY 生效：这三类的"对象"是必需槽位，缺了就无从执行。
// TEST/COMMIT/DEPLOY/NOTE/ASK 有默认值或本身就无需对象，不在本闸范围内。
//
// 实现要点：**不能**把触发词从文本里剥掉再量长度 —— 触发词常常就是信息本身。
// 例如 DEBUG 的触发词表里有 "bug"，「修一下这个 bug」剥完只剩空，会被误判为零信息。
// 因此改为：剥掉虚词后，看**残留长度是否不超过句中命中的最长触发词**。
//
//	"改一下"            → 残留"改"(1) ≤ 最长触发词"改一下"(3) → 回问
//	"修一下这个 bug"     → 残留"修bug"(4) > 最长触发词"bug"(3)  → 不回问
//	"查"                → 残留"查"(1) ≤ "查"(1)              → 回问
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

// slotGateTriggers 返回该意图参与槽位闸的触发词表；不在闸内返回 nil。
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

// applyCommon 填充 空间候选/目标/时间锚点/边界基线/风险基线/确认基线/验收模板/低置信回问。
func (c *TaskClassifier) applyCommon(ti *contract.Intent, kind string, conf float64) {
	text := ti.CorrectedText

	// 4. 空间候选（注册空间名/别名最长匹配，平局不猜；未注册实体只进 Target，不占 Space）
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

	// 时间锚点（全部意图均可带）
	if hint, date, _ := ResolveTimeAnchor(text, nowFn()); hint != "" {
		if ti.Params == nil {
			ti.Params = map[string]string{}
		}
		ti.Params["time_hint"] = hint
		ti.Params["time_date"] = date
	}

	// 7. 风险基线 + 8. 确认基线（非权威；risk 包裁决后回填权威值）
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
	default: // NOTE/QUERY/TEST/ASK 只读或低影响
		ti.Risk = &contract.RiskBaseline{Reversible: true, Impact: contract.ImpactSmall}
		ti.Confirm = contract.ConfirmAuto
	}

	// 9. 验收模板
	ti.Acceptance = acceptanceTemplate(kind, ti.Space, ti.Boundary)

	// 11. 低置信回问（NOTE/ASK/REGISTER_TOOL 无必需槽位，不因槽位缺失回问）
	if kind != contract.IntentUnknown && kind != contract.IntentNote &&
		kind != contract.IntentAsk && kind != contract.IntentRegisterTool &&
		conf < c.ConfThreshold {
		ti.Ask = askForKind(kind)
	}
}

// spaceShadowsThought 报告文本命中的注册空间名/别名是否含「想法/备忘」类词。
// 20 样例方向验证 surfaced：空间名「想法库」含子串「想法」，被 thoughtWords 仲裁抢先判 NOTE——
// 空间名是实体名不是"记想法"触发，命中即遮蔽想法仲裁（事后逻辑审查查漏，SPEC v2 §1#10 已钉死）。
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

// matchSpace 在文本中做注册空间名/别名最长匹配；平局（同长度多命中）→ 不猜（ok=false）。
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
				bestLen = -1 // 平局 → 标记不猜
			}
		}
	}
	if bestLen <= 0 {
		return "", false
	}
	return best, true
}

// editParams 抽取 EDIT 槽位：「把 X 改成 Y」→ action/object/value；否则 action=replace + object=路径。
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

// queryParams 抽取 QUERY 槽位：object 与 上次 语义。
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

// extractReplace 抽取「把 X 改成 Y / 把 X 换成 Y」三槽位：
//   - before 段先取「把」之后的部分（object），再剥离「在 X 里/中」前置子句（其承载空间信息）。
func extractReplace(text string) (object, value string, ok bool) {
	// 缺口 G7：口述自我修正。用户在"不对/说错了"之前的表述作废，只有其后才是真实意图。
	//
	// 原行为：`记一下A，不对，改成记B` 会把"记一下A，不对"整段当成 object。
	//
	// 修正后常省略对象（`把标题改成中文，不对，改成英文`），故承接修正前的对象。
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

// correctionMarkers 是自我修正信号（缺口 G7）。取其**最后一次**出现，支持连续修正。
var correctionMarkers = []string{"不对", "说错了", "打错了", "重新说", "我是说", "应该是"}

// lastCorrection 返回最后一个自我修正标记的起始与结束**字节**下标；无则 (-1, -1)。
func lastCorrection(s string) (int, int) {
	bestStart, bestEnd := -1, -1
	for _, m := range correctionMarkers {
		if i := strings.LastIndex(s, m); i >= 0 && i > bestStart {
			bestStart, bestEnd = i, i+len(m)
		}
	}
	return bestStart, bestEnd
}

// extractReplaceRaw 是未做自我修正处理的原始实现。
//
// 与旧版唯一的语义差别：**对象为空不再直接判失败**（只要求 value 非空），
// 以便 extractReplace 在"改口后省略对象"时承接修正前的对象。
// 是否最终成立由 extractReplace 决定。
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

// stripInClause 剥离「在 X 里/中/里面」子句，返回剩余部分与是否剥离。
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

// extractInClause 抽取「在 X 里/中/里面」的项目/路径实体。
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

// acceptanceTemplate 按意图返回验收模板（verify 契约消费的自然语言判据）。
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

// askForKind 按意图定制回问文案（指出缺什么，不许静默）。
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

// insideBookTitle 判断 [start,end) 区间是否完全落在书名号《…》内。
// 用途：动词/名词匹配排除文档名成分（如《全景开发文档》里的"开发"不是实现动词）。
func insideBookTitle(text string, start, end int) bool {
	i := strings.Index(text, "《")
	j := strings.Index(text, "》")
	if i < 0 || j <= i {
		return false
	}
	return start >= i && end <= j
}
