// Package space isdomain(Space)note table +   before  blockpt space_check. 
// domain =   in  +     (   v2 §13);  posblock this package Check: 
// has  limit = domainvoice  ∩      ∩ base   (Grant)   , defaultreject. 
//  note emptytime     global;   i.e.  ; out-of-scope(BOUNDARY_VIOLATION) becauseconfirm  . 
// this packageonlydependencytgtapprove  + contract. 
package space

// [pseudocode logic layer](review gateartifact;  decideruleauthoritative definition frozen   §3 /    v2 §13, 
//  this layeronlydescribe modulecontrol flow/branch/rejectpath/errorhandle, rule semanticstgtnote"  VSL". )
//
// Load(dir) -> Registry: 
//   seed = in  domain  (global/project/sandbox/vault-notes/vault-creds/external,    )
//   if dir   orno *.space.json: return seed(obj empty->   ,  write )
//   for each dir/*.space.json: resolve  -> overwrite seed samenamedomain(   manifest asapprove)
//   return Registry{Dir, Manifests, Version:1}
//
// Add(m) -> error: 
//   if samenamealreadystore :  file  as <name>.space.json.bak; m.Version = old.Version+1
//   else: m.Version = 1; MkdirAll(dir)
//    listize <name>.space.json(0o600)-> writebackinstoretable
//
// DetectDrift() -> []Drift: 
//   for each manifest m: 
//     issue = verify m.Scope(  /** aftergetobj )   onis store /is obj 
//     if  store : append Drift{m.Name, "scope path store : "+p}  //   -> domain  
//   return issues
//
// Check(r, in) -> Verdict(  VSL frozen§3  deciderule;  first from to ): 
//   m = r.Get(in.Intent.Space)
//   if !ok:                       return deny unknown_space      //      global, clarification
//   if scope path  :            return deny drift              //   i.e.  
//   if overlap(m.Scope, m.Exclude): return deny boundary_violation
//   for cap in in.ToolCaps:
//     if cap ∉ m.Tools:           return deny boundary_violation // out-of-scope,  becauseconfirm  
//     if has  tableand cap ∉ ∪contract.Caps: return deny boundary_violation
//   //  limit  (  VSL: has  limit=domain∩  ∩base   )
//   if !in.Grant.Authorized:       return deny default_deny
//   if needsWrite(intent) && !m.Perms.Write: return deny default_deny
//   if !m.Perms.Read:              return deny default_deny
//   //  domain use: objtgt/onunder ptnamediff domainand voice  cross_refs
//   for other in r.List(): if other!=sid and intentptname other and other∉m.CrossRefs:
//                                  return deny cross_ref_deny
//   return allow(ToolOK= ed  caps)
//
// ResolveScopePath(scopeRoot, target) -> (abs, ok)[M3 #15 path boundary ize]: 
//   //  heavy containment: word  Clean prevent ..    + EvalSymlinks prevent idchainconnect  . 
//   if scopeRoot=="" or target=="": return "", false
//   root  = filepath.Abs(scopeRoot)(word root)
//   realRoot = EvalSymlinks(root);    -> root get  alreadystore  first
//   cand = IsAbs(target) ? target : Join(root, target)
//   clean = Clean(cand)
//   if !within(clean, root):            return "", false   // ..   
//   real = EvalSymlinks(clean);    -> real get  alreadystore  first( allownew objtgt)
//   if !within(real, realRoot):          return "", false   //  idchainconnect  outdomain
//   return clean, true
//   within(p, root): p==root || HasPrefix(p, root+Sep)
//
// normalizeScopes(m): Load/Add timepipe m.Scope    ~  obj Abs+Clean(keepkeep to scope  use). 
// driftOf   : scope   EvalSymlinks after   / obj  -> drift( idchainconnect  ). 
//
// error: JSON    -> Load   ; Add write    -> error    instore. 

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"voicesign-harness/contract"
)

// domainclasstype  (Manifest.Type). 
const (
	TypeGlobal     = "global"
	TypeProject    = "project"
	TypeSandbox    = "sandbox"
	TypeVaultNotes = "vault-notes"
	TypeVaultCreds = "vault-creds"
	TypeExternal   = "external"
)

// Perms isdomainvoice  read/write/    limit(       ). 
type Perms struct {
	Read  bool     `json:"read"`
	Write bool     `json:"write"`
	Exec  []string `json:"exec,omitempty"`
}

// Manifest is  domain        (.space.json,    read). 
type Manifest struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"` // project|sandbox|vault-notes|vault-creds|external|global
	Scope       []string `json:"scope,omitempty"`
	Exclude     []string `json:"exclude,omitempty"`
	Tools       []string `json:"tools,omitempty"`
	Context     []string `json:"context,omitempty"`
	Perms       Perms    `json:"perms"`
	Acceptance  string   `json:"acceptance,omitempty"`
	RiskDefault string   `json:"risk_default,omitempty"`
	CrossRefs   []string `json:"cross_refs,omitempty"`
	Version     int      `json:"version"`
	Path        string   `json:"-"` //   path(in   asempty)
}

// Registry isdomainnote table: name -> *Manifest. Version as   base(cache    ). 
type Registry struct {
	Dir       string
	Manifests map[string]*Manifest
	Version   int
}

// Drift is      : domainname +   describe. 
type Drift struct {
	Manifest string `json:"manifest"`
	Issue    string `json:"issue"`
}

// Grant isbase calluseuseuser    limit(        ). 
type Grant struct {
	Authorized bool     `json:"authorized"`
	Paths      []string `json:"paths,omitempty"`
}

// CheckInput is space_check   in. Contracts as C      table(numdata in,    dependency). 
type CheckInput struct {
	Intent    contract.Intent
	Grant     Grant
	ToolCaps  []string // pipeline rule produceout  call    
	Contracts []contract.ToolContract
}

// Verdict is space_check   decide. Reason ∈ ""|unknown_space|drift|default_deny|boundary_violation|cross_ref_deny. 
type Verdict struct {
	SpaceID string
	Allowed bool
	Reason  string
	ToolOK  []string
}

// builtinTemplates returnbackin  domain  (   ;    v2 §13   domaintable). 
func builtinTemplates() map[string]*Manifest {
	return map[string]*Manifest{
		"global": {
			Name: "global", Type: TypeGlobal,
			Scope: []string{"."}, Perms: Perms{Read: true},
			Tools: []string{"read", "query", "ask"}, RiskDefault: "auto",
			Acceptance: "只读兜底，无写权限",
		},
		"project": {
			Name: "project", Type: TypeProject,
			Perms: Perms{Read: true, Write: true, Exec: []string{"test", "run"}},
			Tools: []string{"file", "git", "search", "test", "run", "read"}, RiskDefault: "light",
			Acceptance: "改动限定在 scope 内，无越界",
		},
		"sandbox": {
			Name: "sandbox", Type: TypeSandbox,
			Perms: Perms{Read: true, Write: true},
			Tools: []string{"file", "run", "search"}, RiskDefault: "auto",
			Acceptance: "临时目录，自动清理，不触主库",
		},
		"vault-notes": {
			Name: "vault-notes", Type: TypeVaultNotes,
			Perms: Perms{Read: true, Write: true},
			Tools: []string{"note", "file-append", "read"}, RiskDefault: "auto",
			Acceptance: "仅追加，不改写历史想法",
		},
		"vault-creds": {
			Name: "vault-creds", Type: TypeVaultCreds,
			Perms: Perms{Read: true}, //   read-only, nowritenooutsend
			Tools: []string{"read"}, RiskDefault: "human",
			Acceptance: "凭证库只读，禁止外发/写",
		},
		"external": {
			Name: "external", Type: TypeExternal,
			Perms: Perms{Read: true},
			Tools: []string{"deploy", "http", "read"}, RiskDefault: "strong",
			Acceptance: "外发动作永远强确认，绑定具体目标",
		},
	}
}

// Load    dir/*.space.json; obj   /asempty -> in  domain  (   ). 
//    manifest bynameoverwritein   samenamedomain. 
func Load(dir string) (*Registry, error) {
	r := &Registry{Dir: dir, Manifests: builtinTemplates(), Version: 1}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return r, nil // obj    ->  in   
		}
		return nil, fmt.Errorf("读取域目录 %s 失败: %w", dir, err)
	}
	loaded := false
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".space.json") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("读取 manifest %s 失败: %w", p, err)
		}
		var m Manifest
		if err := json.Unmarshal(data, &m); err != nil {
			//    manifest   only ed file(prevent   Load   ->Spaces nil->Check panic). 
			// M7    2026-10-03: perms.exec  write bool triggersend path. 
			log.Printf("[space] skip malformed manifest %s: %v", p, err)
			continue
		}
		if m.Name == "" {
			m.Name = strings.TrimSuffix(e.Name(), ".space.json")
		}
		m.Path = p
		normalizeScopes(&m)
		r.Manifests[m.Name] = &m
		loaded = true
	}
	if !loaded {
		// obj store butno manifest ->  returnbackin   (frozen§3: obj empty->in   ). 
		r.Manifests = builtinTemplates()
	}
	return r, nil
}

// Get bynamegetdomain manifest. 
func (r *Registry) Get(id string) (*Manifest, bool) {
	m, ok := r.Manifests[id]
	return m, ok
}

// List returnbacksafety alreadynote domainname(  ,    out). 
func (r *Registry) List() []string {
	out := make([]string, 0, len(r.Manifests))
	for name := range r.Manifests {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Add    dir/<name>.space.json; samenamechangenew base+1(writebefore   file). 
func (r *Registry) Add(m *Manifest) error {
	if m.Name == "" {
		return fmt.Errorf("manifest 缺少 name")
	}
	if r.Dir == "" {
		return fmt.Errorf("registry 未指定落盘目录")
	}
	if old, ok := r.Manifests[m.Name]; ok && old.Path != "" {
		if data, err := os.ReadFile(old.Path); err == nil {
			_ = os.WriteFile(old.Path+".bak", data, 0o600) // writebefore  
		}
		m.Version = old.Version + 1
	} else if m.Version == 0 {
		m.Version = 1
	}
	normalizeScopes(m)
	if err := os.MkdirAll(r.Dir, 0o755); err != nil {
		return fmt.Errorf("创建域目录失败: %w", err)
	}
	p := filepath.Join(r.Dir, m.Name+".space.json")
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 manifest %s 失败: %w", m.Name, err)
	}
	if err := os.WriteFile(p, data, 0o600); err != nil {
		return fmt.Errorf("写盘 %s 失败: %w", p, err)
	}
	m.Path = p
	r.Manifests[m.Name] = m
	return nil
}

// scopeDirs    scope  objtail   /** and /*, get verifyobj . 
func scopeDirs(scopes []string) []string {
	var out []string
	for _, s := range scopes {
		s = strings.TrimSpace(s)
		s = strings.TrimSuffix(s, "/**")
		s = strings.TrimSuffix(s, "/*")
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}

// normalizeScopes pipe m.Scope    ~  obj Abs+Clean(Load/Add timecalluse; keepkeep to scope  use). 
func normalizeScopes(m *Manifest) {
	for i, s := range m.Scope {
		if strings.HasPrefix(s, "~") {
			continue
		}
		abs, err := filepath.Abs(s)
		if err != nil {
			continue
		}
		m.Scope[i] = filepath.Clean(abs)
	}
}

// withinRoot    p is etcat root or at root ofunder(word before   ). 
func withinRoot(p, root string) bool {
	if p == root {
		return true
	}
	return strings.HasPrefix(p, root+string(os.PathSeparator))
}

// deepestExisting from p toon to     store  path(objtgt store timeuseat firstresolve , 
//  allow"new file" scenario: onlyverifyalreadystore  firstchainis   ). 
func deepestExisting(p string) string {
	for {
		if _, err := os.Stat(p); err == nil {
			return p
		}
		parent := filepath.Dir(p)
		if parent == p {
			return p
		}
		p = parent
	}
}

// driftOf returnback  domain     (empty =no  ;  idchainconnect  ). 
func driftOf(m *Manifest) string {
	for _, p := range scopeDirs(m.Scope) {
		//     (~/)    disconnectlang; alreadyrule ize  topath    idchainconnectresolve after store . 
		if strings.HasPrefix(p, "~") || p == "" {
			continue
		}
		real, err := filepath.EvalSymlinks(p)
		if err != nil {
			if os.IsNotExist(err) {
				return "scope 路径不存在（符号链接解析后仍缺失）: " + p
			}
			continue
		}
		fi, err := os.Stat(real)
		if err != nil {
			return "scope 路径不可访问: " + p
		}
		if !fi.IsDir() {
			return "scope 路径不是目录: " + p
		}
	}
	return ""
}

// ResolveScopePath pipedomaininobjtgtpathresolve as topathand  heavy containment verify(M3 #15): 
//   - target  to/ to  ; 
//   - word  Clean after     scopeRoot in(`..`    -> ok=false); 
//   - toclose   EvalSymlinks,   path     scopeRoot    pathin( idchainconnect   -> ok=false); 
//   - target  store timeresolve its  alreadystore  first( allownew file scenario,  because reject). 
//
// returnback safesafety   word  topath clean. calluse data   " objtgtis    basedomain boundaryin". 
func ResolveScopePath(scopeRoot, target string) (string, bool) {
	if strings.TrimSpace(scopeRoot) == "" || strings.TrimSpace(target) == "" {
		return "", false
	}
	root, err := filepath.Abs(scopeRoot)
	if err != nil {
		return "", false
	}
	root = filepath.Clean(root)

	cand := target
	if !filepath.IsAbs(cand) {
		cand = filepath.Join(root, target)
	}
	clean := filepath.Clean(cand)

	//    : word  containment(prevent ..   )
	if !withinRoot(clean, root) {
		return "", false
	}

	//   root( idchainconnectresolve after; rootbase  store ->get  alreadystore  firstandagainresolve )
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		anc := deepestExisting(root)
		if r2, e2 := filepath.EvalSymlinks(anc); e2 == nil {
			realRoot = r2
		} else {
			realRoot = anc
		}
	}

	//    :  idchainconnectresolve after containment(objtgt store ->verify  alreadystore  firstchain   path)
	real, err := filepath.EvalSymlinks(clean)
	if err != nil {
		anc := deepestExisting(clean)
		if r2, e2 := filepath.EvalSymlinks(anc); e2 == nil {
			real = r2
		} else {
			real = anc
		}
	}
	if !withinRoot(real, realRoot) {
		return "", false
	}
	return clean, true
}

// DetectDrift verify domain scope pathstore ity; returnback  listtable(  i.e. domain  ). 
func (r *Registry) DetectDrift() ([]Drift, error) {
	var out []Drift
	for _, name := range r.List() {
		m := r.Manifests[name]
		if issue := driftOf(m); issue != "" {
			out = append(out, Drift{Manifest: name, Issue: issue})
		}
	}
	return out, nil
}

// needsWrite    intentis needneedwrite limit(  VSL: write intentlist). 
func needsWrite(it contract.Intent) bool {
	switch it.Intent {
	case contract.IntentEdit, contract.IntentCommit, contract.IntentDeploy,
		contract.IntentDebug, contract.IntentNote, contract.IntentRegisterTool,
		contract.IntentOrchestrate:
		return true
	default: // QUERY / ASK / TEST
		return false
	}
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// overlap    a and b is has empty  (Scope∩Exclude  empty =  boundary    ). 
func overlap(a, b []string) bool {
	for _, x := range a {
		if contains(b, x) {
			return true
		}
	}
	return false
}

// contractCaps returnbackalreadynote       caps and (no  tabletimereturnback nil). 
func contractCaps(cs []contract.ToolContract) map[string]bool {
	if len(cs) == 0 {
		return nil
	}
	m := map[string]bool{}
	for _, c := range cs {
		for _, cap := range c.Caps {
			m[cap] = true
		}
	}
	return m
}

// Check is  before  blockpt: byfrozen§3  deciderule     , defaultreject. 
func Check(r *Registry, in CheckInput) Verdict {
	sid := in.Intent.Space
	v := Verdict{SpaceID: sid}
	m, ok := r.Get(sid)
	if !ok {
		v.Reason = "unknown_space" //      global, clarification
		return v
	}
	//   i.e.  
	if issue := driftOf(m); issue != "" {
		v.Reason = "drift"
		return v
	}
	// Scope∩Exclude     
	if overlap(m.Scope, m.Exclude) {
		v.Reason = "boundary_violation"
		return v
	}
	//        ⊆ domainvoice  ∩    caps(out-of-scopei.e. BOUNDARY_VIOLATION,  becauseconfirm  )
	ccaps := contractCaps(in.Contracts)
	for _, cap := range in.ToolCaps {
		if !contains(m.Tools, cap) {
			v.Reason = "boundary_violation"
			return v
		}
		if ccaps != nil && !ccaps[cap] {
			v.Reason = "boundary_violation"
			return v
		}
		v.ToolOK = append(v.ToolOK, cap)
	}
	//  limit  asempty -> default_deny
	if !in.Grant.Authorized {
		v.Reason = "default_deny"
		return v
	}
	if !m.Perms.Read {
		v.Reason = "default_deny"
		return v
	}
	if needsWrite(in.Intent) && !m.Perms.Write {
		v.Reason = "default_deny"
		return v
	}
	//  domain use: intentptnamediff domainbut   cross_refs voice  -> cross_ref_deny
	for _, other := range r.List() {
		if other == sid {
			continue
		}
		pointed := (in.Intent.Target != nil && in.Intent.Target.Entity == other) ||
			contains(in.Intent.Context, "project-map:"+other)
		if pointed && !contains(m.CrossRefs, other) {
			v.Reason = "cross_ref_deny"
			return v
		}
	}
	v.Allowed = true
	return v
}
