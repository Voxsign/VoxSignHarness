// teach.go -- useuser** time  word**(VHS-CACHE-001 G2; owner orig "     word"). 
//
// and"serveservicediffname"  diff: **   serveservicenote table **,   isuseuserbase (user_taught). 
// because : ①   tgt  (   ); ②      L1 keep ( then" under then "); 
//
//	③    correctionpathon**  modifychange out**(  K9    data). 
package hotcache

import (
	"errors"
	"strings"
)

// SourceUserTaught isuseuser time  word   tgt (and remote/local  split,    ). 
const SourceUserTaught = "user_taught"

// Teach    word/   -> rule word. emptyvaluereject( emptyword pipecache  become"  all  in"). 
func (c *Cache) Teach(term, canonical string) error {
	term, canonical = strings.TrimSpace(term), strings.TrimSpace(canonical)
	if term == "" || canonical == "" {
		return errors.New("hotcache: Teach requires non-empty term and canonical")
	}
	if term == canonical {
		return errors.New("hotcache: term and canonical are identical, nothing to teach")
	}
	c.PutAlias(term, canonical, SourceUserTaught)
	return nil
}

// Taught returnback has**useuser ed** word(     ). 
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

// ClearTaught only **useuser time  word**(Source=user_taught), 
// **    serveservicediffname**( isfrom /api/services     end value). 
// returnback   num. 
func (c *Cache) ClearTaught() int {
	s := c.state()
	s.mu.Lock()
	defer s.mu.Unlock()
	var kept []Alias
	removed := 0
	for _, a := range s.aliases {
		if a.Source == SourceUserTaught {
			removed++
			continue
		}
		kept = append(kept, a)
	}
	s.aliases = kept
	return removed
}
