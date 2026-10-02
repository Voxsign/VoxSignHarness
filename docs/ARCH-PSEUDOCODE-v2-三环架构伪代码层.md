# VoxSign Harness 架构 v2 · 三环控制系统（伪代码层）

- 版本：arch-v2-pseudo · 2026-10-02 · 供多模型评审
- 北极星：认知闭环周期（record→discuss→control→归因，转一圈的速度）
- 哲学：代码=传动机构；VSL 定义=齿轮规格；模型=引擎；输入=ASR 模糊语音+截图

```go
// ==================== 主循环（用户入口） ====================
func MainLoop(voice string):
    // 1. 无条件先记录（Record 永转，只追加不改写）
    record.Append(voice)
    // 2. 意图解析：模糊→精确（8 类意图 + 纠错 + 指代）
    intent := Intent.Parse(voice)
    // 3. 路由：执行 or 讨论 or 回问
    if intent.confidence >= 0.7 and intent.action in EXEC_ACTIONS:
        Control.Run(intent)              // 快环
    elif intent.action == "ask" or intent.confidence < 0.7:
        Discuss.Run(intent)              // 慢环：聊成定义
    else:
        Receipt.Return("低置信回问", options)   // 不猜最像的

// ==================== Control 环（快环：意图→结果） ====================
func Control.Run(intent):
    // 1. 指代消解（词典 + 上下文 + 截图第四路信号）
    target := Refer.Resolve(intent)          // 跨域歧义 → 升级 Discuss
    // 2. 域选择 + 边界检查（有效权限=平台∩域∩契约∩本次授权，默认拒绝）
    space := Space.Select(intent)            // manifest 漂移检测
    if !Space.Check(space, target, action):
        return Receipt.Return("BOUNDARY_VIOLATION", intent)  // 永不就地放行
    // 3. 风险分级（可逆 × 影响面 × 置信度，三信号）
    risk := Risk.Grade(intent, target, action)
    if risk.irreversible:
        if !Human.Confirm(risk): return      // 不可逆永远人工，不可被学习掉
    // 4. 执行（模型工具循环 or 外部编码代理 Codex/Claude CLI）
    result := Execute(action, target)        // 契约白名单内
    // 5. 独立校验（读实际状态 diff 预期，不读自报）
    ok := Verify.Check(expected, actual)
    // 6. 回执（一屏 4 行：动作/文件/结果/撤销）
    Receipt.Return(4lines)
    // 7. 归因回写（模型错 vs 执行错 → 认知回写，闭环最后一环）
    Attribute(ok, result, intent)

// ==================== Discuss 环（慢环：不确定性→定义） ====================
func Discuss.Run(input):
    // 1. 定位不确定性类型（低置信/跨域歧义/执行失败/归因矛盾）
    kind := Classify(input)
    // 2. 提问协议（只问边界案例和代价，每题带默认值，只问会改变动作的问题）
    questions := AskProtocol.Generate(kind)
    answer := Human.Answer(questions)
    // 3. 共识固化：只改定义层（词典/域/契约/判断语义），不改代码
    VSL.Harden(kind, answer)
    // 4. 若为任务升级 → 回到 Control 重跑（快环受益于新定义）

// ==================== Record 环（永转：碎片→素材） ====================
func Record.Run():
    loop:
        voice := ASR.Listen()               // 语音（M2 收文本，ASR 归外部）
        record.Append(voice)                // 只追加，原汁原味
        Trajectory.Log(voice, intent_hint)  // 轨迹 JSONL append-only
        if Quota.Exceeded(): Compress(oldest)   // 压缩不改写

// ==================== 异常升级矩阵（不就地重试） ====================
// 工具失败/测试红  → 就地重试 1 次（上限）→ 升级 Discuss
// BOUNDARY_VIOLATION → 永不放行 → 升级人工/讨论
// 意图低置信       → 显式回问（候选集收窄到域内）
// 任务中断/崩溃    → 回轨迹恢复点，不重复外发
// 归因矛盾        → 回 Discuss（模型错 or 执行错 or 定义错）

// ==================== 数据流（谁是输入谁是输出） ====================
// 素材库(record) ──检索/证据──► Discuss
// VSL 定义（域/意图/契约/判断/伪代码）──执行依据──► Control
// 轨迹(trajectory) ──► 归因 ──► 认知回写（词典/策略/后续执行）
// 四元组缓存(意图/域/权限/指代) ──► 快路径加速（版本漂移即失效）

// ==================== 与现有 harness 的结构差异 ====================
// Codex/DeepSeek：单循环（prompt→LLM→工具→反馈），北极星=任务完成率
// VoxSign：三环（Record 永转 / Control 快环 / Discuss 慢环），北极星=认知闭环周期
// 差异点：显式定义层(VSL) / 三域认知记忆 / 输入容错(ASR) / 强制归因回写
// 模型角色：引擎（各环共用）；价值不在模型本身，在环的编排与定义层
```

## 评审要点（请模型逐条回答）

1. **可行性**：三环能否落地为 Go 单二进制？哪环最难？
2. **价值**：北极星=认知闭环周期，价值是否真实（vs 单任务完成率）？
3. **先进性**：相对 Codex/DeepSeek harness 是否先进？还是过度设计（Claude 曾质疑"伪需求式过度设计"）？
4. **持续性（最尖锐）**：未来 LLM 越来越强，这套的价值会被侵蚀吗？哪些部分会被模型能力替代，哪些不会？
5. **漏洞与改进**：架构最脆弱处、执行链缺陷、建议精简处。
