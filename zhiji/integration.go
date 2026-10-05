// integration.go —— 知己 · 主循环接入钩子（架构 v1.0 §07 外化契约四件套落地）。
//
// 这是 Phase 0 的对外唯一入口：把意识缓存 / 契约 / 反思线程 / 元层组件
// 组装为可在 pipeline.Run / server 主循环直接挂接的 Zhiji 实例。
//
// 接入点（对应 pipeline.Run 13 阶段）：
//   OnInput(text)         → 阶段 1-3（输入进入）：TouchSTM + MarkInput（输入门控信号）
//   BeforeDecision(query) → 决策前：InjectBaseline（注入 ≤800–1200 token 基线块）
//   OnTaskEnd(log)        → 阶段 13（产物落盘后）：全量日志 + 反馈信号
//   OnDecide(profile)     → 元层路由钩子（影子模式：只记录不改变线上）
//   Start/Stop            → 反思线程启停（独立 goroutine）
//   SyncVault(ctx)        → 外化：深反思产物写云端「我的数据」
package zhiji

import (
	"context"
	"errors"
	"path/filepath"
	"time"
)

// Zhiji 知己运行时（Phase 0 组装入口）。
type Zhiji struct {
	Store    *Store
	Log      *CallLogStore
	Contract Contract
	Registry *Registry
	Router   *Router
	Compressor *Compressor
	Vault      VaultWriter
	Reflector  *Reflector
	RawLog     *RawLog // v1.1 原始观察 append-only（raw.jsonl）

	runCancel context.CancelFunc
}

// NewZhiji 组装 Phase 0 全部组件（dir 为数据目录）。
func NewZhiji(dir string, vault VaultWriter) (*Zhiji, error) {
	if dir == "" {
		return nil, errors.New("zhiji: 数据目录不能为空")
	}
	store, err := NewStore(dir)
	if err != nil {
		return nil, err
	}
	log, err := NewCallLogStore(dir)
	if err != nil {
		return nil, err
	}
	reg, err := NewRegistry(dir)
	if err != nil {
		return nil, err
	}
	router, err := NewRouter(reg, dir)
	if err != nil {
		return nil, err
	}
	compressor := NewCompressor(store)
	reflector := NewReflector(store)
	contract := NewContract(store, log)
	return &Zhiji{
		Store:      store,
		Log:        log,
		Contract:   contract,
		Registry:   reg,
		Router:     router,
		Compressor: compressor,
		Vault:      vault,
		Reflector:  reflector,
		RawLog:     NewRawLog(filepath.Join(dir, "raw.jsonl")),
	}, nil
}

// OnInput 每轮输入（主循环阶段 1-3 调用）。
// 顺序（v1.1 §7.1）：① RawLog.Append 原始观察全保留 → ② 可选规则版 Classify 打四类分 →
// ③ TouchSTM 滚动工作记忆 → ④ MarkInput 输入门控信号。
// summary 为输入摘要/原始文本；importance 由调用方按信号强度给出（默认 3）。
func (z *Zhiji) OnInput(summary string, importance float64) {
	if importance <= 0 {
		importance = 3
	}
	// ① 原始观察 append-only（永不删；即使 RawLog 未配置也不阻塞主链路）
	if z.RawLog != nil {
		_ = z.RawLog.Append(RawObs{
			Text:       summary,
			Provenance: Provenance{Origin: "user_input"},
		})
	}
	item := MemoryItem{
		Text:       summary,
		Layer:      LayerBehavior,
		Domain:     DomainSession,
		Importance: importance,
	}
	// ② 可选：规则版四类打分写回 Kind/KindScore（P0 增强；不改 Importance 与三因子公式）
	if z.Reflector != nil && z.Reflector.DefaultSystemOne != nil {
		scores := z.Reflector.Classify(item)
		best := MemKind("")
		bestScore := 0.0
		for k, v := range scores {
			if v > bestScore {
				best, bestScore = k, v
			}
		}
		if best != "" {
			item.Kind = best
			item.KindScore = bestScore
		}
	}
	z.Store.TouchSTM(item, 10)
	z.Store.MarkInput() // 输入门控：有输入活动，反思线程正常节拍
}

// BeforeDecision 决策前注入基线块（主循环决策阶段调用；返回注入文本供拼装）。
func (z *Zhiji) BeforeDecision(ctx context.Context, query string) (*Baseline, error) {
	return z.Contract.InjectBaseline(ctx, query)
}

// OnTaskEnd 任务结束：全量轨迹日志 + 反馈信号（主循环产物落盘后调用）。
// v1.3：LogTrajectory 落盘后，自动从本任务文本挑 2–3 个低显著关键细节预埋 detail-survival 探针
// （A4；供压缩后测召回率）。探针失败不影响轨迹落盘；Store 未配置时跳过。
func (z *Zhiji) OnTaskEnd(ctx context.Context, log CallLog) error {
	if log.TaskProfile == "" {
		return errors.New("zhiji: OnTaskEnd 需要 task_profile")
	}
	if err := z.Contract.LogTrajectory(ctx, log); err != nil {
		return err
	}
	// A4 探针自动预埋（best-effort）：slot 用 task_profile，细节从本任务文本抽取。
	if z.Store != nil {
		slot := log.TaskProfile
		if slot == "" {
			slot = "auto"
		}
		for _, detail := range extractLowSalienceDetails(taskEndCorpus(log), 3) {
			z.Store.Probe(slot, detail)
		}
	}
	return nil
}

// OnDecide 元层路由钩子（影子模式：只记录推荐，不改变线上；Phase 2 切线上）。
func (z *Zhiji) OnDecide(p TaskProfile) (RouteDecision, error) {
	return z.Router.Decide(p)
}

// Start 启动反思线程（独立 goroutine，独立配额由上层注入）。
func (z *Zhiji) Start(ctx context.Context) {
	runCtx, cancel := context.WithCancel(ctx)
	z.runCancel = cancel
	go z.Reflector.Run(runCtx)
}

// Stop 停止反思线程。
func (z *Zhiji) Stop() {
	if z.runCancel != nil {
		z.runCancel()
		z.runCancel = nil
	}
}

// SyncVault 外化：把待固化记忆/自我模型写云端「我的数据」（外化知己通道）。
// Phase 0 默认同步 LTM 中 source=reflect 的高信号条目；幂等（云端去重由服务侧承担）。
func (z *Zhiji) SyncVault(ctx context.Context) (int, error) {
	if z.Vault == nil {
		return 0, errors.New("zhiji: Vault 未配置（外化通道关闭）")
	}
	n := 0
	for _, it := range z.Store.Search("", 16, time.Now()) {
		if it.Source != "reflect:stm" || it.Status != StatusActive {
			continue
		}
		if err := z.Vault.Write(ctx, VaultEntry{
			Type:   string(it.Layer),
			Text:   it.Text,
			Source: it.Source,
			Domain: string(it.Domain),
		}); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// Compress 上下文压缩入口（主循环长上下文时调用）。
func (z *Zhiji) Compress(ctx context.Context, in CompressInput) (CompressedContext, error) {
	return z.Compressor.Compress(ctx, in)
}

// Close 关闭日志与持久化。
func (z *Zhiji) Close() error {
	z.Stop()
	if err := z.Store.SaveAll(); err != nil {
		return err
	}
	return z.Log.Close()
}
