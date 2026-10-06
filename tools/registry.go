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
			Caps:          []string{"status", "diff", "log", "commit", "checkout"},
			Params:        map[string]string{"args": "[]string,optional"},
			SideEffects:   []string{"读工作区/索引", "commit 不可逆(本地)"},
			AllowedSpaces: []string{"project", "sandbox"},
			Risk: map[string]string{
				"status": "none", "diff": "none", "log": "none",
				"commit": "irreversible", "checkout": "medium",
			},
		},
		{
			Name: "file", Version: "1.0", Source: "builtin",
			Caps:          []string{"read", "write", "append", "exists"},
			Params:        map[string]string{"path": "string,required", "content": "string,optional"},
			SideEffects:   []string{"write/append 修改文件（写前备份到 log_dir/backups）"},
			AllowedSpaces: []string{"project", "sandbox", "vault-notes"},
			Risk: map[string]string{
				"read": "none", "exists": "none", "append": "low", "write": "high",
			},
		},
		{
			Name: "search", Version: "1.0", Source: "builtin",
			Caps:          []string{"text", "symbol"},
			Params:        map[string]string{"pattern": "string,required"},
			SideEffects:   []string{"只读扫描"},
			AllowedSpaces: []string{"global", "project", "sandbox", "vault-notes", "vault-creds"},
			Risk: map[string]string{
				"text": "none", "symbol": "none",
			},
		},
		{
			Name: "test", Version: "1.0", Source: "builtin",
			Caps:          []string{"run"},
			Params:        map[string]string{"command": "[]string,required"},
			SideEffects:   []string{"跑测试（只读为主，可写临时产物）"},
			AllowedSpaces: []string{"project", "sandbox"},
			Risk:          map[string]string{"run": "low"},
		},
		{
			Name: "run", Version: "1.0", Source: "builtin",
			Caps:          []string{"exec"},
			Params:        map[string]string{"command": "[]string,required"},
			SideEffects:   []string{"任意命令（高风险，gate 在 pipeline）"},
			AllowedSpaces: []string{"project", "sandbox"},
			Risk:          map[string]string{"exec": "high"},
		},
		{
			Name: "verify", Version: "1.0", Source: "builtin",
			Caps:          []string{"run"},
			Params:        map[string]string{"kind": "string,required", "args": "[]string,optional"},
			SideEffects:   []string{"独立只读复核（读 fs/重跑命令，不改状态）"},
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
		return nil, fmt.Errorf("读取契约目录 %s 失败: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".contract.json") {
			continue
		}
		data, rerr := os.ReadFile(filepath.Join(dir, e.Name()))
		if rerr != nil {
			return nil, fmt.Errorf("读取契约文件 %s 失败: %w", e.Name(), rerr)
		}
		var c contract.ToolContract
		if jerr := json.Unmarshal(data, &c); jerr != nil {
			return nil, fmt.Errorf("解析契约文件 %s 失败: %w", e.Name(), jerr)
		}
		if err := ValidateContract(c); err != nil {
			return nil, fmt.Errorf("契约文件 %s 不合法: %w", e.Name(), err)
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
		return fmt.Errorf("契约 name 必填")
	}
	if strings.TrimSpace(c.Version) == "" {
		return fmt.Errorf("契约 %q version 必填", c.Name)
	}
	if len(c.Caps) == 0 {
		return fmt.Errorf("契约 %q 至少声明一个 cap", c.Name)
	}
	if len(c.Params) == 0 {
		return fmt.Errorf("契约 %q 至少声明一个 param", c.Name)
	}
	if len(c.Risk) == 0 {
		return fmt.Errorf("契约 %q 必须声明 risk 映射", c.Name)
	}
	for _, cap := range c.Caps {
		lvl, ok := c.Risk[cap]
		if !ok {
			return fmt.Errorf("契约 %q 的 cap %q 缺少 risk 分级", c.Name, cap)
		}
		if !validRiskLevels[lvl] {
			return fmt.Errorf("契约 %q 的 cap %q risk 级别 %q 非法（none|low|medium|high|irreversible）", c.Name, cap, lvl)
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
		return fmt.Errorf("契约校验未过，拒绝注册: %w", err)
	}
	grade := riskGrade(c)
	if !approved {
		return fmt.Errorf("契约 %q 风险等级 %s 未经人工确认（approved=false），拒绝落盘", c.Name, grade)
	}
	if strings.TrimSpace(r.dir) == "" {
		return fmt.Errorf("注册表未绑定契约目录（非 LoadContracts 装载），无法落盘 %q", c.Name)
	}
	c.Source = "voice"
	c.RegisteredAt = time.Now().Format(time.RFC3339)
	path := filepath.Join(r.dir, c.Name+".contract.json")
	if err := os.MkdirAll(r.dir, 0o755); err != nil {
		return fmt.Errorf("创建契约目录失败: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化契约失败: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("写契约临时文件失败: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("提交契约文件失败: %w", err)
	}
	r.Contracts[c.Name] = c
	return nil
}
