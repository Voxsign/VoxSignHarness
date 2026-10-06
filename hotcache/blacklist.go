// blacklist.go -- useuser form"  modify "  state name (§5.1   5  ;  recv A10). 
//
// semantic(and rewrite_scope.go   state   patch): 
//   - rewrite_scope.go is**in  state** useword  :  useword routeby,   modifywrite; 
//   - basefileis**useuser   state** name : pt   modify  after,  word again and basemodifywrite; 
//   - **routeby accept  **(modifywriteandrouteby usewaysplit , see rewrite_scope.go headnote). 
//
// keep ize: JSON map(term ->   origbecause), L1   ; heavystartafter LoadBlacklist   . 
package hotcache

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Blacklist pipe term  inmodifywrite name and  . emptywordreject; heavy    etc(overwriteorigbecause). 
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

// Unblacklist      name ( etc; origbase store returnback false). 
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

// Blacklisted returnbackcurbefore name fast (term -> origbecause), provide  and data. 
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

// isBlacklisted    term is   name in(modifywriteuseway   need  ). 
func (c *Cache) isBlacklisted(term string) bool {
	s := c.state()
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.blacklist[term]
	return ok
}

// SetBlacklistPath    name   path(  periodcalluse). 
func (c *Cache) SetBlacklistPath(p string) { c.blacklistPath = p }

// BlacklistPath returnback name   path(keep ize datause). 
func (c *Cache) BlacklistPath() string { return c.blacklistPath }

// SaveBlacklist pipe name   (  obj     ; and Save same  ). 
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

// LoadBlacklist from     name (      --         , and K6 same  ). 
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
