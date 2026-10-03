# ASR 个性化后台服务（独立 Go 服务）

> 由 VoxSign Harness 实现 + 豆包校验对齐修订，2026-10-04。对齐度 100%（validate-align 六通道，5 处字段偏差已消解）。

## 运行

```bash
go build -o impl .
./impl -addr 127.0.0.1:8080 -data-dir ./data
```

## 端点（7 个，契约字段）

| 端点 | 方法 | 说明 |
|---|---|---|
| /v1/health | GET | 健康检查 → {"ok":true} |
| /v1/correct | POST | 纠错：{text} → {corrected, applied} |
| /v1/term | POST | 教词：{term, variants, source, correction?} → 写 dictionary.json（variants 命中即纠为 term） |
| /v1/dict | GET | 全量词典：terms[] 含 term/variants/category/source |
| /v1/feedback | POST | 反馈：{raw, corrected, accepted, reason} → append feedback.jsonl（✘ 必带 reason） |
| /v1/blacklist | POST | 黑名单：{term, note} → blacklist.json（纠错跳过） |
| /v1/process | POST | 意图+纠错：{text} → {intent, corrected}（corrected_text 兼容） |

## 数据文件（-data-dir 下）

dictionary.json（JSON 对象，含 variants/category/source）/ blacklist.json / feedback.jsonl（append-only）/ traces.jsonl

## 验收

```bash
sh scripts/accept_asr.sh 8911   # 一键验收：12 判据真跑 → RESULT: ALL_PASS
```
校验对齐报告：docs/ASR个性化后台服务-校验对齐报告-20261004.md

## 安装

```bash
sh install.sh            # 默认安装到 bin/（也可指定目录：sh install.sh /usr/local/bin）
bin/asr-service -addr 127.0.0.1:8080 -data-dir ./data   # 运行
curl -s http://127.0.0.1:8080/v1/health                 # → {"ok":true}
```
