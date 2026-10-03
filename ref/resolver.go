// resolver.go —— 代指三层（VHS-ZHIJI-001 R1–R8）。
//
//	① 会话内（本轮说过什么）
//	② 工作记忆（四块板；依赖 P2）
//	③ 知己（**只读**视图；key 从环境读，写回未实现）
//
// 硬判据：**唯一才消解**；多候选/无候选/低置信 ⇒ **回问**（且**必须给候选**）；
// **每次消解留依据**；**来源冲突 ⇒ 中断**；**禁止脑补**。
package ref

import (
	"context"
	"sort"
	"strings"
	"sync"

	"voicesign-harness/plan"
)

// Evidence 是一条消解依据（R：每次消解留依据）。
type Evidence struct {
	Layer  string `json:"layer"` // session | working_memory | zhiji
	Detail string `json:"detail"`
}

// Resolution 是一次代指消解结果。
type Resolution struct {
	Mention    string
	Canonical  string
	Ok         bool     // 唯一命中才为 true
	AskBack    bool     // 需要回问用户
	Candidates []string // **回问必须给候选**（ASK-3b）
	Evidence   []Evidence
	Conflict   bool // 来源冲突 ⇒ 中断
	Reason     string
}

// SessionMemory 是第①层：会话内提及（谁在本轮/近几轮说过）。
type SessionMemory struct {
	mu    sync.Mutex
	items map[string][]string // mention → 候选（按出现顺序去重）
}

// Mention 记一次提及。
func (s *SessionMemory) Mention(mention, canonical string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.items == nil {
		s.items = map[string][]string{}
	}
	for _, c := range s.items[mention] {
		if c == canonical {
			return
		}
	}
	s.items[mention] = append(s.items[mention], canonical)
}

// Candidates 返回该提及在会话内的候选。
func (s *SessionMemory) Candidates(mention string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.items[mention]...)
}

// Resolver 串起三层。
type Resolver struct {
	Session *SessionMemory
	WM      *plan.WorkingMemory
	Zhiji   ZhijiSource
}

// Resolve 消解一个代指。
func (r *Resolver) Resolve(ctx context.Context, mention string) Resolution {
	res := Resolution{Mention: mention}
	var layers []struct {
		layer string
		cands []string
		ev    Evidence
	}

	if r.Session != nil {
		if cs := r.Session.Candidates(mention); len(cs) > 0 {
			sort.Strings(cs)
			layers = append(layers, struct {
				layer string
				cands []string
				ev    Evidence
			}{"session", cs, Evidence{Layer: "session", Detail: "会话内提及：" + strings.Join(cs, ",")}})
		}
	}
	if r.WM != nil {
		if c, ok := r.WM.Resolve(mention); ok {
			layers = append(layers, struct {
				layer string
				cands []string
				ev    Evidence
			}{"working_memory", []string{c}, Evidence{Layer: "working_memory", Detail: "工作记忆活跃实体命中：" + c}})
		}
	}
	if r.Zhiji != nil {
		if evs, err := r.Zhiji.Events(ctx); err == nil {
			seen := map[string]bool{}
			var cs []string
			for _, e := range evs {
				if strings.Contains(e.Text, mention) || strings.Contains(e.Text, strings.TrimSuffix(mention, "那个")) {
					key := e.Source
					if key == "" {
						key = e.ThreadID
					}
					if key != "" && !seen[key] {
						seen[key] = true
						cs = append(cs, key)
					}
				}
			}
			if len(cs) > 0 {
				sort.Strings(cs)
				layers = append(layers, struct {
					layer string
					cands []string
					ev    Evidence
				}{"zhiji", cs, Evidence{Layer: "zhiji", Detail: "知己只读命中 " + strings.Join(cs, ",")}})
			}
		}
	}

	if len(layers) == 0 {
		res.AskBack = true
		res.Reason = "三层都无候选：回问用户（不脑补）"
		return res
	}
	// 收集各层候选并做冲突检测
	for _, l := range layers {
		res.Evidence = append(res.Evidence, l.ev)
		for _, c := range l.cands {
			res.Candidates = append(res.Candidates, c)
		}
	}
	res.Candidates = dedupSorted(res.Candidates)

	// 来源冲突：不同层给出**不同**的候选 ⇒ 中断，交人裁决
	if len(layers) > 1 {
		first := strings.Join(layers[0].cands, "|")
		for _, l := range layers[1:] {
			if strings.Join(l.cands, "|") != first {
				res.Conflict = true
				res.AskBack = true
				res.Reason = "来源冲突（" + layers[0].layer + " vs " + l.layer + "）⇒ 中断，交人裁决"
				return res
			}
		}
	}
	if len(res.Candidates) == 1 {
		res.Canonical = res.Candidates[0]
		res.Ok = true
		res.Reason = "唯一候选才消解"
		return res
	}
	res.AskBack = true
	res.Reason = "多候选（" + itoa(len(res.Candidates)) + " 个）⇒ 回问用户，并给出候选"
	return res
}

func dedupSorted(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
