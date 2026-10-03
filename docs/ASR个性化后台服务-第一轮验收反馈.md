# ASR 个性化后台服务 · 第一轮验收测试反馈（修订输入）

> 测试方：豆包独立验收（真装配，不许桩），2026-10-04。本文件作为 harness 下一轮实现的 document 输入。

## 实测结果（真跑 /tmp/asr_impl -addr 127.0.0.1:8898）

| # | 判据 | 结果 | 证据 |
|---|---|---|---|
| 1 | /v1/health | ✅ PASS | HTTP 200 → OK |
| 2 | /v1/correct | ❌ FAIL | HTTP 404 page not found（需求 §3 判据 3） |
| 3 | /v1/term | ❌ FAIL | HTTP 404 page not found（需求 §3 判据 2 增词） |
| 4 | /v1/dict | ❌ FAIL | HTTP 404 page not found（需求 §3 判据 2 查词） |
| 5 | /v1/feedback | ❌ FAIL | HTTP 404 page not found（需求 §3 判据 4） |
| 6 | /v1/blacklist | ❌ FAIL | HTTP 404 page not found（需求 §3 判据 5） |
| 7 | /v1/process | ⚠️ 部分 | HTTP 200 {"corrected_text":"你好 曼苏","intent":"ORCHESTRATE"}——纠错引擎在但无数据源（/v1/term 404 无法教词） |

数据文件：traces.jsonl 已生成；feedback.jsonl / dictionary.json / blacklist.json 未生成（无对应端点）。

## 缺口清单（下一轮必须补齐）

1. **POST /v1/correct**：{text} → 应用词典映射（如 曼苏→Mansour）+ 黑名单拦截 → {corrected, applied}。
2. **POST /v1/term**：{term, variants, source} → 写入并持久化 dictionary.json。
3. **GET /v1/dict**：返回全量词典 JSON。
4. **POST /v1/feedback**：{raw, corrected, accepted, reason} → 追加 feedback.jsonl（append-only；✘ 必带 reason，空记 user_marked_wrong）。
5. **POST /v1/blacklist**：{term, note} → 写入 blacklist.json（纠错时跳过该词）。
6. **持久化跨重启**：dictionary.json / blacklist.json 启动时加载，重启不丢。
7. **process 纠错生效**：教词（曼苏→Mansour）后，/v1/process 对含"曼苏"文本 corrected_text 返回 "Mansour"。

## 验收通过标准（全部满足才算完成）

- go build 零错误；7 个端点全部 HTTP 200 且语义正确；
- 教词后 correct/process 纠错生效；feedback 后 feedback.jsonl 追加；重启后词典/黑名单仍在。
