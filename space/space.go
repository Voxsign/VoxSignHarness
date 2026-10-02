// Package space 是域（Space）注册表 + 执行前统一拦截点 space_check。
// 域 = 策略入口 + 稳定主键（设计 v2 §13）；真正拦截靠本包 Check：
// 有效权限 = 域声明 ∩ 工具契约 ∩ 本次授权（Grant）的交集，默认拒绝。
// 未注册空间不自动切 global；漂移即失效；越界（BOUNDARY_VIOLATION）不因确认放行。
// 本包只依赖标准库 + contract。
package space

// 【伪代码逻辑层】（评审关卡产物；裁决规则权威定义在冻结契约 §3 / 设计 v2 §13，
//  本层只描述单模块控制流/分支/拒绝路径/异常处理，规则语义标注"搬 VSL"。）
//
// Load(dir) -> Registry：
//   seed = 内置六域模板（global/project/sandbox/vault-notes/vault-creds/external，不落盘）
//   if dir 缺失或无 *.space.json: return seed（目录空→纯模板，不写盘）
//   for each dir/*.space.json: 解析 → 覆盖 seed 同名域（真实 manifest 为准）
//   return Registry{Dir, Manifests, Version:1}
//
// Add(m) -> error：
//   if 同名已存在: 旧文件备份为 <name>.space.json.bak；m.Version = old.Version+1
//   else: m.Version = 1；MkdirAll(dir)
//   序列化 <name>.space.json（0o600）→ 写回内存表
//
// DetectDrift() -> []Drift：
//   for each manifest m：
//     issue = 校验 m.Scope（剥 /** 后取目录）在磁盘上是否存在/是否目录
//     if 不存在: append Drift{m.Name, "scope 路径不存在: "+p}  // 漂移→该域失效
//   return issues
//
// Check(r, in) -> Verdict（搬 VSL 冻结§3 裁决规则；优先级从高到低）：
//   m = r.Get(in.Intent.Space)
//   if !ok:                       return deny unknown_space      // 不自动切 global，回问
//   if scope 路径漂移:            return deny drift              // 漂移即失效
//   if overlap(m.Scope, m.Exclude): return deny boundary_violation
//   for cap in in.ToolCaps:
//     if cap ∉ m.Tools:           return deny boundary_violation // 越界，不因确认放行
//     if 有契约表且 cap ∉ ∪contract.Caps: return deny boundary_violation
//   // 权限交集（搬 VSL：有效权限=域∩工具∩本次授权）
//   if !in.Grant.Authorized:       return deny default_deny
//   if needsWrite(intent) && !m.Perms.Write: return deny default_deny
//   if !m.Perms.Read:              return deny default_deny
//   // 跨域引用：目标/上下文点名了别的域且未声明 cross_refs
//   for other in r.List(): if other!=sid and 意图点名 other and other∉m.CrossRefs:
//                                  return deny cross_ref_deny
//   return allow（ToolOK=通过的 caps）
//
// ResolveScopePath(scopeRoot, target) -> (abs, ok)【M3 #15 路径边界硬化】：
//   // 双重 containment：词法 Clean 防 .. 穿越 + EvalSymlinks 防符号链接逃逸。
//   if scopeRoot=="" or target=="": return "", false
//   root  = filepath.Abs(scopeRoot)（词法根）
//   realRoot = EvalSymlinks(root)；失败 → root 取最深已存在祖先
//   cand = IsAbs(target) ? target : Join(root, target)
//   clean = Clean(cand)
//   if !within(clean, root):            return "", false   // .. 逃逸
//   real = EvalSymlinks(clean)；失败 → real 取最深已存在祖先（允许新建目标）
//   if !within(real, realRoot):          return "", false   // 符号链接逃逸出域
//   return clean, true
//   within(p, root): p==root || HasPrefix(p, root+Sep)
//
// normalizeScopes(m)：Load/Add 时把 m.Scope 的非 ~ 条目 Abs+Clean（保持相对 scope 可用）。
// driftOf 升级：scope 经 EvalSymlinks 后仍缺失/非目录 → drift（符号链接感知）。
//
// 异常：JSON 损坏 → Load 报错；Add 写盘失败 → error 不污染内存。

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

// 域类型常量（Manifest.Type）。
const (
	TypeGlobal     = "global"
	TypeProject    = "project"
	TypeSandbox    = "sandbox"
	TypeVaultNotes = "vault-notes"
	TypeVaultCreds = "vault-creds"
	TypeExternal   = "external"
)

// Perms 是域声明的读/写/可执行权限（交集计算的一臂）。
type Perms struct {
	Read  bool     `json:"read"`
	Write bool     `json:"write"`
	Exec  []string `json:"exec,omitempty"`
}

// Manifest 是一个域的可执行策略单元（.space.json，机器可读）。
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
	Path        string   `json:"-"` // 落盘路径（内置模板为空）
}

// Registry 是域注册表：name → *Manifest。Version 为策略版本（缓存失效联动）。
type Registry struct {
	Dir       string
	Manifests map[string]*Manifest
	Version   int
}

// Drift 是一条漂移记录：域名 + 问题描述。
type Drift struct {
	Manifest string `json:"manifest"`
	Issue    string `json:"issue"`
}

// Grant 是本次调用用户授予的权限（交集计算的另一臂）。
type Grant struct {
	Authorized bool     `json:"authorized"`
	Paths      []string `json:"paths,omitempty"`
}

// CheckInput 是 space_check 的输入。Contracts 为 C 的工具契约表（数据传入，避免包依赖）。
type CheckInput struct {
	Intent    contract.Intent
	Grant     Grant
	ToolCaps  []string // pipeline 规划产出的待调工具动作
	Contracts []contract.ToolContract
}

// Verdict 是 space_check 的裁决。Reason ∈ ""|unknown_space|drift|default_deny|boundary_violation|cross_ref_deny。
type Verdict struct {
	SpaceID string
	Allowed bool
	Reason  string
	ToolOK  []string
}

// builtinTemplates 返回内置六域模板（不落盘；设计 v2 §13 定稿域表）。
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
			Perms: Perms{Read: true}, // 高敏只读，无写无外发
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

// Load 加载 dir/*.space.json；目录缺失/为空 → 内置六域模板（不落盘）。
// 真实 manifest 按名覆盖内置模板同名域。
func Load(dir string) (*Registry, error) {
	r := &Registry{Dir: dir, Manifests: builtinTemplates(), Version: 1}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return r, nil // 目录缺失 → 纯内置模板
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
			// 单个 manifest 损坏只跳过该文件（防整个 Load 失败→Spaces nil→Check panic）。
			// M7 实测 2026-10-03：perms.exec 误写 bool 触发此路径。
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
		// 目录存在但无 manifest → 仍返回内置模板（冻结§3：目录空→内置模板）。
		r.Manifests = builtinTemplates()
	}
	return r, nil
}

// Get 按名取域 manifest。
func (r *Registry) Get(id string) (*Manifest, bool) {
	m, ok := r.Manifests[id]
	return m, ok
}

// List 返回全部已注册域名（排序，稳定输出）。
func (r *Registry) List() []string {
	out := make([]string, 0, len(r.Manifests))
	for name := range r.Manifests {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Add 落盘 dir/<name>.space.json；同名更新版本+1（写前备份旧文件）。
func (r *Registry) Add(m *Manifest) error {
	if m.Name == "" {
		return fmt.Errorf("manifest 缺少 name")
	}
	if r.Dir == "" {
		return fmt.Errorf("registry 未指定落盘目录")
	}
	if old, ok := r.Manifests[m.Name]; ok && old.Path != "" {
		if data, err := os.ReadFile(old.Path); err == nil {
			_ = os.WriteFile(old.Path+".bak", data, 0o600) // 写前备份
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

// scopeDirs 剥掉 scope 条目尾部的 /** 与 /*，取待校验目录。
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

// normalizeScopes 把 m.Scope 的非 ~ 条目 Abs+Clean（Load/Add 时调用；保持相对 scope 可用）。
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

// withinRoot 报告 p 是否等于 root 或位于 root 之下（词法前缀判定）。
func withinRoot(p, root string) bool {
	if p == root {
		return true
	}
	return strings.HasPrefix(p, root+string(os.PathSeparator))
}

// deepestExisting 从 p 向上找到第一个真实存在的路径（目标不存在时用于祖先解析，
// 允许"新建文件"场景：只校验已存在祖先链是否逃逸）。
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

// driftOf 返回单个域的漂移问题（空串=无漂移；符号链接感知）。
func driftOf(m *Manifest) string {
	for _, p := range scopeDirs(m.Scope) {
		// 模板占位（~/）不做磁盘断言；已规范化的绝对路径必须经符号链接解析后仍存在。
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

// ResolveScopePath 把域内目标路径解析为绝对路径并做双重 containment 校验（M3 #15）：
//   - target 相对/绝对均可；
//   - 词法 Clean 后必须落在 scopeRoot 内（`..` 穿越 → ok=false）；
//   - 对结果做 EvalSymlinks，真实路径仍须落在 scopeRoot 的真实路径内（符号链接逃逸 → ok=false）；
//   - target 不存在时解析其最深已存在祖先（允许新建文件场景，不因此拒绝）。
//
// 返回可安全操作的词法绝对路径 clean。调用方据此判定"该目标是否真的在本域边界内"。
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

	// 第一道：词法 containment（防 .. 穿越）
	if !withinRoot(clean, root) {
		return "", false
	}

	// 真实根（符号链接解析后；根本身不存在→取最深已存在祖先并再解析）
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		anc := deepestExisting(root)
		if r2, e2 := filepath.EvalSymlinks(anc); e2 == nil {
			realRoot = r2
		} else {
			realRoot = anc
		}
	}

	// 第二道：符号链接解析后 containment（目标不存在→校验最深已存在祖先链的真实路径）
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

// DetectDrift 校验各域 scope 路径存在性；返回漂移列表（漂移即该域失效）。
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

// needsWrite 报告该意图是否需要写权限（搬 VSL：写意图清单）。
func needsWrite(it contract.Intent) bool {
	switch it.Intent {
	case contract.IntentEdit, contract.IntentCommit, contract.IntentDeploy,
		contract.IntentDebug, contract.IntentNote, contract.IntentRegisterTool:
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

// overlap 报告 a 与 b 是否有非空交集（Scope∩Exclude 非空 = 边界自相矛盾）。
func overlap(a, b []string) bool {
	for _, x := range a {
		if contains(b, x) {
			return true
		}
	}
	return false
}

// contractCaps 返回已注册工具契约的 caps 并集（无契约表时返回 nil）。
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

// Check 是执行前统一拦截点：按冻结§3 裁决规则做交集判定，默认拒绝。
func Check(r *Registry, in CheckInput) Verdict {
	sid := in.Intent.Space
	v := Verdict{SpaceID: sid}
	m, ok := r.Get(sid)
	if !ok {
		v.Reason = "unknown_space" // 不自动切 global，回问
		return v
	}
	// 漂移即失效
	if issue := driftOf(m); issue != "" {
		v.Reason = "drift"
		return v
	}
	// Scope∩Exclude 自相矛盾
	if overlap(m.Scope, m.Exclude) {
		v.Reason = "boundary_violation"
		return v
	}
	// 工具动作必须 ⊆ 域声明 ∩ 契约 caps（越界即 BOUNDARY_VIOLATION，不因确认放行）
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
	// 权限交集为空 → default_deny
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
	// 跨域引用：意图点名了别的域但未在 cross_refs 声明 → cross_ref_deny
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
