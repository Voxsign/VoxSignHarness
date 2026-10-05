// contract.go —— 知己 · 与模型中心的最小契约（架构 v1.0 §07/§08 落地）。
//
// 四类通道：invoke（决策时注入/检索）、工具族（MCP 式读写）、
// feedback（轨迹/结果回写）、events（事件信号，二期占位）。
// 接口语义与架构 §08 接口清单一一对应（inject-baseline / retrieve-context /
// read_awareness_cache / update_self_model / write_long_term_memory /
// log-trajectory / log-feedback / evict）。
package zhiji

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// 注入/检索常量（架构 §07 最小契约四件套）。
const (
	BaselineMaxTokens = 1200 // 基线块上限（≤800–1200 token 量级）
	BaselineMinTokens = 800
	RetrieveTopK      = 8 // 机制/行为层检索 top-k
)

// Baseline 决策时注入的基线块（目标+规则常驻 + 检索补充）。
type Baseline struct {
	Goals      string   `json:"goals"`      // 目标层文本
	Rules      string   `json:"rules"`      // 规则层文本
	Retrieved  []string `json:"retrieved"`  // 机制/行为层 top-k 文本
	TokenBudget int     `json:"token_budget"` // 剩余容量预算提示（Claude context awareness 式）
}

// Contract 最小契约接口（对模型中心/主循环暴露的全部能力）。
type Contract interface {
	// 注入族
	InjectBaseline(ctx context.Context, query string) (*Baseline, error)
	RetrieveContext(ctx context.Context, query string, k int) ([]MemoryItem, error)

	// 工具族（MCP 式）
	ReadAwarenessCache(ctx context.Context, layer Layer, query string) ([]SelfItem, []MemoryItem, error)
	UpdateSelfModel(ctx context.Context, item SelfItem) (SelfItem, error)
	WriteLongTermMemory(ctx context.Context, item MemoryItem) (MemoryItem, error)
	Evict(ctx context.Context, domain Domain) (int, error)

	// 反馈族
	LogTrajectory(ctx context.Context, log CallLog) error
	LogFeedback(ctx context.Context, taskID, outcome string, confidence float64) error
}

// zhijiContract 默认实现（进程内，亚毫秒检索）。
type zhijiContract struct {
	store *Store
	log   *CallLogStore
}

// NewContract 构造契约实现（store 与日志存储必须已初始化）。
func NewContract(store *Store, log *CallLogStore) Contract {
	return &zhijiContract{store: store, log: log}
}

// InjectBaseline 组装决策时注入块：目标+规则常驻 + 当前 query 相关机制/行为 top-k。
// 进程内检索（架构硬约束：p50≈40µs 量级、亚毫秒）。
func (c *zhijiContract) InjectBaseline(ctx context.Context, query string) (*Baseline, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	base := c.store.Baseline()
	var goals, rules strings.Builder
	for _, line := range strings.Split(base, "\n") {
		if strings.HasPrefix(line, "[goal]") {
			goals.WriteString(strings.TrimPrefix(line, "[goal] "))
			goals.WriteString("\n")
		} else if strings.HasPrefix(line, "[rule]") {
			rules.WriteString(strings.TrimPrefix(line, "[rule] "))
			rules.WriteString("\n")
		}
	}
	items := c.store.Search(query, RetrieveTopK, time.Now())
	retrieved := make([]string, 0, len(items))
	for _, it := range items {
		retrieved = append(retrieved, it.Text)
	}
	// 剩余预算：基线长度估算（4 字符≈1 token 量级，粗估）。
	budget := BaselineMaxTokens - (len(goals.String())+len(rules.String()))/4
	if budget < 0 {
		budget = 0
	}
	return &Baseline{
		Goals:       strings.TrimSpace(goals.String()),
		Rules:       strings.TrimSpace(rules.String()),
		Retrieved:   retrieved,
		TokenBudget: budget,
	}, nil
}

// RetrieveContext JIT 检索（机制/行为层三因子 top-k）。
func (c *zhijiContract) RetrieveContext(ctx context.Context, query string, k int) ([]MemoryItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if k <= 0 {
		k = RetrieveTopK
	}
	items := c.store.Search(query, k, time.Now())
	if len(items) == 0 {
		return nil, ErrNotFound
	}
	return items, nil
}

// ReadAwarenessCache 按层/query 读意识缓存（工具族）。
func (c *zhijiContract) ReadAwarenessCache(ctx context.Context, layer Layer, query string) ([]SelfItem, []MemoryItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	self := c.store.SelfModel(layer)
	items := c.store.Search(query, RetrieveTopK, time.Now())
	return self, items, nil
}

// UpdateSelfModel 改目标/规则/机制块（ADD/UPDATE、superseded、版本递增）。
func (c *zhijiContract) UpdateSelfModel(ctx context.Context, item SelfItem) (SelfItem, error) {
	if err := ctx.Err(); err != nil {
		return SelfItem{}, err
	}
	if strings.TrimSpace(item.Text) == "" {
		return SelfItem{}, errors.New("zhiji: self model 文本不能为空")
	}
	return c.store.UpsertSelf(item)
}

// WriteLongTermMemory 写行为层事实/教训（写前验证/去重由 reflect 层执行）。
func (c *zhijiContract) WriteLongTermMemory(ctx context.Context, item MemoryItem) (MemoryItem, error) {
	if err := ctx.Err(); err != nil {
		return MemoryItem{}, err
	}
	if strings.TrimSpace(item.Text) == "" {
		return MemoryItem{}, errors.New("zhiji: 记忆文本不能为空")
	}
	return c.store.WriteMemory(item)
}

// Evict 遗忘（架构 §12：幂律降权可恢复；敏感域清除留痕由上层处理）。
func (c *zhijiContract) Evict(ctx context.Context, domain Domain) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	n := c.store.DecayAndEvict(time.Now(), 0)
	if err := c.store.SaveAll(); err != nil {
		return n, err
	}
	return n, nil
}

// LogTrajectory 任务轨迹全量回写（异步、批量、独立配额——调度由上层负责）。
func (c *zhijiContract) LogTrajectory(ctx context.Context, log CallLog) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.log.Append(log)
}

// LogFeedback outcome 信号（可信度分级写入）。
func (c *zhijiContract) LogFeedback(ctx context.Context, taskID, outcome string, confidence float64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if taskID == "" {
		return errors.New("zhiji: taskID 不能为空")
	}
	return c.log.Append(CallLog{
		ID:         fmt.Sprintf("fb-%d", time.Now().UnixNano()),
		At:         time.Now(),
		TaskProfile: taskID,
		Outcome:    outcome,
		Cost:       0,
	})
}
