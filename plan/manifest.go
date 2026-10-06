// Package plan --  rule   (VHS-PLAN-001).   list**authoritative out** this package. 
//
//  value  (   out,   write): 
//
//	    ← tools.LoadContracts(dir).All()   (6   : file/git/run/search/test/verify)
//	intent  ← contract   Intent*    × **occurproduce code**(  _test.go)   use
//	        -> live 16 / dormant 2(FILE_WRITE, APP_LAUNCH)
//	domain    ← space.Load(dir).List()/Get()      (6 in domain  )
//
// close safesafetygetto: `space.Manifest.Tools` is**   domain diffnamewordtable**
// (deploy/http/read/query/ask/note/file-append etc 11 place   note table and store ), 
// because   list **   valueonlyget  note table**; domaindiffname list `Aliases`, 
// **  **   Tools,  then SK-1    (pipe store   deploy  become  ). 
//
// uniqueity: this packageis**authoritative out**; `eval/projmodel/gen` onlyis time   ,  callusebase num. 
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

// Capability is  **   **  (      note table). 
type Capability struct {
	Name          string            `json:"name"`
	Caps          []string          `json:"caps"`
	Risk          map[string]string `json:"risk"` // cap -> none|low|medium|high|irreversible
	AllowedSpaces []string          `json:"allowed_spaces"`
	SideEffects   []string          `json:"side_effects"`
	NeedsConfirm  map[string]bool   `json:"needs_confirm"` // SK-3:  risk/ reversible cap   as true
	Source        string            `json:"source"`        //      (SK-4)
}

// IntentCapability is  intent  . 
type IntentCapability struct {
	Value   string `json:"value"`
	Live    bool   `json:"live"`    // occurproduce code   use/produceout
	Dormant bool   `json:"dormant"` // onlyhasvoice ,  hasoccurproduce use(  cur     )
	Source  string `json:"source"`
}

// DomainCapability is  domain(space); Aliases isdomain namewordtable(   name, unresolved). 
type DomainCapability struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	RiskDefault string   `json:"risk_default"`
	Read        bool     `json:"read"`
	Write       bool     `json:"write"`
	Exec        []string `json:"exec"`
	Aliases     []string `json:"aliases"`
	Source      string   `json:"source"`
}

// Manifest is  list -- "     " unique value, **now occurbecome**(SK-4). 
type Manifest struct {
	Tools   []Capability       `json:"tools"`
	Intents []IntentCapability `json:"intents"`
	Domains []DomainCapability `json:"domains"`
	Version string             `json:"version"`
}

// ExportManifest from code   out  list. 
//
// reg and sp  as   in,  its** notein**--SK-4 use   become    " is  code". 
// intent  live/dormant needneedread code(occurproduce code use),  code   time**  as listintent**
// (  ,    code), by SK-5 e.g. change . 
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
				Read: man.Perms.Read, Write: man.Perms.Write,
				Exec: append([]string(nil), man.Perms.Exec...), Source: "space/space.go",
			}
			for _, t := range man.Tools {
				if realTools[t] {
					continue //   name is"diffname"(examplee.g. project domain   file/git)
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

// intentInfo is  intent    outclose . 
type intentInfo struct {
	Value string
	Live  bool
}

// scanIntents resolve  contract/contract.go   Intent*   , and  **occurproduce code**
// (  _test.go,   contract  )   use.    use  become  time  (DSH  decide). 
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
	declared := map[string]string{} //   name -> value
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

// repoRoot bybasefile    (opensend/    ).  code   timereturnback ""(  ,   ). 
func repoRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	return filepath.Dir(filepath.Dir(file)) // plan/ ->   root
}
