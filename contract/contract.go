// Package contract 定义 VoxSign harness 全部模块共享的固定数据契约：
// 聊天消息、模型 JSON 动作计划（ActionPlan）、执行回执（Receipt）、用量（Usage）。
// 该包只依赖标准库，且不包含任何运行逻辑，保证跨模块字节级稳定。
package contract

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Role 标识聊天消息的发送方。
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool" // 保留：V0 不使用 OpenAI function-calling 协议
)

// Message 是 OpenAI 风格的一条聊天消息。
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

// Usage 镜像 OpenAI usage 块。
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ToolSchema 描述固定 tool contract 中的一个工具。
// 全部 schema 在启动时一次性渲染进系统提示词，此后逐轮请求前缀字节级一致，
// 使支持 prompt cache 的端点（DeepSeek / 模型中心）命中缓存，降延迟降成本。
type ToolSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"` // JSON Schema 对象
}

// ActionPlan 是模型的 JSON 响应信封（借鉴 dsh Code 模式）：
// 模型一次返回多步动作序列，harness 在一次会话内按序执行并聚合回执，
// 从而把 LLM↔harness 的多轮往返压缩到最少——语音场景延迟敏感，这是核心性能点。
type ActionPlan struct {
	Actions []Action `json:"actions"`
	Final   string   `json:"final"`
}

// Action 是模型请求 harness 执行的一步。
type Action struct {
	Seq       int            `json:"seq"`
	Tool      string         `json:"tool"`
	Args      map[string]any `json:"args"`
	TimeoutMs int            `json:"timeout_ms"`
	Confirm   bool           `json:"confirm"` // 显式声明高危操作需放行
}

// Receipt 是执行一个动作的结果。
type Receipt struct {
	Seq        int    `json:"seq"`
	Tool       string `json:"tool"`
	OK         bool   `json:"ok"`
	Stdout     string `json:"stdout,omitempty"`
	Stderr     string `json:"stderr,omitempty"`
	Err        string `json:"err,omitempty"`
	Blocked    string `json:"blocked,omitempty"` // 安全拦截原因
	DurationMs int64  `json:"duration_ms"`
}

// ParseActionPlan 解析模型原始 JSON 输出（容忍 ```json 代码围栏），
// 校验信封并归一化动作序号（缺省按 1..n 补齐）。解析失败时错误信息保留原始片段，便于轨迹回放。
func ParseActionPlan(raw string) (ActionPlan, error) {
	s := strings.TrimSpace(raw)
	// 剥掉 ```json / ```go / ``` 围栏
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
		return plan, fmt.Errorf("解析动作计划失败: %w; raw=%q", err, truncate(raw, 500))
	}
	if plan.Final == "" && len(plan.Actions) == 0 {
		return plan, fmt.Errorf("动作计划为空: 既没有 actions 也没有 final; raw=%q", truncate(raw, 500))
	}
	for i := range plan.Actions {
		a := &plan.Actions[i]
		if a.Tool == "" {
			return plan, fmt.Errorf("动作 #%d 缺少 tool 字段; raw=%q", i, truncate(raw, 500))
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

// 意图类别常量（input 产出、router/agent/trajectory 消费，全模块共享）。
// M1 类别（旧管线）：TIME/FILE_*/SHELL/APP_LAUNCH/INFO/UNKNOWN。
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

// M2 任务意图类别（SPEC v2 §意图 Schema 八类 + REGISTER_TOOL 自举意图）。
// 与 M1 类别并存：旧管线用 M1 类别，M2 语音驱动开发管线用任务类别。
const (
	IntentNote         = "NOTE"          // 记想法：记一下/存/记
	IntentQuery        = "QUERY"         // 查代码/状态：查/看/找/上次
	IntentEdit         = "EDIT"          // 改代码：改/换成/把…改成
	IntentDebug        = "DEBUG"         // 修 bug：修/报错/为什么失败
	IntentTest         = "TEST"          // 跑测试：跑测试/测一下
	IntentCommit       = "COMMIT"        // 提交：提交/推
	IntentDeploy       = "DEPLOY"        // 部署/外发/生成报表：发/上线/部署
	IntentAsk          = "ASK"           // 问问题：为什么/怎么办/你觉得
	IntentRegisterTool = "REGISTER_TOOL" // 语音注册新工具契约（自举充分条件，8 类之外单独定义）
	IntentOrchestrate  = "ORCHESTRATE"   // 多步编排（组织者式路由）：读多份文档→汇总→生成文件→提交
)

// 确认策略等级（Intent.Confirm，risk 包裁决后回填权威值）。
const (
	ConfirmAuto   = "auto"   // 自动执行 + 标待抽查
	ConfirmLight  = "light"  // 展示 diff 摘要 → 轻确认
	ConfirmStrong = "strong" // diff 预览 + 影响分析 → 强确认
	ConfirmHuman  = "human"  // 不可逆动作，永远人工确认（不可被学习掉）
)

// 风险影响面等级（RiskBaseline.Impact，机械信号参与，不靠模型自评）。
const (
	ImpactSmall  = "small"
	ImpactMedium = "medium"
	ImpactHigh   = "high"
)

// Correction 记录一次纠错（本地词典或模型二次纠错）。
type Correction struct {
	From string `json:"from"`
	To   string `json:"to"`
	Rule string `json:"rule"` // dict | builtin | model
}

// M2 意图 Schema 扩展类型（SPEC v2 §意图 Schema）——与 M1 的 Intent 同构共存。

// Target 是任务意图指向的实体（指代消解产出）。
type Target struct {
	Entity  string `json:"entity,omitempty"`   // 消解后的实体（人名/项目名/文件名/路径）
	RefType string `json:"ref_type,omitempty"` // explicit | anaphora | inferred | space_scope
}

// Boundary 是边界五轴中的范围/排除（画边界理念的实施）。
type Boundary struct {
	Scope   []string `json:"scope,omitempty"`   // 覆盖范围 glob（含相对/绝对路径）
	Exclude []string `json:"exclude,omitempty"` // 显式不碰（.env*、密钥、外部发布）
}

// RiskBaseline 是意图级风险基线（risk 包做权威三信号分级，此处仅基线，Confidence 0 时用 Intent.Confidence）。
type RiskBaseline struct {
	Reversible bool    `json:"reversible"`
	Impact     string  `json:"impact,omitempty"` // small | medium | high
	Confidence float64 `json:"confidence,omitempty"`
}

// 意图冲突仲裁标记（Intent.Conflict，input 分类器产出，回显给用户）。
const (
	ConflictNone         = ""               // 无冲突
	ConflictAskVsOp      = "ask_vs_op"      // 可行性问句被仲裁为 ASK
	ConflictNoteVsDeploy = "note_vs_deploy" // 「发个想法」被仲裁为 NOTE
	ConflictDebugPlan    = "debug_plan"     // 「修 bug 的思路」低置信 → 回问
	ConflictDelete       = "delete"         // 删除动词 → EDIT(action=delete)
	// ConflictNegation：否定词直接支配动作 → ASK 确认，绝不执行（缺口 G1）。
	// 依据 SPEC-v2:49「Ask != '' → 绝不执行」与 VS-HARNESS-001:314「该回问、该拒绝也算正确」。
	ConflictNegation = "negation"
	// ConflictMeta：元指令（开始/继续/推进）被误当可执行命令 → ASK 消歧（缺口 G3）。
	// 「开始测试」是推进对话，不是"跑 go test ./..."。
	ConflictMeta = "meta"
	// ConflictConditional：条件句（如果…就…）被无条件执行 → ASK（缺口 G6）。
	// 系统不替用户守条件，也不得把"有前提的动作"当无条件命令做掉。
	ConflictConditional = "conditional"
)

// Intent 是输入容错层产出的结构化意图（架构文档 §5.5）：
// 意图类别 + 槽位参数 + 置信度 + 纠错后文本；Ask 非空表示需要回问、不得执行。
// M2 扩展字段全部 omitempty：旧管线（M1 类别）消费者与既有 JSON 轨迹不受影响。
type Intent struct {
	Intent        string            `json:"intent"`
	Slots         map[string]string `json:"slots"`
	Confidence    float64           `json:"confidence"`
	CorrectedText string            `json:"corrected_text"`
	Corrections   []Correction      `json:"corrections"`
	Ask           string            `json:"ask"`

	// M2 扩展（语音驱动开发任务意图；omitempty 保持 M1 字节级兼容）。
	RawText    string            `json:"raw_text,omitempty"`   // ASR 原文（与 input.Result.Raw 同值，便于单 JSON 重放）
	Space      string            `json:"space,omitempty"`      // 空间候选（注册名；空 = 待 refer/space_check 兜底，不猜）
	Target     *Target           `json:"target,omitempty"`     // 指代消解结果
	Params     map[string]string `json:"params,omitempty"`     // 动作参数（action/object/value/path/test_kind/time_hint…）
	Boundary   *Boundary         `json:"boundary,omitempty"`   // 范围/排除（边界五轴）
	Risk       *RiskBaseline     `json:"risk,omitempty"`       // 风险基线（非权威）
	Confirm    string            `json:"confirm,omitempty"`    // auto|light|strong|human（基线，risk 包裁决）
	Acceptance string            `json:"acceptance,omitempty"` // 验收条件（模板，verify 契约消费）
	Context    []string          `json:"context,omitempty"`    // 认知切片引用（project-map:xxx / decisions:xxx）
	Conflict   string            `json:"conflict,omitempty"`   // 意图冲突仲裁标记（回显用）
}

// NeedsClarification 报告该意图是否需要回问。
func (i *Intent) NeedsClarification() bool { return i.Ask != "" }

// ToolContract 描述一个工具契约（.contract，六工具 + REGISTER_TOOL 自举）。
// caps 为逐动作白名单；risk 为逐动作风险基线（不可逆动作标 irreversible）。
type ToolContract struct {
	Name          string            `json:"name"`
	Version       string            `json:"version"`
	Caps          []string          `json:"caps"`
	Params        map[string]string `json:"params"` // 参数名 → 类型描述（"string,required"）
	SideEffects   []string          `json:"side_effects"`
	AllowedSpaces []string          `json:"allowed_spaces"`   // project | sandbox | vault-notes | vault-creds | external | global
	Risk          map[string]string `json:"risk"`             // cap → low | medium | high | irreversible
	Source        string            `json:"source,omitempty"` // builtin | voice（REGISTER_TOOL 注册）
	RegisteredAt  string            `json:"registered_at,omitempty"`
}

// 归因六格类别（记录 discuss 阶段的错误分类，回写下一轮上下文注入）。
const (
	AttrInput    = "input"     // 输入/ASR 错误
	AttrContext  = "context"   // 上下文/认知切片不足
	AttrContract = "contract"  // 契约定义缺陷
	AttrModel    = "model"     // 模型判断错误
	AttrExec     = "execution" // 执行/环境错误
	AttrExternal = "external"  // 外部依赖（模型中心/网络/第三方）
)

// Attribution 是一次归因记录（六格分类 + 证据 + 建议，写轨迹与 discuss-log）。
type Attribution struct {
	RequestID  string `json:"request_id"`
	Stage      string `json:"stage"`                // record | discuss | control
	Class      string `json:"class"`                // 归因六格之一
	Detail     string `json:"detail"`               // 发生了什么
	Evidence   string `json:"evidence"`             // 证据（轨迹条目 kind / 文件 / 输出片段）
	Suggestion string `json:"suggestion,omitempty"` // 下次怎么避免
	Ts         string `json:"ts"`
}

// ReceiptView 是一屏四行回执的呈现数据（动作/文件/结果/撤销，SPEC 验收用例 11）。
type ReceiptView struct {
	Action string `json:"action"` // 做了什么（含意图与工具）
	Files  string `json:"files"`  // 涉及文件（无文件 → "—"；多文件 → "N 个文件"）
	Result string `json:"result"` // 结果（OK/FAILED/BOUNDARY_VIOLATION/待确认 + 关键行摘要）
	Undo   string `json:"undo"`   // 撤销方式（git 还原 / 备份路径 / 不可撤销）
}

// RenderReceipt 渲染四行回执文本（手机一屏内，SPEC 验收用例 11 格式）。
func RenderReceipt(r ReceiptView) string {
	lines := []string{
		"动作：" + r.Action,
		"文件：" + r.Files,
		"结果：" + r.Result,
		"撤销：" + r.Undo,
	}
	return strings.Join(lines, "\n")
}
