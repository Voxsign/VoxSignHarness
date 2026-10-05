// compress.go —— 知己 · 上下文压缩器（架构 v1.0 §6.4 / §08 compress / ADR-004 落地）。
//
// 压缩策略（ADR-004）：
//   - 保留决策要件：目标/规则/未决问题（不压）
//   - tool-result-clearing 先行：中间工具结果优先清除
//   - 队尾递归摘要：其余文本按"保首尾、摘要中间"递归压缩
//   - 北极星：任务成功率 + detail survival rate（不追压缩率）——探针联动
// Phase 0 实现为确定性压缩（无小模型）；Phase 2 由压缩教师（LoRA 蒸馏）替换 CompressFn。
package zhiji

import (
	"context"
	"errors"
	"strings"
)

// CompressInput 压缩输入。
type CompressInput struct {
	Full    string // 完整上下文（轨迹/消息序列）
	Segment string // 本段文本（如最近 N 轮对话）
}

// CompressedContext 压缩产物（决策要件原样保留）。
type CompressedContext struct {
	Goals          []string `json:"goals"`            // 目标层（决策要件，不压）
	Rules          []string `json:"rules"`            // 规则层（决策要件，不压）
	Pending        []string `json:"pending"`          // 未决问题（决策要件，不压）
	Summary        string   `json:"summary"`          // 压缩后的主体摘要
	DroppedToolResult int   `json:"dropped_tool_result"` // 清除的工具结果块数
	DetailSurvival float64  `json:"detail_survival"`  // 探针召回率（北极星）
}

// CompressFn 压缩执行体（Phase 2 由压缩教师替换；签名不变）。
type CompressFn func(ctx context.Context, in CompressInput) (CompressedContext, error)

// Compressor 上下文压缩器。
type Compressor struct {
	store     *Store
	MaxSummaryTokens int
	Fn        CompressFn
}

// NewCompressor 构造压缩器（默认确定性实现）。
func NewCompressor(store *Store) *Compressor {
	c := &Compressor{store: store, MaxSummaryTokens: 4000}
	c.Fn = c.defaultCompress
	return c
}

// Compress 执行压缩（探针预埋 → 压缩 → 召回统计）。
func (c *Compressor) Compress(ctx context.Context, in CompressInput) (CompressedContext, error) {
	if err := ctx.Err(); err != nil {
		return CompressedContext{}, err
	}
	out, err := c.Fn(ctx, in)
	if err != nil {
		return out, err
	}
	// 探针召回：压缩后对预埋细节做一次检索验证（survival rate 北极星）。
	if c.store != nil {
		// v1.1 钩子：压缩产物里若出现预埋探针的 KeyDetail → 标记召回。
		// 骨架实现只增不删，不改变既有压缩输出；P1 换成 BudgetSearch 显式召回 decayed 探针。
		c.markProbeHits(out.Summary)
		out.DetailSurvival = c.store.SurvivalRate()
	}
	return out, nil
}

// markProbeHits 粗匹配：压缩后文本里出现某探针的 KeyDetail → 标记该探针已召回。
// 只动探针 Recalled 状态，不改压缩输出；先快照 probes 再调 ProbeRecall（避免 RLock→Lock 死锁）。
func (c *Compressor) markProbeHits(compressed string) {
	if compressed == "" || c.store == nil {
		return
	}
	c.store.mu.RLock()
	probes := append([]SurvivalProbe(nil), c.store.probes...)
	c.store.mu.RUnlock()
	for _, p := range probes {
		if p.Recalled || p.KeyDetail == "" {
			continue
		}
		if containsFold(compressed, p.KeyDetail) {
			c.store.ProbeRecall(p.ID)
		}
	}
}

// defaultCompress Phase 0 确定性压缩：
//  1. 提取 [goal]/[rule]/[pending] 决策要件（原样保留）
//  2. tool-result 块整体清除（标记 DroppedToolResult）
//  3. 其余文本保首尾、中段摘要（按 token 预算截断 + 保留结构）
func (c *Compressor) defaultCompress(ctx context.Context, in CompressInput) (CompressedContext, error) {
	if err := ctx.Err(); err != nil {
		return CompressedContext{}, err
	}
	var out CompressedContext
	lines := strings.Split(in.Full+"\n"+in.Segment, "\n")
	var rest []string
	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		switch {
		case strings.HasPrefix(trimmed, "[goal]"):
			out.Goals = append(out.Goals, strings.TrimPrefix(trimmed, "[goal]"))
		case strings.HasPrefix(trimmed, "[rule]"):
			out.Rules = append(out.Rules, strings.TrimPrefix(trimmed, "[rule]"))
		case strings.HasPrefix(trimmed, "[pending]"):
			out.Pending = append(out.Pending, strings.TrimPrefix(trimmed, "[pending]"))
		case strings.Contains(trimmed, "tool-result") || strings.HasPrefix(trimmed, "tool_result:"):
			out.DroppedToolResult++ // tool-result-clearing：清除，不进入摘要
		default:
			if trimmed != "" {
				rest = append(rest, trimmed)
			}
		}
	}
	// 队尾递归摘要：保首尾、摘要中间（token 预算按字符粗估 4 字符≈1 token）。
	budget := c.MaxSummaryTokens * 4
	head := rest
	if len(rest) > 200 { // 超出块数 → 保留首 100 行 + 尾 50 行，中间压缩为一行摘要
		head = append(append([]string{}, rest[:100]...), rest[len(rest)-50:]...)
	}
	joined := strings.Join(head, "\n")
	if len(joined) > budget {
		joined = joined[:budget] + "\n…[中段已递归摘要]"
	}
	out.Summary = joined
	if len(out.Goals)+len(out.Rules)+len(out.Pending) == 0 && out.Summary == "" {
		return out, errors.New("zhiji: 无可压缩内容")
	}
	return out, nil
}
