// Package memory 是个人上下文沉淀层（架构 §7）。
// V0 阶段本包仅实现「个人词典」：归记忆层管理，输入容错层做变体纠错、
// 模型提示词注入做术语校正，双向复用（架构 §5.4）。
// 本包只依赖标准库（os/json）+ contract，不依赖 config/input/router。
package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"voicesign-harness/contract"
)

// Term 是一条词典条目：正确写法 Term + 常见误识别变体 Variants。
type Term struct {
	Term     string   `json:"term"`
	Variants []string `json:"variants"`
	Category string   `json:"category"`
	Source   string   `json:"source"` // manual | model | builtin
	LastUsed string   `json:"last_used,omitempty"`
}

// Dictionary 是个人词典。Path 为加载来源；AddTerm 时写回该路径。
type Dictionary struct {
	Path    string `json:"-"` // 加载来源；AddTerm 时写回
	Terms   []Term `json:"terms"`
	Version int    `json:"version"`
}

// builtinDictionary 返回架构 §5.4 的五条内置默认词典（source=builtin，不落盘）。
// 注意：同一 Term 的变体按「长变体在前」排列，避免「曼苏」抢先匹配「曼苏尔」。
func builtinDictionary() *Dictionary {
	return &Dictionary{
		Version: 1,
		Terms: []Term{
			{Term: "Mansour", Variants: []string{"美墅", "曼苏尔", "曼苏"}, Category: "人名", Source: "builtin"},
			{Term: "冀总", Variants: []string{"季总"}, Category: "称呼", Source: "builtin"},
			{Term: "model.peterzou.com", Variants: []string{"彼得周点com", "model彼得周"}, Category: "域名", Source: "builtin"},
			{Term: "VoxSign", Variants: []string{"voxsign", "沃克斯赛因"}, Category: "产品名", Source: "builtin"},
			{Term: "center", Variants: []string{"中枢", "森特"}, Category: "架构名", Source: "builtin"},
			{Term: "In scope", Variants: []string{"in 死 cope", "因死 cope", "因斯科普"}, Category: "术语", Source: "builtin"}, // F7 修复：真实测试 R8 ASR 噪声「in 死 cope」→ In scope
		},
	}
}

// LoadDictionary 从 path 加载词典。文件不存在时返回内置默认词典（不落盘）；
// 文件存在但 JSON 损坏则报错。
func LoadDictionary(path string) (*Dictionary, error) {
	d := builtinDictionary()
	d.Path = path

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// 文件不存在 → 内置默认词典，不创建文件。
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

// Correct 逐条目逐变体做大小写不敏感替换：
//   - 含中文的变体：直接子串替换（中文无词边界）；
//   - 纯英文/数字/域名变体：按完整 token 边界替换，避免误匹配子串（如 xvoxsigny）。
//
// 每条真正发生变化的替换记录一条 contract.Correction{From,To,Rule:"dict"}，
// From 取文本中实际出现的写法。返回修正后文本与纠错记录。
func (d *Dictionary) Correct(text string) (string, []contract.Correction) {
	var corrections []contract.Correction
	cur := text
	for _, term := range d.Terms {
		for _, variant := range term.Variants {
			if variant == "" {
				continue
			}
			boundary := !hasCJK(variant) // 纯 ASCII 变体按 token 边界替换
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

// Render 渲染提示词注入块（一句话一行，含正确写法与变体），
// 用于注入模型首条 user 消息前缀（架构 §5.4 双向复用之模型侧）。
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

// AddTerm 追加一条条目并重写 JSON 到 d.Path（保留 source 标记）。
// Path 为空（纯内置词典、未指定落盘路径）时仅追加到内存。
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

// hasCJK 报告字符串是否含中日韩统一表意文字（用于判定走子串还是 token 边界替换）。
func hasCJK(s string) bool {
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}

// isASCIIAlnum 报告字节是否为 ASCII 单词字符（字母/数字/下划线），用于 token 边界判定。
func isASCIIAlnum(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') || b == '_'
}

// replaceCI 在 text 中大小写不敏感地把 from 替换为 to。
// boundary=true 时仅当匹配串两侧均非 ASCII 单词字符才替换（完整 token）。
// 返回替换后的文本与每条实际被替换掉的原文片段（matched != to 才记录）。
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
				// 非完整 token：前进一个字节继续找
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
			// 已经是正确写法，不记录纠错
			sb.WriteString(matched)
		}
		i = end
	}
	return sb.String(), froms
}
