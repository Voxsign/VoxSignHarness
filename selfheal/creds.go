package selfheal

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"voicesign-harness/config"
)

// modelCenterCredsFile 是模型中心凭证文件（~/.modelcenter/creds.txt）。
// 文件形态兼容两种：键值对 "MAIN_KEY=sk-mc-xxx" 或裸行 "sk-mc-xxx"；
// 逐行扫描，跳过 # 注释行，取首个 sk-mc- 开头的 token。
//
// 安全红线：key 绝不进入代码常量 / 日志 / commit / 错误信息。本函数只返回字符串给
// provider 传输层塞进 Authorization 头，不做任何打印。
const modelCenterCredsFile = ".modelcenter/creds.txt"

// ReadModelCenterKey 从 ~/.modelcenter/creds.txt 解析模型中心 key。
// 文件不存在 / 无 sk-mc- token → 返回 ""（调用方据此让诊断层跳过，不报错）。
func ReadModelCenterKey() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	f, err := os.Open(filepath.Join(home, modelCenterCredsFile))
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024), 64*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// 键值对形态：取 '=' 右侧。
		if _, rhs, ok := strings.Cut(line, "="); ok {
			line = strings.TrimSpace(rhs)
		}
		// 行内可能有引号/空格，取首个 sk-mc- 开头 token。
		for _, tok := range strings.Fields(line) {
			tok = strings.Trim(tok, `"' `)
			if strings.HasPrefix(tok, "sk-mc-") {
				return tok
			}
		}
	}
	return ""
}

// PrepareDiagKey 在 provider.NewRegistry 之前调用：若 config 声明了 diag provider
// 且未显式配置 api_key，则从模型中心凭证文件补填 key。
// 已显式配置 api_key 的 diag provider【优先用它】，不被覆盖；
// 凭证缺失 → diag 保持空 key（后续 Chat 401 → 诊断层优雅跳过）。
// 未声明 diag provider → 零动作（诊断层整体跳过，主链不变）。
func PrepareDiagKey(cfg *config.Config) {
	if cfg == nil {
		return
	}
	p, ok := cfg.DiagProvider()
	if !ok {
		return
	}
	if strings.TrimSpace(p.APIKey) != "" {
		return // 显式配置优先
	}
	if key := ReadModelCenterKey(); key != "" {
		p.APIKey = key
		for i := range cfg.Providers {
			if cfg.Providers[i].Name == p.Name {
				cfg.Providers[i].APIKey = key
				break
			}
		}
	}
}
