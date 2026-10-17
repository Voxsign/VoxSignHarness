# 分叉线 B 盘点表（阶段 0′ 交付物）

> 口径：`7cc735f..dd697db`，共 **140** 个提交 = 135 个普通提交 + 5 个合并提交。来源是私有仓库 `ui-v3-doubao` 线（2026-10-05）。dd697db 与公开 main 互不包含：前者独有 140 个，v0.8.1 独有 187 个。
>
> 原则（融合规划第二节）：以公开 main（v0.8.1）为唯一主干，有选择地移植，不整线合并。三类：**移植**（价值高、与 v0.8.1 不冲突）/ **重做**（思路对，要在 v0.8.1 上重写）/ **丢弃**（已被 v0.8.1 吸收，或不在范围内）。
>
> 本表只做归类，不改代码。提交标题里的个人或客户标识已替换为 `[redacted]`。

## 汇总

| 去向 | 提交数 |
|---|---|
| 移植 | 42 |
| 重做 | 60 |
| 丢弃 | 33 |
| 合并提交（不单独归类） | 5 |
| **合计** | **140** |

## 按主题

| 主题 | 提交数 | 去向 | 说明 |
|---|---|---|---|
| iOS 豆包式 UI / 设置 / 登录 | 30 | **移植** | App 仓库；阶段 2。核心是 `1a24478` 2.6(9) 对齐 + `afd72c9` 改名 + `5f54837`/`7981696` 修复；签名/team/bundle 配置类提交按 App 仓库现状处理，不照搬 |
| SSE / EventBus / 观察 | 4 | **移植** | 阶段 2 核心：SSE 细粒度桥 + 意图分类发射点；按 v0.8.1 事件契约逐个 PR |
| 设备 token / 机器码 / 中继 / 心跳 | 8 | **移植** | 阶段 2 设备令牌 → 阶段 4 配对/APNs 前置；`cmd/vhs-relay` 按 v0.8.1 重写接口；含 `d9fa60e`（本地模式设备端点 501 不 panic） |
| 语音适配层 vhs-voice / ASR | 32 | **重做** | 思路在 v0.8.1 上重写；ASR 静音裁剪、超时、健壮性四件套等单点可拆小 PR 移植 |
| 实现/证据门/验证器 — 语音适配层配套 | 22 | **重做** | 与语音适配层绑定一起重写 |
| Runner / 调度 / 串行 / 控制面 | 6 | **重做** | C1 Runner / C2 调度器 / C0 串行 → 阶段 3「每用户 Runner」 |
| 轨迹 / request_id / 日志 | 9 | **丢弃（已被 v0.8.1 吸收）** | v0.8.1 已含 RequestID 透传、safeGo、failed 事件；request_id 断链修复阶段 2 前再逐条对照一次 |
| 域名迁移 / 配置 / 模型中心 | 8 | **丢弃（已被 v0.8.1 吸收）** | v0.8.1 已切新域名；剩余写死地址的配置化归阶段 3 |
| zhiji 自模型架构 | 7 | **丢弃（建议，待 CEO 确认）** | 超出 Issue #30 / v0.9 范围；如要做，单独立项 |
| 远程控制 / 自迭代 / selfheal | 4 | **丢弃（建议，待 CEO 确认）** | 远程控制执行器不在 v0.9 主线；安全面需单独评审 |
| 契约 / 文档 / 架构 / 其他 | 5 | **丢弃（仅作参考）** | 设计稿留在私有仓库作参考，不进公开 main |
| 合并提交 | 5 | — | 不单独移植 |

## 需要 CEO 拍板

1. zhiji 自模型架构（7 个）和远程控制 / selfheal（4 个）：建议 v0.9 不做，是否同意？
2. iOS 移植的落点：App 仓库按 2.6(9) 整体对齐，还是逐项 PR？建议逐项 PR，先移植 `1a24478`。

## 明细（按主题，提交按时间先后排列）

### iOS 豆包式 UI / 设置 / 登录（30，移植）

- `7cca9e0` feat(ios): T3 豆包式全面改造 + Harness 连调
- `a896f03` feat(ios): v2.1 豆包式交互全面落地（18条ADR+先进理念），与 vhs-live e1fa975 同步
- `b1bc92f` fix(intent): 抱怨/元反馈话术归ASK/INFO不再回问（用户三轮截图点名）；fix(degrade): 模型失败人话化带语气词；fix(ios): v2.3 崩溃双保险+去拦截+去已完成前缀；docs: 用…
- `1fa8016` ios-receipt: 截图图片回执闭环 + 豆包视觉（后台侧）
- `0e21752` docs: 同步云端谷歌登录与发布架构 HLD
- `454c6d3` P0 云端模式（谷歌登录+租户+三档配额）同步 + 门禁收敛
- `ace4512` chore(ios): bump VoiceSign to 2.1 (build 2)
- `4483a00` ios: switch automatic signing team to [redacted] ([redacted]) for on-device builds
- `7c8220f` ios: 谷歌登录（云端模式 P1）——PKCE+ASWebAuthenticationSession 授权、换 JWT、登录态持久化；预置本机本地(186:8897)/云端模拟(186:8898)服务器地址
- `e1d2af3` ios: 修复 Google 登录回调选择——仅 https://voxsign.ai 正式域名走正式回调，局域网/本地模拟一律走 http://127.0.0.1（与服务端 VHS_GOOGLE_REDIRECT_UR…
- `2d16020` ios: Google 登录弹窗健壮性——start() 失败即抛错（不再静默挂起）、presentationAnchor 优先 keyWindow、登录失败弹 alert 显示原因
- `b01bdc5` ios: Google 登录改用 iOS 类型 OAuth Client——自定义 scheme 回调（com.googleusercontent.apps.*）真机 100% 截获；PKCE 换 id_token 后云…
- `4291bbf` ios: 修复 Google 回调 scheme——去掉 .apps.googleusercontent.com 后缀（Console 注册的是不含后缀的反向域名 scheme，带后缀报 redirect_uri_mis…
- `5c83edd` ios: 设置 UI 重设计 v3——云道（默认，仅 Google 登录，零配置）/ 自建（默认空列表）双模式；自建添加服务器支持 IP 地址（内网检测+云端转发开关+保存必检连通）与机器码（云道定位→同网直连/异网转发…
- `67330b1` server: 云端验签支持多受信 Google client（VHS_GOOGLE_CLIENT_IDS 逗号分隔）——iOS 类型 client 签发的 id_token aud 为 iOS client ID，原先…
- `e70c7c3` 云道机器码真实闭环：server 新增 /v1/devices/{register,heartbeat,lookup}（设备注册表 devices.json 持久化，register/heartbeat 需 VHS_TO…
- `f6c566d` docs(ui): 入仓 VoxSign 设置 UI 设计方案（云道/自建模式定稿 v1，来自本 session 设计交付）；.gitignore 忽略 Xcode xcuserdata 本机用户态
- `eb7cb9e` ios(ui v3): 99% 豆包式改造落地——顶栏纯黑标题+HarnessDot 状态点+线性齿轮+毛玻璃；空态单行灰字（删启动欢迎语）；输入条 5 态浅色语音状态条+中间按住说话胶囊+上滑-80pt 可逆取消；决策…
- `bd27e0a` ios: 更换 bundle ID 为 ai.voxsign.ios 并固化手动签名配置(VoxSign Dev profile)
- `234b7ef` iOS: 云道地址切换正式域名 https://voxsign.ai（云端 harness 已部署）
- `014f304` fix(server): statusRecorder 实现 http.Flusher，修复 SSE events 500 - logRequests 包装的 statusRecorder 只内嵌 ResponseWri…
- `1168f84` feat(ios): 按住录音实时振幅声波条（豆包式按住有反应）+ 版本 2.3 (build 6)
- `8a6433b` feat(ios): 机器码输入框粘贴净化——从粘贴文本自动提取 XXXX-XXXX-XXXX 机器码（忽略大小写统一大写），丢弃其余文字；版本 2.3(6)→2.4(7)
- `1e5ddb6` feat(ios): 云道转发异网回连——viaRelay 保存/探测转发 URL https://voxsign.ai/relay/<机器码>；版本 2.4(7)→2.5(8)
- `7981696` fix(ios): 解绑/删除服务器修复——swipeActions 常显删除(可删到0台，删空回落云道模式)；新增机器码解绑入口(清除machineCode保留条目，可重新绑定)；删除/解绑后 Connectivity…
- `1a24478` feat(ios): VoxSign 2.6(9) UI 豆包式对齐——顶栏5pt红蓝状态点+会话入口+…菜单、输入条三件套(+添加资料/按住说话红条松开发送)、AI消息轻量信息行+…菜单(去🔔🔊)、回执卡豆包式圆角白卡…
- `afd72c9` refactor(ios): 工程命名 VoiceSign→VoxSign 全面统一 + 消耗/用量显示去除服务端回传依赖
- `5f54837` fix(ios): 录音双宿主卡死修复加固——holdInitiated 防重入 + 红条手势宿主收尾
- `374fae2` test(ios): UI 验证用例——UIVerify2/3/V2/20261005
- `7953286` docs(ios): 服务端回传依赖勘察清单 + iOS UI 豆包式对齐测试报告

### SSE / EventBus / 观察（4，移植）

- `b697fa2` feat(observe): S1 EventBus（进程内，Publish 绝不阻塞；尽力/可靠双档订阅；Replay/Snapshot/ExportStream）
- `5be64d0` feat(observe,main): S2 CLI 滚动进度渲染器（-q/--quiet 退回一次性四行回执）
- `73ae8ab` feat(server,pipeline): S3 SSE 细粒度桥 + 意图分类发射点（最小闭环端到端）
- `7328936` feat(observe,main): S2 REPL 补接 -q/滚动渲染（cmdRepl）

### 设备 token / 机器码 / 中继 / 心跳（8，移植）

- `9f0851e` fix(modelcenter): 同步性价比切换后的档位断言（8fc59ea 遗留）
- `0c06184` fix(intent): 快速理解需求——三处限制误伤大修（用户实测"老是越界/卡住"）
- `b5c4470` remote: 能力询问命中执行器（能控制后台的电脑吗→真实桌面证明）+询问引导
- `60532fe` fix(refer): extractRecentEntities 正则支持混合大小写专名（SPoG）——9e4aae7 实现与测试不一致修复
- `ba41c1a` vhs(orchestrate): 生成《VoiceSign-Harness-全景开发文档》多步编排落地
- `0a19fc3` fix: 语音意图不明直接回复不挂起 + 熔断放宽 + iOS 录音 16k 修复 + 版本号显示
- `94a421e` chore: .gitignore 覆盖本地产物——bin/（vhs-relay agent 二进制）、*.ipa（真机包）、ios/build/
- `d9fa60e` fix(server): 设备注册表仅云端模式创建——本地模式设备端点按 devices_security 契约 501（不 panic），业务端点 s.devices==nil 时跳过设备 token 放行（机器码直连…

### 语音适配层 vhs-voice / ASR（32，重做）

- `a7c3f86` voice: 确认自动放行+写意图默认落project+时间查询触发词
- `ee59346` asr: /v1/asr 校准通道（iOS录音→平台千问ASR）
- `42c4b6e` asr-v2: 个性化热词网关（ASR 服务器层解决个性化，iOS 纯录音零本地识别）
- `e1ddefa` sync: iOS 最新（UI 精修/图标/纯录音 ASR）+ 契约 v2 + 验证器 R14 v3 全量同步
- `cc2a520` docs(req): VHS 语音适配层需求规格 v1——口语去噪/复合指令拆单/域补全/任务编排/语音友好汇总（12 判据，手机端语音入口）
- `1f8adf6` fix(adapt): 适配 DeepSeek 454c6d3 provider 重构——ChatWithFallback→chatWithFallback 降级链(模型调度保留)/恢复 RoundEvidence·Do…
- `6cf6dcb` feat: vhs-voice 语音适配层 v1——口语去噪/复合指令拆单/对象补全/上游编排/会话审计（12 判据真跑 14/14 绿）
- `56fc9ab` vhs(implement): 语音适配层 实现（证据门收敛）
- `b563a50` vhs(implement): 语音适配层 实现（证据门收敛）
- `395a8a2` fix(evidence): 证据门需求驱动——①endpointRefsOf/HandlersOf 正则多级路径(/v1/voice/health 不再截成 /v1/voice)；②移除 ASR 硬编码 P0 判据(词…
- `9a7cb3e` vhs(implement): 语音适配层 实现（证据门收敛）
- `44faecc` vhs(implement): 语音适配层 实现（证据门收敛）
- `5490a9b` vhs(implement): 语音适配层 实现（证据门收敛）
- `727cda1` vhs(implement): 语音适配层 实现（证据门收敛）
- `7bc5fd8` vhs(implement): 语音适配层 实现（证据门收敛）
- `317f183` vhs(implement): 语音适配层 实现（证据门收敛）
- `12f94ab` vhs(implement): 语音适配层 实现（证据门收敛）
- `4bedf78` feat(voice): 语音场景增强完成——写文件真实执行(R6/R7)/天气能力(R22)/口语注册(R11)/COMMIT 默认 project 域；摘要 task_metrics kind 登记+Space 补齐…
- `9d16363` feat(voice): 语音场景增强（3way 合入远程版）——写文件真实执行/天气能力/口语注册/COMMIT 默认 project 域；task_metrics kind 登记+Space 补齐；恢复 Summar…
- `3bc6eac` vhs(implement): 语音适配层 实现（证据门收敛）
- `1ca73a7` vhs(implement): 语音适配层 实现（证据门收敛）
- `b3e1046` vhs(implement): 语音适配层 实现（证据门收敛）
- `67a0b7d` vhs(implement): 语音适配层 实现（证据门收敛）
- `6c13113` vhs(implement): 语音适配层 实现（证据门收敛）
- `7e7eef9` vhs(implement): 语音适配层 实现（证据门收敛）
- `73229b4` vhs(implement): 语音适配层 实现（证据门收敛）
- `ef3d99a` 平台域名迁移 aiops.voxsign.ai（[redacted].com SNI/证书与服务器不符导致 ASR 502 无声音）+ /v1/asr 空文本视为合法结果不再 502；vhs-asr 服务发现默认同步切 …
- `5ac08e3` feat(server): 健壮性四件套（架构 v1 §7）——分级超时/熔断/指数退避重试/bulkhead 有界并发；ASR 平台调用走健壮通道（非幂等不重试仅熔断）；任务总 deadline 10min 硬上限（c…
- `0afcd33` perf(asr): ASR 超时 20s→35s（平台推理偶发 30s 边缘返回不再失败）+ iOS 上传超时 45s 对齐 + build 5
- `7cbece1` perf(asr): 服务端裁剪 WAV 首尾静音（trimWAVSilence+单测）
- `e08e147` fix(server): P0-2 safeGo接线 / P0-3 ASR端口统一8787 / P0-4a三类日志rid / P0-4b rid透传+session桥接
- `b164906` test(e2e,archstub): ASR 架构断言可执行化（D1-D7，研究报告 C.2）

### 实现/证据门/验证器 — 语音适配层配套（22，重做）

- `757fc50` verify: 自动验证器 vhs-validate.py（validate-align R 通道 11 判据）+写意图真实执行
- `8714bcd` fix(vhs): 无人工一次成功 3 洞修复——①证据门判据5 需求端点↔产物HandleFunc差集（复现2次盲区：需求7端点产物2端点仍判P0齐全→现检出缺口5）；②修订/补齐/完善类表述命中implement（带…
- `eda5b77` test(fix): endpoint_gate_verify_test 产物改内联 fixture（2/7）——不随工作区产物演进而失效
- `5460959` fix(dispatch): 洞2 分发级修复——ClassifyTask 显式接入 detectImplementOrchestrate（kind=implement）+ 多动作豁免；否则实现类长任务被 commitT…
- `92d4988` fix(implement): 断链接线——execOrchestrate 接 kind=implement→execImplement(LLM生成+证据门≤5轮收敛+git提交)；server 补 document 通…
- `3607901` fix(provider): normalizeEndpoint 兼容完整 chat 端点——/api/model/chat 不再拼 /chat/completions（真跑 404 no such endpoint，c…
- `08b60c5` fix(implement): execImplement 成功判定——files==nil 才是失败（note 是'编译全绿'信息非错误）；产物已写盘仍判 FAILED 的假失败修复
- `567875f` fix(evidence): 判据5 路径参数归一化——{conversation_id} 不参与匹配，LLM 尾斜杠写法不再死循环
- `c665347` fix(implement): 编译修复轮先用本轮错误确定性清理 import（此前用上轮错误清理本轮代码→错位→3轮耗死）
- `eb3143e` fix(implement): 确定性修复循环——未使用变量按编译行号删除 + import 清理逐轮重编译，LLM 全量重写 3 轮不收敛改为机械兜底
- `1884228` fix(evidence): 判据6 无桩实现——抓 StatusNotImplemented(501)/Placeholder/TODO 占位（真跑实证 resolve/run/tasks 全 501 桩，判据5 端点…
- `e75bead` fix(implement): 桩缺口具体化（提取需求 E1-E6 语义注入）+ sysMain 禁止 501/Placeholder 强化（LLM 泛化缺口下 5 轮仍写桩）
- `3976842` fix(evidence): 判据7 行为级真跑——起产物服务打标准输入断言 E2 parse actions 数组 / E3 decompose tasks 数组（抓软空壳：无501但业务空）
- `34e8594` fix(evidence): 判据7 加严——E2/E3 非空数组断言（tasks:[] 空壳溜过）+ E4 unresolved 状态 + E5 上游投递引用
- `737a9ec` fix(implement): 修订轮注入当前产物代码（防全量重写修A坏B）+ 轮次5→8；编译修复轮同样附当前代码
- `879b5c7` fix(implement): deterministicClean 增删 //Placeholder 占位注释（LLM 看不到中部代码 8 轮修不掉，机械兜底）
- `f7eea87` fix(implement): 编译迭代耗尽返回最后一次产物（编译缺口由证据门驱动修订轮，8轮不再因编译失败早停）+ 编译修复轮3→5
- `d59d396` fix(implement): 修订轮注入改首尾拼接（E4/E5 handler 可见）+ 缺口附实现要点伪代码（unresolved 状态机/上游投递语义）
- `408c63c` fix(implement): 编译修复轮改首尾注入（错误行234的E5类型错误可见——此前仅开头2500看不到，5轮修不掉8轮重投仍失败）
- `566a416` fix(implement): E4/E5 缺口附 Go 行为锚点（unresolved 状态机返回片段/上游投递+task_id收集模式）——8轮耗死实证：LLM 生成时忽略伪代码，给可直接参照的 Go 片段
- `6d44be3` fix(implement): E4/E5 缺口附 Go 行为锚点（unresolved 状态机返回片段/上游投递+task_id收集模式）——8轮耗死实证：LLM 生成时忽略伪代码，给可直接参照的 Go 片段
- `bd164bb` fix: 云端VHS_TOKEN硬保护与本地模式设备接口501 + zhiji SelfItem.Status 补齐（收尾20261005三处遗留）

### Runner / 调度 / 串行 / 控制面（6，重做）

- `d2adac6` docs: VoxSign 架构设计 v1——控制面/数据面分层、自适应心跳、打断/插队/撤回、健壮性四件套、演进路线与 ADR
- `9a2acf4` feat(server): 控制面命令集（架构 v1 §6）——POST /v1/interrupt 全局急停/指定打断（直接 cancel 活动任务、取消传播掐断远端、force 预留清 pending）；POST /…
- `8eb8d5d` feat(heartbeat): 自适应心跳 4 态（架构 v1 §5）——Harness 状态机 idle/busy/decision/standby（频率 2min/15s/5s/5min，idle 持续 5min …
- `f83a8eb` fix(server): C0 受控串行——共享串行闸修复 Options 浅拷贝失效
- `eb685c1` feat(server): C1 Runner 抽象（进程内每任务一 goroutine 执行体）
- `6ec2730` feat(server,config): C2 in-process 调度器（VHS_USE_SCHEDULER 灰度默认 false）

### 轨迹 / request_id / 日志（9，丢弃（已被 v0.8.1 吸收））

- `dd6ff76` pipeline: 补齐 early-return 出口的 final 轨迹写入
- `3232fd0` feat(trajectory): P0-1 登记 13 阶段 6 类 kind（refer/space_check/risk/confirm/verify/attribution）
- `26e09eb` feat(server): P0-2 safeGo 统一后台 goroutine recover
- `98817fd` feat(selfheal): P0-4a Trace 增加 RequestID（本地），诊断日志带 rid
- `91dd430` fix(pipeline): P0-1写入点用常量+write改warning / P0-2 confirm recover / P0-4a&4b request_id
- `996f93d` docs: P0-5 ARCHITECTURE v1重写+根README / P0-6 新建签署请求
- `f2a8a93` fix(server): 修复 ts.Outcome/Status/Role 数据竞争（writeTaskView 无锁读 vs runPipeline 持锁写），go test -race ./server/ 绿
- `42fbd03` feat(pipeline/server): 焊死 request_id 断链点（Run回写o.RequestID/repairFailed/llmSummarize/handleRun/handleVoice）+ iO…
- `190eb18` fix(server): onTaskPanic rid 与 pipeline request_id 同源（ts.RequestID，空则 ts.ID 兜底）

### 域名迁移 / 配置 / 模型中心（8，丢弃（已被 v0.8.1 吸收））

- `5d6fa6e` docs: 同步 VoiceSign.ai 品牌域名更新
- `f251494` fix(impl): 洞4——LLM 实现 prompt 注入需求端点清单(sortedEndpoints)+文档摘要(docBrief)，替代裸 implCapabilityBrief（换需求即失效→修订死循环）；TL…
- `b6c7226` fix(tls): 网关 TLS 证书验证失败致 LLM 全挂——新增 VHS_INSECURE_TLS=1 逃生口（provider/modelcenter 双通道，默认关闭，仅显式设置时跳过证书验证）
- `7d8e150` chore(domain): 域名统一 VoxSign.ai——模型网关 aiops.[redacted].com → aiops.voxsign.ai（config.go 默认表 + model-center.json…
- `59a9588` fix(json): 修复 model-center.json 损坏（域名替换时结构破坏）——恢复 454c6d3 合法版 + voxsign.ai 域名
- `56b2d79` fix(provider): voxsign 网关鉴权头 X-AIops-Key（read key）替代 Authorization Bearer（真跑 401→200）
- `96295d9` fix(config): 兼容 AIOPS_KEY 环境变量（网关读 Key 旧变量名），providers api_key 不再为空
- `6609708` fix: platformAIOpsBase 支持 VHS_PLATFORM_BASE 覆盖（域名迁移：[redacted].com TLS 证书失效时切 voxsign.ai）

### zhiji 自模型架构（7，丢弃（建议，待 CEO 确认））

- `c3532b3` feat(zhiji): Phase 0 落地——意识缓存四层 + 最小契约 + 反思线程（架构 v1.0 §12/§07/§08/§6.3）
- `55760ac` feat(zhiji): M1 前置组件——per-model 注册表 + 规则路由影子模式 + 开发机一键检查
- `d363de0` feat(zhiji): 压缩器 + 外化客户端 + 主循环接入钩子（Phase 0 完整闭环）
- `7c92997` fix(zhiji): Phase 0 编译/测试全绿——SelfItem 补 Status、UpsertSelf/TouchSTM 设默认 active、Registry.save 锁重入死锁修复
- `cd82e51` feat(zhiji): v1.1 自模型架构在途整理 + 编译/测试修复（graph/rawlog/systemone/budget search）
- `f757d4a` feat(zhiji): v1.2 写路径自动接通向量索引（WriteMemory/TouchSTM 锁内快照后放锁再 Upsert）
- `c71a873` chore(zhiji): 保留在途 edges_rules/probe_auto 增强（P0语义/因果边规则优化，备份于 wip-backup）

### 远程控制 / 自迭代 / selfheal（4，丢弃（建议，待 CEO 确认））

- `09020a8` feat(selfheal): 后台自迭代自更新能力第一段——语音能力自举注册闭环（同步 vhs-live 13470fa）
- `ff01d56` selfheal: 注册能力必须真实执行——「查看天气」接 wttr.in 真实天气
- `ccb9f6a` fix(cleanup): 移除 input 残留坏测试 revise_verbs_verify_test.go——detectImplementOrchestrate 已被远程重构删除，测试残留引用致 build fa…
- `9999dbf` feat(server): 设备 token 成为可用凭证 + 天气直通/计时日志/回问优化

### 契约 / 文档 / 架构 / 其他（5，丢弃（仅作参考））

- `2119fc7` test: F5 断言更新为新多件事文案（几件事）
- `11a051f` remote: 远程控制电脑真实执行器（契约→执行闭环，自迭代第二段）
- `0999f0d` docs: iOS X-Request-Id 发版清单——标注与 session_id/speaker_id 合并为一次发版（C4 阶段，不单独发）
- `0950a0a` feat(server): 机器码异网转发基建——SSE 反向中继 + 机器码路由表 + cmd/vhs-relay 常驻 agent
- `70797ce` docs(voxsign-server): VoxSign Server 后端设计 v1 实测锚定版+任务书（闭源策略/企业端原生/能力基线实测锚定/数据飞轮边界逻辑）

### 合并提交（5）

- `f0d84ba` Merge branch 'main' of https://github.com/smithpeter/voicesign-harness
- `b89d4da` Merge origin/main (7c92997 zhiji Phase-0 修复线) 入本地 main
- `43acbf8` merge: 合入远程 origin/main（zhiji 裁剪 + 机器码转发基建 + 设备token凭证）——server.go 两处冲突合并（rid+wall_ms 计时日志；C1 Runner + 结构化计时日志…
- `999c1f9` Merge remote-tracking branch 'origin/main' into ui-v3-doubao
- `dd697db` Merge remote-tracking branch 'origin/main'
