// contract.go --    · and typein      (   v1.0 §07/§08  ly). 
//
//  class  : invoke(decide timenotein/  ),    (MCP formreadwrite), 
// feedback(trace/close write-back), events(eventsignal,  period  ). 
// connect semanticand   §08 connect list  to (inject-baseline / retrieve-context /
// read_awareness_cache / update_self_model / write_long_term_memory /
// log-trajectory / log-feedback / evict). 
package zhiji

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// notein/    (   §07        ). 
const (
	BaselineMaxTokens = 1200 // baseline onlimit(<=800–1200 token   )
	BaselineMinTokens = 800
	RetrieveTopK      = 8 //  restrict/ as    top-k
)

// Baseline decide timenotein baseline (objtgt+rule   +   patchfill). 
type Baseline struct {
	Goals      string   `json:"goals"`      // objtgt  base
	Rules      string   `json:"rules"`      // rule  base
	Retrieved  []string `json:"retrieved"`  //  restrict/ as  top-k  base
	TokenBudget int     `json:"token_budget"` //        show(Claude context awareness form)
}

// Contract     connect (to typein /      safety   ). 
type Contract interface {
	// notein 
	InjectBaseline(ctx context.Context, query string) (*Baseline, error)
	RetrieveContext(ctx context.Context, query string, k int) ([]MemoryItem, error)

	//    (MCP form)
	ReadAwarenessCache(ctx context.Context, layer Layer, query string) ([]SelfItem, []MemoryItem, error)
	UpdateSelfModel(ctx context.Context, item SelfItem) (SelfItem, error)
	WriteLongTermMemory(ctx context.Context, item MemoryItem) (MemoryItem, error)
	Evict(ctx context.Context, domain Domain) (int, error)

	// rev  
	LogTrajectory(ctx context.Context, log CallLog) error
	LogFeedback(ctx context.Context, taskID, outcome string, confidence float64) error
}

// zhijiContract default now(processin,   sec  ). 
type zhijiContract struct {
	store *Store
	log   *CallLogStore
}

// NewContract      now(store andday storestore  alreadyinitstartize). 
func NewContract(store *Store, log *CallLogStore) Contract {
	return &zhijiContract{store: store, log: log}
}

// InjectBaseline   decide timenotein : objtgt+rule   + curbefore query  close restrict/ as top-k. 
// processin  (    end: p50~=40µs   ,   sec). 
func (c *zhijiContract) InjectBaseline(ctx context.Context, query string) (*Baseline, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	base := c.store.Baseline()
	var goals, rules strings.Builder
	for _, line := range strings.Split(base, "\n") {
		if strings.HasPrefix(line, "[goal]") {
			goals.WriteString(strings.TrimPrefix(line, "[goal] "))
			goals.WriteString("\n")
		} else if strings.HasPrefix(line, "[rule]") {
			rules.WriteString(strings.TrimPrefix(line, "[rule] "))
			rules.WriteString("\n")
		}
	}
	items := c.store.Search(query, RetrieveTopK, time.Now())
	retrieved := make([]string, 0, len(items))
	for _, it := range items {
		retrieved = append(retrieved, it.Text)
	}
	//     : baseline    (4 char ~=1 token   ,   ). 
	budget := BaselineMaxTokens - (len(goals.String())+len(rules.String()))/4
	if budget < 0 {
		budget = 0
	}
	return &Baseline{
		Goals:       strings.TrimSpace(goals.String()),
		Rules:       strings.TrimSpace(rules.String()),
		Retrieved:   retrieved,
		TokenBudget: budget,
	}, nil
}

// RetrieveContext JIT   ( restrict/ as  because  top-k). 
func (c *zhijiContract) RetrieveContext(ctx context.Context, query string, k int) ([]MemoryItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if k <= 0 {
		k = RetrieveTopK
	}
	items := c.store.Search(query, k, time.Now())
	if len(items) == 0 {
		return nil, ErrNotFound
	}
	return items, nil
}

// ReadAwarenessCache by /query read  cache(   ). 
func (c *zhijiContract) ReadAwarenessCache(ctx context.Context, layer Layer, query string) ([]SelfItem, []MemoryItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	self := c.store.SelfModel(layer)
	items := c.store.Search(query, RetrieveTopK, time.Now())
	return self, items, nil
}

// UpdateSelfModel modifyobjtgt/rule/ restrict (ADD/UPDATE, superseded,  base add). 
func (c *zhijiContract) UpdateSelfModel(ctx context.Context, item SelfItem) (SelfItem, error) {
	if err := ctx.Err(); err != nil {
		return SelfItem{}, err
	}
	if strings.TrimSpace(item.Text) == "" {
		return SelfItem{}, errors.New("zhiji: self model 文本不能为空")
	}
	return c.store.UpsertSelf(item)
}

// WriteLongTermMemory write as   /  (writebefore  / heavyby reflect    ). 
func (c *zhijiContract) WriteLongTermMemory(ctx context.Context, item MemoryItem) (MemoryItem, error) {
	if err := ctx.Err(); err != nil {
		return MemoryItem{}, err
	}
	if strings.TrimSpace(item.Text) == "" {
		return MemoryItem{}, errors.New("zhiji: 记忆文本不能为空")
	}
	return c.store.WriteMemory(item)
}

// Evict   (   §12:        ;   domain    byon handle). 
func (c *zhijiContract) Evict(ctx context.Context, domain Domain) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	n := c.store.DecayAndEvict(time.Now(), 0)
	if err := c.store.SaveAll(); err != nil {
		return n, err
	}
	return n, nil
}

// LogTrajectory tasktracesafety write-back(diff , approve ,     --call byon responsible). 
func (c *zhijiContract) LogTrajectory(ctx context.Context, log CallLog) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.log.Append(log)
}

// LogFeedback outcome signal(   split write). 
func (c *zhijiContract) LogFeedback(ctx context.Context, taskID, outcome string, confidence float64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if taskID == "" {
		return errors.New("zhiji: taskID 不能为空")
	}
	return c.log.Append(CallLog{
		ID:         fmt.Sprintf("fb-%d", time.Now().UnixNano()),
		At:         time.Now(),
		TaskProfile: taskID,
		Outcome:    outcome,
		Cost:       0,
	})
}
