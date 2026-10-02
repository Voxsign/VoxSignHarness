# VoxSign · iOS 对话界面模拟壳（web/）

M5-2 交付：纯 HTML/CSS/JS 单页应用，零第三方依赖，仿豆包/Grok 对话心智，蓝白主题，移动端优先（≤430px）。供真机/浏览器点按演示，后续可直接打包 WKWebView。

**M6-1b 升级**：执行卡从"轮询模拟"改为 **SSE 真实流式**（`GET /v1/tasks/{id}/events`，契约 `docs/SSE-v1-事件流契约.md`）；角色条实时接 `GET /v1/roles`；说"停"经 SSE `interrupt` 事件即时渲染红条；SSE 不可用时自动回退轮询。

## 文件清单

| 文件 | 作用 |
|---|---|
| `index.html` | 单页壳：顶部角色折叠条 / 红色系统条 / 设置面板 / 对话流 / 决策点区 / 底部麦克风输入条 |
| `styles.css` | 蓝白对话主题（主蓝 `#1f6bff` + 白，不用重 indigo/紫），圆角气泡，桌面居中模拟手机竖屏 |
| `logic.js` | **纯逻辑层**（无 DOM/无网络）：状态机、回执四行解析、轻标签压缩、决策点路由、角色映射、打断状态机。浏览器挂 `window.VSLogic`，node 挂 `module.exports` |
| `app.js` | 服务对接 + DOM 渲染编排：POST /v1/tasks、轮询、answer、rollback、legacy /v1/cancel、webkitSpeechRecognition、localStorage 设置 |
| `test.js` | node 内置断言测试（逻辑层，44 用例） |

## 基准元素 → 实现位置对照表

基准：飞书文档《VoxSign · iOS 对话界面模拟》+ INTERACT-v1 + M3 测试指引 v1.2。

| # | 基准元素 | 实现位置 |
|---|---|---|
| 1 | 对话流：用户=右蓝气泡（语音带声波动画）；Harness=左白气泡；处理中三点 | `app.js appendUserBubble/appendHarnessBubble/appendTyping`；CSS `.row.user/.row.harness/.wave/.typing` |
| 2 | 轻标签：意图/域/风险小徽章（内部细节不放大） | `logic.js compressBadges`；`app.js` 每条 Harness 气泡下挂 `.badges` |
| 3 | 执行卡：实时滚动步骤（意图分类→域裁决→风险分级→确认闸→执行→校验→归因） | `logic.js EXEC_STAGES`；`app.js ensureExecCard/paintExec/advanceExec` |
| 4 | 回执卡：绿色四行（动作/文件/结果/撤销），撤销按钮调 rollback | `logic.js parseReceipt/extractUndo`；`app.js appendReceiptCard/doRollback` → `POST /v1/tasks/{id}/rollback` |
| 5 | 系统条：说"停"→红色条（已生效/未执行/可继续或撤销），可关闭 | `logic.js interruptSystemBar`；`app.js maybeInterrupt/showSystemBar`（legacy `POST /v1/cancel`） |
| 6 | 底部麦克风条：真语音→文本回显可改，否则键盘兜底 + 发送 | `app.js` `webkitSpeechRecognition` 分支；HTML `.inputbar` |
| 7 | 多角色折叠条（Planner/Executor/Verifier，M5-3 预留） | `app.js` role 折叠/切换；`logic.js roleForStatus`；数据接口 `GET /v1/roles` 未实现→本地静态态 |
| 8 | need_ask 候选按钮（点选→`POST /v1/tasks/{id}/answer {answer:id}`）；need_confirm 红条（answer:"执行"）；一屏一个决策点 | `logic.js nextDecisionPoint`；`app.js renderDecision/answerAndResume` |
| 9 | 服务对接：设置 server+Bearer token；轮询到 done/canceled/interrupted；receipt 四行解析；request_id 客户端生成（重试幂等） | `app.js api/submit/startPolling/tick`；`logic.js genRequestId/isTerminal` |

## 端点对接契约（只读，不改 server）

| 调用 | 方法 | 说明 |
|---|---|---|
| `/v1/tasks` | POST | `{text, request_id}` → `{task_id,status}`；同 request_id 重试 → deduped |
| `/v1/tasks/{id}` | GET | 轮询 `{status,question?,options?,receipt?,attribution?,reversible?,error?}` |
| `/v1/tasks/{id}/answer` | POST | `{answer}`（候选 id 或 "执行"） |
| `/v1/tasks/{id}/rollback` | POST | `{ok,restored}` |
| `/v1/status` | GET | 设置页"测试连接"用 |
| `/v1/tasks/{id}/events` | GET(SSE) | **M6 主路**：事件流 stage/need_ask/need_confirm/done/failed/interrupt/canceled；重连带 `?after=<lastSeq>` |
| `/v1/tasks/{id}/cancel` | POST | 说"停"打断（契约新端点；legacy `/v1/cancel` 兼容兜底） |
| `/v1/roles` | GET | **M6** 实时角色 `[{id,label,active}]`，未实现则本地静态 |
| `/v1/cancel` | POST | 说"停"打断（legacy 兼容路由，body `{task_id}`） |

## 测试方式与结果

逻辑层抽成纯函数，node 内置 `assert`（无第三方依赖）：

```bash
cd voicesign-harness/web
node test.js
```

结果（本机 node v22.23.2）：**66 通过，0 失败**（M5=44 + M6 新增 22）。覆盖：
- 终态/决策点状态机（done/canceled/interrupted 终态；need_ask/need_confirm 挂起）
- 回执四行解析（全/半角冒号、缺行、空串容错）
- 撤销按钮裁决（reversible + .bak 提取；不可撤销不给按钮；VHS_BACKUP_PATH 前缀兼容）
- 轻标签压缩（状态/意图/域/风险）
- 一屏一个决策点路由（confirm/ask/receipt/running/error）
- 角色折叠映射（planner/executor/verifier）
- 打断状态机（已生效/未执行/撤销+继续）
- **SSE 线协议解析** `parseSSEBlock`（event:/data:/注释行/坏 JSON 容错）
- **重连幂等** `filterNew`（seq 去重、`?after=<lastSeq>` 重放不重复）
- **打断三语义** `interruptBarFromEvent`（applied/notApplied/canRollback）
- **角色实时** `activeRole` + `stageIndex`；**语音两态** `detectRecognition`（native/webkit/none）

无 node 的环境：页面打开后在 DevTools console 跑 `VSLogic` 同名函数做手测；本仓库已自带 node，推荐直接 `node test.js`。

## M6-1b：SSE 消费与兜底逻辑

- **主路（SSE）**：契约要求 `Authorization: Bearer`，原生 `EventSource` 不能自定义请求头，故用 `fetch + ReadableStream` 手写 SSE 客户端（`app.js openSSE`）：
  - `GET /v1/tasks/{id}/events`，按 `\n\n` 切完整块 → `L.parseSSEBlock` → `L.filterNew(seenSeqs)` 去重 → `handleEvent` 路由。
  - 7 类事件：`stage` 驱动执行卡真实滚动（`step` 中文名定位下标）+ `role` 点亮角色条；`need_ask`/`need_confirm` 渲染候选/红条；`done` 渲染绿色回执卡；`failed`/`canceled` 收尾；`interrupt` 即时渲染红色系统条。
- **重连幂等**：记录 `lastSeq`（收到的最大 seq），断线后 `?after=<lastSeq>` 重连，`filterNew` 去重不重复渲染（对齐契约 §重连幂等）。
- **兜底**：首连 `/events` 即失败（server 未升级 / 404/405）→ 自动回退 `GET /v1/tasks/{id}` 轮询；流中断后已收过事件则 1.5s 重连，多次失败亦回落轮询。
- **角色实时**：启动 `GET /v1/roles`（`[{id,label,active}]`）点亮；失败则本地静态（M5 行为）；运行中随 `stage.role` 事件实时切换。
- **打断即时**：说"停"→ `POST /v1/tasks/{id}/cancel`（legacy `/v1/cancel` 兜底）→ server 立即推 `interrupt` 事件 → 以其为信号渲染红条（优于轮询感知）。

## 浏览器矩阵冒烟结论

- **node 断言**：66/66 全绿；`node --check`（logic/app/test）全过；`python3 -m http.server` 冒烟 index/logic.js/app.js/styles.css 全部 200。
- **语音输入两态**：`detectRecognition` 已在 node 用 mock 覆盖三态（native / webkit / none），`none` 时麦克风按钮聚焦文本框兜底。
- **需人工确认的真浏览器点**（本环境无 GUI 浏览器自动验证，如实说明）：
  1. **Chrome**：`fetch` ReadableStream 流式解析、`webkitSpeechRecognition` 真机拾音、`?after=` 重连——需起好 vhs server + SSE 端点后手动点按一遍。
  2. **Safari（iOS/macOS）**：fetch stream 需 Safari 16.4+；iOS Safari 麦克风走 `webkitSpeechRecognition` 且需授权；SSE 经 fetch（非 EventSource）在 WKWebView 后台保活需实测。
  3. **Bearer 头**：弃用原生 EventSource 后 token 走 fetch 头，避开了 EventSource 不能带头的坑；请确认 server SSE 端点对 Bearer 校验与 REST 一致。

## 如何运行

```bash
# 1) 起一个静态服务（web/ 目录）
cd voicesign-harness/web
python3 -m http.server 8080

# 2) 浏览器/手机访问
#    桌面： http://127.0.0.1:8080/
#    手机：  http://<Mac内网IP>:8080/ （与 Mac 同 Wi-Fi）

# 3) 右上角 ⚙ 填 Harness server 地址（如 http://192.168.x.x:8765）与 Bearer token
#    点"测试连接"应返回 OK · v0.2.0
```

前提：Mac 上已按《手机端真机测试指引-M3.md》起好 `vhs serve`（绑 `0.0.0.0:8765` + token）。本壳只对接端点契约，不修改任何 Go/server 代码。

## 约束遵守

- 只新增 `voicesign-harness/web/`，未触碰 `~/VoxSign`，未推 GitHub，未改 server/Go 代码。
- 判断/裁决逻辑（打断状态机、一屏决策点流转、轮询状态机、撤销裁决）在 `logic.js` / `app.js` 对应模块头部均有【伪代码逻辑层】注释块；纯渲染/样式豁免。
