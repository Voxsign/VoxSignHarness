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
	lastInputAt time.Time // 输入门控信号（有输入才更新）
	importance   float64   // 当前累计重要性（触发深反思阈值判断）
	nextID       int
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
	return writeJSON(filepath.Join(s.dir, "probes.json"), s.probes)
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

// Search 三因子打分检索（架构 §12：w1·Recency + w2·Importance + w3·Relevance）。
// 检索范围：LTM（行为/机制层）+ STM 热区；进程内，微秒级。
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
			rec := math.Pow(0.5, float64(now.Sub(it.LastSeen))/float64(RecencyHalfLife))
			rel := it.Relevance(query)
			score := WRecency*rec + WImportance*clampImportance(it.Importance)/10 + WRelevance*rel
			scored = append(scored, struct {
				it  MemoryItem
				rec float64
			}{it, score})
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
