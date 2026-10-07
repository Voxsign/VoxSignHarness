// mention.go -- 代指 resolve 的概率评分层。
//
// 不写死"那个=X"，而是：
//   1. RecentMentions：最近 N 轮对话提到过的实体（时间衰减）
//   2. 代指模式："那个""这个""最近""刚才""前天""上次"
//   3. 打分：时间衰减 × 实体相似度 × LLM 判断
//   4. top1 高分就 resolve，top1/top2 接近就反问用户
package zhiji

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Mention struct {
	EntityCanonical string    `json:"entity"`
	MentionedAt     time.Time `json:"at"`
}

type MentionTracker struct {
	mu      sync.RWMutex
	dir     string
	recent  []Mention
}

func NewMentionTracker(dir string) *MentionTracker {
	t := &MentionTracker{dir: dir}
	b, err := os.ReadFile(filepath.Join(dir, "mentions.jsonl"))
	if err == nil {
		// 读最后 50 条
		lines := splitLines(string(b))
		start := len(lines) - 50
		if start < 0 {
			start = 0
		}
		for _, line := range lines[start:] {
			if line == "" {
				continue
			}
			var m Mention
			if json.Unmarshal([]byte(line), &m) == nil {
				t.recent = append(t.recent, m)
			}
		}
	}
	return t
}

func splitLines(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == '\n' {
			out = append(out, cur)
			cur = ""
		} else {
			cur += string(r)
		}
	}
	out = append(out, cur)
	return out
}

// Record 记录一次实体提及。
func (t *MentionTracker) Record(canonical string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.recent = append(t.recent, Mention{EntityCanonical: canonical, MentionedAt: time.Now()})
	// 只留最近 100 条
	if len(t.recent) > 100 {
		t.recent = t.recent[len(t.recent)-100:]
	}
	// append-only 落盘
	f, _ := os.OpenFile(filepath.Join(t.dir, "mentions.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	defer f.Close()
	b, _ := json.Marshal(Mention{EntityCanonical: canonical, MentionedAt: time.Now()})
	f.Write(append(b, '\n'))
}

// Score 返回每个实体的代指得分（0-1）。
// 时间衰减：最近提到的得分高，超过 7 天的得分趋近 0。
func (t *MentionTracker) Score(canonicals []string) map[string]float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	scores := map[string]float64{}
	now := time.Now()
	for _, c := range canonicals {
		scores[c] = 0
	}
	for _, m := range t.recent {
		hours := now.Sub(m.MentionedAt).Hours()
		// 指数衰减：1小时内=1.0，24小时=0.5，7天=0.05
		decay := math.Exp(-hours / 24)
		if _, ok := scores[m.EntityCanonical]; ok {
			scores[m.EntityCanonical] += decay
		}
	}
	return scores
}

// TopCandidates 返回得分最高的 topN 个实体。
func (t *MentionTracker) TopCandidates(canonicals []string, topN int) []string {
	scores := t.Score(canonicals)
	type kv struct {
		k string
		v float64
	}
	var arr []kv
	for k, v := range scores {
		arr = append(arr, kv{k, v})
	}
	sort.Slice(arr, func(i, j int) bool { return arr[i].v > arr[j].v })
	if len(arr) > topN {
		arr = arr[:topN]
	}
	var out []string
	for _, a := range arr {
		if a.v > 0 {
			out = append(out, a.k)
		}
	}
	return out
}

// IsDemonstrative 判断这句话是不是在用代指（"那个""这个""最近""刚才""前天"）。
func IsDemonstrative(text string) bool {
	for _, kw := range []string{"那个", "这个", "最近", "刚才", "前天", "上次", "之前说的", "刚说的"} {
		if containsRune(text, kw) {
			return true
		}
	}
	return false
}
