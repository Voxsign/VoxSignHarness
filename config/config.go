// Package config    harness  node  (global / providers / routes / input / memory): 
// in defaultvalue -> JSON   file(VHS_CONFIG ordefaultpath)->   change overwrite(   first ). 
// this packageonlydependencytgtapprove , issafety module   base ;        . 
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
	// MockKind isin  line mock provider   kind(no key     , opensend/ showuse). 
	MockKind = "mock"
	// OpenAIKind is OpenAI compatendpoint  kind. 
	OpenAIKind = "openai"
	// LocalProvider isroutebyto"basely  ,  call LLM"    provider name(e.g. TIME). 
	LocalProvider = "local"
)

// Config is node   rootclose . 
type Config struct {
	Global    Global     `json:"global"`
	Providers []Provider `json:"providers"`
	Routes    []Route    `json:"routes"`
	Input     InputCfg   `json:"input"`
	Memory    MemoryCfg  `json:"memory"`

	// M2 newaddnode(SPACE domainnote table /      /    cache / mobile API serveservice). 
	Spaces    SpacesCfg    `json:"spaces,omitempty"`
	Contracts ContractsCfg `json:"contracts,omitempty"`
	Cache     CacheCfg     `json:"cache,omitempty"`
	Server    ServerCfg    `json:"server,omitempty"`

	// Cloud  end form(VHS_MODE=cloud):      +  user  . nil charsegdefaultvaluesee CloudCfg.Default. 
	Cloud          CloudCfg `json:"cloud,omitempty"`
	ConfigPath     string   `json:"-"` //        filepath
	Warnings       []string `json:"-"` //      (e.g.  API key)
	ForcedRoute    string   `json:"-"` // env VHS_ROUTE:  restrictroutebyname
	ForcedProvider string   `json:"-"` // env VHS_PROVIDER:  restrict provider name
}

// SpacesCfg domainnote table(.space.json manifest obj ). 
type SpacesCfg struct {
	Dir string `json:"dir,omitempty"` // default <log_dir>/spaces
}

// ContractsCfg     obj (.contract.json; REGISTER_TOOL note   place). 
type ContractsCfg struct {
	Dir string `json:"dir,omitempty"` // default <log_dir>/contracts
}

// CacheCfg    cache. 
type CacheCfg struct {
	Dir        string `json:"dir,omitempty"`         // default <log_dir>/cache
	TTLSeconds int    `json:"ttl_seconds,omitempty"` // default 86400(1 day)
}

// ServerCfg mobile HTTP API face(in  + token auth). 
type ServerCfg struct {
	Token string `json:"token,omitempty"` // empty = only 127.0.0.1 base     token;  base       token(start   )
	Bind  string `json:"bind,omitempty"`  // empty = get Global.Addr(default 127.0.0.1:8765)
	// UseScheduler env VHS_USE_SCHEDULER(  , default false): true=C2 processincall  connectmanagetask  ; 
	// false=   path charnode change(C0 EnableSerialGate  occur  bot). 
	UseScheduler bool `json:"use_scheduler,omitempty"`
	// MaxConcurrent env VHS_MAX_CONCURRENT(default 1): call  andsendonlimit(sem   ). >1 time
	//   Runner andsend(C1+ objtgtstate),  againbe C0 EnableSerialGate   ; =1 timecall   serial  . 
	MaxConcurrent int `json:"max_concurrent,omitempty"`
}

// CloudCfg  end form num(VHS_MODE=cloud timestartuse). 
//   see docs/ end    andsend   -20261004.md:    useri.e. user,     ,    JWT. 
type CloudCfg struct {
	GoogleClientID     string `json:"google_client_id,omitempty"`     // env VHS_GOOGLE_CLIENT_ID(Web application, code   use)
	GoogleClientSecret string `json:"google_client_secret,omitempty"` // env VHS_GOOGLE_CLIENT_SECRET
	// env VHS_GOOGLE_CLIENT_IDS:  idsplit  accept  client ID listtable(   aud use). 
	// iOS classtype client  send  id_token aud is iOS client ID,   listinonly  ed  . 
	// asemptytime back GoogleClientID(  ). 
	GoogleClientIDs string `json:"google_client_ids,omitempty"`
	RedirectURI     string `json:"redirect_uri,omitempty"`         // env VHS_GOOGLE_REDIRECT_URI, default https://voicesign.ai/auth/callback
	JWTSecret       string `json:"jwt_secret,omitempty"`           // env VHS_JWT_SECRET;   first start   occurbecomeandkeep ize <log_dir>/cloud/jwt-secret
	FreeDailyTasks  int    `json:"free_daily_tasks,omitempty"`     //     daytask  , default 30
	TrialDays       int    `json:"trial_days,omitempty"`           // new userbody   daynum, default 15(periodtimeby Prime  )
	TokenTTLHours   int    `json:"token_ttl_hours,omitempty"`      //    JWT has period, default 10
}

// Global global num. 
type Global struct {
	LogDir             string `json:"log_dir"`               // traceetcday obj , default ~/.voicesign/harness
	Addr               string `json:"addr"`                  // basely HTTP listenly , default 127.0.0.1:8765
	MaxTurnsDefault    int    `json:"max_turns_default"`     // default max_turns, default 2
	LLMTimeoutMs       int    `json:"llm_timeout_ms"`        //    LLM calluse time, default 60000
	ActionTimeoutMs    int    `json:"action_timeout_ms"`     //    default time, default 15000
	MaxActionTimeoutMs int    `json:"max_action_timeout_ms"` //    time onlimit, default 120000
	AllowHighRisk      bool   `json:"allow_high_risk"`       //       (default false)
	MaxOutputChars     int    `json:"max_output_chars"`      // back  out disconnect, default 4000
	// FastResponseMs fast    value: fast etc"fast type"callusewall-clock ed  asslow  , 
	//   connect  butis  model name error disconnect ( split  slow/    / num cur). 
	// default 10000ms; <=0   izeas 10000. 
	FastResponseMs int `json:"fast_response_ms"`
	// CloudMode  end formopenclose: env VHS_MODE=cloud(    + user  ); empty/its value=basely form. 
	CloudMode bool `json:"cloud_mode,omitempty"`
}

// Provider    typeendpoint(OpenAI compator mock). same  close voice    provider( same model) nowby scenario  type. 
type Provider struct {
	Name           string         `json:"name"`
	Kind           string         `json:"kind"`                      // openai | mock
	Endpoint       string         `json:"endpoint"`                  // openai   
	Model          string         `json:"model"`                     // openai   
	APIKey         string         `json:"api_key,omitempty"`         //  use VHS_API_KEY overwrite
	ResponseFormat *bool          `json:"response_format,omitempty"` // nil = default true( require json_object  form)
	TimeoutMs      int            `json:"timeout_ms,omitempty"`      // 0 = use Global.LLMTimeoutMs
	Params         map[string]any `json:"params,omitempty"`          // byendpoint  (e.g. reasoning_effort)
}

// Route   routeby: by intent + close word  in, first infirst , Default  bot. 
type Route struct {
	Name     string   `json:"name"`
	Intent   []string `json:"intent,omitempty"`    //  in contract.Intent.Intent classdiff
	Match    []string `json:"match,omitempty"`     //  in showwordclose word(  write     )
	Provider string   `json:"provider"`            // provider nameor local
	MaxTurns int      `json:"max_turns,omitempty"` // 0 = use Global.MaxTurnsDefault
	Default  bool     `json:"default,omitempty"`   //  botrouteby
}

// InputCfg  in    num. 
type InputCfg struct {
	IntentConf     float64  `json:"intent_conf,omitempty"`     // intent    value, default 0.6
	LowConfAction  string   `json:"low_conf_action,omitempty"` // ask | model, default ask(model   orchestrate aftercontinue   )
	DictionaryPath string   `json:"dictionary_path,omitempty"` // empty = <memory.dir>/dictionary.json
	Fillers        []string `json:"fillers,omitempty"`         // overwritedefault fillwordtable
}

// MemoryCfg     num. 
type MemoryCfg struct {
	Dir        string `json:"dir,omitempty"`         // default ~/.voicesign/harness/memory
	FactsLimit int    `json:"facts_limit,omitempty"` //   notein numonlimit, default 20
}

// Default returnbackin default  (    §6  showexample providers/routes). 
// note :  occurobj (Memory/Spaces/Contracts/Cache   Dir) place empty, 
// by Load->validate()   end Global.LogDir   after    --
//  then VHS_LOG_DIR overwrite LogDir time occurobj  stop   default ~/.voicesign/harness/*, 
// env      pipenote table/cache/  readwrite useuser  obj (M3 config bug fix ). 
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
			FastResponseMs:     10000,
		},
		Providers: []Provider{

			// 2026-10-03  call  : use_max_completion_tokens  astop charseg   aiops  close 502(0.9s)
			// --alreadyfrom Params   ; MaxTokens onlimitalso   typeusefull token   close 60s 504, harness   onlimit. 
			{Name: "center", Kind: OpenAIKind, Endpoint: "https://aiops.voxsign.ai/api/model/chat", Model: "deepseek-flash", ResponseFormat: &trueVal},
			{Name: "fast", Kind: OpenAIKind, Endpoint: "https://aiops.voxsign.ai/api/model/chat", Model: "deepseek-flash", ResponseFormat: &trueVal},
			{Name: "strong", Kind: OpenAIKind, Endpoint: "https://aiops.voxsign.ai/api/model/chat", Model: "deepseek-v4-pro", ResponseFormat: &trueVal},
			{Name: "deepseek", Kind: OpenAIKind, Endpoint: "https://aiops.voxsign.ai/api/model/chat", Model: "deepseek-flash", ResponseFormat: &trueVal},
				//  typecall (docs/ typecall -  .md): gpt-mini = lineon   bot(gpt-4o-mini    10s  use); 
				// deepseek  listif 402    -> ChatWithFallback   i.e. down ->     gpt-mini,  again  . 
				{Name: "gpt-mini", Kind: OpenAIKind, Endpoint: "https://aiops.voxsign.ai/api/model/chat", Model: "gpt-4o-mini", ResponseFormat: &trueVal},
				// 2026-10-03   chain: gpt4o(gpt-4o) call   6s/3461 char occurbecomebecome ,      at gpt-4o-mini. 
				{Name: "gpt4o", Kind: OpenAIKind, Endpoint: "https://aiops.voxsign.ai/api/model/chat", Model: "gpt-4o", ResponseFormat: &trueVal},

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

// Load resolve   : defaultvalue ->   file(VHS_CONFIG, or ~/.voicesign/harness.json, ./voicesign-harness.json)->   change . 
//  formrefer  configPath butfile store  ->   ;   formrefer anddefaultpathall store  ->  defaultvalue. 
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
		// firstresolve to raw map againbynodeoverwrite:   node(providers/routes)   body  , 
		//    encoding/json to empty  "thenly usealreadyhas  ,  charsegoverwrite"  defaultvalue  
		// (e.g.default provider center   endpoint  infilevoice   provider). 
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

	//   change overwrite(onlycuralready  ). 
	if v := os.Getenv("VHS_API_KEY"); v != "" {
		for i := range cfg.Providers {
			cfg.Providers[i].APIKey = v
		}
	} else if v := os.Getenv("AIOPS_KEY"); v != "" {
		// 2026-10-04 compat change name( before harness   use AIOPS_KEY   closeread Key). 
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
	envBool(&cfg.Server.UseScheduler, "VHS_USE_SCHEDULER")
	envInt(&cfg.Server.MaxConcurrent, "VHS_MAX_CONCURRENT")
	//  end form: VHS_MODE=cloud startuse    / user/  . 
	if os.Getenv("VHS_MODE") == "cloud" {
		cfg.Global.CloudMode = true
	}
	envString(&cfg.Cloud.GoogleClientID, "VHS_GOOGLE_CLIENT_ID")
	envString(&cfg.Cloud.GoogleClientSecret, "VHS_GOOGLE_CLIENT_SECRET")
	envString(&cfg.Cloud.GoogleClientIDs, "VHS_GOOGLE_CLIENT_IDS")
	envString(&cfg.Cloud.RedirectURI, "VHS_GOOGLE_REDIRECT_URI")
	envString(&cfg.Cloud.JWTSecret, "VHS_JWT_SECRET")
	envInt(&cfg.Cloud.FreeDailyTasks, "VHS_FREE_DAILY_TASKS")
	envInt(&cfg.Cloud.TrialDays, "VHS_TRIAL_DAYS")
	envInt(&cfg.Cloud.TokenTTLHours, "VHS_TOKEN_TTL_HOURS")

	// verifyand  ize. 
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
	if c.Global.FastResponseMs <= 0 {
		c.Global.FastResponseMs = 10000
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
	//  base       token(in    boundary, SPEC v2    32  decide: listen   + token). 
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

// ProviderByName byname   provider. 
func (c *Config) ProviderByName(name string) (Provider, bool) {
	for _, p := range c.Providers {
		if p.Name == name {
			return p, true
		}
	}
	return Provider{}, false
}

// IsMockProvider    provider is asin  mock. 
func (c *Config) IsMockProvider(name string) bool {
	p, ok := c.ProviderByName(name)
	return ok && p.Kind == MockKind
}

// DiagProvider returnback name=="diag"  "     type"provider(error    ②  ). 
//    (ok=false)->  disconnect  open  ed,  chain as change. diag.Endpoint      overwrite
// as typein  use disconnectendpoint;   i.e. typein  OpenAI compat  . 
func (c *Config) DiagProvider() (Provider, bool) {
	return c.ProviderByName("diag")
}

// EffectiveMaxTurns returnbackroutebyoccur   max_turns(0 = local   ). 
func (c *Config) EffectiveMaxTurns(r Route) int {
	if r.Provider == LocalProvider {
		return 0
	}
	if r.MaxTurns > 0 {
		return r.MaxTurns
	}
	return c.Global.MaxTurnsDefault
}

// EffectiveResponseFormat returnback provider occur   json_object openclose(nil = true). 
func (c *Config) EffectiveResponseFormat(p Provider) bool {
	return p.ResponseFormat == nil || *p.ResponseFormat
}

// EffectiveTimeoutMs returnback provider occur  calluse time. 
func (c *Config) EffectiveTimeoutMs(p Provider) int {
	if p.TimeoutMs > 0 {
		return p.TimeoutMs
	}
	return c.Global.LLMTimeoutMs
}

// DictionaryPath returnbackoccur    word path. 
func (c *Config) DictionaryPath() string {
	if strings.TrimSpace(c.Input.DictionaryPath) != "" {
		return c.Input.DictionaryPath
	}
	return filepath.Join(c.Memory.Dir, "dictionary.json")
}

// SpacesDir returnbackoccur  domainnote tableobj (.space.json). 
func (c *Config) SpacesDir() string {
	return c.Spaces.Dir
}

// ContractsDir returnbackoccur      obj (.contract.json). 
func (c *Config) ContractsDir() string {
	return c.Contracts.Dir
}

// CacheDir returnbackoccur     cacheobj . 
func (c *Config) CacheDir() string {
	return c.Cache.Dir
}

// ServerBind returnbackoccur  mobile API listenly . 
func (c *Config) ServerBind() string {
	return c.Server.Bind
}

// isLocalhost    host is asback ly (127.0.0.1 / ::1 / localhost / empty host). 
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
