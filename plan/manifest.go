// Package plan —— 自规划能力（VHS-PLAN-001）。能力清单**权威导出**在本包。
//
// 真值来源（机械导出，不手写）：
//
//	工具  ← tools.LoadContracts(dir).All()   （6 契约：file/git/run/search/test/verify）
//	意图  ← contract 的 Intent* 常量 × **生产代码**（非 _test.go）真实引用
//	        → live 16 / dormant 2（FILE_WRITE、APP_LAUNCH）
//	域    ← space.Load(dir).List()/Get()      （6 内置域模板）
//
// 关键安全取向：`space.Manifest.Tools` 是**另一套域级别名词表**
// （deploy/http/read/query/ask/note/file-append 等 11 处在契约注册表里并不存在），
// 因此能力清单的**工具真值只取契约注册表**；域别名单列 `Aliases`，
// **不得**混进 Tools，否则 SK-1 会虚报（把不存在的 deploy 说成能力）。
//
// 唯一性：本包是**权威导出**；`eval/projmodel/gen` 只是临时验证器，应调用本函数。
package plan

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"voicesign-harness/space"
	"voicesign-harness/tools"
)

// Capability 是一条**可执行**能力（来自工具契约注册表）。
type Capability struct {
	Name          string            `json:"name"`
	Caps          []string          `json:"caps"`
	Risk          map[string]string `json:"risk"` // cap → none|low|medium|high|irreversible
	AllowedSpaces []string          `json:"allowed_spaces"`
	SideEffects   []string          `json:"side_effects"`
	NeedsConfirm  map[string]bool   `json:"needs_confirm"` // SK-3：高风险/不可逆 cap 必须为 true
	Source        string            `json:"source"`        // 来源可审计（SK-4）
}

// IntentCapability 是一条意图能力。
type IntentCapability struct {
	Value   string `json:"value"`
	Live    bool   `json:"live"`    // 生产代码真的引用/产出
	Dormant bool   `json:"dormant"` // 只有声明、没有生产引用（不得当可执行能力）
	Source  string `json:"source"`
}

// DomainCapability 是一个域（space）；Aliases 是域级名词表（非契约名，unresolved）。
type DomainCapability struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	RiskDefault string   `json:"risk_default"`
	Read        bool     `json:"read"`
	Write       bool     `json:"write"`
	Aliases     []string `json:"aliases"`
	Source      string   `json:"source"`
}

// Manifest 是能力清单 —— 「我能做什么」的唯一真值，**现场生成**（SK-4）。
type Manifest struct {
	Tools   []Capability       `json:"tools"`
	Intents []IntentCapability `json:"intents"`
	Domains []DomainCapability `json:"domains"`
	Version string             `json:"version"`
}

// ExportManifest 从代码真实导出能力清单。
//
// reg 与 sp 作为形参传入，使其**可注入**——SK-4 用一个合成契约证明"不是硬编码"。
// 意图的 live/dormant 需要读源码（生产代码引用），源码不可得时**降级为不列意图**
// （不猜、不硬编码），由 SK-5 如实变红。
func ExportManifest(reg *tools.Registry, sp *space.Registry) Manifest {
	m := Manifest{Version: "plan-manifest/1"}
	realTools := map[string]bool{}

	if reg != nil {
		for _, c := range reg.All() {
			caps := append([]string(nil), c.Caps...)
			sort.Strings(caps)
			need := map[string]bool{}
			for capName, risk := range c.Risk {
				if risk == "high" || risk == "irreversible" {
					need[capName] = true
				}
			}
			m.Tools = append(m.Tools, Capability{
				Name:          c.Name,
				Caps:          caps,
				Risk:          c.Risk,
				AllowedSpaces: append([]string(nil), c.AllowedSpaces...),
				SideEffects:   append([]string(nil), c.SideEffects...),
				NeedsConfirm:  need,
				Source:        "tools/registry.go",
			})
			realTools[c.Name] = true
		}
		sort.Slice(m.Tools, func(i, j int) bool { return m.Tools[i].Name < m.Tools[j].Name })
	}

	if sp != nil {
		for _, name := range sp.List() {
			man, ok := sp.Get(name)
			if !ok {
				continue
			}
			d := DomainCapability{
				Name: name, Type: man.Type, RiskDefault: man.RiskDefault,
				Read: man.Perms.Read, Write: man.Perms.Write, Source: "space/space.go",
			}
			for _, t := range man.Tools {
				if realTools[t] {
					continue // 契约名不是"别名"（例如 project 域里的 file/git）
				}
				d.Aliases = append(d.Aliases, t)
			}
			sort.Strings(d.Aliases)
			m.Domains = append(m.Domains, d)
		}
	}

	for _, it := range scanIntents() {
		m.Intents = append(m.Intents, IntentCapability{
			Value: it.Value, Live: it.Live, Dormant: !it.Live, Source: "contract/contract.go",
		})
	}
	return m
}

// intentInfo 是一条意图常量的导出结果。
type intentInfo struct {
	Value string
	Live  bool
}

// scanIntents 解析 contract/contract.go 的 Intent* 常量，并统计**生产代码**
// （非 _test.go、非 contract 包）里的引用。测试引用不构成运行时能力（DSH 裁决）。
func scanIntents() []intentInfo {
	root := repoRoot()
	if root == "" {
		return nil
	}
	src, err := os.ReadFile(filepath.Join(root, "contract", "contract.go"))
	if err != nil {
		return nil
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filepath.Join(root, "contract", "contract.go"), src, 0)
	if err != nil {
		return nil
	}
	declared := map[string]string{} // 常量名 → 值
	for _, d := range file.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, n := range vs.Names {
				if !strings.HasPrefix(n.Name, "Intent") || i >= len(vs.Values) {
					continue
				}
				bl, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || bl.Kind != token.STRING {
					continue
				}
				declared[n.Name] = strings.Trim(bl.Value, `"`)
			}
		}
	}

	live := map[string]bool{}
	_ = filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi == nil {
			return nil
		}
		if fi.IsDir() {
			switch fi.Name() {
			case ".git", "dist":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		if strings.Contains(p, string(filepath.Separator)+"contract"+string(filepath.Separator)) {
			return nil
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return nil
		}
		for name := range declared {
			if strings.Contains(string(b), "contract."+name) {
				live[name] = true
			}
		}
		return nil
	})

	var out []intentInfo
	for name, value := range declared {
		out = append(out, intentInfo{Value: value, Live: live[name]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Value < out[j].Value })
	return out
}

// repoRoot 由本文件位置推导（开发/测试环境）。源码不可得时返回 ""（降级，不猜）。
func repoRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	return filepath.Dir(filepath.Dir(file)) // plan/ → 仓库根
}
