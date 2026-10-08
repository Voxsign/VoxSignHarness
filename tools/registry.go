// Package tools  now    note table( in    + REGISTER_TOOL langaudio  )and tool execution . 
//
//  boundary(frozen   §tools): this package[  ] forbid--space_check and risk  decide  pipeline  ; 
// this packageonlyresponsible:   verify / note    / by caps     .  ed forbid    pipeline rootbase     . 
package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"voicesign-harness/contract"
)

// Registry is    note table. Contracts as name->  ; dir aslangaudionote   obj (  out, 
//  modifychangefrozen out status). 
type Registry struct {
	Contracts map[string]contract.ToolContract `json:"contracts"`
	dir       string
}

// builtinContracts returnbackin     @1.0(git/file/search/test/run/verify). 
// risk   cap overwrite:  reversible  tgt irreversible, provide risk  andhumanconfirm decide. 
func builtinContracts() []contract.ToolContract {
	return []contract.ToolContract{
		{
			Name: "git", Version: "1.0", Source: "builtin",
			Caps:          []string{"status", "diff", "log", "commit", "checkout", "clone"},
			Params:        map[string]string{"args": "[]string,optional"},
			SideEffects:   []string{"read workspace/index", "commit irreversible (local)", "clone downloads a repo (network)"},
			AllowedSpaces: []string{"project", "sandbox"},
			Risk: map[string]string{
				"status": "none", "diff": "none", "log": "none",
				"commit": "irreversible", "checkout": "medium", "clone": "medium",
			},
		},
		{
			Name: "file", Version: "1.0", Source: "builtin",
			Caps:          []string{"read", "write", "append", "exists"},
			Params:        map[string]string{"path": "string,required", "content": "string,optional"},
			SideEffects:   []string{"write/append files (backed up to log_dir/backups before write)"},
			AllowedSpaces: []string{"project", "sandbox", "vault-notes"},
			Risk: map[string]string{
				"read": "none", "exists": "none", "append": "low", "write": "high",
			},
		},
		{
			Name: "search", Version: "1.0", Source: "builtin",
			Caps:          []string{"text", "symbol"},
			Params:        map[string]string{"pattern": "string,required"},
			SideEffects:   []string{"read-only scan"},
			AllowedSpaces: []string{"global", "project", "sandbox", "vault-notes", "vault-creds"},
			Risk: map[string]string{
				"text": "none", "symbol": "none",
			},
		},
		{
			Name: "test", Version: "1.0", Source: "builtin",
			Caps:          []string{"run"},
			Params:        map[string]string{"command": "[]string,required"},
			SideEffects:   []string{"run tests (mostly read-only; may write temp artifacts)"},
			AllowedSpaces: []string{"project", "sandbox"},
			Risk:          map[string]string{"run": "low"},
		},
		{
			Name: "run", Version: "1.0", Source: "builtin",
			Caps:          []string{"exec"},
			Params:        map[string]string{"command": "[]string,required"},
			SideEffects:   []string{"arbitrary command (high risk; gate in pipeline)"},
			AllowedSpaces: []string{"project", "sandbox"},
			Risk:          map[string]string{"exec": "high"},
		},
		{
			Name: "verify", Version: "1.0", Source: "builtin",
			Caps:          []string{"run"},
			Params:        map[string]string{"kind": "string,required", "args": "[]string,optional"},
			SideEffects:   []string{"independent read-only review (read fs/re-run commands, no state change)"},
			AllowedSpaces: []string{"global", "project", "sandbox", "vault-notes", "vault-creds"},
			Risk:          map[string]string{"run": "none"},
		},
	}
}

// LoadContracts   note table: first in    , again   dir/*.contract.json(langaudionote ). 
// dir   /asempty -> onlyin   (   ). 
func LoadContracts(dir string) (*Registry, error) {
	r := &Registry{Contracts: map[string]contract.ToolContract{}, dir: dir}
	for _, c := range builtinContracts() {
		r.Contracts[c.Name] = c
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return r, nil
		}
		return nil, fmt.Errorf("failed to read contract dir %s: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".contract.json") {
			continue
		}
		data, rerr := os.ReadFile(filepath.Join(dir, e.Name()))
		if rerr != nil {
			return nil, fmt.Errorf("failed to read contract file %s: %w", e.Name(), rerr)
		}
		var c contract.ToolContract
		if jerr := json.Unmarshal(data, &c); jerr != nil {
			return nil, fmt.Errorf("failed to parse contract file %s: %w", e.Name(), jerr)
		}
		if err := ValidateContract(c); err != nil {
			return nil, fmt.Errorf("contract file %s invalid: %w", e.Name(), err)
		}
		r.Contracts[c.Name] = c
	}
	return r, nil
}

// Get bynameget  . 
func (r *Registry) Get(name string) (contract.ToolContract, bool) {
	c, ok := r.Contracts[name]
	return c, ok
}

// All byname  returnbacksafety   (  ity  , thenat  and  ). 
func (r *Registry) All() []contract.ToolContract {
	names := make([]string, 0, len(r.Contracts))
	for n := range r.Contracts {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]contract.ToolContract, 0, len(names))
	for _, n := range names {
		out = append(out, r.Contracts[n])
	}
	return out
}

var validRiskLevels = map[string]bool{
	"none": true, "low": true, "medium": true, "high": true, "irreversible": true,
}

// ValidateContract verify  charsegfinish ity: name/version/caps/params   , risk overwrite   cap. 
func ValidateContract(c contract.ToolContract) error {
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("contract name is required")
	}
	if strings.TrimSpace(c.Version) == "" {
		return fmt.Errorf("contract %q version is required", c.Name)
	}
	if len(c.Caps) == 0 {
		return fmt.Errorf("contract %q must declare at least one cap", c.Name)
	}
	if len(c.Params) == 0 {
		return fmt.Errorf("contract %q must declare at least one param", c.Name)
	}
	if len(c.Risk) == 0 {
		return fmt.Errorf("contract %q must declare a risk mapping", c.Name)
	}
	for _, cap := range c.Caps {
		lvl, ok := c.Risk[cap]
		if !ok {
			return fmt.Errorf("contract %q cap %q missing risk level", c.Name, cap)
		}
		if !validRiskLevels[lvl] {
			return fmt.Errorf("contract %q cap %q risk level %q invalid (none|low|medium|high|irreversible)", c.Name, cap, lvl)
		}
	}
	return nil
}

// riskGrade from   risk    split  note risketc (providehumanconfirmbefore show;  end gate   pipeline). 
func riskGrade(c contract.ToolContract) string {
	for _, lvl := range c.Risk {
		if lvl == "irreversible" {
			return "human"
		}
	}
	for _, lvl := range c.Risk {
		if lvl == "high" {
			return "strong"
		}
	}
	return "light"
}

// Register   REGISTER_TOOL   flow. 
//
// [pseudocode logic layer]( writemodule; risk grading  semantic  VSL,  placeonlywritecontrol flow/rejectpath)
//
// control flow: 
//  1. ValidateContract(c):   name/version/caps/params or  cap   risk -> reject,    . 
//  2. riskGrade(c):   irreversible -> human;  then  high -> strong;  then light. 
//     (onlyuseatback / show; is   by pipeline   risk  decideandhumanconfirmdecide . )
//  3. humanconfirm : approved == false -> returnbackerror[and  write ](useexample 9:  approveapprove   ). 
//  4. approved == true: 
//     a. patch Source="voice", RegisteredAt=curbeforetimetime . 
//     b. write <dir>/<name>.contract.json(orig write). 
//     c. become afteronly upsert  instore Contracts(first  become againmodifyinstore, prevent state). 
//     rejectpath: dir asempty(note table  LoadContracts but )-> reject  ; write    -> rollbackinstore. 
func (r *Registry) Register(c contract.ToolContract, approved bool) error {
	if err := ValidateContract(c); err != nil {
		return fmt.Errorf("contract validation failed, refusing to register: %w", err)
	}
	grade := riskGrade(c)
	if !approved {
		return fmt.Errorf("contract %q risk level %s not human-approved (approved=false); refusing to persist", c.Name, grade)
	}
	if strings.TrimSpace(r.dir) == "" {
		return fmt.Errorf("registry not bound to a contract dir (not loaded via LoadContracts); cannot persist %q", c.Name)
	}
	c.Source = "voice"
	c.RegisteredAt = time.Now().Format(time.RFC3339)
	path := filepath.Join(r.dir, c.Name+".contract.json")
	if err := os.MkdirAll(r.dir, 0o755); err != nil {
		return fmt.Errorf("failed to create contract dir: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize contract: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("failed to write contract temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("failed to commit contract file: %w", err)
	}
	r.Contracts[c.Name] = c
	return nil
}
