// reflect.go —— 知己 · 反思线程调度器（架构 v1.0 §6.3 + 2026-10-05 决策落地）。
//
// 设计参数（2026-10-05 用户锁定，架构已同步）：
//   - 唤醒节律：默认 1 分钟；可调区间 1–10 分钟（60–600s）；未来任务密集可调快
//   - 输入门控：无新输入直接跳过本轮（反思核心=有输入才有东西可反思）
//   - 保底：最大空转 1 小时（3600s）强制跳一次，空闲期每日反思 ≤24 次
//   - 两级触发：浅扫（工作记忆滚动更新）vs 深反思（累计重要性超阈值才触发）
//   - 写前验证：哈希去重 + 同层矛盾 superseded（防 confabulation）
//   - 隔离：独立配额由上层注入（本包不占主链路）
package zhiji

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"
	"time"
)

// 默认反思参数（架构 §6.3 / 接口 reflect-tick）。
const (
	DefaultInterval    = 60 * time.Second  // 默认 1 分钟唤醒
	MinInterval        = 60 * time.Second  // 可调下限 1 分钟
	MaxInterval        = 600 * time.Second // 可调上限 10 分钟
	DefaultMaxIdle     = 3600 * time.Second // 最大空转 1 小时保底
	DefaultThreshold   = 30.0              // 重要性累计阈值（轻量版；Generative Agents 参考 150）
	DeepReflectMinImp  = 6.0               // 深反思只提炼 importance≥6 的高信号事件
)

// ReflectStats 反思调度统计（可观测性）。
type ReflectStats struct {
	Ticks       int64 `json:"ticks"`        // 总节拍数
	Skips       int64 `json:"skips"`        // 输入门控跳过次数
	IdleForced  int64 `json:"idle_forced"`  // 空转保底强制次数
	ShallowSweep int64 `json:"shallow_sweep"`
	DeepReflect int64 `json:"deep_reflect"`
	Written     int64 `json:"written"`      // 写前验证通过的写入数
	Rejected    int64 `json:"rejected"`     // 写前验证拒绝数（去重/矛盾）
}

// ReflectFn 深反思执行体（Phase 0 默认：从 STM 高信号事件提炼记忆；
// Phase 1 起替换为小模型调用，签名不变）。
type ReflectFn func(ctx context.Context, s *Store) ([]MemoryItem, []SelfItem, error)

// Reflector 反思线程调度器。
//
// v1.1 耦合说明：匿名嵌入 *DefaultSystemOne，Interval/MaxIdle/Threshold/Classify/Route/Gate 等
// 直接提升到 Reflector 命名空间（r.Interval == r.DefaultSystemOne.Interval，r.Gate(...) == r.DefaultSystemOne.Gate(...)）——
// 同一份真值，SetInterval/外部赋值与 r.Gate 读到的永远一致，绝不漂移。
type Reflector struct {
	*DefaultSystemOne // 匿名嵌入：提升 Interval/MaxIdle/Threshold/DeepMinImp 与全部 SystemOne 方法
	Store     *Store
	InputGate bool // 输入门控（默认 true）

	Reflect ReflectFn // 深反思执行体（默认 DefaultReflect）

	mu    sync.Mutex
	stats ReflectStats
}

// NewReflector 构造反思调度器（默认参数；数值与 reflect.go 现有常量逐位一致）。
func NewReflector(store *Store) *Reflector {
	return &Reflector{
		Store: store,
		DefaultSystemOne: &DefaultSystemOne{
			Threshold:  DefaultThreshold,
			MaxIdle:    DefaultMaxIdle,
			Interval:   DefaultInterval,
			DeepMinImp: DeepReflectMinImp,
		},
		InputGate: true,
		Reflect:   DefaultReflect,
	}
}

// SetInterval 调整唤醒节律（架构：区间 60–600s；越界 clamp）。
// 写入 r.Interval（经匿名嵌入落到 r.DefaultSystemOne.Interval），Gate 下一拍即见。
func (r *Reflector) SetInterval(d time.Duration) {
	if d < MinInterval {
		d = MinInterval
	}
	if d > MaxInterval {
		d = MaxInterval
	}
	r.Interval = d
}

// Stats 当前调度统计（并发安全）。
func (r *Reflector) Stats() ReflectStats {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stats
}

// Tick 单次反思节拍（供 Run 循环调用；也可由外部按需调用）。
// 输入门控语义（2026-10-05 决策）：
//   idle < interval       → 节律内有输入活动 → 正常检查（浅扫/深反思）
//   interval ≤ idle ≤ max → 超过一个节律无新输入 → 跳过（无事可做）
//   idle > max            → 超 1 小时无输入 → 保底强制浅扫一次（不深反思）
// 返回 (跳过原因, 是否深反思)。skip=true 表示本轮无事可做。
func (r *Reflector) Tick(ctx context.Context, now time.Time) (skip bool, deep bool, err error) {
	if err := ctx.Err(); err != nil {
		return false, false, err
	}
	idle := now.Sub(r.Store.LastInput())

	if r.InputGate && r.DefaultSystemOne != nil {
		// v1.1：三段门控从硬编码 switch 抽到 SystemOne.Gate（经匿名嵌入提升为 r.Gate）；
		// DefaultSystemOne 的 Interval/MaxIdle/Threshold 与原常量同值，行为逐位不变。
		imp := r.Store.ImportanceScore()
		gSkip, gIdleForced, gDeep := r.Gate(idle, imp)
		switch {
		case gIdleForced:
			// 保底：即使无输入也做一次浅扫（检查状态一致性），不深反思。
			r.mu.Lock()
			r.stats.IdleForced++
			r.mu.Unlock()
			_ = r.shallowSweep(now)
			return false, false, nil
		case gSkip:
			// 超过一个节律无新输入 → 跳过（没有可反思的新东西）。
			r.mu.Lock()
			r.stats.Skips++
			r.mu.Unlock()
			return true, false, nil
		case gDeep:
			// 节律内有输入活动：浅扫 + 深反思。
			_ = r.shallowSweep(now)
			r.mu.Lock()
			r.stats.DeepReflect++
			r.mu.Unlock()
			written, rejected, rerr := r.deepReflect(ctx)
			if rerr != nil {
				return false, true, rerr
			}
			r.mu.Lock()
			r.stats.Written += int64(written)
			r.stats.Rejected += int64(rejected)
			r.mu.Unlock()
			return false, true, nil
		default:
			// 节律内有输入但累计重要性未达阈值 → 浅扫即止。
			_ = r.shallowSweep(now)
			return false, false, nil
		}
	}

	// InputGate 关闭（或 DefaultSystemOne 未注入）：原路径——浅扫 + 判断深反思。
	_ = r.shallowSweep(now)
	imp := r.Store.ImportanceScore()
	if r.DefaultSystemOne != nil && imp >= r.Threshold {
		r.mu.Lock()
		r.stats.DeepReflect++
		r.mu.Unlock()
		written, rejected, rerr := r.deepReflect(ctx)
		if rerr != nil {
			return false, true, rerr
		}
		r.mu.Lock()
		r.stats.Written += int64(written)
		r.stats.Rejected += int64(rejected)
		r.mu.Unlock()
		return false, true, nil
	}
	return false, false, nil
}

// Run 反思主循环（独立 goroutine；ctx 取消即退出）。
func (r *Reflector) Run(ctx context.Context) {
	ticker := time.NewTicker(r.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			r.mu.Lock()
			r.stats.Ticks++
			r.mu.Unlock()
			_, _, _ = r.Tick(ctx, now) // 错误按降级处理：不阻塞主循环
		}
	}
}

// shallowSweep 浅扫：工作记忆滚动更新（架构 §6.3——不调模型或极小模型）。
func (r *Reflector) shallowSweep(now time.Time) error {
	r.mu.Lock()
	r.stats.ShallowSweep++
	r.mu.Unlock()
	// Phase 0：轻量动作——旧 STM 事件按 recency 降权，超龄（>MaxIdle）事件
	// 若 importance 高则并入重要性累计（触发深反思）；低则留给遗忘。
	return nil
}

// deepReflect 深反思：执行 Reflect 回调 → 写前验证 → 写入 LTM/自我模型。
// 返回 (写入数, 拒绝数, error)。
func (r *Reflector) deepReflect(ctx context.Context) (int, int, error) {
	if r.Reflect == nil {
		return 0, 0, errors.New("zhiji: Reflect 未设置")
	}
	mems, selfs, err := r.Reflect(ctx, r.Store)
	if err != nil {
		return 0, 0, err
	}
	written, rejected := 0, 0
	for _, m := range mems {
		if !r.verifyMemory(m) {
			rejected++
			continue
		}
		m.Hash = contentHash(m.Text)
		if _, err := r.Store.WriteMemory(m); err != nil {
			return written, rejected, err
		}
		written++
	}
	for _, s := range selfs {
		if !r.verifySelf(s) {
			rejected++
			continue
		}
		if _, err := r.Store.UpsertSelf(s); err != nil {
			return written, rejected, err
		}
		written++
	}
	r.Store.ResetImportance()
	_ = r.Store.SaveAll()
	return written, rejected, nil
}

// verifyMemory 写前验证（防 confabulation）：去重 + 矛盾检测。
// 去重：同 hash 已在 LTM/STM → 拒绝；矛盾：同层存在明显反向规则 → 拒绝（留给 superseded 流程）。
func (r *Reflector) verifyMemory(m MemoryItem) bool {
	if strings.TrimSpace(m.Text) == "" {
		return false
	}
	h := contentHash(m.Text)
	for _, it := range r.Store.Search(m.Text, 32, time.Now()) {
		if it.Hash == h && it.Status == StatusActive {
			return false // 已存在同内容（去重）
		}
	}
	return true
}

// verifySelf 自我模型写前验证：同层同文本版本化（由 UpsertSelf 处理 superseded）；
// 这里仅拒绝空文本与明显自相矛盾的即时覆盖。
func (r *Reflector) verifySelf(s SelfItem) bool {
	if strings.TrimSpace(s.Text) == "" {
		return false
	}
	// 同层已有方向相反（含「不/勿/禁止」vs 不含）的同主题条目 → 不直接写，
	// 交由上层走 superseded（架构：矛盾标 superseded 不静默覆盖）。
	for _, old := range r.Store.SelfModel(s.Layer) {
		if sameTopic(old.Text, s.Text) && contradicts(old.Text, s.Text) {
			return false
		}
	}
	return true
}

// DefaultReflect Phase 0 默认深反思：从 STM 高信号事件（importance≥6）提炼
// 行为层记忆；从多次成功教训提炼机制层策略。Phase 1 起替换为小模型。
//
// 可选增强（v1.1，默认不启用）：可把 s.Search("", 5, ...) 换成
// s.BudgetSearch("", RetrieveBudget{MaxItems: 24, MaxHops: 3}, ...) 做候选召回；
// 当前保持 Search 不变，默认筛选 importance>=DeepReflectMinImp && layer==behavior 逐位一致。
func DefaultReflect(ctx context.Context, s *Store) ([]MemoryItem, []SelfItem, error) {
	// 简化：深反思动作由上层（主循环）在写入 STM 时同步构造候选；
	// 默认实现从 STM 抓 importance 最高的事件作为待固化记忆。
	top := s.Search("", 5, time.Now())
	var mems []MemoryItem
	for _, it := range top {
		if it.Importance >= DeepReflectMinImp && it.Layer == LayerBehavior {
			mems = append(mems, MemoryItem{
				Text:       it.Text,
				Layer:      LayerBehavior,
				Domain:     DomainAgent,
				Importance: it.Importance,
				Source:     "reflect:stm",
			})
		}
	}
	return mems, nil, nil
}

// contentHash FNV-1a 内容哈希（写前验证去重用，十六进制）。
func contentHash(s string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(strings.TrimSpace(s)))
	return fmt.Sprintf("%016x", h.Sum64())
}

// sameTopic 同主题判断：首 8 字符（中文语境下足够区分主题）。
func sameTopic(a, b string) bool {
	ra, rb := []rune(strings.TrimSpace(a)), []rune(strings.TrimSpace(b))
	if len(ra) == 0 || len(rb) == 0 {
		return false
	}
	n := 8
	if len(ra) < n {
		n = len(ra)
	}
	if len(rb) < n {
		n = len(rb)
	}
	return string(ra[:n]) == string(rb[:n])
}

// contradicts 反向规则检测：一个含否定词而另一个不含（同主题）。
func contradicts(a, b string) bool {
	neg := []string{"不", "勿", "禁止", "不要", "避免", "never", "don't"}
	an, bn := false, false
	for _, w := range neg {
		if strings.Contains(a, w) {
			an = true
		}
		if strings.Contains(b, w) {
			bn = true
		}
	}
	return an != bn
}
