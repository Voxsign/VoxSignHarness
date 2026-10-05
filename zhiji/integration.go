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
	"time"
)

// Zhiji 知己运行时（Phase 0 组装入口）。
type Zhiji struct {
	Store      *Store
	Log        *CallLogStore
	Contract   Contract
	Registry   *Registry
	Router     *Router
	Compressor *Compressor
	Vault      VaultWriter
	Reflector  *Reflector

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
	}, nil
}

// OnInput 每轮输入：写 STM 热区事件 + 输入门控信号（主循环阶段 1-3 调用）。
// summary 为输入摘要/原始文本；importance 由调用方按信号强度给出（默认 3）。
func (z *Zhiji) OnInput(summary string, importance float64) {
	if importance <= 0 {
		importance = 3
	}
	z.Store.TouchSTM(MemoryItem{
		Text:       summary,
		Layer:      LayerBehavior,
		Domain:     DomainSession,
		Importance: importance,
	}, 10)
	z.Store.MarkInput() // 输入门控：有输入活动，反思线程正常节拍
}

// BeforeDecision 决策前注入基线块（主循环决策阶段调用；返回注入文本供拼装）。
func (z *Zhiji) BeforeDecision(ctx context.Context, query string) (*Baseline, error) {
	return z.Contract.InjectBaseline(ctx, query)
}

// OnTaskEnd 任务结束：全量轨迹日志 + 反馈信号（主循环产物落盘后调用）。
func (z *Zhiji) OnTaskEnd(ctx context.Context, log CallLog) error {
	if log.TaskProfile == "" {
		return errors.New("zhiji: OnTaskEnd 需要 task_profile")
	}
	return z.Contract.LogTrajectory(ctx, log)
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
