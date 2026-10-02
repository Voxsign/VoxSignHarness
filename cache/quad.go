// Package cache 实现「不再问」四元组缓存（M2 任务卡 #8 / 设计 v2 §14.1）。
//
// 命中语义：当 (意图, 域, 权限, 指代已消解) 四元组完全复现上次已确认的情形时，
// 直接复用上一次的决策（"不再问"），避免确认疲劳。策略/契约/域版本一变 → 全失效。
// 持久化为单个 JSON 文件，带 policy version 与 per-entry TTL。零第三方依赖。
package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// QuadKey 是四元组主键：意图 / 域 / 权限 / 指代（已消解）。四元全等才算同一情形。
type QuadKey struct {
	Intent string `json:"intent"`
	Space  string `json:"space"`
	Perm   string `json:"perm"`
	Ref    string `json:"ref"`
}

// Entry 是一条缓存的确认决策。Version = 写入时的 policy 版本（版本漂移即整条失效）。
type Entry struct {
	Key       QuadKey   `json:"key"`
	Decision  string    `json:"decision"`
	Version   int       `json:"version"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Store 是四元组缓存的持久句柄。Path 为 JSON 文件路径；Version 为当前 policy 版本；
// TTL 为每条 entry 的有效期。entries/mu 为实现细节（未导出，不破坏冻结 API 形状）。
type Store struct {
	Path    string        `json:"path"`
	Version int           `json:"version"`
	TTL     time.Duration `json:"ttl_seconds"`

	mu      sync.RWMutex
	entries map[QuadKey]Entry
}

// diskFormat 是落盘 JSON 形态。
type diskFormat struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

// Open 打开（或新建）四元组缓存。文件不存在 → 空缓存、Version=1；存在则恢复。
func Open(path string, ttl time.Duration) (*Store, error) {
	if filepath.Ext(path) == "" {
		return nil, fmt.Errorf("cache 路径应指向 .json 文件: %q", path)
	}
	s := &Store{Path: path, Version: 1, TTL: ttl, entries: map[QuadKey]Entry{}}
	data, err := os.ReadFile(path)
	if err == nil {
		var df diskFormat
		if jerr := json.Unmarshal(data, &df); jerr != nil {
			return nil, fmt.Errorf("解析缓存文件 %s 失败: %w", path, jerr)
		}
		s.Version = df.Version
		if s.Version < 1 {
			s.Version = 1
		}
		for _, e := range df.Entries {
			s.entries[e.Key] = e
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("读取缓存文件 %s 失败: %w", path, err)
	}
	return s, nil
}

// persist 把当前状态原子落盘（写临时文件再 rename，避免半截 JSON）。
func (s *Store) persist() error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return fmt.Errorf("创建缓存目录失败: %w", err)
	}
	df := diskFormat{Version: s.Version}
	for _, e := range s.entries {
		df.Entries = append(df.Entries, e)
	}
	data, err := json.MarshalIndent(df, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化缓存失败: %w", err)
	}
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("写缓存临时文件失败: %w", err)
	}
	if err := os.Rename(tmp, s.Path); err != nil {
		return fmt.Errorf("提交缓存文件失败: %w", err)
	}
	return nil
}

// Get 查四元组决策。
//
// 【伪代码逻辑层】（必写模块；命中/失效规则语义搬 VSL，此处只写控制流与拒绝路径）
//
// 控制流：
//  1. RLock；查 map[QuadKey]Entry。
//  2. 未命中 → ("", false)。
//  3. 命中后两道失效闸（任一不过即 miss，并惰性剔除该条）：
//     a. 版本闸：entry.Version != s.Version → 策略/契约/域版本变过，整条作废 → miss。
//     b. TTL 闸：now >= entry.ExpiresAt → 过期 → miss。
//  4. 两闸都过 → (entry.Decision, true)，pipeline 据此"不再问"。
//
// 异常：读盘失败不在这里（Open 已加载）；惰性剔除后不强制落盘（下次 Set/Bump 顺带持久化）。
func (s *Store) Get(k QuadKey) (string, bool) {
	s.mu.RLock()
	e, ok := s.entries[k]
	s.mu.RUnlock()
	if !ok {
		return "", false
	}
	if e.Version != s.Version {
		s.invalidate(k)
		return "", false
	}
	if !time.Now().Before(e.ExpiresAt) {
		s.invalidate(k)
		return "", false
	}
	return e.Decision, true
}

// invalidate 惰性删除单条（写锁）。
func (s *Store) invalidate(k QuadKey) {
	s.mu.Lock()
	delete(s.entries, k)
	s.mu.Unlock()
}

// Set 写入/覆盖一条四元组决策，带当前 policy 版本与 TTL，随后持久化。
//
// 【伪代码逻辑层】
//  1. Lock；now := time.Now()。
//  2. upsert：entries[k] = Entry{Key:k, Decision, Version:s.Version, ExpiresAt: now+TTL}。
//  3. persist() 落盘（原子 rename）。
//  4. 失败 → 返回错误（不静默丢）。
func (s *Store) Set(k QuadKey, decision string) error {
	s.mu.Lock()
	s.entries[k] = Entry{
		Key:       k,
		Decision:  decision,
		Version:   s.Version,
		ExpiresAt: time.Now().Add(s.TTL),
	}
	err := s.persist()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	return nil
}

// InvalidateSpace 作废某域的全部缓存（该域 manifest 漂移/重建时调用）。
//
// 【伪代码逻辑层】
//  1. Lock。
//  2. 遍历 entries：Key.Space == space → delete。
//  3. persist() 落盘。
func (s *Store) InvalidateSpace(space string) error {
	s.mu.Lock()
	for k := range s.entries {
		if k.Space == space {
			delete(s.entries, k)
		}
	}
	err := s.persist()
	s.mu.Unlock()
	return err
}

// BumpPolicyVersion 策略/契约/域版本变化 → 全失效。
//
// 【伪代码逻辑层】
//  1. Lock。
//  2. s.Version++（旧 entries 携带旧版本号，Get 的版本闸会把它们全部判为 miss）。
//  3. persist() 落盘新版本号（entries 保留但逻辑上全失效，便于审计）。
func (s *Store) BumpPolicyVersion() error {
	s.mu.Lock()
	s.Version++
	err := s.persist()
	s.mu.Unlock()
	return err
}
