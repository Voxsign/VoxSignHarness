// Package modelcenter --  typein   calluse     andcalluse(ASR-MODEL-02). 
//
//   name**  **: default / diagnose / learn(L1); bot  type**from  read**,  change. 
// base onlystartuse default( type `deepseek-flash`, ASR-EXT-006 §2   fixpos); 
// diagnose / learn    owner refer  type -> `enabled:false`(fail-closed, C3). 
//
// safesafety  (ASR-EXT-006 §4): 
//   - key **onlyfrom  change  / .env read**,     code,   writeday /commit/    ; 
//   -   is JSON(   use YAML). 
package modelcenter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Channel is       name(L1). 
type Channel string

const (
	ChannelDefault  Channel = "default"
	ChannelPlan     Channel = "plan"
	ChannelResearch Channel = "research"
	ChannelDiagnose Channel = "diagnose"
	ChannelLearn    Channel = "learn"
)

// Channels returnbacksafety     name. 
func AllChannels() []Channel {
	return []Channel{ChannelDefault, ChannelPlan, ChannelResearch, ChannelDiagnose, ChannelLearn}
}

// ChannelConfig is       . 
type ChannelConfig struct {
	Enabled        bool   `json:"enabled"`
	Provider       string `json:"provider"`
	ModelID        string `json:"model_id"` //  change;      emptyor TBD and enabled:false(C3)
	TimeoutMS      int    `json:"timeout_ms"`
	MaxConcurrency int    `json:"max_concurrency"`
	WriteBack      bool   `json:"write_back"` // L2: onlyhas learn  as true(C2)
	// Tier is**  **(fast/quality): and"useway(  )"splitopen,   pipe     same list. 
	Tier string `json:"tier"`
}

// GatewayConfig is  in (AIOps)  . ChatPath use**  path**,   (A5). 
type GatewayConfig struct {
	BaseURL   string `json:"base_url"`
	ChatPath  string `json:"chat_path"`
	APIKeyEnv string `json:"api_key_env"` // onlystore**change name**,  store key
}

// Config is typein   . 
type Config struct {
	ContractVersion string        `json:"contract_version"`
	Gateway         GatewayConfig `json:"gateway"`
	// Tiers is   ->  type id( "      " ⇒ onlymodify  ). 
	Tiers    map[string]string        `json:"tiers"`
	Channels map[string]ChannelConfig `json:"channels"`
}

// Load read JSON   andverify(C1–C5). YAML   reject(C4). 
func Load(path string) (Config, error) {
	if !strings.EqualFold(filepath.Ext(path), ".json") {
		return Config{}, fmt.Errorf("C4 violation: config must be .json (YAML explicitly disallowed): %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("failed to read config: %w", err)
	}
	var c Config
	dec := json.NewDecoder(strings.NewReader(string(data)))
	if err := dec.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("config is not valid JSON: %w", err)
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Validate    ASR-MODEL-02   C1–C5(fail-closed:    i.e.rejectstart ). 
// TierQuality is   name( data: default   **  **referto ). 
const TierQuality = "quality"

// TierFast isfast  name. 
const TierFast = "fast"

// ResolveModel returnback      use  type id:    form model_id  first,  thenget tier. 
// **fail-closed**:  use store   tier ⇒   ( allow  getdefault). 
func (c Config) ResolveModel(ch Channel) (string, error) {
	cc, ok := c.Channels[string(ch)]
	if !ok {
		return "", fmt.Errorf("channel %q does not exist", ch)
	}
	if cc.ModelID != "" && cc.ModelID != "TBD" {
		return cc.ModelID, nil
	}
	if cc.Tier == "" {
		return "", fmt.Errorf("channel %q has neither model_id nor tier", ch)
	}
	m, ok := c.Tiers[cc.Tier]
	if !ok {
		return "", fmt.Errorf("channel %q references non-existent tier %q (fail-closed)", ch, cc.Tier)
	}
	return m, nil
}

func (c Config) Validate() error {
	// C1:   name  is  ,       allreject. 
	for name, cc := range c.Channels {
		if cc.Tier != "" {
			if _, ok := c.Tiers[cc.Tier]; !ok {
				return fmt.Errorf("channel %q references non-existent tier %q (fail-closed, no silent default)", name, cc.Tier)
			}
		}
	}
	// ⑤ **split    ize**: default     referto quality  . 
	if cc, ok := c.Channels[string(ChannelDefault)]; ok && cc.Tier == TierQuality {
		return fmt.Errorf("C6 violation: default channel must not point to %s tier (even regular calls would use the most expensive -> tiering broken)", TierQuality)
	}
	if len(c.Channels) != len(AllChannels()) {
		return fmt.Errorf("C1 violation: channels must be exactly %v, got %d", AllChannels(), len(c.Channels))
	}
	for _, ch := range AllChannels() {
		if _, ok := c.Channels[string(ch)]; !ok {
			return fmt.Errorf("C1 violation: missing channel %q", ch)
		}
	}
	// C2: onlyhas learn  bywriteback. 
	if c.Channels[string(ChannelDefault)].WriteBack {
		return fmt.Errorf("C2 violation: default.write_back must be false")
	}
	if c.Channels[string(ChannelDiagnose)].WriteBack {
		return fmt.Errorf("C2 violation: diagnose.write_back must be false")
	}
	if !c.Channels[string(ChannelLearn)].WriteBack {
		return fmt.Errorf("C2 violation: learn.write_back must be true")
	}
	// C3: enabled ⇒ model_id  emptyand  TBD(no model_id  calluse  attribution). 
	for _, ch := range AllChannels() {
		cc := c.Channels[string(ch)]
		if !cc.Enabled {
			continue
		}
		//  type by model_id refer , **or**by tier resolve (  +useway  ). 
		if (strings.TrimSpace(cc.ModelID) == "" || strings.EqualFold(cc.ModelID, "TBD")) && cc.Tier == "" {
			return fmt.Errorf("C3 violation: channel %q enabled but has neither model_id nor tier (must not enable without a model)", ch)
		}
	}
	// C5: learn  writeer. 
	if c.Channels[string(ChannelLearn)].MaxConcurrency != 1 {
		return fmt.Errorf("C5 violation: learn.max_concurrency must be 1 (single writer)")
	}
	// in    calluse. 
	if strings.TrimSpace(c.Gateway.BaseURL) == "" || strings.TrimSpace(c.Gateway.ChatPath) == "" {
		return fmt.Errorf("incomplete gateway config: gateway.base_url / gateway.chat_path are required")
	}
	if strings.TrimSpace(c.Gateway.APIKeyEnv) == "" {
		return fmt.Errorf("incomplete gateway config: gateway.api_key_env required (store only the env var name, never the key)")
	}
	return nil
}

// LoadDotEnv read `.env`(KEY=VALUE  )and**only fill   **   change . 
// returnbackfillfill num; **  returnback,   or     value**. 
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
			continue //     change  first
		}
		if err := os.Setenv(k, v); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
