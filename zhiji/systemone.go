// systemone.go —— 知己 · System One 控制器抽象（架构 v1.1 §6.3 / G4 落地）。
//
// 把 reflect.go 里硬编码的门控/分类/路由/停止/写前验证抽成接口；
// DefaultSystemOne 用与现有常量同值的默认实现，Tick/Verify 行为逐位不变（32/32 断言全绿前提）。
// P2 换 Qwen3-4B+LoRA：所有模型输出走 AskNoul/AskChoice 有界校验，失败回退本默认实现。
//
// 关键耦合：Interval/MaxIdle/Threshold 只存一份（在 DefaultSystemOne 里），
// Reflector 通过匿名嵌入 *DefaultSystemOne 提升访问（r.Interval == r.DefaultSystemOne.Interval），
// 测试改 r.Threshold / r.MaxIdle / SetInterval 时 r.Gate 读到的是同一份值，绝不漂移。
package zhiji

import (
	"math"
	"strings"
	"time"
)

// SystemOne 慢思考的快速控制器（System One）。
type SystemOne interface {
	// Gate 输入门控三段：返回(skip, idleForced, deep)
	Gate(idle time.Duration, importance float64) (skip, idleForced, deep bool)
	// Classify 写入侧四类重叠打分（episodic/semantic/procedural/preference）
	Classify(item MemoryItem) map[MemKind]float64
	// Route 检索路由：四视图激活概率 + 多跳深度
	Route(query string) (views map[EdgeRel]float64, hops int)
	// Assess 证据评估：返回(enough, lowValue) 停止决策
	Assess(evidence []MemoryItem) (enough, lowValue bool)
	// VerifyMemory/VerifySelf 写前验证（接口化现有 reflect.go 启发式）
	VerifyMemory(m MemoryItem) bool
	VerifySelf(s SelfItem) bool
}

// DefaultSystemOne 规则版默认控制器。
// Interval/MaxIdle/Threshold/DeepMinImp 与 reflect.go 现有常量同值——
// Reflector 内嵌本结构后，Tick 行为与今天逐位一致。
type DefaultSystemOne struct {
	Threshold  float64       // 深反思累计重要性阈值（=DefaultThreshold 30）
	MaxIdle    time.Duration // 最大空转保底（=DefaultMaxIdle 3600s）
	Interval   time.Duration // 唤醒节律（=DefaultInterval 60s；可调 60–600s）
	DeepMinImp float64       // 深反思提炼下限（=DeepReflectMinImp 6.0）
}

// Gate 三段语义（与 reflect.go Tick 现有 switch 逐位对齐）：
//
//	idle > MaxIdle   → idleForced（保底浅扫，不深反思）
//	idle >= Interval → skip（超过一个节律无输入）
//	否则             → shallow + importance>=Threshold → deep
func (d *DefaultSystemOne) Gate(idle time.Duration, importance float64) (skip, idleForced, deep bool) {
	if idle > d.MaxIdle {
		return false, true, false
	}
	if idle >= d.Interval {
		return true, false, false
	}
	return false, false, importance >= d.Threshold
}

// 时间状语词表（含其一 → episodic / temporal 视图命中）。
var timeWords = []string{"昨天", "今天", "上周", "刚才", "刚"}

// Classify 规则版四类打分（P0 冻结规则，P2 换 Noul 模型）：
//
//	含"我喜欢/我讨厌/偏好"           → preference=0.8
//	含"怎么/如何/步骤/先...再/第一步" → procedural=0.8
//	含时间词（昨天/今天/上周/刚才/刚） → episodic=0.7
//	否则                             → semantic=0.6
//	其余类 0.1 兜底。
func (d *DefaultSystemOne) Classify(item MemoryItem) map[MemKind]float64 {
	text := item.Text
	out := map[MemKind]float64{
		MemKindEpisodic:   0.1,
		MemKindSemantic:   0.1,
		MemKindProcedural:  0.1,
		MemKindPreference: 0.1,
	}
	fired := false
	if hasAny(text, []string{"我喜欢", "我讨厌", "偏好"}) {
		out[MemKindPreference] = 0.8
		fired = true
	}
	// procedural：怎么/如何/步骤/第一步；或同时含"先"与"再"（近似正则 先.*再）
	if hasAny(text, []string{"怎么", "如何", "步骤", "第一步"}) ||
		(strings.Contains(text, "先") && strings.Contains(text, "再")) {
		out[MemKindProcedural] = 0.8
		fired = true
	}
	if hasAny(text, timeWords) {
		out[MemKindEpisodic] = 0.7
		fired = true
	}
	if !fired {
		out[MemKindSemantic] = 0.6
	}
	return out
}

// Route 规则版路由（P0 冻结规则）：
//
//	含时间词                       → temporal=0.7
//	含"为什么/导致/因为/所以"      → causal=0.7
//	含具体实体名（大写英文词 或 中文专名长度>=2）→ entity=0.6
//	否则                           → semantic=0.6
//	hops=3。多视图可同时激活。
func (d *DefaultSystemOne) Route(query string) (views map[EdgeRel]float64, hops int) {
	views = map[EdgeRel]float64{}
	fired := false
	if hasAny(query, timeWords) {
		views[EdgeRelTemporal] = 0.7
		fired = true
	}
	if hasAny(query, []string{"为什么", "导致", "因为", "所以"}) {
		views[EdgeRelCausal] = 0.7
		fired = true
	}
	if hasEntityName(query) {
		views[EdgeRelEntity] = 0.6
		fired = true
	}
	if !fired {
		views[EdgeRelSemantic] = 0.6
	}
	return views, 3
}

// Assess 规则版证据评估（P0 简化版）：
//
//	evidence 为空              → lowValue=true（再搜也没用）
//	evidence >=3 条           → enough=true（证据够了）
//	其余                      → 继续扩
func (d *DefaultSystemOne) Assess(evidence []MemoryItem) (enough, lowValue bool) {
	if len(evidence) == 0 {
		return false, true
	}
	if len(evidence) >= 3 {
		return true, false
	}
	return false, false
}

// VerifyMemory 写前验证（纯启发式部分：空文本拒绝；Store 相关去重仍在 Reflector.verifyMemory）。
func (d *DefaultSystemOne) VerifyMemory(m MemoryItem) bool {
	return strings.TrimSpace(m.Text) != ""
}

// VerifySelf 写前验证（纯启发式部分：空文本拒绝；同层矛盾检测仍在 Reflector.verifySelf）。
func (d *DefaultSystemOne) VerifySelf(s SelfItem) bool {
	return strings.TrimSpace(s.Text) != ""
}

// ---- 有界输出校验（P2 用；校验失败 → 回退 DefaultSystemOne + 写 decision JSONL fallback）----

// AskNoul 校验二元命题概率 ∈[0,1] 且非 NaN；不合法即 false（调用方回退规则）。
func AskNoul(p float64) bool {
	return !math.IsNaN(p) && p >= 0 && p <= 1
}

// AskChoice 校验互斥分类分布：每项非负非 NaN，且归一和≈1（容差 0.05）。
// 注意：本函数只用于 P2 模型输出的 Choice 分布；Route() 的多视图权重和不必为 1。
func AskChoice(dist map[EdgeRel]float64) bool {
	sum := 0.0
	for _, v := range dist {
		if v < 0 || math.IsNaN(v) {
			return false
		}
		sum += v
	}
	return math.Abs(sum-1.0) < 0.05
}

// hasAny 大小写不敏感地判断 s 是否包含 subs 中任一片段（中文不受影响）。
func hasAny(s string, subs []string) bool {
	for _, sub := range subs {
		if containsFold(s, sub) {
			return true
		}
	}
	return false
}

// hasEntityName 粗判实体：仅当 query 含大写英文词（明确专名信号，如 OpenAI/VHS/GPT）时判实体。
// P0 占位启发式；P1 换正则/NER 命名实体抽取。
// 注意：不再把"任意 rune 长度>=2 的中文片段"判为实体——中文无空格分词，那样会让语义兜底分支对所有中文查询恒不可达
// （与 Route"默认无信号→semantic"测试相悖）。
func hasEntityName(query string) bool {
	for _, r := range query {
		if r >= 'A' && r <= 'Z' {
			return true
		}
	}
	return false
}
