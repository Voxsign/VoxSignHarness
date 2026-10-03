// blacklist.go —— 用户显式「这个改错了」的动态黑名单（§5.1 第 5 件；验收 A10）。
//
// 语义（与 rewrite_scope.go 的静态登记互补）：
//   - rewrite_scope.go 是**内置静态**通用词登记：通用词可路由、不可改写；
//   - 本文件是**用户驱动动态**黑名单：点〔这个改错了〕后，该词不再参与文本改写；
//   - **路由不受影响**（改写与路由两用途分离，见 rewrite_scope.go 头注）。
//
// 持久化：JSON map（term → 登记原因），L1 风格；重启后 LoadBlacklist 恢复。
package hotcache

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Blacklist 把 term 加入改写黑名单并落盘。空词拒绝；重复登记幂等（覆盖原因）。
func (c *Cache) Blacklist(term, note string) error {
	term = strings.TrimSpace(term)
	if term == "" {
		return errors.New("hotcache: Blacklist 需要非空的 term")
	}
	s := c.state()
	s.mu.Lock()
	if s.blacklist == nil {
		s.blacklist = map[string]string{}
	}
	s.blacklist[term] = strings.TrimSpace(note)
	s.mu.Unlock()
	return c.SaveBlacklist()
}

// Unblacklist 撤销一条黑名单（幂等；原本不存在返回 false）。
func (c *Cache) Unblacklist(term string) bool {
	s := c.state()
	s.mu.Lock()
	_, ok := s.blacklist[term]
	if ok {
		delete(s.blacklist, term)
	}
	s.mu.Unlock()
	if ok {
		_ = c.SaveBlacklist()
	}
	return ok
}

// Blacklisted 返回当前黑名单快照（term → 原因），供审计与判据。
func (c *Cache) Blacklisted() map[string]string {
	s := c.state()
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]string, len(s.blacklist))
	for k, v := range s.blacklist {
		out[k] = v
	}
	return out
}

// isBlacklisted 报告 term 是否在黑名单中（改写用途的查询要拦它）。
func (c *Cache) isBlacklisted(term string) bool {
	s := c.state()
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.blacklist[term]
	return ok
}

// SetBlacklistPath 设置黑名单落盘路径（装配期调用）。
func (c *Cache) SetBlacklistPath(p string) { c.blacklistPath = p }

// BlacklistPath 返回黑名单落盘路径（持久化判据用）。
func (c *Cache) BlacklistPath() string { return c.blacklistPath }

// SaveBlacklist 把黑名单落盘（缺失目录自动创建；与 Save 同纪律）。
func (c *Cache) SaveBlacklist() error {
	if c.blacklistPath == "" {
		return nil
	}
	bl := c.Blacklisted()
	b, err := json.MarshalIndent(bl, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(c.blacklistPath); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	return os.WriteFile(c.blacklistPath, b, 0o600)
}

// LoadBlacklist 从磁盘恢复黑名单（缺失不报错 —— 缺失能降级不崩溃，与 K6 同纪律）。
func (c *Cache) LoadBlacklist() error {
	if c.blacklistPath == "" {
		return nil
	}
	b, err := os.ReadFile(c.blacklistPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	m := map[string]string{}
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	s := c.state()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blacklist == nil {
		s.blacklist = map[string]string{}
	}
	for k, v := range m {
		s.blacklist[k] = v
	}
	return nil
}
