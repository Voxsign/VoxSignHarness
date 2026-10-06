// Package search  now  Go    code  (search   , M2 task  #7 /  recv  ). 
//     dependency,  callout  grep:    filepath.Walk +     .  heavy ignore glob. 
// this packageas   ,   pseudocode logic layerclose . 
package search

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Hit is   in: file +  id(1-based)+   orig  +  inkindclass. 
type Hit struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	LineText string `json:"line_text"`
	Kind     string `json:"kind"` // symbol: func|type|const|var; text: "text"
}

// Options     . Roots empty -> "."; Ignore bypathseg glob(filepath.Match)ed ; 
// MaxFiles/MaxHits asprevent ityonlimit(<=0 getdefault). 
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

// isIgnored  disconnect  pathseg(obj /filename)is  in ignore glob. 
//  in   ignore  formi.e. ed(linksameits  ). 
func isIgnored(name string, ignore []string) bool {
	for _, pat := range ignore {
		pat = strings.TrimSpace(pat)
		if pat == "" {
			continue
		}
		if ok, _ := filepath.Match(pat, name); ok {
			return true
		}
		//  keep "**/vendor"  class: endseg  i.e. 
		if idx := strings.LastIndex(pat, "/"); idx >= 0 {
			if ok, _ := filepath.Match(pat[idx+1:], name); ok {
				return true
			}
		}
	}
	return false
}

// collectFiles recv  roots under rulefilepath,  heavy ignore and maxFiles. 
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
			return nil, fmt.Errorf("scan root %q not accessible: %w", root, err)
		}
		if !fi.IsDir() {
			out = append(out, root)
			continue
		}
		err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil //  edno limit 
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
				return errMaxHits // triggersend beforeinstop
			}
			return nil
		})
		_ = err
	}
	return out, nil
}

var errMaxHits = fmt.Errorf("file hit limit reached")

// lineCount returnback in disconnectafter    innum. 
func hitCap(opts Options) int {
	if opts.MaxHits > 0 {
		return opts.MaxHits
	}
	return defaultMaxHits
}

// FindSymbol    Go tgt  define(func/type/const/var),   Go   ,  heavy ignore. 
func FindSymbol(name string, opts Options) ([]Hit, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("symbol name is empty")
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

// appendSymbolHits    file     classvoice posthen. 
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

// FindText     (rune safesafetyby strings.Contains   ),  heavy ignore. 
func FindText(pattern string, opts Options) ([]Hit, error) {
	if pattern == "" {
		return nil, fmt.Errorf("search pattern is empty")
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
		//  ed    restrictfile
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
