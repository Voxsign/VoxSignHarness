# ASR 个性化后台服务 · 需求规格 v3（独立 Go 服务）

> 整理人：豆包（VHS 测试线），2026-10-04。用途：作为 VoxSign Harness 长程实现任务的输入文档（document）。

## 1. 目标

生成一个**独立运行的 Go 后台服务**，实现 ASR 识别文本的**个性化纠错与学习闭环**：用户教过的词（词典）、标错的词（黑名单）、确认过的映射（反馈），服务要能记住并在后续纠错中应用——"越来越懂你"。

## 2. 技术约束

- Go 语言，**仅标准库，零第三方依赖**（不许用外部包）。
- **可独立运行**：通过 flag 或环境变量配置监听地址（默认 127.0.0.1）与数据目录（默认 ./data）。
- **数据持久化**：feedback 用 **JSONL append-only**；dictionary/blacklist 用 JSON 文件。
- 代码**不留 TODO/占位/伪代码**，自包含、可直接 go build 通过。
- 中文文本处理（UTF-8）。

## 3. P0 能力（全部必须实现，逐条验收）

| # | 端点 | 行为 | 数据 |
|---|---|---|---|
| 1 | GET /v1/health | 健康检查 → `{"ok":true}` | 无 |
| 2 | GET /v1/dict | 返回全量词典 JSON（terms 数组：term/variants/category/source） | dictionary.json |
| 3 | POST /v1/term | 增词（body: term/variants/source），写回词典并持久化 | dictionary.json |
| 4 | POST /v1/correct | 个性化纠错：输入 {text}，应用词典映射（如 曼苏→Mansour）与黑名单拦截，返回 {corrected, applied:[…]} | 读 dictionary/blacklist |
| 5 | POST /v1/process | 意图分类+纠错：输入 {text}，返回 {intent, corrected}（intent ∈ NOTE/QUERY/EDIT/…，含纠错结果） | 读 dictionary/blacklist |
| 6 | POST /v1/feedback | 反馈学习：输入 {raw, corrected, accepted, reason}，**追加** feedback.jsonl（append-only，✘ 必带 reason） | feedback.jsonl |
| 7 | POST /v1/blacklist | 黑名单：输入 {term, note}，写入 blacklist.json（"这个改错了"→ 纠错时跳过该词） | blacklist.json |
| 8 | 鉴权（可选） | 环境变量配 token 时，请求头 Authorization: Bearer <token> 校验；未配则关闭 | 无 |

## 4. 验收判据（真装配，不许桩）

1. `go build` 零错误（仅标准库）。
2. 服务启动后 `GET /v1/health` 返回 `{"ok":true}`。
3. `POST /v1/correct {"text":"你好 曼苏"}` → corrected 含 `Mansour`（词典生效）。
4. `POST /v1/feedback` 后，data/feedback.jsonl **追加**一条记录（含 raw/corrected/accepted/reason）。
5. `POST /v1/term {"term":"Mansour","variants":["曼苏"]}` 后，data/dictionary.json 含该词条（持久化）。
6. 重启服务后词典/黑名单仍在（持久化跨重启）。
7. 所有端点 JSON 响应，中文不乱码。

## 5. 交付形态

- 产物目录：harness-output/ASR个性化后台服务/（main.go + go.mod + README.md）
- README 说明：启动命令、端点清单、数据文件位置。
