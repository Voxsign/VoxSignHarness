// Package ground 是 M3 #37「认知切片 context 注入」的实现：
// 在每任务模型调用 / 回问之前，把两类数据源渲染成一段固定上下文块：
//   - project-map：space 注册表（注册域名 + 项目根目录文件清单）
//   - decisions：<log_dir>/decisions.jsonl（历次确认裁决：auto/light/strong/human + 放行/拒绝 + 理由）
//
// 设计不变量（SPEC v2 §2.37）：
//   - 缺失行为：数据源文件不存在/为空 → 空 context，不报错，Ask 照常走。
//   - 长度限制：渲染块字节上限 + 决策条目上限，超出截断最旧（常量 + 可选 env）。
//   - 只读注入：ground 只【读】认知切片给模型/人；绝不反写词典/策略/风险阈值。
//   - 数据源只准用 harness 自己的 <log_dir>，绝不碰 ~/.voicesign 用户真实文件。
package ground

// 【伪代码逻辑层】（必写模块：注入协议属判断类逻辑；规则语义搬 VSL，此处只写控制流）
//
// Render() -> Snapshot：
//   pm = 渲染 project-map：
//        for name in spaces.List():
//           m = spaces.Get(name)
//           line = "project-map:" + name + "(" + m.Type + ")"
//           for scope in m.Scope: line += " [" + scope + 目录前 8 项文件清单 + "]"
//   ds = ReadDecisions(<log_dir>/decisions.jsonl)   // 缺失 → 空切片
//   ds = 截断：if len(ds) > MaxDecisions: ds = ds[len-MaxDecisions:]
//   block = "认知切片（越旧越靠前，最旧截断）：\n"
//         + join(pm, "\n")
//         + "\n近期裁决：\n" + join(ds.render(), "\n")
//   if len(block bytes) > MaxBytes: 按字节截尾并加 "…(截断)"
//   return Snapshot{ProjectMap: pm, Decisions: ds, Block: block}
//
// RecordDecision(d) -> error：
//   打开（O_APPEND|O_CREATE）<log_dir>/decisions.jsonl（0o600）
//   序列化 d 一行 → 写盘；失败不阻断只读任务（#44）。
//
// 异常：space 注册表为 nil → project-map 空；log_dir 不可写 → RecordDecision 返回 error 但 Render 不炸。

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"voicesign-harness/space"
)

const (
	// DefaultMaxBytes 是渲染块默认字节上限（约一屏上下文）。
	DefaultMaxBytes = 2000
	// DefaultMaxDecisions 是保留的最近裁决条目上限。
	DefaultMaxDecisions = 20
	// decisionsFile 是裁决日志文件名（<log_dir>/decisions.jsonl）。
	decisionsFile = "decisions.jsonl"
)

// Decision 是一次确认闸裁决的落盘记录（人可见 + 下一轮注入）。
type Decision struct {
	Ts       string `json:"ts"`
	TaskID   string `json:"task_id"`
	Intent   string `json:"intent"`
	Decision string `json:"decision"` // auto|light|strong|human
	Confirm  string `json:"confirm"`  // approved|rejected|auto_skipped
	Reason   string `json:"reason"`
}

// Snapshot 是渲染好的认知切片（注入到 prompt / 回问 / 确认文案前）。
type Snapshot struct {
	ProjectMap []string   `json:"project_map"`
	Decisions  []Decision `json:"decisions"`
	Block      string     `json:"block"`
}

// Ground 持有 log_dir 与 space 注册表；零值可用（各方法 nil-safe）。
type Ground struct {
	LogDir       string
	Spaces       *space.Registry
	MaxBytes     int
	MaxDecisions int
}

// New 构造 Ground；maxBytes/maxDecisions 取默认值（<=0 时）。
func New(logDir string, spaces *space.Registry) *Ground {
	return &Ground{LogDir: logDir, Spaces: spaces}
}

func (g *Ground) maxBytes() int {
	if g.MaxBytes > 0 {
		return g.MaxBytes
	}
	return DefaultMaxBytes
}

func (g *Ground) maxDecisions() int {
	if g.MaxDecisions > 0 {
		return g.MaxDecisions
	}
	return DefaultMaxDecisions
}

// Render 渲染认知切片块。数据源缺失 → 空块，不报错。
func (g *Ground) Render() Snapshot {
	snap := Snapshot{}

	// project-map：注册域名 + scope 目录清单
	if g.Spaces != nil {
		for _, name := range g.Spaces.List() {
			m, ok := g.Spaces.Get(name)
			if !ok {
				continue
			}
			line := "project-map:" + name + "(" + m.Type + ")"
			if listing := g.scopeListing(m); listing != "" {
				line += " " + listing
			}
			snap.ProjectMap = append(snap.ProjectMap, line)
		}
	}

	// decisions：读 <log_dir>/decisions.jsonl，截断最旧
	snap.Decisions = g.readDecisions()

	// 渲染块
	var sb strings.Builder
	sb.WriteString("认知切片（最旧已截断）：\n")
	if len(snap.ProjectMap) == 0 {
		sb.WriteString("  （无注册域）\n")
	} else {
		for _, p := range snap.ProjectMap {
			sb.WriteString("  " + p + "\n")
		}
	}
	sb.WriteString("近期裁决：\n")
	if len(snap.Decisions) == 0 {
		sb.WriteString("  （无）\n")
	} else {
		for _, d := range snap.Decisions {
			fmt.Fprintf(&sb, "  [%s] %s %s/%s：%s\n", d.Ts, d.Intent, d.Decision, d.Confirm, d.Reason)
		}
	}
	snap.Block = truncateBytes(sb.String(), g.maxBytes())
	return snap
}

// scopeListing 列出 manifest scope 目录下前 8 项（机器清单，人读项目结构）。
func (g *Ground) scopeListing(m *space.Manifest) string {
	var parts []string
	for _, scope := range m.Scope {
		scope = strings.TrimSuffix(strings.TrimSuffix(scope, "/**"), "/*")
		if scope == "" || scope == "." || strings.HasPrefix(scope, "~") {
			continue
		}
		entries, err := os.ReadDir(scope)
		if err != nil {
			continue
		}
		names := make([]string, 0, 8)
		for _, e := range entries {
			if e.IsDir() {
				names = append(names, e.Name()+"/")
			} else {
				names = append(names, e.Name())
			}
			if len(names) >= 8 {
				break
			}
		}
		if len(names) > 0 {
			parts = append(parts, scope+"={"+strings.Join(names, ",")+"}")
		}
	}
	return strings.Join(parts, " ")
}

// readDecisions 读取并反序列化 decisions.jsonl；文件缺失/损坏行 → 空切片，不报错。
func (g *Ground) readDecisions() []Decision {
	if g.LogDir == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(g.LogDir, decisionsFile))
	if err != nil {
		return nil // 缺失 → 空认知切片（不报错）
	}
	var all []Decision
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var d Decision
		if json.Unmarshal([]byte(line), &d) == nil {
			all = append(all, d)
		}
	}
	// 截断最旧：只保留最近 maxDecisions 条
	if len(all) > g.maxDecisions() {
		all = all[len(all)-g.maxDecisions():]
	}
	// 稳定顺序（旧→新，便于阅读）
	sort.SliceStable(all, func(i, j int) bool { return all[i].Ts < all[j].Ts })
	return all
}

// RecordDecision 在确认闸落盘一条裁决（追加；失败不阻断只读任务）。
func (g *Ground) RecordDecision(d Decision) error {
	if g.LogDir == "" {
		return nil
	}
	if err := os.MkdirAll(g.LogDir, 0o700); err != nil {
		return fmt.Errorf("创建 log_dir 失败: %w", err)
	}
	b, err := json.Marshal(d)
	if err != nil {
		return fmt.Errorf("序列化裁决失败: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(g.LogDir, decisionsFile), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("打开 decisions.jsonl 失败: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("写裁决失败: %w", err)
	}
	return nil
}

// truncateBytes 按字节上限截断（rune 安全：截到边界后若落在多字节中间则回退）。
func truncateBytes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := s[:max]
	// 回退到最后一个完整 rune
	for i := len(cut); i > 0; i-- {
		if r := cut[i-1]; r < 0x80 || r >= 0xC0 {
			return cut[:i] + "…(截断)"
		}
	}
	return cut + "…(截断)"
}
