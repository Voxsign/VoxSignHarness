// model.go —— 知己 · 意识缓存数据模型（架构 v1.0 §12 字段级落地）。
//
// 四层自我模型（目标/规则/机制/行为）+ 记忆条目（STM/LTM）+ 调用日志。
// 字段与架构 §12 一致：version/superseded_by/confidence/source_trajectory，
// 三因子 importance/recency/relevance，域 domain(user|session|agent)。
package zhiji

import (
	"encoding/json"
	"time"
)

// Layer 自我模型四层（架构 §6.1）。
type Layer string

const (
	LayerGoal      Layer = "goal"      // 目标层：当前主目标、长期意图、优先级
	LayerRule      Layer = "rule"      // 规则层：操作约束、价值观、边界、不可变内核
	LayerMechanism Layer = "mechanism" // 机制层：被验证有效的策略/技能
	LayerBehavior  Layer = "behavior"  // 行为层：用户画像、行为教训、失败案例
)

// Domain 记忆访问域（架构 §12：user/session/agent 三档）。
type Domain string

const (
	DomainUser    Domain = "user"    // 跨会话用户级（最持久）
	DomainSession Domain = "session" // 单会话内
	DomainAgent   Domain = "agent"   // 系统自身演进
)

// Status 记忆条目状态（架构 §12：active|superseded|decayed）。
type Status string

const (
	StatusActive     Status = "active"
	StatusSuperseded Status = "superseded" // 被新版本替代，不静默覆盖
	StatusDecayed    Status = "decayed"    // 遗忘降权（可恢复，不硬删）
)

// SelfItem 一条自我模型条目（架构 §12 字段级）。
type SelfItem struct {
	ID              string    `json:"id"`
	Layer           Layer     `json:"layer"`
	Text            string    `json:"text"`
	Version         int       `json:"version"`
	SupersededBy    string    `json:"superseded_by,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
	SourceTrajectory string   `json:"source_trajectory,omitempty"` // 来源轨迹引用（可审计）
	Confidence      float64   `json:"confidence"`                  // 0–1，外部信号分级
}

// MemoryItem 一条记忆条目（STM 热区 / LTM 归档共用）。
type MemoryItem struct {
	ID         string    `json:"id"`
	Text       string    `json:"text"`
	Layer      Layer     `json:"layer,omitempty"` // 机制/行为层归属
	Domain     Domain    `json:"domain"`
	Status     Status    `json:"status"`
	Importance float64   `json:"importance"` // 1–10
	CreatedAt  time.Time `json:"created_at"`
	LastSeen   time.Time `json:"last_seen"`
	Hash       string    `json:"hash,omitempty"` // 写前验证去重用
	Source     string    `json:"source,omitempty"`
}

// Relevance 计算当前 query 的相关性（架构 §12：三因子打分输入之一）。
// 由 contract.go 的检索侧调用；此处提供字段级默认实现（token 重叠 + 关键词命中）。
func (m *MemoryItem) Relevance(query string) float64 {
	if query == "" || m.Text == "" {
		return 0
	}
	// 极简重叠度：query 中的连续词在 text 中出现的比例（0–1）。
	qWords := splitWords(query)
	if len(qWords) == 0 {
		return 0
	}
	hit := 0
	for _, w := range qWords {
		if containsFold(m.Text, w) {
			hit++
		}
	}
	return float64(hit) / float64(len(qWords))
}

// CallLog 一条调用日志（架构 §12：全量埋点，自举训练集原料）。
type CallLog struct {
	ID          string    `json:"id"`
	At          time.Time `json:"at"`
	TaskProfile string    `json:"task_profile"`
	Model       string    `json:"model"`
	Outcome     string    `json:"outcome"` // success|failed|retried|judge
	Cost        float64   `json:"cost"`    // 相对成本（token 估算）
	Retry       int       `json:"retry"`
	DetailProbe string    `json:"detail_probe,omitempty"` // detail survival 探针文本
}

// SurvivalProbe detail survival 探针（架构 §12：预埋低显著关键细节，compaction 后测召回）。
type SurvivalProbe struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	KeyDetail string    `json:"key_detail"` // 预埋的低显著关键细节
	Slot      string    `json:"slot"`       // 探针位（如 "call:retry:reason"）
	Recalled  bool      `json:"recalled"`   // compaction 后是否仍可召回
}

// Marshal 统一 JSON 序列化入口（日志/存储共用）。
func (c *CallLog) Marshal() []byte {
	b, _ := json.Marshal(c)
	return b
}

func splitWords(s string) []string {
	var out []string
	var cur []rune
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '，' || r == '。' || r == '、' || r == '？' || r == '！' {
			if len(cur) > 0 {
				out = append(out, string(cur))
				cur = nil
			}
			continue
		}
		cur = append(cur, r)
	}
	if len(cur) > 0 {
		out = append(out, string(cur))
	}
	return out
}

func containsFold(s, sub string) bool {
	return len(sub) > 0 && containsFoldImpl(s, sub)
}

// containsFoldImpl 大小写不敏感的包含判断（中文不受影响，英文词根宽松匹配）。
func containsFoldImpl(s, sub string) bool {
	ls, lsub := toLower(s), toLower(sub)
	return len(ls) >= len(lsub) && containsSeq(ls, lsub)
}

func toLower(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		out = append(out, r)
	}
	return string(out)
}

func containsSeq(s, sub string) bool {
	rs, rsub := []rune(s), []rune(sub)
	for i := 0; i+len(rsub) <= len(rs); i++ {
		eq := true
		for j := range rsub {
			if rs[i+j] != rsub[j] {
				eq = false
				break
			}
		}
		if eq {
			return true
		}
	}
	return false
}

