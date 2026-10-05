// store.go —— 知己 · 意识缓存存储层（架构 v1.0 §6/§12 落地）。
//
// 本机 JSON 文件存储（假设：文件系统 JSON/轻量索引，无数据库）：
//   self_model.json  四层自我模型（版本化，superseded 不静默覆盖）
//   stm.json         STM 热区（当前目标/激活规则/最近 N 轮事件）
//   ltm.json         LTM 归档（情节事件 + 语义条目）
//   probes.jsonl     detail survival 探针
// 写操作原子化（tmp+rename）；三因子打分检索；遗忘=降权可恢复（不硬删）。
package zhiji

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ErrNotFound 检索/读取无结果。
var ErrNotFound = errors.New("zhiji: not found")

// Default 检索权重（架构 §12：w1·Recency + w2·Importance + w3·Relevance）。
const (
	WRecency     = 0.4
	WImportance  = 0.35
	WRelevance   = 0.25
	RecencyHalfLife = 30 * time.Minute // 幂律衰减半衰期
)

// Store 意识缓存存储（线程安全）。
type Store struct {
	dir string

	mu          sync.RWMutex
	selfModel   []SelfItem  // 四层自我模型（含 superseded 历史）
	stm         []MemoryItem // STM 热区（按 LastSeen 降序滚动）
	ltm         []MemoryItem // LTM 归档
	probes      []SurvivalProbe
	Graph       *Graph      // v1.1 关系图（graph.json；空图时 BudgetSearch 退化）
	Vec         *VectorIndex // v1.2 可选向量索引（nil 退化为纯词法；BudgetSearch 混合锚点）
	lastInputAt time.Time   // 输入门控信号（有输入才更新）
	importance  float64     // 当前累计重要性（触发深反思阈值判断）
	nextID      int
}

// NewStore 创建/加载存储。dir 不存在会自动创建。
func NewStore(dir string) (*Store, error) {
	if dir == "" {
		return nil, errors.New("zhiji: dir 不能为空")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, lastInputAt: time.Now()}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// ---- 持久化 ----

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := readJSON(filepath.Join(s.dir, "self_model.json"), &s.selfModel); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := readJSON(filepath.Join(s.dir, "stm.json"), &s.stm); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := readJSON(filepath.Join(s.dir, "ltm.json"), &s.ltm); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := readJSON(filepath.Join(s.dir, "probes.json"), &s.probes); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// v1.1 关系图（graph.json 不存在视为空图，与其他四 JSON 一致）
	if s.Graph == nil {
		s.Graph = NewGraph()
	}
	if err := s.Graph.Load(filepath.Join(s.dir, "graph.json")); err != nil {
		return err
	}
	s.nextID = 1
	for _, it := range s.selfModel {
		if n, ok := parseNumSuffix(it.ID); ok && n >= s.nextID {
			s.nextID = n + 1
		}
	}
	return nil
}

// SaveAll 原子写全量（后台整理/反思固化后调用）。
func (s *Store) SaveAll() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := writeJSON(filepath.Join(s.dir, "self_model.json"), s.selfModel); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(s.dir, "stm.json"), s.stm); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(s.dir, "ltm.json"), s.ltm); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(s.dir, "probes.json"), s.probes); err != nil {
		return err
	}
	// v1.1 关系图（独立 graph.json；前面四个 JSON 写失败即返回，不影响它们）
	if s.Graph != nil {
		return s.Graph.Save(filepath.Join(s.dir, "graph.json"))
	}
	return nil
}

// ---- 自我模型 ----

// UpsertSelf 写一条自我模型（ADD/UPDATE 语义，架构 §12 写入纪律）：
// 同 layer+text 则版本递增（旧版标 superseded，不静默覆盖）。
func (s *Store) UpsertSelf(item SelfItem) (SelfItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if item.ID == "" {
		item.ID = fmt.Sprintf("self-%d", s.nextID)
		s.nextID++
	}
	item.UpdatedAt = time.Now()
	item.Confidence = clamp01(item.Confidence)
	if item.Status == "" {
		item.Status = StatusActive // 新建条目默认 active；superseded 替换逻辑见下（旧条目置 superseded，新条目保持 active）
	}

	// 查找同层同文本旧条目 → superseded
	for i := range s.selfModel {
		old := &s.selfModel[i]
		if old.Layer == item.Layer && strings.EqualFold(old.Text, item.Text) && old.Status != StatusSuperseded {
			item.Version = old.Version + 1
			old.SupersededBy = item.ID
			old.Status = StatusSuperseded
			break
		}
	}
	if item.Version == 0 {
		item.Version = 1
	}
	s.selfModel = append(s.selfModel, item)
	return item, nil
}

// SelfModel 返回当前版本的四层自我模型（含层过滤）。
func (s *Store) SelfModel(layer Layer) []SelfItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []SelfItem
	for _, it := range s.selfModel {
		if it.Status != StatusActive {
			continue
		}
		if layer == "" || it.Layer == layer {
			out = append(out, it)
		}
	}
	return out
}

// Baseline 生成注入基线块（架构 §07：目标+规则常驻，≤800–1200 token 量级）。
func (s *Store) Baseline() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var b strings.Builder
	for _, layer := range []Layer{LayerGoal, LayerRule} {
		for _, it := range s.selfModel {
			if it.Status != StatusActive || it.Layer != layer {
				continue
			}
			b.WriteString("[" + string(layer) + "] " + it.Text + "\n")
		}
	}
	return strings.TrimSpace(b.String())
}

// ---- 记忆条目 ----

// WriteMemory 写一条记忆（行为层教训/机制层策略；写前验证见 reflect.go）。
func (s *Store) WriteMemory(item MemoryItem) (MemoryItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if item.ID == "" {
		item.ID = fmt.Sprintf("mem-%d", s.nextID)
		s.nextID++
	}
	now := time.Now()
	item.CreatedAt = now
	item.LastSeen = now
	item.Importance = clampImportance(item.Importance)
	if item.Status == "" {
		item.Status = StatusActive
	}
	if item.Domain == "" {
		item.Domain = DomainSession
	}
	s.ltm = append(s.ltm, item)
	return item, nil
}

// TouchSTM 向 STM 热区写事件（每轮消息后异步抽取调用；滚动窗口）。
func (s *Store) TouchSTM(item MemoryItem, window int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if item.ID == "" {
		item.ID = fmt.Sprintf("mem-%d", s.nextID)
		s.nextID++
	}
	item.LastSeen = time.Now()
	if item.Status == "" {
		item.Status = StatusActive // STM 热区=激活态；否则 Search/写前验证/外化全部跳过它（写不进去=检索不到）
	}
	s.stm = append(s.stm, item)
	// 滚动窗口：保留最近 window 条（架构 §6.3 浅扫=工作记忆滚动更新）
	if window > 0 && len(s.stm) > window {
		drop := len(s.stm) - window
		s.stm = append([]MemoryItem(nil), s.stm[drop:]...)
	}
	// 重要性累计（触发深反思阈值判断）
	s.importance += item.Importance
}

// MarkInput 输入门控信号（架构 2026-10-05 决策：无新输入跳过反思）。
func (s *Store) MarkInput() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastInputAt = time.Now()
}

// LastInput 最近输入时间。
func (s *Store) LastInput() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastInputAt
}

// ImportanceScore 当前累计重要性（供 reflect-tick 阈值判断）。
func (s *Store) ImportanceScore() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.importance
}

// ResetImportance 深反思后清零累计（架构 §6.3：超阈值才深反思）。
func (s *Store) ResetImportance() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.importance = 0
}

// scoreItem 三因子打分（从 Search 内部抽出复用；公式与权重逐位不变：0.4/0.35/0.25，半衰期 30min）。
// rec = 0.5^((now-LastSeen)/HalfLife)；score = 0.4·rec + 0.35·(importance/10) + 0.25·relevance。
func (s *Store) scoreItem(it MemoryItem, query string, now time.Time) float64 {
	rec := math.Pow(0.5, float64(now.Sub(it.LastSeen))/float64(RecencyHalfLife))
	rel := it.Relevance(query)
	return WRecency*rec + WImportance*clampImportance(it.Importance)/10 + WRelevance*rel
}

// Search 三因子打分检索（架构 §12：w1·Recency + w2·Importance + w3·Relevance）。
// 检索范围：LTM（行为/机制层）+ STM 热区；进程内，微秒级。行为与 v1.0 逐位一致。
func (s *Store) Search(query string, k int, now time.Time) []MemoryItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var scored []struct {
		it  MemoryItem
		rec float64
	}
	merge := func(list []MemoryItem) {
		for _, it := range list {
			if it.Status != StatusActive {
				continue
			}
			scored = append(scored, struct {
				it  MemoryItem
				rec float64
			}{it, s.scoreItem(it, query, now)})
		}
	}
	merge(s.ltm)
	merge(s.stm)
	sort.SliceStable(scored, func(i, j int) bool { return scored[i].rec > scored[j].rec })
	if k <= 0 || k > len(scored) {
		k = len(scored)
	}
	out := make([]MemoryItem, 0, k)
	for i := 0; i < k; i++ {
		out = append(out, scored[i].it)
	}
	return out
}

// ---- v1.1 预算检索闭环（架构 v1.1 §7.2）----

// 默认检索预算（知己小库适配；空值字段取这些常量）。
const (
	DefaultBudgetMaxItems = 24              // 总条目上限（远小于 Jev-Mem 的 60）
	DefaultBudgetMaxHops  = 3               // 图扩展跳数上限（远小于 Jev-Mem 的 8）
	DefaultBudgetDeadline = 2 * time.Second // 死线（主循环每轮都注入，比 Jev-Mem 的 15s 紧）
	DefaultBudgetMinScore = 0.1             // 候选低于此分不再扩展
	DefaultSeedCount      = 12              // 词法锚点数（Jev-Mem 30）
	DefaultExpandTopK     = 3               // 每轮沿激活视图扩 top-3 邻居
)

// RetrieveBudget 预算检索的硬约束（触顶必停）。
type RetrieveBudget struct {
	MaxItems  int           // 总条目上限（默认 24）
	MaxTokens int           // 注入 token 预算（对齐 800–1200；P0 不强制截断）
	MaxHops   int           // 图扩展跳数上限（默认 3）
	Deadline  time.Duration // 死线（默认 2s）
	MinScore  float64       // 候选低于此分不再扩展（默认 0.1）
}

// normalize 零值补默认常量。
func (b *RetrieveBudget) normalize() {
	if b.MaxItems <= 0 {
		b.MaxItems = DefaultBudgetMaxItems
	}
	if b.MaxHops <= 0 {
		b.MaxHops = DefaultBudgetMaxHops
	}
	if b.Deadline <= 0 {
		b.Deadline = DefaultBudgetDeadline
	}
	if b.MinScore <= 0 {
		b.MinScore = DefaultBudgetMinScore
	}
}

// Trace 预算检索闭环轨迹（落 decision JSONL；影子模式可审计对比线上 Search）。
type Trace struct {
	SeedCount  int        `json:"seed_count"`
	Hops       int        `json:"hops"`
	ViewsHit   []EdgeRel  `json:"views_hit,omitempty"`
	StopReason string     `json:"stop_reason"` // enough|low_value|budget|deadline
}

// RetrieveSeeds 词法种子（复用 scoreItem 排序取前 n；STM∪LTM active 合并，与 Search 同源）。
func (s *Store) RetrieveSeeds(query string, n int, now time.Time) []MemoryItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	type scoredItem struct {
		it    MemoryItem
		score float64
	}
	var all []scoredItem
	for _, it := range s.ltm {
		if it.Status == StatusActive {
			all = append(all, scoredItem{it, s.scoreItem(it, query, now)})
		}
	}
	for _, it := range s.stm {
		if it.Status == StatusActive {
			all = append(all, scoredItem{it, s.scoreItem(it, query, now)})
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].score > all[j].score })
	if n <= 0 || n > len(all) {
		n = len(all)
	}
	out := make([]MemoryItem, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, all[i].it)
	}
	return out
}

// hybridSeeds 混合锚点：词法种子 ∪ 向量召回种子（按 nodeID 去重，词法打分顺序在前）。
// Vec==nil 或向量无召回时，逐位返回 RetrieveSeeds 结果——现有 BudgetSearch 行为零变化。
// 老路径一行不动；向量召回的 nodeID 回填到 ltm/stm 里对应 active 条目，未入库即跳过。
func (s *Store) hybridSeeds(query string, n int, now time.Time) []MemoryItem {
	base := s.RetrieveSeeds(query, n, now)
	if s.Vec == nil {
		return base
	}
	vhits := s.Vec.Search(query, n)
	if len(vhits) == 0 {
		return base
	}
	// 建 nodeID -> active 条目视图（ltm∪stm）；RetrieveSeeds 已释放读锁，这里重新取锁，不嵌套。
	s.mu.RLock()
	byID := map[string]MemoryItem{}
	for _, it := range s.ltm {
		if it.Status == StatusActive {
			byID[it.ID] = it
		}
	}
	for _, it := range s.stm {
		if it.Status == StatusActive {
			byID[it.ID] = it
		}
	}
	s.mu.RUnlock()
	have := map[string]bool{}
	out := make([]MemoryItem, 0, len(base)+len(vhits))
	for _, it := range base {
		have[it.ID] = true
		out = append(out, it)
	}
	for _, h := range vhits {
		if have[h.NodeID] {
			continue
		}
		it, ok := byID[h.NodeID]
		if !ok {
			continue
		}
		have[h.NodeID] = true
		out = append(out, it)
	}
	return out
}

// ExpandByGraph 沿图边找 seedID 的邻居（rel=0 表示不按视图过滤），
// 回填到 ltm/stm 里对应 ID 的条目；找不到则空（图上邻居 ID 未入库即跳过）。
func (s *Store) ExpandByGraph(seedID string, rel EdgeRel, limit int) []MemoryItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Graph == nil {
		return nil
	}
	var edges []Edge
	if rel == "" {
		edges = s.Graph.Neighbors(seedID)
	} else {
		edges = s.Graph.Neighbors(seedID, rel)
	}
	neighborIDs := map[string]bool{}
	for _, e := range edges {
		switch {
		case e.From == seedID:
			neighborIDs[e.To] = true
		case e.To == seedID:
			neighborIDs[e.From] = true
		}
	}
	if limit > 0 && len(neighborIDs) > limit {
		ids := make([]string, 0, len(neighborIDs))
		for id := range neighborIDs {
			ids = append(ids, id)
		}
		neighborIDs = map[string]bool{}
		for i := 0; i < limit && i < len(ids); i++ {
			neighborIDs[ids[i]] = true
		}
	}
	byID := map[string]MemoryItem{}
	for _, it := range s.ltm {
		byID[it.ID] = it
	}
	for _, it := range s.stm {
		byID[it.ID] = it
	}
	var out []MemoryItem
	for id := range neighborIDs {
		if it, ok := byID[id]; ok {
			out = append(out, it)
		}
	}
	return out
}

// BudgetSearch 预算检索闭环（架构 v1.1 §7.2：Route → 取 12 种子 → 循环打分/Assess/沿激活视图扩 top-3 邻居 → 硬预算触顶必停）。
// 停止原因：enough（证据够）| low_value（再搜没用）| budget（条目/跳数触顶）| deadline（超时）。
// 空图时退化为 Search 超集（结果不比 Search 差）。
func (s *Store) BudgetSearch(query string, b RetrieveBudget, now time.Time) ([]MemoryItem, Trace) {
	b.normalize()
	trace := Trace{StopReason: "budget"}

	// 空图退化：直接退化为 Search 超集（MaxItems 上限内全部 active 条目）
	if s.Graph == nil || s.Graph.IsEmpty() {
		out := s.Search(query, b.MaxItems, now)
		trace.SeedCount = len(out)
		return out, trace
	}

	// 路由（P0 规则版）
	one := &DefaultSystemOne{}
	views, hops := one.Route(query)
	if hops <= 0 {
		hops = b.MaxHops
	} else if hops > b.MaxHops {
		hops = b.MaxHops
	}
	for rel, w := range views {
		if w >= 0.1 {
			trace.ViewsHit = append(trace.ViewsHit, rel)
		}
	}

	deadline := now.Add(b.Deadline)
	seeds := s.hybridSeeds(query, DefaultSeedCount, now)
	trace.SeedCount = len(seeds)

	// 闭环 beam：去重收集（按 scoreItem 最终排序收口）
	collected := map[string]MemoryItem{}
	var order []string
	add := func(it MemoryItem) bool {
		if _, ok := collected[it.ID]; ok {
			return false
		}
		collected[it.ID] = it
		order = append(order, it.ID)
		return true
	}
	for _, it := range seeds {
		add(it)
	}

	current := seeds
	for hop := 0; hop < hops; hop++ {
		if time.Now().After(deadline) {
			trace.StopReason = "deadline"
			break
		}
		if len(collected) >= b.MaxItems {
			trace.StopReason = "budget"
			break
		}
		// 沿激活视图扩 top-3 邻居
		newIDs := []string{}
		for _, it := range current {
			for rel, w := range views {
				if w < 0.1 {
					continue
				}
				for _, nb := range s.ExpandByGraph(it.ID, rel, DefaultExpandTopK) {
					if add(nb) {
						newIDs = append(newIDs, nb.ID)
					}
					if len(collected) >= b.MaxItems {
						break
					}
				}
				if len(collected) >= b.MaxItems {
					break
				}
			}
			if len(collected) >= b.MaxItems {
				break
			}
		}
		trace.Hops++
		// Assess：基于当前全部证据做停止决策
		var ev []MemoryItem
		for _, id := range order {
			ev = append(ev, collected[id])
		}
		enough, lowValue := one.Assess(ev)
		if enough {
			trace.StopReason = "enough"
			break
		}
		if lowValue {
			trace.StopReason = "low_value"
			break
		}
		if len(newIDs) == 0 {
			// 图上无新邻居可扩 → 停
			trace.StopReason = "budget"
			break
		}
		// 下一轮从新加入的条目继续扩展
		current = current[:0]
		for _, id := range newIDs {
			current = append(current, collected[id])
		}
	}

	// 收口：按 scoreItem 降序，截到 MaxItems
	type scoredItem struct {
		it    MemoryItem
		score float64
	}
	var all []scoredItem
	for _, id := range order {
		it := collected[id]
		all = append(all, scoredItem{it, s.scoreItem(it, query, now)})
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].score > all[j].score })
	if len(all) > b.MaxItems {
		all = all[:b.MaxItems]
	}
	out := make([]MemoryItem, 0, len(all))
	for _, si := range all {
		out = append(out, si.it)
	}
	return out, trace
}

// DecayAndEvict 遗忘（架构 §12 遗忘纪律）：低分区降权可恢复，不硬删。
// lowThreshold=0.15 默认；active 条目分数低于阈值标 StatusDecayed（可恢复）。
func (s *Store) DecayAndEvict(now time.Time, lowThreshold float64) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if lowThreshold <= 0 {
		lowThreshold = 0.15
	}
	n := 0
	for i := range s.ltm {
		it := &s.ltm[i]
		if it.Status != StatusActive || it.Domain == DomainUser { // 用户级最持久不降权
			continue
		}
		rec := math.Pow(0.5, float64(now.Sub(it.LastSeen))/float64(RecencyHalfLife))
		score := WRecency*rec + WImportance*clampImportance(it.Importance)/10
		if score < lowThreshold {
			it.Status = StatusDecayed
			n++
		}
	}
	return n
}

// Probe 记录 detail survival 探针（预埋低显著关键细节）。
func (s *Store) Probe(slot, keyDetail string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.probes = append(s.probes, SurvivalProbe{
		ID:        fmt.Sprintf("probe-%d", s.nextID),
		CreatedAt: time.Now(),
		KeyDetail: keyDetail,
		Slot:      slot,
	})
	s.nextID++
}

// ProbeRecall 标记探针已召回（compaction 后测 detail survival rate）。
func (s *Store) ProbeRecall(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.probes {
		if s.probes[i].ID == id {
			s.probes[i].Recalled = true
			return true
		}
	}
	return false
}

// SurvivalRate detail survival rate（架构 §04 北极星之一）。
func (s *Store) SurvivalRate() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.probes) == 0 {
		return 1
	}
	recalled := 0
	for _, p := range s.probes {
		if p.Recalled {
			recalled++
		}
	}
	return float64(recalled) / float64(len(s.probes))
}

// ---- 工具 ----

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func clampImportance(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 10 {
		return 10
	}
	return v
}

func parseNumSuffix(id string) (int, bool) {
	i := len(id) - 1
	for i >= 0 && id[i] >= '0' && id[i] <= '9' {
		i--
	}
	if i == len(id)-1 {
		return 0, false
	}
	n := 0
	for _, r := range id[i+1:] {
		n = n*10 + int(r-'0')
	}
	return n, true
}
