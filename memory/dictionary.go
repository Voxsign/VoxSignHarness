// Package memory is  onunder    (   §7). 
// V0 stagethis packageonly now"  word ":     manage ,  in    changebodycorrection, 
//  type showwordnotein  lang pos,  to use(   §5.4). 
// this packageonlydependencytgtapprove (os/json)+ contract,  dependency config/input/router. 
package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"voicesign-harness/contract"
)

// Term is  word  obj: pos write  Term +  see  diffchangebody Variants. 
type Term struct {
	Term     string   `json:"term"`
	Variants []string `json:"variants"`
	Category string   `json:"category"`
	Source   string   `json:"source"` // manual | model | builtin
	LastUsed string   `json:"last_used,omitempty"`
}

// Dictionary is  word . Path as    ; AddTerm timewriteback path. 
type Dictionary struct {
	Path    string `json:"-"` //     ; AddTerm timewriteback
	Terms   []Term `json:"terms"`
	Version int    `json:"version"`
}

// builtinDictionary returnback   §5.4    in defaultword (source=builtin,    ). 
// note : same  Term  changebodyby" changebody before" list,   "  " first  "   ". 
func builtinDictionary() *Dictionary {
	return &Dictionary{
		Version: 1,
		Terms: []Term{
			{Term: "VoxSign", Variants: []string{"voxsign", "沃克斯赛因"}, Category: "产品名", Source: "builtin"},
			{Term: "center", Variants: []string{"中枢", "森特"}, Category: "架构名", Source: "builtin"},
		},
	}
}

// LoadDictionary from path   word . file store timereturnbackin defaultword (   ); 
// filestore but JSON   then  . 
func LoadDictionary(path string) (*Dictionary, error) {
	d := builtinDictionary()
	d.Path = path

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// file store  -> in defaultword ,    file. 
			return d, nil
		}
		return nil, fmt.Errorf("读取词典 %s 失败: %w", path, err)
	}
	if err := json.Unmarshal(data, d); err != nil {
		return nil, fmt.Errorf("解析词典 %s 失败: %w", path, err)
	}
	d.Path = path
	return d, nil
}

// Correct   obj changebody   write     : 
//   -  in  changebody:  connect    (in noword boundary); 
//   -    /numchar/domainnamechangebody: byfinish  token  boundary  ,        (e.g. xvoxsigny). 
//
//    possendoccurchangeize        contract.Correction{From,To,Rule:"dict"}, 
// From get basein  outnow write . returnbackfixposafter baseandcorrection  . 
func (d *Dictionary) Correct(text string) (string, []contract.Correction) {
	var corrections []contract.Correction
	cur := text
	for _, term := range d.Terms {
		for _, variant := range term.Variants {
			if variant == "" {
				continue
			}
			boundary := !hasCJK(variant) //   ASCII changebodyby token  boundary  
			var froms []string
			cur, froms = replaceCI(cur, variant, term.Term, boundary)
			for _, from := range froms {
				corrections = append(corrections, contract.Correction{
					From: from,
					To:   term.Term,
					Rule: "dict",
				})
			}
		}
	}
	return cur, corrections
}

// Render    showwordnotein ( sent   ,  pos write andchangebody), 
// useatnotein typefirst  user   before (   §5.4  to useof typeside). 
func (d *Dictionary) Render() string {
	var sb strings.Builder
	sb.WriteString("个人词典（输出请使用正确写法）：")
	for _, t := range d.Terms {
		fmt.Fprintf(&sb, "\n- 正确写法：%s", t.Term)
		if len(t.Variants) > 0 {
			fmt.Fprintf(&sb, "；常见误识别：%s", strings.Join(t.Variants, "、"))
		}
	}
	return sb.String()
}

// AddTerm      objandheavywrite JSON to d.Path(keep  source tgt ). 
// Path asempty( in word ,  refer   path)timeonly  toinstore. 
func (d *Dictionary) AddTerm(t Term) error {
	d.Terms = append(d.Terms, t)
	if d.Path == "" {
		return nil
	}
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化词典失败: %w", err)
	}
	if dir := filepath.Dir(d.Path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("创建词典目录失败: %w", err)
		}
	}
	if err := os.WriteFile(d.Path, data, 0o600); err != nil {
		return fmt.Errorf("写回词典 %s 失败: %w", d.Path, err)
	}
	return nil
}

// hasCJK   char  is  inday   table  char(useat     alsois token  boundary  ). 
func hasCJK(s string) bool {
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}

// isASCIIAlnum   charnodeis as ASCII  wordchar (char /numchar/under line), useat token  boundary  . 
func isASCIIAlnum(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') || b == '_'
}

// replaceCI   text in  write   lypipe from   as to. 
// boundary=true timeonlycur    side   ASCII  wordchar only  (finish  token). 
// returnback  after  baseand    be    orig  seg(matched != to only  ). 
func replaceCI(text, from, to string, boundary bool) (string, []string) {
	if from == "" || len(text) < len(from) {
		return text, nil
	}
	lowText := strings.ToLower(text)
	lowFrom := strings.ToLower(from)

	var sb strings.Builder
	var froms []string
	i := 0
	for i < len(text) {
		if len(text)-i < len(from) {
			sb.WriteString(text[i:])
			break
		}
		idx := strings.Index(lowText[i:], lowFrom)
		if idx < 0 {
			sb.WriteString(text[i:])
			break
		}
		start := i + idx
		end := start + len(from)

		if boundary {
			leftOK := start == 0 || !isASCIIAlnum(text[start-1])
			rightOK := end >= len(text) || !isASCIIAlnum(text[end])
			if !leftOK || !rightOK {
				//  finish  token: before   charnodecontinuecontinue 
				sb.WriteString(text[i : start+1])
				i = start + 1
				continue
			}
		}

		matched := text[start:end]
		sb.WriteString(text[i:start])
		if matched != to {
			sb.WriteString(to)
			froms = append(froms, matched)
		} else {
			// already ispos write ,    correction
			sb.WriteString(matched)
		}
		i = end
	}
	return sb.String(), froms
}
