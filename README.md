# VoiceSign Harness

VoiceSign 的**编排 / 可观测 / 校验骨架（harness）**：把「语音/文本指令」从原文一路串到「四行回执」，
中间经 13 阶段 pipeline（清洗→纠错→意图→指代→空间门禁→风险分级→确认→执行→自愈→独立校验→归因→缓存→回执）。

- 纯 Go 标准库，**零外部依赖**。
- 线A = `server/`（HTTP 入口、任务表、串行闸、确认桥）；线B = `cmd/vhs-asr`（独立 ASR 个性化服务）。

## 与 `~/voxsign` 的关系

本仓库是 harness（执行面/可观测/校验），`~/voxsign` 是上游产品/客户端侧。两者以
`contracts/intent-v1.schema.json`（意图唯一权威）+ HTTP 解耦，互不 import。
详见 `ARCHITECTURE.md` §12。

## 验证方法入口

```bash
# 可执行回归门（本机 macOS 27 上必须 CGO_ENABLED=0，见 ARCHITECTURE.md §8）
CGO_ENABLED=0 ~/go-sdk/go/bin/go test ./...
CGO_ENABLED=0 ~/go-sdk/go/bin/go test -tags archstub ./e2e -run TestArch -v
```

- 架构全貌与分层：`ARCHITECTURE.md`（v1）。
- 性能基准：`bench/startup_bench.py`。
- 判据方法论：各包 `*_criteria_test.go`（先红后绿、机械对账）。

## 签署状态入口

验收用例集 `eval/cases/cases.jsonl`（17 条）。签署流程与待办见：
**`eval/cases/RATIFICATION-REQUEST-2026-10-05.md`**（待 Peter/指定签署人审阅签署）。
历史提案：`eval/cases/RATIFICATION-PROPOSAL-2026-10-03.md`。

## 常用命令

| 命令 | 作用 |
|---|---|
| `go run . serve` | 起线A server |
| `go run . repl` | 交互式 REPL |
| `go run ./cmd/vhs-asr` | 起线B ASR 服务（:8787） |
