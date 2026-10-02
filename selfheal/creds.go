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

// PrepareDiagKey 在 provider.NewRegistry 之前调用，对声明的 diag provider 做两件事：
//  1. 超时预算：未显式配置 TimeoutMs 时默认 30000ms（避免干等 Global.LLMTimeoutMs=60s）；
//     超时→传输层返回错误→诊断层优雅跳过，不阻断主链。
//  2. 凭证：若未显式配置 api_key，从模型中心凭证文件补填。
//
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
	// ① diag 默认 30s 超时预算（独立于 key 来源；显式配置不覆盖）。
	if p.TimeoutMs <= 0 {
		for i := range cfg.Providers {
			if cfg.Providers[i].Name == p.Name {
				cfg.Providers[i].TimeoutMs = DiagDefaultTimeoutMs
				break
			}
		}
	}
	// ② 凭证补填（显式 api_key 优先）。
	if strings.TrimSpace(p.APIKey) != "" {
		return
	}
	if key := ReadModelCenterKey(); key != "" {
		for i := range cfg.Providers {
			if cfg.Providers[i].Name == p.Name {
				cfg.Providers[i].APIKey = key
				break
			}
		}
	}
}

// DiagDefaultTimeoutMs 是 diag provider 未显式配置 TimeoutMs 时的默认超时（30s）。
const DiagDefaultTimeoutMs = 30000
