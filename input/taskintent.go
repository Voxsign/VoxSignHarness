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
	registerTriggers = []string{"加一个工具", "加个工具", "注册工具", "新增工具", "加个新工具", "加一个新工具"}
	deleteTriggers   = []string{"删掉", "删除", "去掉", "移除", "清空"}
	feasibleAsk      = []string{"能不能", "可不可以", "是否可以", "行不行"}
	thoughtWords     = []string{"想法", "备忘"}
	statusQuestion   = []string{"好了吗", "弄好了吗", "搞定了吗", "改好了吗", "改没改", "改了没", "改了吗", "弄了吗"}
	debugPlanWords   = []string{"思路", "怎么做", "方案", "打算"}
	noteTriggers     = []string{"记一下", "记下来", "记下", "记个", "记住", "记录一下", "记录", "存档", "存个", "存到"}
	queryTriggers    = []string{"查一下", "查", "找一下", "找", "上次", "搜一下", "搜", "看看", "看"}
	editTriggers     = []string{"改成", "换成", "改一下", "修改", "替换", "改"}
	debugTriggers    = []string{"报错", "为什么失败", "崩溃", "闪退", "出错", "bug", "修一下", "修这个", "修那个", "修一修", "修"}
	testTriggers     = []string{"跑测试", "跑一下", "测一下", "跑个测试", "测试"}
	commitTriggers   = []string{"提交", "推上去", "推到"}
	deployTriggers   = []string{"部署", "上线", "生成报表", "发到", "发布"}
	askTriggers      = []string{"为什么", "怎么办", "你觉得", "是什么意思", "怎么弄", "如何"}
	defaultExcludes  = []string{".env*", "node_modules"}
)

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

	// 1. 冲突仲裁（优先级最高）
	switch {
	case containsAny(text, registerTriggers):
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

	// 2a. 显式「把 X 改成/换成 Y」→ EDIT（优先于 COMMIT/DEPLOY 等触发词，如「把提交按钮改成中文」）
	if _, _, ok := extractReplace(text); ok {
		return c.fill(ti, contract.IntentEdit, 0.9, c.editParams(text))
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
	return ti
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
	case contract.IntentCommit, contract.IntentDeploy:
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
			if object == "" || value == "" {
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
