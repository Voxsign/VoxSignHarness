# INTERFACE-FREEZE-M2 —— M2 分片实现的接口冻结契约（过渡工件，SPEC v2 将吸收为权威）

- 版本：freeze-v1.0 · 2026-10-02 · 组织者冻结，供分片 Subagent 并行实现
- 性质：**接口签名与分片边界在此冻结**；SPEC v2（分片 A 产出）可细化语义定义，但 API 形状如需变更必须回报组织者裁决，不得自行改接口
- 共识依据：docs/详细设计-语音驱动开发-v2定稿-20261002.md · docs/VSL-v2-判断语义层.md · docs/VSL-共识-未来开发规范五原则.md · docs/BOUNDARY-AUDIT-边界审计方法论.md · docs/SPEC-v1-可执行规格书.md
- 已定决策（用户拍板，勿再讨论）：M2 输入=ASR 文本；执行主体=模型工具循环+外部编码代理均可；同一项目同时单任务；单二进制 Go；回执一屏 4 行；不可逆永远人工；域权限=交集默认拒绝；验收门=12 用例+go test 全绿+单二进制+性能基线

---

## 0. 已经落地的地基（分片只读消费，禁止修改）

| 文件 | 内容 | 状态 |
|---|---|---|
| contract/contract.go | M2 八类意图常量（IntentNote/Query/Edit/Debug/Test/Commit/Deploy/Ask/RegisterTool）、Confirm*、Impact*、Target、Boundary、RiskBaseline、Conflict*、ToolContract、Attribution、Attr* 六格、ReceiptView、RenderReceipt | 已实现+测试绿 |
| config/config.go | Spaces/Contracts/Cache/Server 四节 + SpacesDir()/ContractsDir()/CacheDir()/ServerBind() + VHS_TOKEN + 非本机绑定无 token 告警 | 已实现+测试绿 |
| input/taskintent.go | TaskClassifier、SpaceHint、ClassifyTask（八类+REGISTER_TOOL、冲突仲裁、空间候选、边界/风险/确认基线、验收模板；含伪代码逻辑层注释） | 已实现+测试绿 |
| input/timeanchor.go | ResolveTimeAnchor（今天/明天/后天/昨天/前天/周X/下周X/本周X，裸周X 歧义标记） | 已实现+测试绿 |

全量 `go test ./...` 当前全绿；`BenchmarkPipelineProcess ≈ 7.8µs < 9.9µs` 基线未破坏。

## 1. 分片边界（目录互斥，谁都不动别人的目录）

| 分片 | 拥有的产物（新建目录/文件） | 禁止触碰 |
|---|---|---|
| A 定义+样例 | docs/SPEC-v2-可执行规格书.md、docs/20-任务方向验证记录.md、data/20-tasks.jsonl；可为 input 包**新增**测试文件（不改已有文件） | 任何 .go 生产代码（除非经组织者裁决）；B/C/D 的目录 |
| B space/refer/risk | space/、refer/、risk/（含测试）；data/spaces/*.space.json 样例 | contract/ config/ input/ 既有文件；verify/ search/ cache/ tools/ |
| C verify/search/cache/tools | verify/、search/、cache/、tools/（含测试） | contract/ config/ input/ 既有文件；space/ refer/ risk/ |
| D pipeline/server/接线 | pipeline/、server/、main.go 扩展、e2e 新增、bench 新增（等 B/C 落地后） | 不改 A/B/C 已交付的 .go 文件（如需微调报组织者） |

## 2. 共享约定

- Go 1.22 兼容、零第三方依赖（只允许标准库）；模块 `voicesign-harness`
- 包注释（// Package xxx …）+ gofmt + go vet 干净；测试用标准 testing 包
- 错误信息中文，风格对齐 M1（fmt.Errorf("…: %w", …)）
- 所有数据形态：**JSON 优先（机器可读原则）**；manifest/contract/policy 用 `.space.json` / `.contract.json`（零依赖下的机器形态，SPEC v2 记录该裁决：YAML 需第三方依赖违背零依赖原则）
- 伪代码层规范（共识五原则 §5）：必写模块在实现文件头部写【伪代码逻辑层】注释块（代码骨架+中文注释，CLRS 式，**不可编译**）；语义/规则内容标注"搬 VSL"，不重复规则本体；简单薄封装豁免
- 可用本机外部编码代理（codex 0.154.0 / claude 2.1.284 CLI）辅助生成，但产物必须过 gofmt/vet/test 自检；禁止删除既有文件

## 3. 各包 API 冻结签名

### space/（B）
```go
type Manifest struct {
    Name string `json:"name"`
    Type string `json:"type"` // project|sandbox|vault-notes|vault-creds|external|global
    Scope, Exclude, Tools, Context []string
    Perms Perms
    Acceptance string `json:"acceptance"`
    RiskDefault string `json:"risk_default"`
    CrossRefs []string
    Version int
    Path string `json:"-"`
}
type Perms struct { Read bool; Write bool; Exec []string }
type Registry struct{ Dir string; Manifests map[string]*Manifest; Version int }
func Load(dir string) (*Registry, error)         // 加载 dir/*.space.json；目录为空/缺失 → 内置六域模板（不落盘）
func (r *Registry) Get(id string) (*Manifest, bool)
func (r *Registry) List() []string
func (r *Registry) Add(m *Manifest) error        // 落盘 dir/<name>.space.json；同名更新版本+1（写前备份）
type Drift struct{ Manifest, Issue string }
func (r *Registry) DetectDrift() ([]Drift, error) // scope 路径存在性+目录结构校验；漂移即失效该域
type Grant struct{ Authorized bool; Paths []string }
type CheckInput struct {
    Intent    contract.Intent
    Grant     Grant
    ToolCaps  []string                  // pipeline 规划产出的待调工具动作
    Contracts []contract.ToolContract   // C 的 tools registry 提供（数据型，避免包依赖）
}
type Verdict struct {
    SpaceID string; Allowed bool
    Reason  string // "" | "unknown_space" | "drift" | "default_deny" | "boundary_violation" | "cross_ref_deny"
    ToolOK  []string
}
func Check(r *Registry, in CheckInput) Verdict
```
- 裁决规则：未注册空间 → deny（不自动切 global，回问）；漂移 → deny；工具动作 ⊆ manifest.Tools ∩ 契约 caps，越界 → BOUNDARY_VIOLATION（不因确认放行）；Scope∩Exclude 非空 → boundary_violation；权限交集为空 → default_deny；cross_refs 未声明跨域引用 → deny
- 内置六域模板：global(只读兜底)/project(参数化)/sandbox(临时+过期)/vault-notes(追加)/vault-creds(高敏只读)/external(永远强确认)；B 同时交付 data/spaces/ 下 4 个真实域样例（voicesign-harness、voxbuybot、vault-notes、vault-creds）

### refer/（B）
```go
type RecentEntity struct{ Space, Entity, Kind, Ts string } // kind: file|project|person|space
type Resolver struct {
    Dict    *memory.Dictionary
    Recent  []RecentEntity                       // pipeline 注入（按 Space 过滤）
    ModelFn func(q string, cands []string) (entity string, conf float64) // 语言层，可 nil
}
func New(dict *memory.Dictionary) *Resolver
func (r *Resolver) Resolve(intent *contract.Intent, spaceID string) (*contract.Intent, error)
    // 词典层(100%) → 上下文规则层(它/那个文件/上次 → Recent) → 语言层(可选) → 低置信 Ask
    // 规则层低置信不许静默吃掉：必须转交 ModelFn 或 Ask；跨域歧义 → Ask「哪个域？」
    // 多候选排序：(space 匹配, Ts 新)；顶级平局 → Ask，不猜
func (r *Resolver) Solidify(entity, variant string) error // 确认后固化进词典（AddTerm）
```
- 伪代码必写模块

### risk/（B）
```go
type ImpactInput struct{ RefCount int; HasTest bool; Heat int } // 机械信号，不靠模型自评
func StaticImpact(in ImpactInput) string  // RefCount≥8→high; ≥3→medium; else small（SPEC v2 可细化）
type Signals struct{ Revertible bool; Impact string; Confidence float64 }
type Decision struct{ Level, Reason string } // auto|light|strong|human
func Evaluate(intent contract.Intent, imp ImpactInput) Decision
    // 决策矩阵（设计 v2 §4）：不可逆→human(永远人工，不可学习掉)；可逆+小+高→auto；可逆+小+低→auto(待抽查)；
    // 可逆+中→light；可逆+高→strong；不可逆意图(COMMIT/DEPLOY/DELETE/外发/凭证) 无视其余信号 → human
type Guard struct{...}
func NewGuard() *Guard
func (g *Guard) ShouldDowngrade(path string) bool // 同路径连续>3 次强确认 → 降级汇总待复核
```
- risk.go 主体 <150 行（计数范围=Evaluate+StaticImpact+Signals/Decision 定义；Guard/ImpactInput/测试不计——SPEC v2 记录该裁决）
- 伪代码必写模块

### verify/（C）
```go
type Result struct{ Status, Evidence, Detail string } // pass|fail|unverifiable|partial
type Spec struct{ Kind string; Args []string; BaseDir string } // Kind: test|diff|grep|file
type Verifier struct{ BaseDir string }
func (v *Verifier) Run(spec Spec) (Result, error)
    // 独立校验器：自己跑 go test / 自己读 fs diff / grep，绝不读执行器自报 status（防自证）
    // test: 自己执行 args 命令并读输出；diff: 读实际文件内容对期望；grep: 搜索断言
```
- 伪代码必写模块

### search/（C，豁免伪代码——薄封装）
```go
type Hit struct{ File string; Line int; LineText, Kind string }
type Options struct{ Roots, Ignore []string; MaxFiles, MaxHits int }
func FindSymbol(name string, opts Options) ([]Hit, error) // 纯 Go 扫描；Go 标识符 func/type/const/var；尊重 ignore glob
func FindText(pattern string, opts Options) ([]Hit, error) // 子串匹配，尊重 ignore
```

### cache/（C）
```go
type QuadKey struct{ Intent, Space, Perm, Ref string }
type Entry struct{ Key QuadKey; Decision string; Version int; ExpiresAt time.Time }
type Store struct{ Path string; Version int; TTL time.Duration }
func Open(path string, ttl time.Duration) (*Store, error)
func (s *Store) Get(k QuadKey) (string, bool)
func (s *Store) Set(k QuadKey, decision string) error
func (s *Store) InvalidateSpace(space string) error
func (s *Store) BumpPolicyVersion() error // 策略/契约/域版本变化 → 全失效
```
- 命中 → "不再问"；{意图,域,权限,指代已消解} 四元组；持久化 JSON+版本+TTL
- 伪代码必写模块

### tools/（C）
```go
type Registry struct{ Contracts map[string]contract.ToolContract }
func LoadContracts(dir string) (*Registry, error) // 内置六契约(git/file/search/test/run/verify @1.0) + dir/*.contract.json
func (r *Registry) Get(name string) (contract.ToolContract, bool)
func (r *Registry) All() []contract.ToolContract
func ValidateContract(c contract.ToolContract) error // name/caps/params/risk 必填；risk 覆盖每个 cap
func (r *Registry) Register(c contract.ToolContract, approved bool) error
    // REGISTER_TOOL：Validate → risk 分级 → 人工确认(approved) → 写 dir/<name>.contract.json；未批准 → error 不落盘
type Executor struct{ BaseDir string; Timeout time.Duration }
func (e *Executor) Exec(tool string, args map[string]any, c contract.ToolContract) (contract.Receipt, error)
    // git/file/test/run/search/verify 按 caps 执行；EDIT 前写备份到 <log_dir>/backups（四行回执"撤销"行的依据）
    // 只执行 pipeline 已过 space_check + risk 裁决的动作（gate 在 pipeline）
```
- REGISTER_TOOL 伪代码必写模块；六工具执行器薄封装豁免

### pipeline/（D）
```go
type Options struct {
    Cfg *config.Config; Dict *memory.Dictionary; Spaces *space.Registry
    Refer *refer.Resolver; Cache *cache.Store; Verifier *verify.Verifier
    Tools *tools.Registry; Exec *tools.Executor
    ConfirmFn func(taskID, question string) (bool, error) // CLI=stdin / server=HTTP
    Trace *trajectory.Trajectory
}
type Outcome struct {
    RequestID string; Intent contract.Intent; Verdict space.Verdict
    Decision risk.Decision; Confirmed bool; Receipts []contract.Receipt
    Verify verify.Result; Attribution contract.Attribution
    View contract.ReceiptView; Ask string
    LoopMs, NetMs int64 // 认知闭环：Loop=含人工等待墙钟；Net=剔除等待净执行
}
func Run(ctx context.Context, o *Options, text string) (Outcome, error)
    // ①input_raw 轨迹 → ②clean → ③dict correct → ④TaskClassify → ⑤refer 消解 → ⑥space select+Check
    // → ⑦risk 分级 → ⑧确认(按决策 auto/light/strong/human) → ⑨tools 执行 → ⑩verify → ⑪归因+轨迹 → ⑫cache 四元组 → ⑬四行回执
    // 认知闭环计量（SPEC v2 缺口 45 裁决）：start=input_raw 写入完成；end=归因写入完成
func Summary(o *Options, since time.Time) (string, error) // 每日摘要：轨迹聚合，按域/失败分组，手机可读
```
- 伪代码必写模块（Run 的控制流）

### server/（D）
```go
type Server struct{...}
func New(cfg *config.Config, o *pipeline.Options) *Server
func (s *Server) Start() error
    // POST /v1/run {text} → Outcome；GET /v1/task/{id}；POST /v1/confirm {task_id,approved}；POST /v1/cancel；GET /v1/health
    // 认证：cfg.Server.Token / VHS_TOKEN；缺 token 仅允许 127.0.0.1 本机
```
- 路由胶水豁免伪代码

### main.go（D）
- 扩展子命令：`run "<text>"`（一次性管线→四行回执）、`serve`、`repl`、`task`（跑 20 样例验证）、`summary`、`compare`（单工对比，可降级为脚本/报告）、`version`（保留）

## 4. SPEC v2（A 产出）格式要求（共识五原则对齐）

文件：docs/SPEC-v2-可执行规格书.md；头部带版本号（v2.x）+ 失效机制（policy_version 与 cache/space 注册表版本联动，策略变化即失效——原则④活的规范）。

1. **54 条缺口逐条回应表**：列 = # / 级别 / 缺口 / 裁定（采纳补定义|已在M1实现|降级P2|UNKNOWN）/ 约束式定义（禁止/必须验证/完成定义，不规定实现——原则①）/ **可执行验证器**（指向具体 go test 函数名或可复现命令，无验证器的条目标注"未完成"——原则③）/ 落地位置
2. **40-49 条北极星相关必须给可执行定义**：#45 认知闭环起止事件与计入口径（start=input_raw 写入完成、end=归因写入完成；含人工等待墙钟=Loop，另报净执行 Net；M2 目标值=可测阈值）、#46 10倍指数定位（愿景表述 vs 验收口径）、#47 20 条样例通过标准、#48 核心用例测试夹具、#49 单工对比容差、#40 撤销协议、#41 四行回执字段、#42-43 归因六格+回写 discuss、#44 轨迹事件规范+脱敏
3. **12 条验收用例 → 验证器清单**：每条 = 复现步骤 + 断言（指向具体 go test / 命令），可独立复现（原则③）
4. **7 个必写模块伪代码逻辑层小节**：space_check / 指代消解 / 风险分级 / verify / 四元缓存 / REGISTER_TOOL / 意图冲突仲裁（=input.TaskClassifier，我已完成实现+逻辑层注释，A 写权威小节，我做交叉核验）；表述=代码骨架+中文注释（CLRS 式），不可编译；语义类内容标注"搬 VSL"（原则⑤）
5. **机器可读优先**：所有 manifest/contract/policy 定义给 JSON 形态 + 一句话人话外壳（原则②）；记录"YAML→JSON"的零依赖裁决
6. **伪代码层与 VSL 分工**：VSL 管"做什么"（意图/域/契约/判断语义），伪代码管"怎么做"（单模块控制流/分支/异常路径）；伪代码里出现规则定义 → 搬回 VSL

## 5. 20 条真实任务样例验证（A 产出，任务卡 #1）

- 来源 4 类 × 5 条：VoxBuyBot 沙特女装（改中文文案/查订单状态/记想法/跑测试/修报错）、consultant 医疗耗材（查库存/记联系人/改报价模板/部署报表/问政策）、模型中心运维（查日志/修 API 超时/跑基准/部署更新/问告警）、想法库整理（记想法/整理标签/查旧想法/合并条目/导出）
- 每条产出：意图 JSON（用 TaskClassifier 实跑）+ 四行回执 + 归因记录；跑通用 M1 demo pipeline-lite（clean+dict+TaskClassifier+stub 执行器）
- 输出：docs/20-任务方向验证记录.md（手机可读表格）+ data/20-tasks.jsonl（机器可读，每条含 意图/回执/归因 + 通过标准 + 验证器）
- 样例文本用真实风格口语（含 ASR 常见错：同音/漏字），验证词典纠错与指代路径

## 6. 交付与自检要求（每个分片）

1. 产物落在自己拥有的目录（§1 边界），不删除任何既有文件，不覆盖他人文件
2. 必写模块：先写【伪代码逻辑层】注释骨架，评审通过（自检对照 §4.4 语义）后填实现；交付时报告"伪代码关卡已过"
3. `gofmt -l .` 干净、`go vet ./...` 干净、`go test ./...` 全绿（自己包 + 不破坏全量）
4. 报告中列出：改动/新增文件绝对路径、测试结果、与冻结接口的偏差（如有必须先 send_message 报组织者，不得自行改接口）
5. 可用本机 codex / claude CLI 辅助编码，但自检责任在自己

## 7. 已知陷阱（已完成探索，勿重复）

- contract.Intent 的 M1 类别（TIME/FILE_*）与 M2 类别（EDIT/…）并存：M2 管线只用 TaskClassifier 产出的 M2 类别；M1 管线不动
- 模块 go.mod 是 `module voicesign-harness`（无域前缀）；import 一律 `voicesign-harness/<pkg>`
- 本目录**不是 git 仓库**（git rev-parse 失败）：撤销协议靠 <log_dir>/backups 备份，不依赖 git 还原（有 git 的项目域另说）
- 测试全部用 t.TempDir()，禁止写 ~/.voicesign / ~/VoxSign / 项目外路径
- CJK 字符串按字节处理会出 bug（timeanchor 已踩过坑）：涉及子串边界一律用 []rune 或组合模式
