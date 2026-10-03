README.md

VoiceSign 个性化 ASR 后台 · 运行说明

构建与启动
源码运行：go run ./cmd/voicesign-asr -config ./config.json -data-dir ~/.voicesign/asr -addr :8080
构建二进制：go build -o voicesign-asr ./cmd/voicesign-asr
启动二进制：./voicesign-asr -config ./config.json -data-dir ~/.voicesign/asr -addr :8080
默认监听：http://127.0.0.1:8080
说明：Go 单二进制、独立服务，与 VoiceSign Harness 解耦；配置均为 JSON，支持热加载。

端点
GET  /v1/health      健康检查
POST /v1/process     文本 → 标准化意图 JSON，主入口
POST /v1/correct     仅纠错，返回纠错后文本与纠错明细
POST /v1/dictionary  词典增删改查，支持语音指令转操作
POST /v1/feedback    反馈学习：用户修改 / 确认 / 否认
WS   /v1/stream      流式处理，全双工预留

调用示例
curl -X POST http://127.0.0.1:8080/v1/process \
  -H 'Content-Type: application/json' \
  -d '{"text":"把那个模块的错误提示改成中文","session_id":"s1","audio_meta":{"engine":"system","confidence":0.86,"lang":"zh"}}'

数据文件
默认目录：~/.voicesign/asr/
custom-dictionary.json      个性化词典：口语/错词 → 标准写法
context-memory.json          指代四元组缓存、上下文、确认固化结果
user-preferences.json        用户偏好：确认策略、默认域、纠错强度等
project-knowledge.json       项目地图、领域术语、决策摘要
exceptions-knowledge.jsonl   异常知识库，append-only
traces-asr.jsonl             全链路轨迹，append-only
usage-patterns.json          使用模式统计：高频说法、意图、时段、确认率

运行约定
本地规则优先，模型兜底；模型超时默认 3s，超时降级并标记 degraded。
所有输入永久留底，纠错与清洗均为派生结果。
未知意图默认落入 global 只读域；ASR 后台只建议域，不授权。
个性化组件故障时 fail-open，保证语音输入链路始终有响应。