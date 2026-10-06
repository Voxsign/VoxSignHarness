// compress.go --    · onunder    (   v1.0 §6.4 / §08 compress / ADR-004  ly). 
//
//     (ADR-004): 
//   - keep decide need : objtgt/rule/ decide  (  )
//   - tool-result-clearing first : middle  close  first  
//   -  tail   need: its  baseby"keepfirsttail,  needmiddle"    
//   -    : taskbecome rate + detail survival rate(    rate)--    
// Phase 0  nowas  ity  (no  type); Phase 2 by    (LoRA   )   CompressFn. 
package zhiji

import (
	"context"
	"errors"
	"strings"
)

// CompressInput    in. 
type CompressInput struct {
	Full    string // finish onunder (trace/   list)
	Segment string // baseseg base(e.g.   N  to )
}

// CompressedContext   artifact(decide need origkindkeep ). 
type CompressedContext struct {
	Goals          []string `json:"goals"`            // objtgt (decide need ,   )
	Rules          []string `json:"rules"`            // rule (decide need ,   )
	Pending        []string `json:"pending"`          //  decide  (decide need ,   )
	Summary        string   `json:"summary"`          //   after  body need
	DroppedToolResult int   `json:"dropped_tool_result"` //      close  num
	DetailSurvival float64  `json:"detail_survival"`  //    backrate(   )
}

// CompressFn     body(Phase 2 by      ; signature change). 
type CompressFn func(ctx context.Context, in CompressInput) (CompressedContext, error)

// Compressor onunder    . 
type Compressor struct {
	store     *Store
	MaxSummaryTokens int
	Fn        CompressFn
}

// NewCompressor      (default  ity now). 
func NewCompressor(store *Store) *Compressor {
	c := &Compressor{store: store, MaxSummaryTokens: 4000}
	c.Fn = c.defaultCompress
	return c
}

// Compress     (     ->    ->  back  ). 
func (c *Compressor) Compress(ctx context.Context, in CompressInput) (CompressedContext, error) {
	if err := ctx.Err(); err != nil {
		return CompressedContext{}, err
	}
	out, err := c.Fn(ctx, in)
	if err != nil {
		return out, err
	}
	//    back:   afterto   node       (survival rate    ). 
	if c.store != nil {
		// v1.1   :   artifact ifoutnow      KeyDetail -> tgt  back. 
		//    nowonlyadd  ,  modifychange has   out; P1  become BudgetSearch  form back decayed   . 
		c.markProbeHits(out.Summary)
		out.DetailSurvival = c.store.SurvivalRate()
	}
	return out, nil
}

// markProbeHits    :   after base outnow     KeyDetail -> tgt    already back. 
// only    Recalled status,  modify   out; firstfast  probes againcall ProbeRecall(   RLock->Lock   ). 
func (c *Compressor) markProbeHits(compressed string) {
	if compressed == "" || c.store == nil {
		return
	}
	c.store.mu.RLock()
	probes := append([]SurvivalProbe(nil), c.store.probes...)
	c.store.mu.RUnlock()
	for _, p := range probes {
		if p.Recalled || p.KeyDetail == "" {
			continue
		}
		if containsFold(compressed, p.KeyDetail) {
			c.store.ProbeRecall(p.ID)
		}
	}
}

// defaultCompress Phase 0   ity  : 
//  1.  get [goal]/[rule]/[pending] decide need (origkindkeep )
//  2. tool-result   body  (tgt  DroppedToolResult)
//  3. its  basekeepfirsttail, inseg need(by token    disconnect + keep close )
func (c *Compressor) defaultCompress(ctx context.Context, in CompressInput) (CompressedContext, error) {
	if err := ctx.Err(); err != nil {
		return CompressedContext{}, err
	}
	var out CompressedContext
	lines := strings.Split(in.Full+"\n"+in.Segment, "\n")
	var rest []string
	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		switch {
		case strings.HasPrefix(trimmed, "[goal]"):
			out.Goals = append(out.Goals, strings.TrimPrefix(trimmed, "[goal]"))
		case strings.HasPrefix(trimmed, "[rule]"):
			out.Rules = append(out.Rules, strings.TrimPrefix(trimmed, "[rule]"))
		case strings.HasPrefix(trimmed, "[pending]"):
			out.Pending = append(out.Pending, strings.TrimPrefix(trimmed, "[pending]"))
		case strings.Contains(trimmed, "tool-result") || strings.HasPrefix(trimmed, "tool_result:"):
			out.DroppedToolResult++ // tool-result-clearing:   ,   in need
		default:
			if trimmed != "" {
				rest = append(rest, trimmed)
			}
		}
	}
	//  tail   need: keepfirsttail,  needmiddle(token   bychar    4 char ~=1 token). 
	budget := c.MaxSummaryTokens * 4
	head := rest
	if len(rest) > 200 { //  out num -> keep first 100   + tail 50  , middle  as   need
		head = append(append([]string{}, rest[:100]...), rest[len(rest)-50:]...)
	}
	joined := strings.Join(head, "\n")
	if len(joined) > budget {
		joined = joined[:budget] + "\n…[中段已递归摘要]"
	}
	out.Summary = joined
	if len(out.Goals)+len(out.Rules)+len(out.Pending) == 0 && out.Summary == "" {
		return out, errors.New("zhiji: 无可压缩内容")
	}
	return out, nil
}
