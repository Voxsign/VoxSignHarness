// registry.go —— 知己 · 元层 per-model 能力注册表（架构 v1.0 §6.4 / §08 model-registry 落地）。
//
// 按模型定制压缩与路由的数据基础（M1 前置组件，不依赖 GPU/网关）：
//   - 弱模型 → 保守压缩（少压多留骨架）
//   - 强模型 → 激进压缩（依赖检索回补）
// 持久化到 model_registry.json（JSON 文件，与 Store 同目录风格）。
package zhiji

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// CompressionDensity 压缩密度策略（架构 §6.4：弱保守 / 强激进）。
type CompressionDensity string

const (
	DensityConservative CompressionDensity = "conservative" // 弱模型：少压多留骨架
	DensityAggressive   CompressionDensity = "aggressive"   // 强模型：依赖检索回补
)

// ModelProfile 单模型能力画像（架构 §6.4 字段）。
type ModelProfile struct {
	ID                string             `json:"id"`
	NominalWindow     int                `json:"nominal_window"`      // 标称上下文窗口（token）
	EffectiveWindow   int                `json:"effective_window"`    // 实测有效窗口（token）
	LostInMiddle      float64            `json:"lost_in_middle"`      // Lost-in-middle 敏感度 0–1
	FormatPref        string             `json:"format_pref"`         // 格式偏好（json|markdown|xml…）
	InstructionFollow float64            `json:"instruction_follow"`  // 指令遵循强弱 0–1
	PriceClass        string             `json:"price_class"`         // 价格档（cheap|mid|premium）
	Strengths         []string           `json:"strengths"`           // 强项标签
	Density           CompressionDensity `json:"density"`             // 压缩密度策略
}

// Registry per-model 能力注册表（线程安全，JSON 持久化）。
type Registry struct {
	path      string
	mu        sync.RWMutex
	profiles  map[string]ModelProfile
	defaultID string
}

// NewRegistry 创建/加载注册表（dir 不存在自动创建）。
func NewRegistry(dir string) (*Registry, error) {
	if dir == "" {
		return nil, errors.New("zhiji: 注册表目录不能为空")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	r := &Registry{path: filepath.Join(dir, "model_registry.json"), profiles: map[string]ModelProfile{}}
	b, err := os.ReadFile(r.path)
	if err == nil {
		if err := json.Unmarshal(b, &r.profiles); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return r, nil
}

// Register 注册/更新模型画像。
func (r *Registry) Register(p ModelProfile) error {
	if p.ID == "" {
		return errors.New("zhiji: 模型 ID 不能为空")
	}
	if p.Density == "" {
		p.Density = DensityConservative // 未知模型默认保守压缩（不冒险丢信息）
	}
	r.mu.Lock()
	r.profiles[p.ID] = p
	if r.defaultID == "" {
		r.defaultID = p.ID
	}
	r.mu.Unlock()
	return r.save()
}

// SetDefault 设置默认模型（路由兜底）。
// 注意：save() 内部自取 RLock，此处必须先 Unlock 再 save——
// RWMutex 不可重入，持写锁再读锁即自死锁（与 Register() 的先 Unlock 后 save 同款）。
func (r *Registry) SetDefault(id string) error {
	r.mu.Lock()
	if _, ok := r.profiles[id]; !ok {
		r.mu.Unlock()
		return ErrNotFound
	}
	r.defaultID = id
	r.mu.Unlock()
	return r.save()
}

// Get 取单模型画像。
func (r *Registry) Get(id string) (ModelProfile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.profiles[id]
	if !ok {
		return ModelProfile{}, ErrNotFound
	}
	return p, nil
}

// Default 默认模型（路由兜底）。
func (r *Registry) Default() (ModelProfile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.defaultID == "" {
		return ModelProfile{}, ErrNotFound
	}
	return r.profiles[r.defaultID], nil
}

// List 全部画像（按 ID 排序，稳定输出）。
func (r *Registry) List() []ModelProfile {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ModelProfile, 0, len(r.profiles))
	for _, p := range r.profiles {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// CompressionPolicy 压缩密度策略（架构 §6.4：按模型定制压缩的输入）。
func (r *Registry) CompressionPolicy(id string) (CompressionDensity, error) {
	p, err := r.Get(id)
	if err != nil {
		return "", err
	}
	return p.Density, nil
}

func (r *Registry) save() error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, err := json.MarshalIndent(r.profiles, "", "  ")
	if err != nil {
		return err
	}
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, r.path)
}
