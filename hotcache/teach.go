// teach.go —— 用户**临时教的词**（VHS-CACHE-001 G2；Peter 原话「最近我说的词」）。
//
// 与"服务别名"的区别：**它不在服务注册表里**，来源是用户本身（user_taught）。
// 因此：① 必须标来源（可审计）；② 必须能落 L1 持久（否则"教了下次就忘"）；
//
//	③ 必须在纠错路径上**真的改变输出**（照 K9 三条判据）。
package hotcache

import (
	"errors"
	"strings"
)

// SourceUserTaught 是用户临时教的词的来源标记（与 remote/local 区分，可审计）。
const SourceUserTaught = "user_taught"

// Teach 教一个词/说法 → 规范词。空值拒绝（教空词会把缓存污染成"什么都能命中"）。
func (c *Cache) Teach(term, canonical string) error {
	term, canonical = strings.TrimSpace(term), strings.TrimSpace(canonical)
	if term == "" || canonical == "" {
		return errors.New("hotcache: Teach 需要非空的 term 与 canonical")
	}
	if term == canonical {
		return errors.New("hotcache: term 与 canonical 相同，无需教")
	}
	c.PutAlias(term, canonical, SourceUserTaught)
	return nil
}

// Taught 返回所有**用户教过**的词（来源可审计）。
func (c *Cache) Taught() []Alias {
	s := c.state()
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Alias
	for _, a := range s.aliases {
		if a.Source == SourceUserTaught {
			out = append(out, a)
		}
	}
	return out
}
