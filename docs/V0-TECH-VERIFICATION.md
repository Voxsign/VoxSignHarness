# VoxSign Harness V0 技术底座验证报告



* 日期：2026-10-01・环境：macOS 27.0（Apple Silicon arm64）・Go 1.26.0

* 范围：**技术底座验证**（任何架构方案下不变的硬底座）；功能实现（agent 编排 /tools/ 服务 / README）留待新架构研究线定稿后按里程碑推进

* 代码：`voicesign-harness/`（零外部依赖，仅标准库）



***

## 1. 验证结论总表



| 项                   | 结果                                           | 证据                                                               |
| ------------------- | -------------------------------------------- | ---------------------------------------------------------------- |
| go test 全量（含 -race） | ✅ 10 包全绿                                     | `go test -race -count=1 ./...` 全 ok                              |
| go vet / gofmt      | ✅ 无告警 / 无未格式化文件                              | 见构建输出                                                            |
| 单二进制构建              | ✅ 1.6MB（-s -w）                               | `dist/vhs-darwin-arm64`                                          |
| 跨平台交叉编译             | ✅ 5 平台一条命令                                   | `./build.sh`：darwin/linux × arm64/amd64 + windows/amd64，单条命令全出   |
| 启动延迟（version 子命令）   | ✅ 中位数 **16.1ms**（目标 <50ms）                   | 30 次采样 min 15.3 /max 17.1                                        |
| 模型中心连通              | ✅ 网关可达（HTTP 200，元数据返回）                       | 本机 curl [https://model.peterzou.com](https://model.peterzou.com) |
| 模型中心 chat 连通        | ⏳ 需有效 `VHS_API_KEY`（协议已由 httptest 验证）        | 可选集成测试自动跳过                                                       |
| 意图契约 v1 骨架          | ✅ contract.Intent + 6 类确定性分类 + 槽位 + 置信度 + 回问 | input 包测试全绿                                                      |
| 轨迹 JSONL            | ✅ append-only、0600、逐条落盘、并发安全                 | trajectory 包测试全绿                                                 |
| 纠错端到端演示（硬验收）        | ✅ 美墅→Mansour 全链路跑通                           | e2e 测试（见 §4）                                                     |



***

## 2. 模块边界（认识 / 接口 / 执行 / 沉淀 / 契约 / 基座）



```
contract  共享数据契约（Message/ActionPlan/Receipt/Intent/Usage）——独立文件，不依赖任何包
config    五节配置基座（global/providers/routes/input/memory；文件+env）
input     认识层：清洗 → 词典纠错 → 意图JSON(置信度) → 回问
router    认识层：意图+关键词 → (provider, max_turns)
provider  接口层：OpenAI 兼容多端点客户端 + mock（可插拔 Provider 接口）
memory    沉淀层：个人词典（输入纠错与提示词注入双向复用）
trajectory 沉淀层：append-only JSONL（ASR 原文无条件保留，按 request_id 可重放）
tools     执行层：尚未实现（待新架构定稿，V0 演示用 demo shim 证明链路）
agent     编排层：尚未实现（待新架构定稿）
```

依赖方向单向：contract ← config ← {input, router, provider, memory, trajectory}；input 通过 `Correcter` 接口消费词典，不反向依赖 memory。



***

## 3. 性能量级（本机实测，`go test -bench=. -benchtime=150x ./bench/`）



| 基准                 | 结果             | 说明                    |
| ------------------ | -------------- | --------------------- |
| 二进制冷启动             | 中位数 **16.1ms** | 远超 <50ms 目标（单二进制优势兑现） |
| 输入容错管线（含词典纠错 + 意图） | **9.9µs/op**   | 语音场景毫秒级延迟预算中可忽略       |
| 路由解析               | **301ns/op**   | —                     |
| mock provider 调用   | **10ns/op**    | 纯内存                   |
| 轨迹写入               | **5.2µs/op**   | 逐条落盘含 JSON 序列化        |

复现：`./build.sh && python3 bench/startup_bench.py && go test -bench=. ./bench/`



***

## 4. 硬验收：纠错端到端演示（e2e 测试）

链路：**ASR 模糊文本 → 清洗 → 词典纠错 → 意图 JSON → 路由 → provider → 执行 → 回执 → 轨迹落盘**



```
输入(ASR 原文)：  "帮我打开美墅的文件夹看看有什么"
① raw 保留：      轨迹无条件记录原文（原始证据）
② clean：        "打开美墅的文件夹看看有什么"（去填充词"帮我"）
③ correct：      内置词典 美墅→Mansour → "打开Mansour的文件夹看看有什么"
                 corrections:[{from:美墅, to:Mansour, rule:dict}]
④ intent：       FILE_LIST, slots:{path:Mansour}, confidence 0.8（≥阈值 0.6，不回问）
⑤ router：       file 场景 → provider=mock, max_turns=1
⑥ provider：     mock chat 返回合法 ActionPlan（闭环内有模型调用）
⑦ 执行：         演示执行器真实 list_dir（os.ReadDir 于 Mansour 目录）→ 回执 OK
⑧ 轨迹：         input_raw / input_correct / intent / receipts / final 五条 JSONL 落盘
```

断言覆盖：原文保留、纠错记录、意图正确、路由正确、回执成功、轨迹可回溯原文 / 纠错 / 意图 / 回执。

另两个演示：TIME 本地直通（零 LLM、route=local、get\_time 执行）；低置信回问（UNKNOWN → ask 非空，不执行任何动作）。

> 说明：正式 tools 注册表（shell/read_file/…/ 风险分级）与 agent 编排循环属后续里程碑；当前以最小演示执行器（demo shim）证明端到端链路成立。



***

## 5. 连通性状态



| 目标                                                  | 状态                                                                                                     |
| --------------------------------------------------- | ------------------------------------------------------------------------------------------------------ |
| 协议层（OpenAI 兼容客户端）                                   | ✅ httptest 全场景验证（端点规范化 / 鉴权 /json\_object/ 参数透传 / 429 重试 / 401 不重试 / 500 耗尽 / 畸形 JSON/tool\_calls 防御）  |
| mock 闭环                                             | ✅ 无 key 可跑通全链路                                                                                         |
| [model.peterzou.com](https://model.peterzou.com) 网关 | ✅ 可达（HTTP 200，`{"service":"modelcenter-gateway","default_model":"gpt-4o-mini",...}`）                   |
| 真实 chat 调用                                          | ⏳ 设置 `VHS_API_KEY` 后运行 `go test -run TestIntegrationRealEndpoint -v ./provider/...` 自动启用；当前未设置 key 故跳过 |



***

## 6. 已知事项（供新架构整合时参考）



1. **Go 陷阱（已修复并测试固化）**：`encoding/json` 对非空切片做数组解码是 "就地复用已有元素、逐字段覆盖"，会导致文件声明的 provider 继承默认 provider 字段 ——config.Load 已改为 raw-map 按节覆盖（providers/routes 整体替换、global/input/memory 保留默认合并语义）。

2. **INFO/UNKNOWN 语义**：按架构文档 §5.2 对齐 ——INFO 有触发词集（翻译 / 总结 / 摘要 / 问答 / 搜索 / 查一下 / 解释 / 研究，0.4 永不回问交模型）；无任何触发词 → UNKNOWN（0.2 必回问）。

3. **路径槽位抽取**：剥离动词前缀（打开 / 查看 / 读取 / 看看 / 显示 / 读）后再取路径，`打开Mansour的文件夹` → `Mansour`。

4. **词典变体排序约定**：同一条目的变体按长在前排列（如 曼苏尔 在 曼苏 前），否则短变体会抢先切分长变体；`AddTerm` 已提供（source 标记保留），模型回写（dict\_add 工具）待后续。

5. **密钥纪律**：provider 客户端任何路径不记录 API key；轨迹不含密钥；配置文件 0600。

6. **尚未实现（等新架构定稿）**：agent 编排循环、tools 注册表与风险分级、server、main 子命令（run/serve/repl）、README、模型二次纠错（D1 第二层）、会话 / 画像 / 事实（沉淀层其余部分）、dict\_add 工具。



***

## 7. 复现命令



```
cd voicesign-harness
./build.sh                  # vet + test + 五平台交叉编译 → dist/
python3 bench/startup_bench.py   # 启动延迟采样
go test -bench=. ./bench/   # 性能量级
go test -race ./...         # 全量测试
```