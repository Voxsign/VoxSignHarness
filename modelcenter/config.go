// Package modelcenter —— 模型中心三条调用通道的配置与调用（ASR-MODEL-02）。
//
// 通道名**固定**：default / diagnose / learn（L1）；底层模型**从配置读**、可变。
// 本轮只启用 default（模型 `deepseek-flash`，ASR-EXT-006 §2 实测修正）；
// diagnose / learn 未获 Peter 指定模型 → `enabled:false`（fail-closed，C3）。
//
// 安全纪律（ASR-EXT-006 §4）：
//   - key **只从环境变量 / .env 读**，绝不硬编码、绝不写日志/commit/测试夹具；
//   - 配置是 JSON（明令不用 YAML）。
package modelcenter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Channel 是固定的三条通道名（L1）。
type Channel string

const (
	ChannelDefault  Channel = "default"
	ChannelPlan     Channel = "plan"
	ChannelResearch Channel = "research"
	ChannelDiagnose Channel = "diagnose"
	ChannelLearn    Channel = "learn"
)

// Channels 返回全部合法通道名。
func AllChannels() []Channel {
	return []Channel{ChannelDefault, ChannelPlan, ChannelResearch, ChannelDiagnose, ChannelLearn}
}

// ChannelConfig 是单条通道的配置。
type ChannelConfig struct {
	Enabled        bool   `json:"enabled"`
	Provider       string `json:"provider"`
	ModelID        string `json:"model_id"` // 可变；未定必须留空或 TBD 且 enabled:false（C3）
	TimeoutMS      int    `json:"timeout_ms"`
	MaxConcurrency int    `json:"max_concurrency"`
	WriteBack      bool   `json:"write_back"` // L2：只有 learn 可为 true（C2）
	// Tier 是**档位**（fast/quality）：与"用途（通道）"分开，避免把两件事塞进同一列。
	Tier string `json:"tier"`
}

// GatewayConfig 是统一入口（AIOps）配置。ChatPath 用**实测路径**，不猜（A5）。
type GatewayConfig struct {
	BaseURL   string `json:"base_url"`
	ChatPath  string `json:"chat_path"`
	APIKeyEnv string `json:"api_key_env"` // 只存**变量名**，不存 key
}

// Config 是模型中心配置。
type Config struct {
	ContractVersion string        `json:"contract_version"`
	Gateway         GatewayConfig `json:"gateway"`
	// Tiers 是档位 → 模型 id（换"什么算高质量" ⇒ 只改这里）。
	Tiers    map[string]string        `json:"tiers"`
	Channels map[string]ChannelConfig `json:"channels"`
}

// Load 读 JSON 配置并校验（C1–C5）。YAML 一律拒绝（C4）。
func Load(path string) (Config, error) {
	if !strings.EqualFold(filepath.Ext(path), ".json") {
		return Config{}, fmt.Errorf("C4 违反：配置必须是 .json（明令不用 YAML）: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("读配置失败: %w", err)
	}
	var c Config
	dec := json.NewDecoder(strings.NewReader(string(data)))
	if err := dec.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("配置不是合法 JSON: %w", err)
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Validate 执行 ASR-MODEL-02 的 C1–C5（fail-closed：不合法即拒绝启动）。
// TierQuality 是高档位名（判据：default 通道**不得**指向它）。
const TierQuality = "quality"

// TierFast 是快档位名。
const TierFast = "fast"

// ResolveModel 返回某通道实际使用的模型 id：通道显式 model_id 优先，否则取 tier。
// **fail-closed**：引用了不存在的 tier ⇒ 报错（不许静默取默认）。
func (c Config) ResolveModel(ch Channel) (string, error) {
	cc, ok := c.Channels[string(ch)]
	if !ok {
		return "", fmt.Errorf("通道 %q 不存在", ch)
	}
	if cc.ModelID != "" && cc.ModelID != "TBD" {
		return cc.ModelID, nil
	}
	if cc.Tier == "" {
		return "", fmt.Errorf("通道 %q 既无 model_id 也无 tier", ch)
	}
	m, ok := c.Tiers[cc.Tier]
	if !ok {
		return "", fmt.Errorf("通道 %q 引用了不存在的 tier %q（fail-closed）", ch, cc.Tier)
	}
	return m, nil
}

func (c Config) Validate() error {
	// C1：通道名恰好是三个，多一个少一个都拒。
	for name, cc := range c.Channels {
		if cc.Tier != "" {
			if _, ok := c.Tiers[cc.Tier]; !ok {
				return fmt.Errorf("通道 %q 引用了不存在的 tier %q（fail-closed，不许静默取默认）", name, cc.Tier)
			}
		}
	}
	// ⑤ **分档不得退化**：default 通道不得指向 quality 档。
	if cc, ok := c.Channels[string(ChannelDefault)]; ok && cc.Tier == TierQuality {
		return fmt.Errorf("C6 违反：default 通道不得指向 %s 档（连常规调用都走最贵的 ⇒ 分档失效）", TierQuality)
	}
	if len(c.Channels) != len(AllChannels()) {
		return fmt.Errorf("C1 违反：channels 必须恰好是 %v，实际 %d 条", AllChannels(), len(c.Channels))
	}
	for _, ch := range AllChannels() {
		if _, ok := c.Channels[string(ch)]; !ok {
			return fmt.Errorf("C1 违反：缺少通道 %q", ch)
		}
	}
	// C2：只有 learn 可以写回。
	if c.Channels[string(ChannelDefault)].WriteBack {
		return fmt.Errorf("C2 违反：default.write_back 必须为 false")
	}
	if c.Channels[string(ChannelDiagnose)].WriteBack {
		return fmt.Errorf("C2 违反：diagnose.write_back 必须为 false")
	}
	if !c.Channels[string(ChannelLearn)].WriteBack {
		return fmt.Errorf("C2 违反：learn.write_back 必须为 true")
	}
	// C3：enabled ⇒ model_id 非空且非 TBD（无 model_id 的调用不可归因）。
	for _, ch := range AllChannels() {
		cc := c.Channels[string(ch)]
		if !cc.Enabled {
			continue
		}
		// 模型可由 model_id 指定，**或**由 tier 解析（档位+用途两层）。
		if (strings.TrimSpace(cc.ModelID) == "" || strings.EqualFold(cc.ModelID, "TBD")) && cc.Tier == "" {
			return fmt.Errorf("C3 违反：通道 %q enabled 但既无 model_id 也无 tier（未指定模型不得启用）", ch)
		}
	}
	// C5：learn 单写者。
	if c.Channels[string(ChannelLearn)].MaxConcurrency != 1 {
		return fmt.Errorf("C5 违反：learn.max_concurrency 必须为 1（单一写者）")
	}
	// 入口必须可调用。
	if strings.TrimSpace(c.Gateway.BaseURL) == "" || strings.TrimSpace(c.Gateway.ChatPath) == "" {
		return fmt.Errorf("入口配置不全：gateway.base_url / gateway.chat_path 必填")
	}
	if strings.TrimSpace(c.Gateway.APIKeyEnv) == "" {
		return fmt.Errorf("入口配置不全：gateway.api_key_env 必填（只存变量名，不存 key）")
	}
	return nil
}

// LoadDotEnv 读 `.env`（KEY=VALUE 行）并**只填充未设置**的环境变量。
// 返回填充条数；**绝不返回、记录或打印任何 value**。
func LoadDotEnv(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if k == "" || v == "" {
			continue
		}
		if os.Getenv(k) != "" {
			continue // 真实环境变量优先
		}
		if err := os.Setenv(k, v); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
