// Package ground is M3 #37"     context notein"  now: 
//   task typecalluse / clarificationofbefore, pipe classnumdata   become seg  onunder  : 
//   - project-map: space note table(note domainname +  objrootobj filelist)
//   - decisions: <log_dir>/decisions.jsonl(  confirm decide: auto/light/strong/human +   /reject +  by)
//
//   invariant(SPEC v2 §2.37): 
//   -    as: numdata file store /asempty -> empty context,    , Ask    . 
//   -   limitrestrict:    charnodeonlimit + decide  objonlimit,  out disconnect  (   +    env). 
//   - read-onlynotein: ground only[read]    give type/ ;   revwriteword /  /risk value. 
//   - numdata onlyapproveuse harness     <log_dir>,     ~/.voicesign useuser  file. 
package ground

// [pseudocode logic layer]( writemodule: notein    disconnectclass  ; rule semantics  VSL,  placeonlywritecontrol flow)
//
// Render() -> Snapshot: 
//   pm =    project-map: 
//        for name in spaces.List():
//           m = spaces.Get(name)
//           line = "project-map:" + name + "(" + m.Type + ")"
//           for scope in m.Scope: line += " [" + scope + obj before 8  filelist + "]"
//   ds = ReadDecisions(<log_dir>/decisions.jsonl)   //    -> empty  
//   ds =  disconnect: if len(ds) > MaxDecisions: ds = ds[len-MaxDecisions:]
//   block = "    (    before,    disconnect): \n"
//         + join(pm, "\n")
//         + "\n period decide: \n" + join(ds.render(), "\n")
//   if len(block bytes) > MaxBytes: bycharnode tailand  "…( disconnect)"
//   return Snapshot{ProjectMap: pm, Decisions: ds, Block: block}
//
// RecordDecision(d) -> error: 
//    open(O_APPEND|O_CREATE)<log_dir>/decisions.jsonl(0o600)
//    listize d    -> write ;     disconnectread-onlytask(#44). 
//
// error: space note tableas nil -> project-map empty; log_dir   write -> RecordDecision returnback error but Render   . 

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"voicesign-harness/space"
)

const (
	// DefaultMaxBytes is   defaultcharnodeonlimit(   onunder ). 
	DefaultMaxBytes = 2000
	// DefaultMaxDecisions iskeep     decide objonlimit. 
	DefaultMaxDecisions = 20
	// decisionsFile is decideday filename(<log_dir>/decisions.jsonl). 
	decisionsFile = "decisions.jsonl"
)

// Decision is  confirm  decide     (  see + under  notein). 
type Decision struct {
	Ts       string `json:"ts"`
	TaskID   string `json:"task_id"`
	Intent   string `json:"intent"`
	Decision string `json:"decision"` // auto|light|strong|human
	Confirm  string `json:"confirm"`  // approved|rejected|auto_skipped
	Reason   string `json:"reason"`
}

// Snapshot is        (noteinto prompt / clarification / confirm  before). 
type Snapshot struct {
	ProjectMap []string   `json:"project_map"`
	Decisions  []Decision `json:"decisions"`
	Block      string     `json:"block"`
}

// Ground keephas log_dir and space note table;  value use(    nil-safe). 
type Ground struct {
	LogDir       string
	Spaces       *space.Registry
	MaxBytes     int
	MaxDecisions int
}

// New    Ground; maxBytes/maxDecisions getdefaultvalue(<=0 time). 
func New(logDir string, spaces *space.Registry) *Ground {
	return &Ground{LogDir: logDir, Spaces: spaces}
}

func (g *Ground) maxBytes() int {
	if g.MaxBytes > 0 {
		return g.MaxBytes
	}
	return DefaultMaxBytes
}

func (g *Ground) maxDecisions() int {
	if g.MaxDecisions > 0 {
		return g.MaxDecisions
	}
	return DefaultMaxDecisions
}

// Render        . numdata    -> empty ,    . 
func (g *Ground) Render() Snapshot {
	snap := Snapshot{}

	// project-map: note domainname + scope obj list
	if g.Spaces != nil {
		for _, name := range g.Spaces.List() {
			m, ok := g.Spaces.Get(name)
			if !ok {
				continue
			}
			line := "project-map:" + name + "(" + m.Type + ")"
			if listing := g.scopeListing(m); listing != "" {
				line += " " + listing
			}
			snap.ProjectMap = append(snap.ProjectMap, line)
		}
	}

	// decisions: read <log_dir>/decisions.jsonl,  disconnect  
	snap.Decisions = g.readDecisions()

	//    
	var sb strings.Builder
	sb.WriteString("认知切片（最旧已截断）：\n")
	if len(snap.ProjectMap) == 0 {
		sb.WriteString("  （无注册域）\n")
	} else {
		for _, p := range snap.ProjectMap {
			sb.WriteString("  " + p + "\n")
		}
	}
	sb.WriteString("近期裁决：\n")
	if len(snap.Decisions) == 0 {
		sb.WriteString("  （无）\n")
	} else {
		for _, d := range snap.Decisions {
			fmt.Fprintf(&sb, "  [%s] %s %s/%s：%s\n", d.Ts, d.Intent, d.Decision, d.Confirm, d.Reason)
		}
	}
	snap.Block = truncateBytes(sb.String(), g.maxBytes())
	return snap
}

// scopeListing listout manifest scope obj underbefore 8  (  list,  read objclose ). 
func (g *Ground) scopeListing(m *space.Manifest) string {
	var parts []string
	for _, scope := range m.Scope {
		scope = strings.TrimSuffix(strings.TrimSuffix(scope, "/**"), "/*")
		if scope == "" || scope == "." || strings.HasPrefix(scope, "~") {
			continue
		}
		entries, err := os.ReadDir(scope)
		if err != nil {
			continue
		}
		names := make([]string, 0, 8)
		for _, e := range entries {
			if e.IsDir() {
				names = append(names, e.Name()+"/")
			} else {
				names = append(names, e.Name())
			}
			if len(names) >= 8 {
				break
			}
		}
		if len(names) > 0 {
			parts = append(parts, scope+"={"+strings.Join(names, ",")+"}")
		}
	}
	return strings.Join(parts, " ")
}

// readDecisions readgetandrev listize decisions.jsonl; file  /    -> empty  ,    . 
func (g *Ground) readDecisions() []Decision {
	if g.LogDir == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(g.LogDir, decisionsFile))
	if err != nil {
		return nil //    -> empty    (   )
	}
	var all []Decision
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var d Decision
		if json.Unmarshal([]byte(line), &d) == nil {
			all = append(all, d)
		}
	}
	//  disconnect  : onlykeep    maxDecisions  
	if len(all) > g.maxDecisions() {
		all = all[len(all)-g.maxDecisions():]
	}
	//     ( ->new, thenat read)
	sort.SliceStable(all, func(i, j int) bool { return all[i].Ts < all[j].Ts })
	return all
}

// RecordDecision  confirm      decide(  ;     disconnectread-onlytask). 
func (g *Ground) RecordDecision(d Decision) error {
	if g.LogDir == "" {
		return nil
	}
	if err := os.MkdirAll(g.LogDir, 0o700); err != nil {
		return fmt.Errorf("创建 log_dir 失败: %w", err)
	}
	b, err := json.Marshal(d)
	if err != nil {
		return fmt.Errorf("序列化裁决失败: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(g.LogDir, decisionsFile), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("打开 decisions.jsonl 失败: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("写裁决失败: %w", err)
	}
	return nil
}

// truncateBytes bycharnodeonlimit disconnect(rune safesafety:  to boundaryafterif   charnodemiddlethenback ). 
func truncateBytes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := s[:max]
	// back to after  finish  rune
	for i := len(cut); i > 0; i-- {
		if r := cut[i-1]; r < 0x80 || r >= 0xC0 {
			return cut[:i] + "…(截断)"
		}
	}
	return cut + "…(截断)"
}
