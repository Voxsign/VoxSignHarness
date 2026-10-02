// Package config 加载 harness 五节配置（global / providers / routes / input / memory）：
// 内置默认值 → JSON 配置文件（VHS_CONFIG 或默认路径）→ 环境变量覆盖（最高优先级）。
// 本包只依赖标准库，是全部模块的编译基座；不记录任何密钥。
package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	// MockKind 是内置离线 mock provider 的 kind（无 key 跑通闭环，开发/演示用）。
	MockKind = "mock"
	// OpenAIKind 是 OpenAI 兼容端点的 kind。
	OpenAIKind = "openai"
	// LocalProvider 是路由到「本地直通、不调 LLM」的特殊 provider 名（如 TIME）。
	LocalProvider = "local"
)

// Config 是五节配置的根结构。
type Config struct {
	Global    Global     `json:"global"`
	Providers []Provider `json:"providers"`
	Routes    []Route    `json:"routes"`
	Input     InputCfg   `json:"input"`
	Memory    MemoryCfg  `json:"memory"`

	// M2 新增节（SPACE 域注册表 / 工具契约 / 四元组缓存 / 手机 API 服务）。
	Spaces    SpacesCfg    `json:"spaces,omitempty"`
	Contracts ContractsCfg `json:"contracts,omitempty"`
	Cache     CacheCfg     `json:"cache,omitempty"`
	Server    ServerCfg    `json:"server,omitempty"`

	ConfigPath     string   `json:"-"` // 实际加载的配置文件路径
	Warnings       []string `json:"-"` // 非致命问题（如缺 API key）
	ForcedRoute    string   `json:"-"` // env VHS_ROUTE：强制路由名
	ForcedProvider string   `json:"-"` // env VHS_PROVIDER：强制 provider 名
}

// SpacesCfg 域注册表（.space.json manifest 目录）。
type SpacesCfg struct {
	Dir string `json:"dir,omitempty"` // 默认 <log_dir>/spaces
}

// ContractsCfg 工具契约目录（.contract.json；REGISTER_TOOL 注册落盘处）。
type ContractsCfg struct {
	Dir string `json:"dir,omitempty"` // 默认 <log_dir>/contracts
}

// CacheCfg 四元组缓存。
type CacheCfg struct {
	Dir        string `json:"dir,omitempty"`         // 默认 <log_dir>/cache
	TTLSeconds int    `json:"ttl_seconds,omitempty"` // 默认 86400（1 天）
}

// ServerCfg 手机 HTTP API 面（内网 + token 认证）。
type ServerCfg struct {
	Token string `json:"token,omitempty"` // 空 = 仅 127.0.0.1 本机访问免 token；非本机绑定必须配 token（启动告警）
	Bind  string `json:"bind,omitempty"`  // 空 = 取 Global.Addr（默认 127.0.0.1:8765）
}

// Global 全局参数。
type Global struct {
	LogDir             string `json:"log_dir"`               // 轨迹等日志目录，默认 ~/.voicesign/harness
	Addr               string `json:"addr"`                  // 本地 HTTP 监听地址，默认 127.0.0.1:8765
	MaxTurnsDefault    int    `json:"max_turns_default"`     // 默认 max_turns，默认 2
	LLMTimeoutMs       int    `json:"llm_timeout_ms"`        // 单次 LLM 调用超时，默认 60000
	ActionTimeoutMs    int    `json:"action_timeout_ms"`     // 单动作默认超时，默认 15000
	MaxActionTimeoutMs int    `json:"max_action_timeout_ms"` // 动作超时硬上限，默认 120000
	AllowHighRisk      bool   `json:"allow_high_risk"`       // 高危命令放行（默认 false）
	MaxOutputChars     int    `json:"max_output_chars"`      // 回执输出截断，默认 4000
}

// Provider 一个模型端点（OpenAI 兼容或 mock）。同一网关可声明多个 provider（不同 model）实现按场景选模型。
type Provider struct {
	Name           string         `json:"name"`
	Kind           string         `json:"kind"`                      // openai | mock
	Endpoint       string         `json:"endpoint"`                  // openai 必填
	Model          string         `json:"model"`                     // openai 必填
	APIKey         string         `json:"api_key,omitempty"`         // 可用 VHS_API_KEY 覆盖
	ResponseFormat *bool          `json:"response_format,omitempty"` // nil = 默认 true（请求 json_object 模式）
	TimeoutMs      int            `json:"timeout_ms,omitempty"`      // 0 = 用 Global.LLMTimeoutMs
	Params         map[string]any `json:"params,omitempty"`          // 按端点透传（如 reasoning_effort）
}

// Route 一条路由：按 意图 + 关键词 命中，先命中先得，Default 兜底。
type Route struct {
	Name     string   `json:"name"`
	Intent   []string `json:"intent,omitempty"`    // 命中 contract.Intent.Intent 类别
	Match    []string `json:"match,omitempty"`     // 命中提示词关键词（大小写不敏感子串）
	Provider string   `json:"provider"`            // provider 名或 local
	MaxTurns int      `json:"max_turns,omitempty"` // 0 = 用 Global.MaxTurnsDefault
	Default  bool     `json:"default,omitempty"`   // 兜底路由
}

// InputCfg 输入容错层参数。
type InputCfg struct {
	IntentConf     float64  `json:"intent_conf,omitempty"`     // 意图置信度阈值，默认 0.6
	LowConfAction  string   `json:"low_conf_action,omitempty"` // ask | model，默认 ask（model 澄清编排在后续里程碑）
	DictionaryPath string   `json:"dictionary_path,omitempty"` // 空 = <memory.dir>/dictionary.json
	Fillers        []string `json:"fillers,omitempty"`         // 覆盖默认填充词表
}

// MemoryCfg 记忆层参数。
type MemoryCfg struct {
	Dir        string `json:"dir,omitempty"`         // 默认 ~/.voicesign/harness/memory
	FactsLimit int    `json:"facts_limit,omitempty"` // 事实注入条数上限，默认 20
}

// Default 返回内置默认配置（含文档 §6 的示例 providers/routes）。
// 注意：派生目录（Memory/Spaces/Contracts/Cache 的 Dir）此处留空，
// 由 Load→validate() 在最终 Global.LogDir 确定后统一推导——
// 否则 VHS_LOG_DIR 覆盖 LogDir 时派生目录仍停留在旧默认 ~/.voicesign/harness/*，
// env 隔离运行会把注册表/缓存/契约读写进用户真实目录（M3 config bug 修复）。
func Default() Config {
	home, _ := os.UserHomeDir()
	logDir := filepath.Join(home, ".voicesign", "harness")
	trueVal := true
	return Config{
		Global: Global{
			LogDir:             logDir,
			Addr:               "127.0.0.1:8765",
			MaxTurnsDefault:    2,
			LLMTimeoutMs:       60000,
			ActionTimeoutMs:    15000,
			MaxActionTimeoutMs: 120000,
			AllowHighRisk:      false,
			MaxOutputChars:     4000,
		},
		Providers: []Provider{
			{Name: "center", Kind: OpenAIKind, Endpoint: "https://model.peterzou.com/v1", Model: "gpt-6-luna", Params: map[string]any{"use_max_completion_tokens": true}, ResponseFormat: &trueVal},
			{Name: "fast", Kind: OpenAIKind, Endpoint: "https://model.peterzou.com/v1", Model: "gpt-6-luna", Params: map[string]any{"use_max_completion_tokens": true}, ResponseFormat: &trueVal},
			{Name: "strong", Kind: OpenAIKind, Endpoint: "https://model.peterzou.com/v1", Model: "gpt-6-luna", Params: map[string]any{"use_max_completion_tokens": true}, ResponseFormat: &trueVal},
			{Name: "deepseek", Kind: OpenAIKind, Endpoint: "https://api.deepseek.com", Model: "deepseek-flash", ResponseFormat: &trueVal},
			{Name: "openai", Kind: OpenAIKind, Endpoint: "https://api.openai.com/v1", Model: "gpt-5.4-mini", ResponseFormat: &trueVal},
			{Name: "gemini", Kind: OpenAIKind, Endpoint: "https://generativelanguage.googleapis.com/v1beta/openai", Model: "gemini-3.8-flash", ResponseFormat: &trueVal},
			{Name: "mock", Kind: MockKind, Model: "mock"},
		},
		Routes: []Route{
			{Name: "time", Intent: []string{"TIME"}, Provider: LocalProvider},
			{Name: "file", Intent: []string{"FILE_READ", "FILE_WRITE", "FILE_LIST"}, Provider: "center", MaxTurns: 1},
			{Name: "intent", Intent: []string{"INFO"}, Match: []string{"翻译", "总结", "摘要", "问答"}, Provider: "fast", MaxTurns: 1},
			{Name: "complex", Match: []string{"代码", "脚本", "debug", "部署", "分析", "研究", "架构"}, Provider: "strong", MaxTurns: 2},
			{Name: "default", Provider: "center", MaxTurns: 2, Default: true},
		},
		Input: InputCfg{
			IntentConf:    0.6,
			LowConfAction: "ask",
		},
		Memory: MemoryCfg{
			FactsLimit: 20,
		},
		Spaces:    SpacesCfg{},
		Contracts: ContractsCfg{},
		Cache: CacheCfg{
			TTLSeconds: 86400,
		},
	}
}

// Load 解析配置：默认值 → 配置文件（VHS_CONFIG，或 ~/.voicesign/harness.json、./voicesign-harness.json）→ 环境变量。
// 显式指定 configPath 但文件不存在 → 报错；未显式指定且默认路径都不存在 → 纯默认值。
func Load(configPath string) (Config, error) {
	cfg := Default()
	if configPath == "" {
		configPath = os.Getenv("VHS_CONFIG")
	}
	if configPath == "" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			if fi, err := os.Stat(filepath.Join(home, ".voicesign", "harness.json")); err == nil && !fi.IsDir() {
				configPath = filepath.Join(home, ".voicesign", "harness.json")
			}
		}
	}
	if configPath == "" {
		if fi, err := os.Stat("voicesign-harness.json"); err == nil && !fi.IsDir() {
			configPath = "voicesign-harness.json"
		}
	}
	if configPath != "" {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return cfg, fmt.Errorf("读取配置文件 %s 失败: %w", configPath, err)
		}
		// 先解析到 raw map 再按节覆盖：切片节（providers/routes）必须整体替换，
		// 避免 encoding/json 对非空切片"就地复用已有元素、逐字段覆盖"导致默认值泄漏
		// （如默认 provider center 的 endpoint 混入文件声明的 provider）。
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(data, &raw); err != nil {
			return cfg, fmt.Errorf("解析配置文件 %s 失败: %w", configPath, err)
		}
		if v, ok := raw["global"]; ok {
			if err := json.Unmarshal(v, &cfg.Global); err != nil {
				return cfg, fmt.Errorf("解析配置文件 %s global 节失败: %w", configPath, err)
			}
		}
		if v, ok := raw["providers"]; ok {
			var ps []Provider
			if err := json.Unmarshal(v, &ps); err != nil {
				return cfg, fmt.Errorf("解析配置文件 %s providers 节失败: %w", configPath, err)
			}
			cfg.Providers = ps
		}
		if v, ok := raw["routes"]; ok {
			var rs []Route
			if err := json.Unmarshal(v, &rs); err != nil {
				return cfg, fmt.Errorf("解析配置文件 %s routes 节失败: %w", configPath, err)
			}
			cfg.Routes = rs
		}
		if v, ok := raw["input"]; ok {
			if err := json.Unmarshal(v, &cfg.Input); err != nil {
				return cfg, fmt.Errorf("解析配置文件 %s input 节失败: %w", configPath, err)
			}
		}
		if v, ok := raw["memory"]; ok {
			if err := json.Unmarshal(v, &cfg.Memory); err != nil {
				return cfg, fmt.Errorf("解析配置文件 %s memory 节失败: %w", configPath, err)
			}
		}
		if v, ok := raw["spaces"]; ok {
			if err := json.Unmarshal(v, &cfg.Spaces); err != nil {
				return cfg, fmt.Errorf("解析配置文件 %s spaces 节失败: %w", configPath, err)
			}
		}
		if v, ok := raw["contracts"]; ok {
			if err := json.Unmarshal(v, &cfg.Contracts); err != nil {
				return cfg, fmt.Errorf("解析配置文件 %s contracts 节失败: %w", configPath, err)
			}
		}
		if v, ok := raw["cache"]; ok {
			if err := json.Unmarshal(v, &cfg.Cache); err != nil {
				return cfg, fmt.Errorf("解析配置文件 %s cache 节失败: %w", configPath, err)
			}
		}
		if v, ok := raw["server"]; ok {
			if err := json.Unmarshal(v, &cfg.Server); err != nil {
				return cfg, fmt.Errorf("解析配置文件 %s server 节失败: %w", configPath, err)
			}
		}
		cfg.ConfigPath = configPath
	}

	// 环境变量覆盖（仅当已设置）。
	if v := os.Getenv("VHS_API_KEY"); v != "" {
		for i := range cfg.Providers {
			cfg.Providers[i].APIKey = v
		}
	}
	if v := os.Getenv("VHS_PROVIDER"); v != "" {
		cfg.ForcedProvider = v
	}
	if v := os.Getenv("VHS_ROUTE"); v != "" {
		cfg.ForcedRoute = v
	}
	envString(&cfg.Global.LogDir, "VHS_LOG_DIR")
	envString(&cfg.Global.Addr, "VHS_ADDR")
	envInt(&cfg.Global.MaxTurnsDefault, "VHS_MAX_TURNS")
	envBool(&cfg.Global.AllowHighRisk, "VHS_ALLOW_HIGH_RISK")
	envFloat(&cfg.Input.IntentConf, "VHS_INTENT_CONF")
	envString(&cfg.Input.LowConfAction, "VHS_LOW_CONF_ACTION")
	envString(&cfg.Server.Token, "VHS_TOKEN")

	// 校验与归一化。
	if err := cfg.validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	if c.Global.MaxTurnsDefault < 1 {
		c.Global.MaxTurnsDefault = 1
	}
	if c.Global.LLMTimeoutMs <= 0 {
		c.Global.LLMTimeoutMs = 60000
	}
	if c.Global.MaxActionTimeoutMs <= 0 {
		c.Global.MaxActionTimeoutMs = 120000
	}
	if c.Global.ActionTimeoutMs <= 0 {
		c.Global.ActionTimeoutMs = 15000
	}
	if c.Global.ActionTimeoutMs > c.Global.MaxActionTimeoutMs {
		c.Global.ActionTimeoutMs = c.Global.MaxActionTimeoutMs
	}
	if c.Global.MaxOutputChars <= 0 {
		c.Global.MaxOutputChars = 4000
	}
	if c.Global.MaxOutputChars > 1<<20 {
		c.Global.MaxOutputChars = 1 << 20
	}
	if strings.TrimSpace(c.Global.LogDir) == "" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			c.Global.LogDir = filepath.Join(home, ".voicesign", "harness")
		} else {
			c.Global.LogDir = filepath.Join(".", "vhs-logs")
		}
	}
	if strings.TrimSpace(c.Global.Addr) == "" {
		c.Global.Addr = "127.0.0.1:8765"
	}
	if c.Input.IntentConf <= 0 {
		c.Input.IntentConf = 0.6
	}
	if c.Input.IntentConf > 1 {
		c.Input.IntentConf = 1
	}
	if c.Input.LowConfAction == "" {
		c.Input.LowConfAction = "ask"
	}
	if c.Memory.Dir == "" {
		c.Memory.Dir = filepath.Join(c.Global.LogDir, "memory")
	}
	if c.Memory.FactsLimit <= 0 {
		c.Memory.FactsLimit = 20
	}
	if strings.TrimSpace(c.Spaces.Dir) == "" {
		c.Spaces.Dir = filepath.Join(c.Global.LogDir, "spaces")
	}
	if strings.TrimSpace(c.Contracts.Dir) == "" {
		c.Contracts.Dir = filepath.Join(c.Global.LogDir, "contracts")
	}
	if strings.TrimSpace(c.Cache.Dir) == "" {
		c.Cache.Dir = filepath.Join(c.Global.LogDir, "cache")
	}
	if c.Cache.TTLSeconds <= 0 {
		c.Cache.TTLSeconds = 86400
	}
	if strings.TrimSpace(c.Server.Bind) == "" {
		c.Server.Bind = c.Global.Addr
	}
	// 非本机绑定必须配 token（内网暴露边界，SPEC v2 缺口 32 裁决：监听范围 + token）。
	if c.Server.Token == "" && !isLocalhost(c.Server.Bind) {
		c.Warnings = append(c.Warnings,
			fmt.Sprintf("server 绑定 %s 但未配置 token（VHS_TOKEN）：内网可访问且无认证，建议设置 token", c.Server.Bind))
	}

	seen := map[string]bool{}
	for i := range c.Providers {
		p := &c.Providers[i]
		if strings.TrimSpace(p.Name) == "" {
			return fmt.Errorf("providers[%d]: name 必填", i)
		}
		if seen[p.Name] {
			return fmt.Errorf("providers: provider 名重复: %q", p.Name)
		}
		seen[p.Name] = true
		switch p.Kind {
		case MockKind:
			if p.Model == "" {
				p.Model = "mock"
			}
		case OpenAIKind:
			if strings.TrimSpace(p.Endpoint) == "" {
				return fmt.Errorf("providers[%q]: openai 必须提供 endpoint", p.Name)
			}
			if strings.TrimSpace(p.Model) == "" {
				return fmt.Errorf("providers[%q]: openai 必须提供 model", p.Name)
			}
			if strings.TrimSpace(p.APIKey) == "" {
				c.Warnings = append(c.Warnings, fmt.Sprintf("provider %q 未配置 api_key：若端点需鉴权将返回 401（可用 VHS_API_KEY）", p.Name))
			}
		default:
			return fmt.Errorf("providers[%q]: 不支持的 kind %q（仅 %s/%s）", p.Name, p.Kind, OpenAIKind, MockKind)
		}
	}

	hasDefault := false
	for i := range c.Routes {
		r := &c.Routes[i]
		if strings.TrimSpace(r.Name) == "" {
			return fmt.Errorf("routes[%d]: name 必填", i)
		}
		if strings.TrimSpace(r.Provider) == "" {
			return fmt.Errorf("routes[%q]: provider 必填", r.Name)
		}
		if r.Provider != LocalProvider && !seen[r.Provider] {
			return fmt.Errorf("routes[%q]: provider %q 未在 providers 表中定义", r.Name, r.Provider)
		}
		if r.Default {
			if hasDefault {
				return fmt.Errorf("routes: 存在多个 default 路由")
			}
			hasDefault = true
		}
	}
	if !hasDefault {
		return fmt.Errorf("routes: 必须存在一条 default 路由")
	}
	return nil
}

// ProviderByName 按名查找 provider。
func (c *Config) ProviderByName(name string) (Provider, bool) {
	for _, p := range c.Providers {
		if p.Name == name {
			return p, true
		}
	}
	return Provider{}, false
}

// IsMockProvider 报告 provider 是否为内置 mock。
func (c *Config) IsMockProvider(name string) bool {
	p, ok := c.ProviderByName(name)
	return ok && p.Kind == MockKind
}

// EffectiveMaxTurns 返回路由生效的 max_turns（0 = local 直通）。
func (c *Config) EffectiveMaxTurns(r Route) int {
	if r.Provider == LocalProvider {
		return 0
	}
	if r.MaxTurns > 0 {
		return r.MaxTurns
	}
	return c.Global.MaxTurnsDefault
}

// EffectiveResponseFormat 返回 provider 生效的 json_object 开关（nil = true）。
func (c *Config) EffectiveResponseFormat(p Provider) bool {
	return p.ResponseFormat == nil || *p.ResponseFormat
}

// EffectiveTimeoutMs 返回 provider 生效的调用超时。
func (c *Config) EffectiveTimeoutMs(p Provider) int {
	if p.TimeoutMs > 0 {
		return p.TimeoutMs
	}
	return c.Global.LLMTimeoutMs
}

// DictionaryPath 返回生效的个人词典路径。
func (c *Config) DictionaryPath() string {
	if strings.TrimSpace(c.Input.DictionaryPath) != "" {
		return c.Input.DictionaryPath
	}
	return filepath.Join(c.Memory.Dir, "dictionary.json")
}

// SpacesDir 返回生效的域注册表目录（.space.json）。
func (c *Config) SpacesDir() string {
	return c.Spaces.Dir
}

// ContractsDir 返回生效的工具契约目录（.contract.json）。
func (c *Config) ContractsDir() string {
	return c.Contracts.Dir
}

// CacheDir 返回生效的四元组缓存目录。
func (c *Config) CacheDir() string {
	return c.Cache.Dir
}

// ServerBind 返回生效的手机 API 监听地址。
func (c *Config) ServerBind() string {
	return c.Server.Bind
}

// isLocalhost 报告 host 是否为回环地址（127.0.0.1 / ::1 / localhost / 空 host）。
func isLocalhost(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	return host == "" || host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func envString(dst *string, key string) {
	if v := os.Getenv(key); v != "" {
		*dst = v
	}
}
func envInt(dst *int, key string) {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			*dst = n
		}
	}
}
func envFloat(dst *float64, key string) {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			*dst = f
		}
	}
}
func envBool(dst *bool, key string) {
	if v := os.Getenv(key); v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			*dst = true
		case "0", "false", "no", "off":
			*dst = false
		}
	}
}
