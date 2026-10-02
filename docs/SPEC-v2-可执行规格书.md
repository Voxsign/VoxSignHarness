# VoxSign Harness · SPEC v2（可执行规格书）

- **版本：v2.0 · 2026-10-02 · 分片 A 产出（定义层，不写生产代码）**
- 取代：`docs/SPEC-v1-可执行规格书.md`（v1 底稿保留作历史；本文件为权威）
- 上游契约：`docs/INTERFACE-FREEZE-M2.md`（API 形状冻结，本文件只定义语义，不改签名）
- 共识依据：详细设计 v2 / VSL-v2 判断语义层 / 五原则 / BOUNDARY-AUDIT 方法论
- 北极星：**认知闭环周期** record → discuss → control → 归因，转一圈的速度（#45/#46 见下）

## 0. 失效机制（五原则④活的规范）

```json
{
  "policy": {
    "policy_version": 2,
    "spec": "SPEC-v2-可执行规格书.md",
    "spec_version": "v2.0",
    "invalidates": {
      "on_policy_change": true,
      "on_space_registry_bump": true,
      "on_contract_bump": true
    }
  }
}
```
人话外壳：**策略版本号一变，四元组缓存全失效、space 注册表与工具契约版本联动 bump；任何策略/域/契约改动即 `cache.BumpPolicyVersion()`，不再信旧缓存。**

裁决记录：
- **YAML→JSON（零依赖）**：manifest/contract/policy 一律 `.space.json` / `.contract.json`，不用 YAML——标准库无 YAML 解析器，引第三方依赖违背「单二进制、零第三方依赖」冻结条款（freeze §2）。v1 里的 YAML 示例仅为示意，机器形态以此处 JSON 为准。

---

## 1. 54 条缺口逐条回应表（research-inputs/16）

裁定取值：**采纳补定义** / **已在冻结/M1 实现** / **降级 P2** / **UNKNOWN**。
验证器列：指向具体 `go test` 函数或可复现命令；无验证器标「未完成」（五原则③）。

| # | 级 | 缺口（摘要） | 裁定 | 约束式定义（禁止/必须验证/完成） | 可执行验证器 | 落地位置 |
|---|---|---|---|---|---|---|
| 1 | P0 | M2 交付边界（12卡/六工具/闭环哪些必交） | 已在冻结/M1 实现 | 分片边界见 freeze §1；A=docs+data+input测试，B=space/refer/risk，C=verify/search/cache/tools，D=pipeline/server。禁止越界改他人目录 | `go test ./...` 全绿即边界自检 | freeze §1 |
| 2 | P0 | M1 仓库位置/构建/测试基线 | 已在冻结/M1 实现 | 模块 `voicesign-harness`（无域前缀）；`go test ./...` 必须全绿；分类器基准 `BenchmarkPipelineProcess < 9.9µs` | `go test ./... && go test ./bench -run xxx -bench BenchmarkPipelineProcess` | go.mod / bench |
| 3 | P0 | M1 各包接口与数据结构兼容 | 已在冻结/M1 实现 | contract 为唯一共享契约；M2 新增字段全部 `omitempty`，M1 类别（TIME/FILE_*）与 M2 类别（EDIT…）并存，M2 管线只用 TaskClassifier 产出 | `go test ./contract ./input` | contract/contract.go |
| 4 | P0 | record→discuss→control 状态机/discuss IO/存储 | 采纳补定义 | 三通道=轨迹 JSONL append-only 的三个 stage：record(input_raw/纠错/意图)、discuss(归因/决策理由/回写)、control(执行/校验/回执)。discuss 输出=一条 `kind=attribution` 轨迹 + 一条 `discuss-log`；**禁止** discuss 自动改策略/词典（须人确认） | `pipeline.TestPipelineWritesAttribution`（实测 PASS） | trajectory/ + pipeline |
| 5 | P0 | 执行链先后关系 | 采纳补定义 | 必须按 freeze §3 pipeline.Run 十三步序：input_raw→clean→dict纠错→TaskClassify→refer消解→space select+Check→risk分级→确认→tools执行→verify→归因+轨迹→cache四元组→四行回执。**禁止** 执行在 space_check/risk 之前 | `pipeline.TestPipelineOrdering`（实测 PASS，串行闸） | pipeline |
| 6 | P0 | 执行主体（EDIT/DEBUG 谁做） | 已在冻结/M1 实现 | 用户拍板：模型工具循环 + 外部编码代理均可；harness 只做 gate（space_check+risk）与独立校验，不自己生成代码 | `tools.Executor.Exec` 按 caps 执行（C） | tools/ |
| 7 | P0 | 正式意图 Schema（类型/必填/枚举/默认/非法） | 采纳补定义 | 见 §1.1 意图 JSON Schema；非法输入（空文本/无触发词）必须 UNKNOWN+Ask，**禁止**静默归某类 | `TestTaskClassifyUnknownAsks` | input + contract.Intent |
| 8 | P0 | RUN/DELETE/REGISTER_TOOL 归属 | 采纳补定义 | DELETE→`EDIT(action=delete)`（不可逆包→human）；RUN/性能→`TEST(test_kind=bench)`；REGISTER_TOOL=第 9 类意图（八类之外单列） | `TestTaskClassifyConflicts`（delete）、`TestTaskClassifyEightClasses`（REGISTER_TOOL） | input/taskintent.go |
| 9 | P0 | 意图→工具动作映射；COMMIT含push? DEPLOY区分 | 采纳补定义 | COMMIT=**本地** git commit（默认不 push）；push 单独视为不可逆 human。DEPLOY 覆盖 部署/上线/外发/生成报表——外发/external 永远强确认。**禁止** COMMIT 默认带 push | `tools.LoadContracts` 六契约 @1.0（C） | tools/ + contract.ToolContract |
| 10 | P0 | 意图-域冲突判定（NOTE 指向 project） | 采纳补定义 | NOTE/记想法 必须落 vault-notes 语义域，不得写 project；「发个想法」按 note_vs_deploy 仲裁为 NOTE。出界→回显修正后再执行。**thoughtWords×想法库子串碰撞已修复**：空间名/别名命中含「想法/备忘」时跳过 thoughtWords 仲裁；noteTriggers 去裸「记」 | `Test20SampleValidation`（已修复，#17/#18/#20 回正 EDIT/QUERY/DEPLOY，20/20） | input + space |
| 11 | P0 | 回问/人工确认协议（选项/绑定/超时/取消/状态变化重确认） | 采纳补定义 | `Ask != ""` → 绝不执行（冻结）；确认=一条消息+绑定 task_id；确认后若 risk/space 裁决变化必须重新确认；超时→取消且不落盘不可逆动作；BOUNDARY_VIOLATION 不可被确认放行 | `pipeline.TestPipelineConfirmStrategy` + `server.TestServerRunAndTaskGet`（均实测 PASS）；`input.TestTaskClassifyLowConfAsks` 守低置信回问 | server/ + pipeline.ConfirmFn |
| 12 | P0 | Manifest 缺 exclude 必填轴；UNKNOWN 阻断 | 已在冻结/M1 实现 | Manifest.Exclude 必填（默认 [".env*","node_modules"]）；scope∩exclude 非空=boundary_violation；未注册空间→deny（不自动切 global） | `space.Load/Check`（B） | space/Manifest |
| 13 | P0 | 完整域清单；project 类型 vs 实例 | 已在冻结/M1 实现 | 五类模板：global/project(参数化)/sandbox/vault-notes/vault-creds/external。project 按 scope 作参数，不按项目开域；运行时主键=space id（不是类型） | `space.Load` 内置六模板（B） | space/Registry |
| 14 | P0 | 域边界合并优先级（意图boundary/Manifest/契约/global） | 采纳补定义 | 有效权限=平台∩Manifest∩契约caps∩本次授权 **交集**，默认拒绝；风险分级不得突破权限上限；切域不继承授权 | `space.Check`（B）：default_deny/boundary_violation/cross_ref_deny | space/Check |
| 15 | P0 | 路径边界执行语义（相对/glob/符号链接/穿越/检查后变化） | 采纳补定义 | 必须 resolve 为绝对路径后再判 scope；`..` 穿越→deny；符号链接逃逸出 scope→deny（双 `EvalSymlinks` 硬化）；检查与执行之间路径被换→不读自报、verify 重读 fs | **已覆盖（M3 升级）**：`space.TestResolveScopePath_DotDotTraversal`/`TestResolveScopePath_SymlinkEscapeRejected`/`TestResolveScopePath_NestedSymlinkDirEscape`/`TestDrift_SymlinkAware` + `verify.TestRun_SymlinkEscapeRejected`/`TestRun_GrepSkipsEscapedSymlinkDir`/`TestRun_DotDotTraversalStillRejected`（均实测 PASS） | space/ + verify |
| 16 | P0 | 执行白名单匹配（参数/cwd/env/shell/子进程） | 采纳补定义 | 工具动作必须 ⊆ Manifest.Tools ∩ 契约 caps；exec 白名单逐命令；**禁止** shell 拼接绕过（参数与命令分开传）；子进程同受边界约束 | `tools.TestExecutor_FileWriteBackupAndRead`/`TestExecutor_TestPassAndFail`（实测 PASS） | tools/Executor |
| 17 | P0 | cross_refs 授权语义（跨域数据流向） | 采纳补定义 | 跨域引用必须在源 Manifest 的 cross_refs 显式声明；未声明→cross_ref_deny；跨域读出的数据禁止流向 external/外发，除非 external 域强确认 | `space.TestCheck_CrossRefDeny`/`TestCheck_CrossRefAllowedWhenDeclared`（实测 PASS） | space/ |
| 18 | P0 | vault-creds 可用操作/敏感处理 | 采纳补定义 | vault-creds=高敏**只读**；禁止原文入轨迹/回执（脱敏：仅留 `vault-creds:<key>` 占位）；禁止注入持久化；读取仍需强确认 | `space.Check` vault-creds 用例（B）+ 轨迹脱敏（#44） | space/ + trajectory |
| 19 | P0 | 漂移判定基准/时机/失效/重建 | 已在冻结/M1 实现 | DetectDrift=校验 scope 路径存在性+目录结构；漂移即失效该域并回问重建；时机=registry 加载时 + 每次 Check 前 | `space.DetectDrift`（B） | space/Registry |
| 20 | P0 | 风险三信号计算/取值/决策表 | 已在冻结/M1 实现 | ①可逆=意图类型硬映射（不可学习掉）②影响=静态机械信号(RefCount≥8 high/≥3 medium)；M3 由保守 0 升级为**真实引用计数**，且**排除 log_dir** 自引用 ③置信=软信号只用于回问；决策矩阵见 freeze risk.Evaluate | `risk.TestEvaluate_*` + `pipeline.TestMechanicalImpactRealRefCount`/`TestMechanicalImpactExcludesLogDir`（均实测 PASS） | risk/ + pipeline |
| 21 | P0 | 风险置信 vs 意图置信关系 | 采纳补定义 | RiskBaseline.Confidence=0 时回退 Intent.Confidence；冲突取**较低**者（保守）；缺失→按低置信处理（倾向回问） | `risk.TestEvaluate_BaselineIrreversible`/`TestEvaluate_AutoSmallLow`（实测 PASS） | risk/ |
| 22 | P0 | 禁止 vs 人工确认边界（BOUNDARY_VIOLATION 不可放行） | 已在冻结/M1 实现 | BOUNDARY_VIOLATION 恒 deny，**禁止**因用户确认放行（硬规则） | `space.Check`（B） | space/ |
| 23 | P0 | 不可逆动作清单 | 采纳补定义 | 不可逆（无视其余信号→human）：DELETE、外发/发布、本地 commit、deploy、vault-creds 出域。checkout/reset 改工作区→medium 可逆。**禁止**把不可逆标 auto | `TestTaskClassifyIrreversibleConfirm` | input + risk |
| 24 | P0 | 六工具逐动作契约（参数/返回/副作用/超时） | 已在冻结/M1 实现 | git/file/search/test/run/verify @1.0，逐 cap 列 risk；validate 必填 name/caps/params/risk | `tools.ValidateContract/LoadContracts`（C） | tools/ |
| 25 | P0 | REGISTER_TOOL 能力来源与授权 | 已在冻结/M1 实现 | 注册=写 `.contract.json` 元数据（不生成代码）；流程 Validate→risk分级→**人工 approved**→落盘；未批准 error 不落盘 | `tools.Register`（C） | tools/ |
| 26 | P0 | 验收条件机器可执行（NL acceptance→校验项/谁批） | 采纳补定义 | acceptanceTemplate 产出 NL 判据→规划层翻译成 `verify.Spec{Kind,Args}`；由 verify 独立执行，人在回执看结果。acceptance/context **不参与授权** | `verify.Run`（C）+ acceptanceTemplate（input） | verify/ + input |
| 27 | P0 | verify 独立性边界（预期/实际/防自证） | 已在冻结/M1 实现 | verify 自己跑 go test/读 fs diff/grep，**禁止**读执行器自报 status；预期来自 acceptance，实际来自 fs | `verify.Run`（C） | verify/ |
| 28 | P0 | 校验结果模型（pass/fail/unverifiable/partial；能否继续 commit/deploy） | 采纳补定义 | Result.Status∈pass/fail/unverifiable/partial；**fail/unverifiable → 禁止 commit/deploy**；partial→回执高亮待人工 | `verify.TestRun_Unverifiable`/`TestRun_DoesNotTrustExecutorClaim`（实测 PASS） | verify/Result |
| 29 | P0 | 中断/部分失败（超时/取消/崩溃/重试避免重复提交） | 采纳补定义 | COMMIT/DEPLOY/外发 必须幂等：带 request_id 去重，重试前查轨迹是否已落不可逆动作；超时/取消→标记 control=aborted，不自动重发 | **全覆盖（M4）**：`server.TestRequestIDDedup`（同 request_id 二次 200 deduped 不重复执行）+ `e2e.TestPipelineSerialGate`（单任务串行闸）+ `e2e.TestPipelineCommitHumanConfirm`（不可逆人工确认）（均实测 PASS） | pipeline/ + trajectory + server |
| 30 | P0 | 并发与工作区保护 | 已在冻结/M1 实现 | 用户拍板：同一项目同时单任务；执行前不碰用户未提交改动（EDIT 前备份） | pipeline 单任务锁（D） | pipeline |
| 31 | P0 | 手机 API 协议（task id/状态/回问/确认/取消端点） | 已在冻结/M1 实现 | POST /v1/run、GET /v1/task/{id}、POST /v1/confirm、POST /v1/cancel、GET /v1/health | `server` 测试（D） | server/ |
| 32 | P0 | 手机 API 认证/暴露边界 | 已在冻结/M1 实现 | cfg.Server.Token / VHS_TOKEN；缺 token 仅 127.0.0.1；非本机绑定无 token→告警 | `config.ServerBind` + token 测试 | config/ |
| 33 | P1 | 语音输入边界（音频还是 ASR 文本） | 已在冻结/M1 实现 | 用户拍板：M2 输入=ASR 文本；录音/转写/播报由基础设施（豆包 App）负责 | `pipeline.Run(ctx, o, text string)` | pipeline |
| 34 | P1 | 指代上下文模型（会话/窗口/上一条/多候选排序） | 已在冻结/M1 实现 | 词典层(100%)→上下文规则层(它/那个文件/上次→Recent)→语言层(可选)；多候选按(space匹配,Ts新)排序，顶级平局→Ask 不猜 | `refer.Resolver`（B） | refer/ |
| 35 | P1 | 实体词典初版/别名冲突/更新/位置映射 | 已在冻结/M1 实现 | 内置 5 条（Mansour/冀总/model.peterzou.com/VoxSign/center）；确认后 Solidify→AddTerm；纠错留 Correction 记录 | `memory.LoadDictionary/Correct`、`Test20SampleValidation`（#7/#11 纠错） | memory/ |
| 36 | P1 | 时间锚点规范（参考时间/时区/落库/歧义） | 已在冻结/M1 实现 | now=本地时区；落库 `time_date=YYYY-MM-DD`；裸周 X 且=今天或已过→ambiguous 回问 | `TestResolveTimeAnchor` | input/timeanchor.go |
| 37 | P1 | context 引用解析注入协议（project-map/decisions） | **已定义已实现（M3，ground 包）** | 见 §2.37：project-map 注册表渲染 + `<log_dir>/decisions.jsonl` 确认裁决记录；只读注入、缺失不阻断、按字节/条数上限截断 | `ground.TestRecordAndRenderDecisions`/`TestRenderMissingDataSources`/`TestByteCap`/`TestTruncateOldestDecisions`（均实测 PASS）；注入点见 §2.37 | ground/ + pipeline |
| 38 | P1 | 四元组缓存键/命中/失效 | 已在冻结/M1 实现 | 键={Intent,Space,Perm,Ref}；指代已消解+域内命中→不再问；policy/space/contract 版本变化→全失效 | `cache.Get/Set/InvalidateSpace/BumpPolicyVersion`（C） | cache/ |
| 39 | P1 | search 范围/忽略/结果/截断/符号精度 | 已在冻结/M1 实现 | Options.Roots/Ignore/MaxFiles/MaxHits；Go 标识符符号定位；尊重 ignore glob | `search.FindSymbol/FindText`（C） | search/ |
| 40 | P1 | **撤销协议** | 采纳补定义 | 见 §2.40：无 git 仓库→`<log_dir>/backups` 备份；有 git 项目域→git restore；四行回执「撤销」行必须可执行或明示不可撤销 | `tools.Executor` EDIT 前写备份（C）；回执 `Test20SampleReceiptFields` | tools/ + §2.40 |
| 41 | P1 | **回执字段规范**（多文件/无文件/失败/待确认/不可撤销） | 采纳补定义 | 见 §2.41：四行=动作/文件/结果/撤销，各情形表达 | `contract.RenderReceipt`、`Test20SampleReceiptFields` | contract/ReceiptView + §2.41 |
| 42 | P1 | **归因六格+证据规则** | 采纳补定义 | 见 §2.42：六格定义+证据引用规则 | `20-tasks.jsonl` 每条 attribution；`Test20SampleValidation` | contract.Attribution + §2.42 |
| 43 | P1 | **归因回写 discuss 行为** | 采纳补定义 | 见 §2.43：写轨迹 kind=attribution + discuss-log，下一轮注入；不自动改词典/策略 | `pipeline.TestPipelineWritesAttribution`（实测 PASS） | trajectory/ + §2.43 |
| 44 | P1 | **轨迹事件规范+脱敏** | 采纳补定义 | 见 §2.44：任务关联/阶段时间戳/证据引用/敏感脱敏/写失败处理 | `trajectory.TestWriteAppendsParseableLines`/`TestConcurrentWrites` + `e2e.TestPipelineSummary`（均实测 PASS） | trajectory/ + §2.44 |
| 45 | P1 | **认知闭环计量口径** | 采纳补定义 | 见 §2.45：start/end 事件、LoopMs vs NetMs、M2 可测阈值 | `pipeline.TestPipelineWritesAttribution`（Outcome.LoopMs/NetMs 计量，实测 PASS） | pipeline/ + §2.45 |
| 46 | P1 | **10 倍指数可验证定义** | 采纳补定义 | 见 §2.46：M2=愿景表述，验收=环比改善方向 | 见 §2.46 | §2.46 |
| 47 | P1 | **20 条样例输入/环境/预期/通过标准/模拟范围** | 已在冻结/M1 实现 | 见 `docs/20-任务方向验证记录.md` + `data/20-tasks.jsonl`；通过=分类≥17/20、四行齐全、六格可填 | `go test ./input -run Test20Sample`（实测 20/20） | docs/ + data/ |
| 48 | P1 | **核心用例测试夹具与精确断言** | 采纳补定义 | 见 §3 验证器清单：漂移/越界/独立校验/归因夹具=各自 t.TempDir() 构造最小 fs | §3 各行 | 各包 _test.go |
| 49 | P1 | **单工对比基线/任务选择/计时/等快容差** | 采纳补定义 | 见 §2.49：基线操作/3 任务/计时范围/容差。M3 已由脚本升级为 `main.go:cmdCompare` 实跑：同一 `data/20-tasks.jsonl` 两配置（ground off vs on）对比 | `vhs compare`（= `main.go:cmdCompare`，实跑：20 任务全跑通、意图通过率 85%、归因分布 context×15/input×2/model×3） | bench/ + §2.49 + main.go |
| 50 | P1 | 目标 OS/Go 版本/依赖约束（单二进制是否依赖 git/grep/远程模型） | 采纳补定义 | Go 1.22、零第三方依赖（仅标准库）；git/grep/test 作为**外部命令**经工具契约调用（非 Go 库依赖）；远程模型=provider HTTP | `go build ./...` + `vhs version` | go.mod / provider |
| 51 | P1 | 配置发现加载（.space/.contract 位置/版本/重复/热更新） | 已在冻结/M1 实现 | config.SpacesDir()/ContractsDir()/CacheDir()；目录空→内置模板不落盘；同名更新 version+1（写前备份）；热更新=重新 Load | `config` 测试 | config/ |
| 52 | P2 | 每日摘要聚合窗口/字段/输出/触发/分组 | **已细化（M3，原降级 P2）** | 每日一条消息，按域+失败分组；M3 `pipeline.Summary` 已做跨任务聚合 | `pipeline.TestSummaryAggregation` + `e2e.TestPipelineSummary`（均实测 PASS） | pipeline.Summary |
| 53 | P2 | 性能验收口径（9.9µs 范围/缓存命中率/端到端） | 采纳补定义 | 9.9µs=**纯分类器** BenchmarkPipelineProcess（不含网络/模型/执行）；缓存命中率与端到端延迟目标 M3 定 | `BenchmarkPipelineProcess`（实测 5.5µs<9.9µs） | bench/ |
| 54 | P2 | risk.go <150 行计数范围/冲突优先级 | 已在冻结/M1 实现 | 计数=Evaluate+StaticImpact+Signals/Decision 定义；Guard/ImpactInput/测试不计；与可维护性冲突时优先可维护（<150 是指导线非铁律） | `wc -l risk/risk.go`（B） | risk/ |

**54 条裁定统计**（逐行复核）：
- 采纳补定义：#4,5,7,9,10,11,14,15,16,17,18,21,23,26,28,29,40,41,42,43,44,45,46,48,49,50,53 = **27**
- 已在冻结/M1 实现：#1,2,3,6,8,12,13,19,20,22,24,25,27,30,31,32,33,34,35,36,38,39,47,51,54 = **25**
- 降级 P2：#52 = **1**（M3 已细化，见下）
- UNKNOWN：#37 = **1**（M3 已实现为 ground 包，见下）
- 合计 27+25+1+1 = **54**（无遗漏）。**M4 收尾后全部 54 条均带实测验证器，无残留"未完成"**：#4/#5/#11/#43/#45 落 pipeline/server/e2e；#15 符号链接逃逸硬化、#37 ground、#52 摘要、#49 cmdCompare 在 M3 落地；**#29 request_id 幂等去重在 M4 由 `server.TestRequestIDDedup` 补为全覆盖**。当前仅余「残留已知限制」（非缺口）：refer 候选仅歧义分支产出、interrupted 需重提交、真实 git 仅限注册 project 域。

### 1.1 意图 JSON Schema（#7 机器可读形态）

```json
{
  "intent": "EDIT",
  "raw_text": "ASR 原文留底",
  "corrected_text": "词典纠错后",
  "corrections": [{"from":"季总","to":"冀总","rule":"dict"}],
  "confidence": 0.9,
  "ask": "",
  "space": "voxbuybot",
  "target": {"entity":"voxbuybot","ref_type":"explicit"},
  "params": {"action":"replace","object":"错误提示","value":"中文"},
  "boundary": {"scope":["voxbuybot/**"],"exclude":[".env*","node_modules"]},
  "risk": {"reversible":true,"impact":"medium","confidence":0},
  "confirm": "light",
  "acceptance": "改动限定在 voxbuybot/** 内，无越界改动",
  "context": ["project-map:voxbuybot"],
  "conflict": ""
}
```
人话外壳：**一句口语 → 一个结构化任务书（意图+指向谁+改什么+边界+风险+做到啥算完）；ask 非空就是没听懂、不许动手。**
必填：intent、confidence、corrected_text。枚举：intent∈八类+REGISTER_TOOL；confirm∈auto/light/strong/human；impact∈small/medium/high；conflict∈{ask_vs_op,note_vs_deploy,debug_plan,delete,negation,meta,conditional,multi_action,""}。非法→UNKNOWN+ask。

---

## 2. 北极星 #40–#49 可执行定义（必须落地，freeze §4.2）

### 2.37 认知切片 context 注入（#37，原 UNKNOWN → 落地）
- **数据源**（只准用 harness 自己的 `<log_dir>`，绝不碰 `~/.voicesign`）：
  1. **project-map**：space 注册表逐域渲染 `project-map:<域名>(<类型>) <scope目录前8项清单>`；
  2. **decisions**：`<log_dir>/decisions.jsonl`，一行一条：`{ts,task_id,intent,decision,confirm,reason}`，在**确认闸落盘**（auto 决策记 `auto_skipped`，人工记 `approved/rejected`）。
- **更新时机**：每任务在 Ask 回问 / 确认文案 / 外部代理 prompt 构建**前**刷新渲染（注入点必须是活代码，不得是死路径）。
- **长度限制**：渲染块默认 ≤2000 字节、decisions 保留最近 20 条（常量 `ground.DefaultMaxBytes=2000` / `ground.DefaultMaxDecisions=20`，可由字段覆写）；超出**按字节截尾**（rune 安全）、decisions **截断最旧**。
- **缺失行为**：任一数据源文件不存在/为空 → 渲染占位空块（`（无注册域）/（无）`），**不报错、Ask 照常走**。
- **只读注入**：ground 只读认知切片喂模型/人，绝不反写词典/策略/风险阈值。
- **验证器**：`ground.TestRecordAndRenderDecisions`（记录并渲染）、`ground.TestRenderMissingDataSources`（缺失数据源不报错）、`ground.TestByteCap`（字节上限）、`ground.TestTruncateOldestDecisions`（截断最旧）——均实测 PASS；注入点见 `pipeline/pipeline_test.go` Ask/ContextBlock 分支。

### 2.40 撤销协议
- **无 git 仓库**（本 harness 目录即如此，freeze §7）：每次 EDIT/DELETE 执行**前**，把目标文件拷到 `<log_dir>/backups/<UTCts>.<原名>.bak`；回执「撤销」行写该路径。
- **有 git 的项目域**：优先 `git stash/create 备份分支`，回执「撤销」行写 `git restore <file>` / `git checkout <ref>`。
- **跨步骤**：一个回执含多文件时，备份目录一次快照 N 个；撤销=整目录还原。
- 不可逆动作（COMMIT/DEPLOY/外发）→ 撤销行固定写「不可撤销（不可逆，已人工确认）」，**禁止**给假撤销承诺。
- 验证：`tools.Executor` EDIT 后 `t.TempDir()` 里 bak 文件存在（C）；`Test20SampleReceiptFields` 守住四行格式。

### 2.41 四行回执字段规范（contract.RenderReceipt）
四行固定：`动作：` / `文件：` / `结果：` / `撤销：`。各情形：
| 情形 | 文件行 | 结果行 | 撤销行 |
|---|---|---|---|
| 单文件改 | `voxbuybot/x.go` | `OK（轻确认后执行）` | bak 路径 |
| 多文件改 | `N 个文件` | `OK（改了 N 文件+关键行摘要）` | bak 目录 |
| 无文件（NOTE/QUERY/ASK/TEST） | `—` | `OK` / `未执行（需回问：…）` | `—（只读）` |
| 失败 | 涉及文件 | `FAILED：<原因一行>` | 备份仍在 |
| 待确认（human） | 涉及文件 | `待确认（不可逆，等人工放行）` | `不可撤销（已人工确认后）` |
| 越界拦截 | — | `BOUNDARY_VIOLATION：<原因>` | `—（未执行）` |
- 验证：`Test20SampleReceiptFields`（四行四标签齐全）+ contract 测试。

### 2.42 归因六格 + 证据规则
六格（contract.Attribution.Class）：`input`(ASR/纠错错) / `context`(认知切片不足) / `contract`(契约/触发词定义缺陷) / `model`(模型判断错) / `execution`(执行/环境错) / `external`(模型中心/网络/第三方)。
- **证据规则**：每条归因必须带 `evidence`（指向轨迹条目 kind / 文件 / 输出片段），**禁止**空证据下结论。
- 识别顺序：先看 input 是否被纠错（#7/#11→input）；再看触发词是否误命中（#17/#18/#20→contract）；再看是否冲突仲裁（→model）；执行后 verify fail→execution/external。
- 本批 20 样例实填见 `data/20-tasks.jsonl`（model×14、input×2、contract×3）。

### 2.43 归因回写 discuss
- discuss 阶段产出两条轨迹：`kind=attribution`（六格+证据+建议）+ `kind=discuss-log`（人可见结论）。
- **下一轮注入**：attribution 作为认知切片注入模型首条上下文（「上次这类错误归 input，词典已固化」）。
- **禁止**：归因**不自动**改词典/策略/风险阈值——只记录+建议；改策略必须人拍板（五原则：理解≠授权）。
- 验证：`pipeline.TestPipelineWritesAttribution`（实测 PASS）；20 样例 jsonl 钉住归因形态。

### 2.44 轨迹事件规范 + 脱敏
- 每条轨迹事件 JSONL 字段：`request_id`（任务关联主键）、`stage`(record/discuss/control)、`kind`(input_raw/clean/correct/classify/refer/space_check/risk/confirm/exec/verify/attribution)、`ts`（阶段时间戳 RFC3339）、`evidence`（引用的前序 request_id/文件/输出片段）。
- **脱敏**：vault-creds 原文、token、密钥**禁止入轨迹**——替换为 `vault-creds:<key>` / `***` 占位。
- **写入失败处理**：轨迹写盘失败**不阻断只读任务**，但阻断不可逆动作（control 阶段写失败→不 commit/deploy）；失败留 stderr 日志。
- 验证：`trajectory.TestWriteAppendsParseableLines`/`TestConcurrentWrites`（实测 PASS）；vault-creds 脱敏由 space 用例守住。

### 2.45 认知闭环计量口径（北极星，必须可测）
- **start 事件** = `input_raw` 轨迹写入完成的时刻。
- **end 事件** = `attribution`（归因记录）写入完成的时刻。
- **双口径**：
  - `LoopMs` = end−start **墙钟**，含人工等待（confirm 等用户按键/手机回复的整段等待都算）。
  - `NetMs` = 剔除人工等待后的**净执行**毫秒（机器干活时间）。
- 人工等待 = confirmFn 阻塞段 + 回问等待段；LoopMs−NetMs = 等待 Ms。
- **M2 目标值（可测阈值）**：单任务 `NetMs < 500ms`（不含外部模型/工具真实耗时，仅本地 Go 管线）；`LoopMs` 不设硬阈值（取决于人），但必须双报。验收看 NetMs 环比下降、等待 Ms 占比随缓存命中率上升而下降。
- 验证：`pipeline.TestPipelineWritesAttribution`（Outcome.LoopMs/NetMs 双报，实测 PASS）；本地分类器段已实测 ~5.5µs 级（bench）。

### 2.46 「10 倍指数」定位
- **M2 = 愿景表述**，不是 M2 的验收量化指标（否则无法证伪）。
- M2 验收口径 = **环比改善方向**：每轮迭代 NetMs、等待占比、20 样例通过率、缓存命中率须**单调不劣化**；不要求本版达成 10×。
- 10× 是 5–10 个迭代后、跨周/跨月回看的方向锚点，由 `vhs compare`（#49）与每日摘要累积数据支撑。

### 2.47 20 条样例通过标准
- 定义：`docs/20-任务方向验证记录.md` + `data/20-tasks.jsonl`；4 来源×5 条口语样例。
- 通过：意图分类与人工标注一致 **≥17/20**；四行回执四字段齐全；归因六格可填。
- 实测：**20/20**（门槛达标；首版 17/20 的 3 条同源 finding 已由组织者修复 thoughtWords×想法库碰撞，见样例记录 §3）。
- 模拟能力范围（本阶段允许）：clean+dict+TaskClassifier+stub 回执；**不接** refer/space/risk/verify/tools 真执行。
- 验证器：`go test ./input -run Test20Sample -v`。

### 2.48 核心用例测试夹具
- 统一 `t.TempDir()` 构造最小 fs（禁止写 ~/.voicesign/~/VoxSign/项目外）。
- 漂移夹具：写一个 `.space.json` 指到不存在目录→DetectDrift 检出。
- 越界夹具：EDIT 目标落在 Exclude/`.env*`→space.Check=BOUNDARY_VIOLATION。
- 独立校验夹具：构造文件改动 + 期望 diff→verify.Run 读 fs 对预期（不读执行器自报）。
- 归因夹具：跑一条错误路径→Outcome.Attribution.Class∈六格。
- 落点：各包 _test.go（B/C/D）；本批已落地 input 侧 20 样例夹具。

### 2.49 单工对比（vhs compare）
- **基线**：同一任务用键盘/IDE 手做，计时从「开始读任务」到「验收通过」。
- **3 任务选择**：跨三类各一——一个 EDIT（改文案）、一个 DEBUG（修报错）、一个 NOTE/QUERY（查/记）。
- **计时范围**：语音侧 = #45 的 LoopMs（含说+等+回执）；键盘侧 = 人操作墙钟。
- **等快容差**：±10% 内算「等快」；目标 ≥2/3 任务语音更快或等快（SPEC v1 任务卡 12）。
- 实现可降级为脚本/报告（freeze §3 main compare）。
- **M3 实跑结果**（`vhs compare` = `main.go:cmdCompare`，对 `data/20-tasks.jsonl` 跑 ground off vs on）：20 任务全跑通；context on 配置意图通过率 **85%**；归因分布 **context×15 / input×2 / model×3**。键盘侧基线仍由人手测（本 harness 不自动代填），故「10 倍指数/等快容差」本轮只取**语音侧环比改善方向**，不夸大为绝对快于人手。

---

## 3. 12 条验收用例 → 验证器清单（每条可独立复现）

| # | 复现步骤 | 可复现断言（go test / 命令） |
|---|---|---|
| 1 NOTE+指代+时间锚点 | 跑「记一下冀总那个厂房下周一出报价」 | `TestTaskClassifyNoteWithTimeAnchor`：Intent=NOTE、Ask=""、time_hint=下周一、time_date 非空 |
| 2 词典纠错 美墅→Mansour | 跑「美墅那个项目聊到哪了」前过 dict.Correct | `memory` 词典测试 + `Test20SampleValidation` #7/#11（季总→冀总、彼得周点com→…） |
| 3 跨域指代回问 | 「那个文件改好了吗」两域都有 Recent 实体 | `refer.TestResolve_CrossDomainAmbiguous`（实测 PASS）：必须 Ask「哪个域？」不猜 |
| 4 「它」指上一条回执 | Recent 注入上一回执文件，跑「它改好了吗」 | `refer.TestResolve_AnaphoraToRecentFile`（实测 PASS）：命中 Recent 不再问 |
| 5 project 域内 EDIT 放行 | 在注册 project 域内跑 EDIT | `space.TestCheck_ProjectEditAllowed`（实测 PASS）：Allowed=true |
| 6 vault-creds 外发拦截 | 对 vault-creds 发起外发动作 | `space.TestCheck_VaultCredsExternalBlocked`（实测 PASS）：Reason=boundary_violation/cross_ref_deny |
| 7 漂移检测失效 | manifest scope 指向已删目录 | `space.TestCheck_DriftDetectedAndDenied`（实测 PASS）：检出漂移并 deny |
| 8 可逆+小+高 → auto | ImpactInput{RefCount:1,HasTest:true,Heat:0} | `risk.TestEvaluate_AutoSmallHigh`（实测 PASS）：Level=auto |
| 9 不可逆永远 human | DELETE/外发/commit/deploy | `TestTaskClassifyIrreversibleConfirm`：Confirm=human、Risk.Reversible=false |
| 10 verify 读实际不读自报 | 改文件后跑 verify，断言与 fs 一致 | `verify.TestRun_DoesNotTrustExecutorClaim`（实测 PASS）：Status 来自 fs 不来自执行器 stdout |
| 11 回执四行 | 构造 ReceiptView 渲染 | `Test20SampleReceiptFields`：恰好四行、四标签齐全 |
| 12 执行后归因 | 跑一条错误路径看 Outcome.Attribution | `pipeline.TestPipelineWritesAttribution`（实测 PASS） |

---

## INTERACT-v1 交互协议（M3 新增 · M4 升级 server 契约）

> 一句话人话：手机端投一句语音，拿一个 task_id，轮询到「要你拍板」时回一句 answer——回问可**续跑**、确认可放行、做完可 rollback——**一屏永远只有一个决策点**。

### 端点表（当前生效）
| 方法 | 路径 | 请求体 → 响应 | 语义 |
|---|---|---|---|
| POST | `/v1/tasks` | `{text, space?, request_id?}` → **202** `{task_id, status}` | 提交任务；带 request_id 时幂等去重（见约束 4） |
| GET | `/v1/tasks/{id}` | → 200 `{task_id,status,question?,options[],receipt?,attribution?,reversible?,error?}` | 轮询；options=结构化候选 `[{id,label}]`；receipt=四行回执，attribution=六格 |
| POST | `/v1/tasks/{id}/answer` | `{answer}` → 200 `{ok,approved}` 或 `{ok,resumed:true}` / **409** | 回答当前**唯一**决策点（按状态分流，见约束 1） |
| POST | `/v1/tasks/{id}/rollback` | → 200 `{ok,restored}` / **409/404** | 从**具体** `.bak` 精确回滚，**仅可逆任务** |
| POST | `/v1/tasks/{id}/cancel` | → 200 `{ok}` | M6 新形态取消/打断：立即广播 `interrupt` 事件后转 `canceled`（详见 SSE 章节） |
| GET | `/v1/tasks/{id}/events` | → `text/event-stream` | M6 SSE 事件流；`?after=<seq>` 幂等重连（详见 SSE 章节） |
| GET | `/v1/roles` | → 角色清单 | 角色实时（planner/executor/verifier），任务态带 `role` 字段（M5） |
| GET | `/v1/status` | → 200 `{version,uptime,ok}` | 存活探针 |

旧端点保留但标 **deprecated**：`/v1/run`、`/v1/task/{id}`、`/v1/confirm`、`/v1/cancel`（M6 起由 `POST /v1/tasks/{id}/cancel` 取代，旧路径兼容）、`/v1/health`（新手机面一律走 `/v1/tasks*`）。

### 状态词机（writeTaskView 输出）
`running`（执行中）/ `need_ask`（低置信回问，带 question + 结构化 options，**挂起等 answer 续跑**）/ `need_confirm`（强确认红条，等 answer 放行/拒绝）/ `done`（完成，带 reversible+receipt+attribution）/ `canceled`（出错/取消）/ `interrupted`（M4：重启恢复出的未完成任务，**不自动续跑**，需重新提交）。旧字段 `waiting_confirm` 仅兼容旧端点。

### 约束（必须，不可绕过）
1. **一屏一决策点（仍成立），但 answer 不再 409 终结回问**：answer 按任务状态分流——`need_confirm` → 确认桥（放行/拒绝）；`need_ask` → 以 `澄清：<answer>` 自由文本续跑，或候选 id 点选映射为强关键词前缀续跑同一任务（返回 `{ok,resumed:true}`）；其余状态（done/running/canceled/interrupted）→ **409**「当前无待回答的决策点」。一次只答当前这一个决策点。
2. **rollback 仅可逆 + 精确备份路径**：`status != done` 或意图不可逆（COMMIT/DEPLOY/DELETE 等）→ **409**；无备份/目标未知 → 404。备份路径由 executor 以结构化标记 `VHS_BACKUP_PATH: <绝对 .bak>` 上报，回执「撤销」行显示该**具体** `.bak`，rollback 精确还原到它（不再「扫最新 .bak」）。
3. **认证**：配了 token → 必须 `Authorization: Bearer <token>` 或 `X-Token`，否则 401；未配 token → **仅本机回环**，外部 403。
4. **request_id 幂等去重（M4）**：同一 `request_id` 二次提交 → 命中既有任务直接 **200 deduped**，**绝不重复执行**不可逆动作。

### 状态持久化（M4）
每次状态迁移后 `persist(ts)` → `<log_dir>/tasks/<id>.json`（tmp+rename 原子写）。server 启动时 `restore()`：`done/canceled` 原样入表（历史可查、done 仍可 rollback）；`running/need_confirm/need_ask` → 标 `interrupted`（不重建 confirmCh/cancel，防死等）。

### 验证器（均实测 PASS）
- 生命周期 + 确认闸：`server.TestTasksLifecycleConfirm`
- answer 无未决决策点 → 409：`server.TestAnswerNoPendingConflict`
- **need_ask 挂起续跑**：自由文本澄清续跑 `server.TestAskResumeViaAnswer`；候选 id 点选续跑 `server.TestAskAnswerByOptionID`
- **options 结构化**：`pipeline.TestAskOptionsStructured`（`{id,label}`）、`pipeline.TestAskOptionsIncludeReferCandidates`（意图+refer 候选合并）、`refer.TestResolveOptionsStructured`（refer 候选导出）
- **request_id 幂等去重**：`server.TestRequestIDDedup`
- **状态持久化恢复**：`server.TestTaskPersistenceRestore`
- **精确备份回滚**：`tools.TestExecutorBackupPathReported`、`pipeline.TestReceiptShowsBackupPath`、`server.TestRollbackNote`（精确恢复）；不可逆回滚 → 409：`server.TestRollbackIrreversible409`
- 存活探针：`server.TestStatusEndpoint`

### 残留已知限制（M4 后）
- refer 结构化候选**仅在歧义分支产出**（无歧义时 options 为空，`refer.TestResolveOptionsEmptyWhenNoCandidates`）。
- server 重启后 `interrupted` 任务**需重新提交**（不自动续跑，防死等）。
- 真实项目 git 端到端**仅限注册 project 域**（COMMIT 根=project scope 根；禁 amend/force/reset，回执带 git log 证据——见 §2/§6）。

---

## SSE-v1 事件流（M6-1 新增 · server/web/iOS 三方契约）

> 权威契约 = **`docs/SSE-v1-事件流契约.md`（v1.0 冻结）**。本节为 SPEC v2 的约束式摘要；冲突以冻结契约为准。
> 一句话人话：任务跑起来后，手机端开一条长连接实时看执行卡一行行滚、决策点弹出来、点「停」立刻收到打断信号——断线重连不重不漏。

### 端点与重连
- `GET /v1/tasks/{id}/events`，`Content-Type: text/event-stream`，全来源必带 `Authorization: Bearer <token>`；连接保持到任务终态（done/failed/canceled）后关闭。
- **重连幂等**：客户端记 `lastSeq`，断线重连带 `?after=<lastSeq>`（或标准 `Last-Event-ID` 头）；server 重放 `seq > after` 的全部事件，**不重复、不丢**；after 缺省=从头。事件记录与任务持久化同生命周期。
- 旧轮询 `GET /v1/tasks/{id}` 保留——SSE 客户端以它做断线兜底。

### 七类事件（data 必含单调递增 `seq`，从 1 起；缺字段不得崩溃）
| event | data | 语义 |
|---|---|---|
| `stage` | seq, role, phase, step | 阶段滚动行（role=planner/executor/verifier；step=意图分类/域裁决/风险分级/确认闸/执行/校验/归因） |
| `need_ask` | seq, question, options:`[{id,label}]` | 回问决策点（一屏一个；answer 后续跑继续推） |
| `need_confirm` | seq, question | 强确认红条（answer:"执行"放行） |
| `done` | seq, receipt, attribution, reversible, role | 完成（receipt=四行文本） |
| `failed` | seq, error | 失败终止 |
| `interrupt` | seq, applied, notApplied, canRollback | 打断生效广播（已生效/未执行/可撤销三语义） |
| `canceled` | seq | 取消终态 |

### 打断语义（交互 v2.1 规则 2）
用户说「停」或点停止 → `POST /v1/tasks/{id}/cancel`（legacy `/v1/cancel` 兼容）→ server 立即向该任务 SSE 连接广播 `interrupt`，随后 `canceled` 关闭。SSE 客户端以 `interrupt` 为即时信号（优于轮询）；轮询兜底以 `canceled` 状态为准。

### 验证器（均实测 PASS，`go test ./server -run TestSSE`）
- 事件序列有序（stage→need_ask→…→done，seq 递增）：`server.TestSSEEventSequence`
- 重连幂等（`?after` 只收其后事件无重复）：`server.TestSSEReconnectIdempotent`
- 决策点事件挂起到达、answer 后续跑推流：`server.TestSSENeedAskDecisionPoint`
- 打断即时性（cancel 后 interrupt 先于轮询可见终态）：`server.TestSSEInterruptImmediacy`
- 角色实时：`server.TestRolesEndpoint` / `TestTaskStatusHasRole`（M5 roles 特性，server 全包已转绿）。

### 消费端（M6-1b / M6-2，按各自 shard 交付报告引用）
- **web/**：手写 `fetch`+`ReadableStream` SSE 客户端（Bearer）+ `?after` 重连 + 轮询兜底 + 角色实时；`node web/test.js` = **66 通过 / 0 失败**（本机实跑取证）。
- **ios/**：`voicesign-harness/ios/` SwiftUI 工程（VoiceSign.xcodeproj + Core/VSLogic.swift + Core/SSEParser.swift），**BUILD SUCCEEDED、逻辑断言 61/61**——来源=iOS shard 交付报告（本环境无法跑 Swift，按报告如实引用；模拟器沙箱受限）。


---

## 4. 七个必写模块 · 伪代码逻辑层（CLRS 式骨架，不可编译；语义规则标注「搬 VSL」）

> 分工：VSL 管「做什么」（意图/域/契约/判断语义），本节管「怎么做」（单模块控制流/分支/异常）。凡出现意图判断规则本体 → 标注搬 VSL。

### 4.1 space_check（B）
```text
function Check(registry, in) -> Verdict:
    space := registry.Get(in.Intent.Space)
    if space 未注册:                        return deny("unknown_space")   # 不自动切 global
    if registry.DetectDrift(space):         return deny("drift")          # 漂移即失效
    if space.Scope ∩ in.Intent.Boundary.Exclude ≠ ∅:
                                            return deny("boundary_violation")
    # 有效权限 = 平台∩Manifest∩契约caps∩本次授权 交集（交集为空→default_deny）
    allowedTools := space.Tools ∩ in.ToolCaps ∩ 契约caps
    if allowedTools == ∅:                   return deny("default_deny")
    if 本次动作跨域 且 space.CrossRefs 未声明: return deny("cross_ref_deny")
    return allow(space.ID, allowedTools)
    # 裁决语义（什么算越界/默认拒绝）→ 搬 VSL（freeze §13 三模型共识）
```

### 4.2 指代消解 refer（B）
```text
function Resolve(intent, spaceID):
    if intent.Target.Entity 已在词典命中:   return intent          # 代码层 100%
    if text 含 它/那个文件/上次:
        cand := Recent[spaceID] 里按 (space匹配, Ts新) 排序
        if 唯一最高:                        return 填 Target=命中
        if 顶级平局:                        Ask("哪个域/哪个？")    # 不猜
    if ModelFn ≠ nil:
        ent, conf := ModelFn(text, cand)
        if conf < 阈值:                     Ask(...)               # 规则层低置信不许静默吃
        else:                               return 填 Target
    else:                                   Ask(...)
    # 「哪个指代哪个实体」的判断语义 → 搬 VSL §14
```

### 4.3 风险分级 risk（B）
```text
function Evaluate(intent, imp) -> Decision:
    if 意图 ∈ {COMMIT, DEPLOY, DELETE, 外发, vault-creds出域}:
        return human("不可逆，永远人工")      # 硬规则，不可学习掉
    impact := StaticImpact(imp)             # RefCount≥8 high / ≥3 medium / else small
    if 可逆 & impact==small & 高置信:        return auto(标待抽查)
    if 可逆 & impact==small & 低置信:        return auto(待抽查+回执高亮)
    if 可逆 & impact==medium:               return light(diff 摘要)
    if 可逆 & impact==high:                  return strong(diff 预览+影响)
    # 三信号取值/决策矩阵语义 → 搬 VSL（设计 v2 §4）
```

### 4.4 verify 独立校验（C）
```text
function Run(spec) -> Result:
    switch spec.Kind:
      test:   out := 自己执行 spec.Args 的 go test 命令; 读退出码与输出
              status := pass(退出0) / fail(非0) / unverifiable(超时)
      diff:   actual := 自己读 fs 目标文件; 对期望; 不一致→fail
      grep:   自己搜断言; 命中→pass
    # 禁止读执行器自报 status（防自证）；预期来自 acceptance，实际来自 fs
    return Result{status, evidence, detail}
    # 「什么算完成/通过」→ acceptanceTemplate（语义搬 VSL）
```

### 4.5 四元缓存 cache（C）
```text
function Get(QuadKey{Intent,Space,Perm,Ref}):
    e := store[key]
    if e 存在 & now<e.ExpiresAt & e.Version==store.Version:  return e.Decision(hit→不再问)
    return miss
function BumpPolicyVersion():
    store.Version += 1; 落盘; 全 key 因版本不符自动失效
function InvalidateSpace(space): 删该 space 前缀的 key
# 键语义/何时问 → 搬 VSL（冻结缓存绑定规则）
```

### 4.6 REGISTER_TOOL（C）
```text
function Register(contract, approved):
    err := ValidateContract(contract)        # name/caps/params/risk 必填，risk 覆盖每 cap
    if err: return err
    d := risk.Evaluate(REGISTER_TOOL意图, 机械信号)
    if not approved: return err("未人工批准，不落盘")   # 自举充分条件
    写 ContractsDir/<name>.contract.json (写前备份, version+1)
    return ok
# 「语音注册=注册元数据不生成代码」语义 → 搬 VSL（设计 v2 §7 自举）
```

### 4.7 意图冲突仲裁 = input.TaskClassifier（A 写权威小节，交叉核验 taskintent.go）
**与 `input/taskintent.go` 头部【伪代码逻辑层】注释逐段比对结论：一致，无矛盾。** 权威控制流如下（优先级从高到低）：
```text
function ClassifyTask(text) -> Intent:
    if text 空/纯空白:                       return UNKNOWN+Ask
    text := runes 截断到 512
    # —— 1. 冲突仲裁（先于单类触发）——
    if 含 registerTriggers:                  return REGISTER_TOOL(0.95)
    if 含 deleteTriggers:                    return EDIT(action=delete, conflict=delete)  # 不可逆包→human
    # 否定仲裁（缺口 G1，**最优先**）：否定词直接支配动作 → ASK(conflict=negation)，绝不执行
    #   评审 P1 补齐：勿/请勿/切勿/无需/不再/免了；裸"别/勿"按后随动词判定
    if 否定仲裁命中: return ASK(conflict=negation)
    # 元指令仲裁（缺口 G3）：短句「开始/继续+动作」是对话控制，不是执行命令 → ASK(conflict=meta)
    #   长句豁免（M7 真机回归 TestCodexNineRegressions#8：长句元指令不得 Ask）
    if 元指令仲裁命中: return ASK(conflict=meta)
    # 条件句仲裁（缺口 G6）：「如果测试通过就提交」的前提不能被忽略 → ASK(conflict=conditional)
    #   限定窗口：标记之后 12 字内须有动作词（M7 真机长句含"如果/就"但必须保持不 Ask）
    if 条件句仲裁命中: return ASK(conflict=conditional)
    # 多动作检测（缺口 G5）：「查一下库存，然后记一下结果，最后提交」只做第一件会静默丢弃其余
    #   判据=顺序连接词（然后/接着/最后…）切分后出现 ≥2 个**不同**动作意图；
    #   排在 extractReplace 提前返回之前；ORCHESTRATE 任务已由计划承载故不触发
    if 多动作检测命中: return ASK(conflict=multi_action)
    if 含 feasibleAsk(能不能/可不可以/是否可以/行不行): return ASK(conflict=ask_vs_op)
    if 含 thoughtWords(想法/备忘) 且 该词不在空间名/别名内: return NOTE(conflict=note_vs_deploy)  # 已修：想法库=实体名不触发
    if 含 statusQuestion(好了吗/改了吗…):      return QUERY
    if 含 debugTriggers & debugPlanWords:     return ASK(0.5, conflict=debug_plan)   # 修思路≠修
    # —— 2a. 显式「把 X 改成/换成 Y」——
    if extractReplace 命中:                   return EDIT(0.9, object/value)
    # —— 2b. 单类触发（长词优先，CJK 子串匹配）——
    按 NOTE>QUERY>DEBUG>TEST>COMMIT>DEPLOY>ASK>EDIT 顺序首命中
    TEST 含 性能/bench/基准 → test_kind=bench
    # —— 3. 无触发词 ——
    return UNKNOWN+Ask
    # fill(): 空间候选最长匹配(平局不猜) / 时间锚点 / 风险+确认基线 / acceptance 模板
    # 低置信(<阈值 且 非 NOTE/ASK/REGISTER_TOOL) → 定制 Ask；否则清空 Ask
```
**交叉核验记录（诚实指出的偏差/缺口）**：
1. 头部注释 §1 顺序与代码 `switch` 一致（register→delete→feasible→thought→status→debug_plan）。✅
2. 注释 §2a 把「把 X 改成 Y」放在单类触发之前——代码确实如此（`extractReplace` 在单类 switch 前）。✅
3. **历史 surfaced finding（已修复）**：首版「空间名子串可命中 thoughtWords」（即 #17/#18/#20 误判 NOTE）已由组织者修复——空间名/别名命中含「想法/备忘」时跳过 thoughtWords 仲裁、noteTriggers 去裸「记」。当前 §4.7 权威流水中的 thoughtWords 段已隐含该前提，断言 20/20。
4. 仲裁产出的 `conflict` 标记取值与 contract 常量一致（ask_vs_op/note_vs_deploy/debug_plan/delete/negation/meta/conditional/multi_action）。✅
5. **M5-1 surfaced finding（已修复）**：「在笔记里记下 M4 测试」被 TEST 词「测试」抢先误判 TEST——根因 noteTriggers 缺「记下/记个」，句尾「测试」成首个命中。修复：noteTriggers 补「记下/记个」，且 2b 单类开关 NOTE 本就先于 TEST 命中。规则：NOTE 语境词（记下/记一下/记个/记录，含笔记/想法语境）**优先于** TEST 词；句尾「测试」是被记录对象→NOTE。**例外不回归**：句子不含 NOTE 词、仅独立 TEST 触发（跑一下测试/执行测试/测试这个函数）→仍 TEST。验证器 `input.TestTriggerCollisionNoteVsTest`（正反例 + QUERY 不回归，实测 PASS）；20 样例仍 20/20。

---

## 5. 机器可读形态速查（manifest / contract / policy）

### 5.1 域 Manifest（`.space.json`，人话外壳：「这个域能碰哪、能干什么、做到啥算完」）
```json
{
  "name": "voxbuybot",
  "type": "project",
  "scope": ["/path/to/voxbuybot/**"],
  "exclude": [".env*", "node_modules"],
  "tools": ["search", "file", "git", "test", "verify"],
  "perms": {"read": true, "write": true, "exec": ["go build", "go test"]},
  "context": ["project-map:voxbuybot"],
  "acceptance": "改动限定在 scope 内，无越界改动",
  "risk_default": "auto",
  "cross_refs": [],
  "version": 1
}
```

### 5.2 工具契约（`.contract.json`，人话外壳：「这个工具每一步能干啥、副作用、风险」）
```json
{
  "name": "git", "version": "@1.0",
  "caps": ["status", "diff", "log", "commit", "checkout"],
  "params": {"path": "string,required", "range": "string,optional"},
  "side_effects": ["修改工作区/索引", "commit 不可逆(本地)"],
  "allowed_spaces": ["project", "sandbox"],
  "risk": {"commit": "irreversible", "checkout": "medium", "status": "none"}
}
```

### 5.3 策略（policy，见 §0 头部）

---

## 6. 与冻结接口的一致性声明

- 本 SPEC 未改任何冻结 API 签名；所有定义为约束式（禁止/必须验证/完成），实现细节交 B/C/D。
- surfaced finding（thoughtWords×想法库子串碰撞，#17/#18/#20）**已由组织者修复**：空间名命中含「想法/备忘」时跳过 thoughtWords 仲裁、noteTriggers 去裸「记」。本 SPEC §1#10、§4.7、20 样例记录 §3 三处已同步为「已修复，验证器回正」，20/20 通过。
- surfaced finding（M5-1：NOTE 语境词 vs TEST 触发词碰撞，「在笔记里记下 M4 测试」误判 TEST）**已修复**：noteTriggers 补「记下/记个」，NOTE 先于 TEST 命中；独立 TEST 触发（跑一下测试/执行测试/测试这个函数）不回归。验证器 `input.TestTriggerCollisionNoteVsTest`（§4.7 第 5 条）。
- 新增文件：`docs/SPEC-v2-可执行规格书.md`（本文件）、`docs/20-任务方向验证记录.md`、`data/20-tasks.jsonl`、`input/taskintent_20sample_test.go`（测试）。未删改任何既有 .go。
- **M3 安全修复记录**：
  - **human 四元缓存双排除**：缓存 Get/Set 两侧都**不得**对 Confirm=human 的任务自动放行缓存命中（人工确认不可被四元组静默覆盖）——验证器 `pipeline.TestCacheNeverAutoApprovesHuman`（实测 PASS）。
  - **派生目录跟随 VHS_LOG_DIR**：cache/space/contracts 等派生目录必须跟随 `VHS_LOG_DIR` 环境变量联动；未设 env 时回退默认——验证器 `config.TestDerivedDirsFollowLogDirEnv` / `TestDerivedDirsDefaultWhenNoEnv`（均实测 PASS）。
  - 配套：#15 符号链接逃逸硬化（space `EvalSymlinks` + verify 重读 fs 双保险）见 §1#15，验证器已钉。
- **M3 落地小结**：#37 ground 认知切片、#20 真实引用计数、#52 摘要聚合、#49 cmdCompare、INTERACT-v1 server 面均已落地并各带实测验证器。
- **M4 落地记录**（均实测 PASS，先取证后落笔）：
  - **#29 request_id 幂等去重补为全覆盖**：`server.TestRequestIDDedup`。
  - **need_ask 由"同步回问/answer 409 终结"升级为"挂起续跑"**：`server.TestAskResumeViaAnswer`（自由文本澄清续跑）、`server.TestAskAnswerByOptionID`（候选 id 点选续跑）；options 结构化 `{id,label}`：`pipeline.TestAskOptionsStructured`、`pipeline.TestAskOptionsIncludeReferCandidates`、`refer.TestResolveOptionsStructured`。INTERACT-v1 章节已同步（answer 不再 409 终结、options 结构化、一屏原则仍成立）。
  - **任务状态持久化**：落盘 `<log_dir>/tasks/<id>.json`（原子写），重启历史可查、done 可 rollback、未完成标 `interrupted`——`server.TestTaskPersistenceRestore`。
  - **summary Net 口径**：纯管线（无 LLM）任务不计入 Net 均值——`pipeline.TestSummaryNetExcludesNoLLM`。
  - **executor 备份路径结构化契约**：`VHS_BACKUP_PATH: <绝对 .bak>` 标记 + 回执撤销行显示具体 .bak——`tools.TestExecutorBackupPathReported`、`pipeline.TestReceiptShowsBackupPath`、`server.TestRollbackNote`（精确恢复）。
  - **真实项目 git 端到端**：COMMIT 执行根=project scope 根、未提交改动计数进确认文案、禁 amend/force/reset、回执带 git log 证据——`pipeline.TestGitCommitInProjectRoot`、`pipeline.TestCommitRefusedNoExec`（无 executor 时拒绝 COMMIT）。
- **残留已知限制（M4 后，非缺口）**：refer 结构化候选仅歧义分支产出；server 重启后 `interrupted` 需重新提交；真实项目 git 端到端仅限注册 project 域。**54 条缺口均已带可执行验证器，无"未完成"项。**
- **M6 落地记录**（先取证后落笔；server 测试均 `go test -v` 实跑 PASS）：
  - **SSE 事件流（M6-1a，server）**：`GET /v1/tasks/{id}/events` + `POST /v1/tasks/{id}/cancel` + 七事件类型（stage/need_ask/need_confirm/done/failed/interrupt/canceled）+ `seq` 单调递增 + `?after` 幂等重连（不重不漏）+ 打断即时广播。权威契约 `docs/SSE-v1-事件流契约.md`。验证器：`server.TestSSEEventSequence`、`TestSSEReconnectIdempotent`、`TestSSENeedAskDecisionPoint`、`TestSSEInterruptImmediacy`（全 PASS）。
  - **web 消费端（M6-1b）**：`web/` 手写 fetch+ReadableStream SSE 客户端（Bearer）+ `?after` 重连 + 轮询兜底 + `GET /v1/roles` 角色实时。`node web/test.js` = **66/66**（本机实跑取证）。
  - **iOS 壳（M6-2）**：`ios/` SwiftUI 工程 VoiceSign.xcodeproj（VSLogic/SSEParser 等），**BUILD SUCCEEDED、逻辑断言 61/61**——来源=iOS shard 交付报告（本环境无法跑 Swift，按报告引用；模拟器沙箱受限已如实标注）。
  - roles 特性（M5）：`server.TestRolesEndpoint`/`TestTaskStatusHasRole` 现均 PASS，server 全包已转绿。
- **iOS 验证缺口（如实记录）**：iOS 逻辑断言 61/61 来自 iOS shard 报告，本 Mac 环境未自行编译/跑模拟器；端到端真机/模拟器联网联调仍待后续。
- **M7 标点后处理兜底（备路径，未接线）**：`input.Punctuate(text string) string`——句末规则补标点（陈述→。/疑问词命中→？）、已标点不动（幂等）、纯代码/数字/英文/URL 不加；LLM 精修经 `input.LMRefiner` 接口预留（未配 key 恒 nil=纯规则，`input.SetLMRefiner` 注入）。**不接线 pipeline**：iOS 侧 addsPunctuation 主路径验证结果出来后，由组织者决定是否在 Clean→Correct→Classify 之间插入；启用条件=iOS 实测系统开关/locale 影响识别标点。验证器 `input.TestPunctuateSentenceEnd`/`TestPunctuateQuestion`/`TestPunctuateIdempotent`/`TestPunctuateNoPunctuationForCode`（均 PASS）。

---

## 7. 需求库 · 外部研究吸收登记（Instinct 研究 2026-10-02）

> 来源：Instinct 设计思路研究 × VoxSign Harness 对照（飞书 docx `VrObdVey9oHhXHx9wWcchtBDnEh`，2026-10-02）。用户已拍板吸收/删除。**对应关系 ≠ 已验证防护有效性**——有效性仍由 VoxSign 自身任务集与对照测试证明。

### 7.1 六项伪需求删除清单（已拍板，不再立项）
| # | 伪需求 | 删除原因 |
|---|---|---|
| D1 | 主动回呼提醒 | 无实际场景（个人语音工具，不需要 Agent 主动外呼） |
| D2 | 邮件摘要回执 | 当前阶段伪需求（回执走四行 + SSE，不做邮件通道） |
| D3 | 短信/WhatsApp 桥接器预留 | YAGNI——交互面建在自有 iOS/web 壳，不提前建通道适配层 |
| D4 | 按域速率护栏 | 本地契约工具无 Resy 式封号场景；失控循环由最大步骤数+超时+执行预算覆盖 |
| D5 | 每日摘要抽 3 条人工复核 | 不做抽样复核；仅失败/低置信/高风险任务进待复核列表（见 #52） |
| D6 | 每周 10 任务 / 50% 命中率硬门槛 | 降级为低成本指标记录，**不作验收门槛**（自举临界点由频率数据自然判断） |

来源标注：以上 D1–D6 均出自「Instinct 研究 2026-10-02」对照结论。

### 7.2 五项可吸收条目状态表
| # | 条目 | 当前状态 | 证据 / 落点 |
|---|---|---|---|
| A | 风险三信号（可逆硬规则/影响面机械信号/置信软信号），且**置信度低/未知按高风险、不得作为放行依据** | **三信号已实现；"置信低按高风险不放行"待补强（P1）** | 三信号=`risk/risk.go`（`risk.TestEvaluate_*`）；但现状「可逆+小+低置信」仍 auto 执行（置信仅做高亮软信号），未升级为高风险——**待补强 P1** |
| B | verify 读实际状态不读执行器自报 | **已实现** | `verify.TestRun_DoesNotTrustExecutorClaim`、`TestRun_Unverifiable`（M2，fail/unverifiable→禁 commit/deploy） |
| C | 打断三件事分离：「停止执行 / 撤销后续访问 / 删除已读数据」分三条命令 | **interrupt 三语义已有；分命令分离待落地（P2）** | SSE `interrupt` 事件带 applied/notApplied/canRollback 三语义（`server.TestSSEInterruptImmediacy`）；但三件事尚未拆成独立端点/动作——**待落地 P2** |
| D | 强确认顺序：展示 diff/影响分析 → 用户确认 → 执行（顺序不可颠倒） | **当前为文本确认，diff 展示未实现（升级点 P2）** | COMMIT 确认为文本确认（`pipeline.TestGitCommitInProjectRoot`）；diff/影响分析预览未做——**升级点 P2** |
| E | 低风险静默执行（QUERY/NOTE auto 级） | **已吸收** | QUERY/NOTE 可逆小步走 auto；真机证据 task-1790960674771695000 = done |

### 7.3 一致性自检
- 本节为**外部研究吸收登记**，不新增 §1 缺口行、不改 54 条裁定统计；A/C/D 三项标 P1/P2 升级点，属已登记待办，**不产生新的「未完成」误标**。
- A（置信低按高风险）补强时应同步修订 §1#21「冲突取较低者」与 `risk.Evaluate`，并补 `risk` 验证器；C/D 落地时分别挂 SSE/强确认章节。

---

## 8. M7 模型接入落地记录（2026-10-02）

> 本节登记 M7 真机模型中心接入的修复与运维机制；**不新增 §1 缺口行、不改 54 条裁定统计**。

### 8.1 provider Endpoint 修复
- `config.go` 三处 `model.peterzou.com` → **`https://model.peterzou.com/v1`**。
- 根因：原 URL 无 `/v1` 路径，normalize 后拼 `/chat/completions` → 网关 404；new-api 实际挂 `/v1/chat/completions`。
- `deepseek` / `api.openai.com` / `gemini` 路径**不变**（官方兼容路径本就不同）。

### 8.2 pipeline 两处输出修复
- **意图分类 LLM 回退**（`pipeline.llmIntentFallback`）：规则低置信/UNKNOWN + 问句特征 → fast(gpt-4o-mini) 补分类；**覆盖成功后必须清 Ask 残留**——否则 `NeedsClarification` 仍触发回问（真机「答非所问」根因）。
- **QUERY 回答**（`pipeline.queryLLMAnswer`）：search 后调 fast 生成中文回答；**JSON 壳解出**（`json_object` response_format，键不稳定，`text/response/content/answer` 多键兼容）；失败降级友好文案（预算用尽提示），不再输出空壳「OK（自动执行）」。
- **四行回执展示**（`pipeline.renderView`）：工具 stdout 有实质纯文本（QUERY 回答/降级文案）时，Result 行展示该回答（截断 120 字），不再只显示「OK（自动执行）」。

### 8.3 模型中心日预算机制（运维侧，服务器 `/opt/modelcenter`）
- `custom.json` `cost_guard.daily_budget_usd`：**5 → 20**（2026-10-02）+ `mode=block`。
- `cost-state.json` 由 `cost_guard.sh` 从 new-api logs（type=2，quota $1 = 500000）重算。
- 网关 PM2 `modelcenter-gateway` 按 cost-state blocked 拦截（`daily_budget_exceeded`）。
- 预算恢复验证：`curl POST /v1/chat/completions`（gpt-4o-mini）返回正常 completion。

### 8.4 验证器现状（如实）
- 本次修复**未新增 Go 测试函数**（改动为 config 常量 + pipeline 两函数输出修复）。验证靠：
  1. `provider.Chat` 直调返回 `{"intent":"QUERY","confidence":0.85}`；
  2. 实连任务：复杂问句 → done + 真实回答、NOTE 基线不回归；
  3. 全量 `go test ./...` 绿。
- **待补（可选，后续）**：为 `renderView` 展示 / JSON 解壳补 pipeline 包测试函数（函数已存在）——本轮回正只动文档，未加。

### 8.5 性能基线
- M7 复跑 `BenchmarkTaskClassify`：**中位 5.7µs**（样本 8122/5707/5465 ns），仍 < 9.9µs 红线（§1#53），接线 LLM 回退后本地纯分类段未劣化（LLM 为网络旁路，不计 NetMs）。

### 8.6 外部模型诊断修复（Codex/gpt-6-luna，2026-10-02）
> 背景：用户要求改用外部优秀模型诊断（不再本体系自诊断）。通道已打通：**`POST https://model.peterzou.com/v1/codex`**（gpt-6-luna，订阅 CLI 通道，body=`{"prompt":...}`；**不是** `/v1/codex/chat/completions`——后者 Invalid URL）。

**真机复现（22:04）**：同一句「我现在测试一下…看看效果怎么样…」规则分类器误判 **NOTE 0.85**（口语长问句），原 LLM fallback 触发条件（仅 UNKNOWN/低置信）不生效 → refer(NOTE)=true → 「这个」触发 Ask → need_ask。两层根因：a) 规则对口语长问句误判高置信；b) `refer.ResolveOptions` 不区分意图，QUERY 高置信口语「这个」也命中 lineTriggers、候选空 → 写 Ask「你说的『这个』指的是哪个？」。

**外部模型诊断结论（Codex/gpt-6-luna）**：pipeline 调 refer 前按**意图门控**（勿改 `NeedsClarification`——会吞掉其他合法 Ask）。

**落地（pipeline.go，先取证后落笔）**：
- `shouldResolveRefer(it)`：口语问句特征（?？吗呢怎么如何为什么哪）→ **false**（「这个/那个」是口语代词，非指代歧义）；NOTE/EDIT/COMMIT → true；QUERY → 仅 Confidence<0.8 才进 refer；其他 → true。伪代码块在函数上方。
- `llmIntentFallback` 触发扩展：含问句特征且规则未判 QUERY（UNKNOWN/低置信/误判其他）→ **一律调 LLM 复查**（本例：规则 NOTE 0.85 +「效果怎么样」→ 复查判 QUERY 0.85）。
- `queryLLMAnswer` 加固：解壳键补 `message`（模型曾输出 `{"intent":"NOTE","confidence":0.8,"message":...}`）；prompt 强化「只输出回答文本，禁止 JSON/分类/结构化」；解壳后仍是 JSON 壳 → 降级文案（不把 JSON 透传给用户）。

**验证器（均实测 PASS，`go test ./pipeline -run TestShouldResolveReferGate` 等）**：
- `pipeline.TestShouldResolveReferGate`（8 用例门控矩阵）
- `pipeline.TestColloquialQuestionNoReferAsk`（22:04 原句在无 LLM 环境下不 Ask）
- `pipeline.TestQueryHighConfidenceSkipsReferAsk`（QUERY 0.85 +「这个」不 Ask）

**实连证据**：22:04 原句 → **done**、intent=QUERY（fallback 复查）、ask=None、四行回执 Result 行显示真实回答（「我现在测试一下…重点是把能力…」）。

**Codex 通道运维备忘**：POST `/v1/codex`（gpt-6-luna）单并发 FIFO、排队 >60s→429 busy、执行 >240s→502、按字符计费；gpt-6-luna 按量端点 400（**仅 /v1/codex 可用**）。后续诊断/改码可选此通道。
