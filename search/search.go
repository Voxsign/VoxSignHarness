// Package search 实现纯 Go 递归代码扫描（search 契约，M2 任务卡 #7 / 验收冒烟）。
// 零第三方依赖、不调外部 grep：自己 filepath.Walk + 行级匹配。尊重 ignore glob。
// 本包为薄封装，豁免伪代码逻辑层关卡。
package search

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Hit 是一次命中：文件 + 行号（1-based）+ 该行原文 + 命中种类。
type Hit struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	LineText string `json:"line_text"`
	Kind     string `json:"kind"` // symbol: func|type|const|var；text: "text"
}

// Options 扫描选项。Roots 空 → "."；Ignore 按路径段 glob（filepath.Match）过滤；
// MaxFiles/MaxHits 为防御性上限（<=0 取默认）。
type Options struct {
	Roots    []string
	Ignore   []string
	MaxFiles int
	MaxHits  int
}

const (
	defaultMaxFiles = 4096
	defaultMaxHits  = 1000
)

// isIgnored 判断某个路径段（目录/文件名）是否命中 ignore glob。
// 命中任一 ignore 模式即跳过（连同其子树）。
func isIgnored(name string, ignore []string) bool {
	for _, pat := range ignore {
		pat = strings.TrimSpace(pat)
		if pat == "" {
			continue
		}
		if ok, _ := filepath.Match(pat, name); ok {
			return true
		}
		// 支持 "**/vendor" 这类：末段匹配即可
		if idx := strings.LastIndex(pat, "/"); idx >= 0 {
			if ok, _ := filepath.Match(pat[idx+1:], name); ok {
				return true
			}
		}
	}
	return false
}

// collectFiles 收集 roots 下常规文件路径，尊重 ignore 与 maxFiles。
func collectFiles(opts Options, goOnly bool) ([]string, error) {
	roots := opts.Roots
	if len(roots) == 0 {
		roots = []string{"."}
	}
	maxFiles := opts.MaxFiles
	if maxFiles <= 0 {
		maxFiles = defaultMaxFiles
	}
	var out []string
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		fi, err := os.Stat(root)
		if err != nil {
			return nil, fmt.Errorf("扫描根 %q 不可访问: %w", root, err)
		}
		if !fi.IsDir() {
			out = append(out, root)
			continue
		}
		err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // 跳过无权限项
			}
			base := info.Name()
			if info.IsDir() {
				if isIgnored(base, opts.Ignore) {
					return filepath.SkipDir
				}
				return nil
			}
			if isIgnored(base, opts.Ignore) {
				return nil
			}
			if goOnly && !strings.HasSuffix(base, ".go") {
				return nil
			}
			out = append(out, path)
			if len(out) >= maxFiles {
				return errMaxHits // 触发提前中止
			}
			return nil
		})
		_ = err
	}
	return out, nil
}

var errMaxHits = fmt.Errorf("达到文件数上限")

// lineCount 返回命中截断后的最大命中数。
func hitCap(opts Options) int {
	if opts.MaxHits > 0 {
		return opts.MaxHits
	}
	return defaultMaxHits
}

// FindSymbol 定位 Go 标识符定义（func/type/const/var），纯 Go 扫描，尊重 ignore。
func FindSymbol(name string, opts Options) ([]Hit, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("符号名为空")
	}
	files, err := collectFiles(opts, true)
	if err != nil {
		return nil, err
	}
	cap := hitCap(opts)
	quoted := regexp.QuoteMeta(name)
	patterns := []struct {
		kind string
		re   *regexp.Regexp
	}{
		{"func", regexp.MustCompile(`^func(\s+\([^)]*\)\s+|\s+)` + quoted + `\b`)},
		{"type", regexp.MustCompile(`^type\s+` + quoted + `\b`)},
		{"var", regexp.MustCompile(`^var\s+` + quoted + `\b`)},
		{"const", regexp.MustCompile(`^const\s+` + quoted + `\b`)},
	}
	var hits []Hit
	for _, f := range files {
		appendSymbolHits(f, patterns, &hits, cap)
		if len(hits) >= cap {
			break
		}
	}
	return hits, nil
}

// appendSymbolHits 在单个文件里逐行跑四类声明正则。
func appendSymbolHits(path string, patterns []struct {
	kind string
	re   *regexp.Regexp
}, hits *[]Hit, cap int) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	line := 0
	for sc.Scan() {
		line++
		t := sc.Text()
		for _, p := range patterns {
			if p.re.MatchString(strings.TrimSpace(t)) {
				*hits = append(*hits, Hit{File: path, Line: line, LineText: strings.TrimSpace(t), Kind: p.kind})
				if len(*hits) >= cap {
					return
				}
			}
		}
	}
}

// FindText 子串匹配（rune 安全由 strings.Contains 承担），尊重 ignore。
func FindText(pattern string, opts Options) ([]Hit, error) {
	if pattern == "" {
		return nil, fmt.Errorf("搜索模式为空")
	}
	files, err := collectFiles(opts, false)
	if err != nil {
		return nil, err
	}
	cap := hitCap(opts)
	var hits []Hit
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		// 跳过疑似二进制文件
		if bytesContainsNUL(data) {
			continue
		}
		sc := bufio.NewScanner(strings.NewReader(string(data)))
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		line := 0
		for sc.Scan() {
			line++
			t := sc.Text()
			if strings.Contains(t, pattern) {
				hits = append(hits, Hit{File: f, Line: line, LineText: strings.TrimSpace(t), Kind: "text"})
				if len(hits) >= cap {
					return hits, nil
				}
			}
		}
	}
	return hits, nil
}

func bytesContainsNUL(b []byte) bool {
	for _, c := range b {
		if c == 0 {
			return true
		}
	}
	return false
}
