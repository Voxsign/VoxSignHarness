// entity.go -- 命名实体记忆层。
//
// 不只是"用户自己叫什么"，而是所有用户提到的人/项目/公司/地点：
//   "冀总" → Amber，consultant 客户
//   "Peter" → 商业伙伴
//   "<ALIAS>" → 用户 <USER> 自己
//   "那个沙特项目" → 具体哪个项目
//
// 架构：
//   1. EntityStore 持久化所有实体（canonical name + aliases + description）
//   2. OnInput 时识别代指模式（"X总""X先生""那个X"），查 store resolve
//   3. 未见过的代指 → 标记，问用户"X是谁？"
//   4. BeforeDecision 把相关实体描述注入 LLM context
package zhiji

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type EntityType string

const (
	EntityPerson    EntityType = "person"
	EntityProject   EntityType = "project"
	EntityCompany   EntityType = "company"
	EntityPlace     EntityType = "place"
)

type Entity struct {
	ID         string     `json:"id"`
	Type       EntityType `json:"type"`
	Canonical  string     `json:"canonical"`           // 标准名（"Amber"）
	Aliases    []string   `json:"aliases"`             // 所有代指（["冀总","老冀","Amber"]）
	Desc       string     `json:"desc,omitempty"`      // 描述（"consultant 客户"）
	Confidence float64    `json:"confidence"`
	Status     string     `json:"status"`           // active/superseded
	Source     string     `json:"source,omitempty"` // R9-D8 知己多源: voice-local|external-hub|longwall
}

type EntityStore struct {
	mu      sync.RWMutex
	dir     string
	entities []Entity
	nextID  int
}

func NewEntityStore(dir string) (*EntityStore, error) {
	s := &EntityStore{dir: dir, nextID: 1}
	b, err := os.ReadFile(filepath.Join(dir, "entities.json"))
	if err == nil {
		_ = json.Unmarshal(b, &s.entities)
		for _, e := range s.entities {
			if e.ID >= "e-0" {
				// rough nextID
			}
		}
	}
	s.nextID = len(s.entities) + 1
	return s, nil
}

// save 不加锁——调用者必须已持锁。
func (s *EntityStore) save() error {
	b, _ := json.MarshalIndent(s.entities, "", "  ")
	return os.WriteFile(filepath.Join(s.dir, "entities.json"), b, 0o600)
}

// Resolve 把一个代指 resolve 到实体。先查 canonical，再查 aliases。
// 支持同音字："季总"和"冀总"近音 → 同一个人。
func (s *EntityStore) Resolve(mention string) *Entity {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.entities {
		e := &s.entities[i]
		if e.Status != "active" {
			continue
		}
		if e.Canonical == mention {
			return e
		}
		for _, a := range e.Aliases {
			if a == mention {
				return e
			}
		}
		// 同音字代指："季总"≈"冀总"
		if isHomophone([]rune(mention)[0], []rune(e.Canonical)[0]) && len([]rune(mention)) == len([]rune(e.Canonical)) {
			return e
		}
	}
	return nil
}

// Upsert 新增/更新实体。同 canonical 名合并。
func (s *EntityStore) Upsert(e Entity) Entity {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.entities {
		if s.entities[i].Status == "active" && s.entities[i].Canonical == e.Canonical {
			s.entities[i].Desc = e.Desc
			s.entities[i].Aliases = appendUnique(s.entities[i].Aliases, e.Aliases...)
			s.entities[i].Confidence = e.Confidence
			_ = s.save()
			return s.entities[i]
		}
	}
	e.ID = sprintf("e-%d", s.nextID)
	s.nextID++
	if e.Status == "" {
		e.Status = "active"
	}
	s.entities = append(s.entities, e)
	_ = s.save()
	return e
}

func appendUnique(slice []string, items ...string) []string {
	for _, it := range items {
		found := false
		for _, s := range slice {
			if s == it {
				found = true
				break
			}
		}
		if !found {
			slice = append(slice, it)
		}
	}
	return slice
}

func sprintf(format string, args ...interface{}) string {
	return fmt.Sprintf(format, args...)
}

// AllActive 返回所有 active 实体（用于注入 context）。
func (s *EntityStore) AllActive() []Entity {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Entity
	for _, e := range s.entities {
		if e.Status == "active" {
			out = append(out, e)
		}
	}
	return out
}
