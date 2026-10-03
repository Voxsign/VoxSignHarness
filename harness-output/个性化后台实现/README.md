# 个性化后台实现（ASR 个性化后台服务）

> 由 VoxSign Harness 实现 + 豆包验收补齐，2026-10-04。

## 运行

```bash
go build -o impl .
./impl -addr 127.0.0.1:8080 -data-dir ./data
```

## 端点（7 个）

| 端点 | 方法 | 说明 |
|---|---|---|
| /v1/health | GET | 健康检查 → {"ok":true} |
| /v1/correct | POST | 纠错：{text} → {corrected, applied} |
| /v1/term | POST | 教词：{term, correction} → 写 dictionary.jsonl |
| /v1/dict | GET | 全量词典 |
| /v1/feedback | POST | 反馈：{raw, corrected, accepted, reason} → append feedback.jsonl |
| /v1/blacklist | POST | 黑名单：{term, note} → blacklist.json |
| /v1/process | POST | 纠错+意图分类：{text} → {corrected_text, intent} |

## 数据文件（-data-dir 下）

dictionary.jsonl（教词）/ blacklist.json（黑名单）/ feedback.jsonl（反馈 append-only）/ traces.jsonl（轨迹）

验收报告：docs/ASR个性化后台服务-验收报告-20261004.md（9/9 PASS）
