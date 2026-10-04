语音适配层 vhs-voice —— 运行说明

一、构建与启动
  go build ./...
  VHS_VOICE_ADDR=127.0.0.1:8950 VHS_UPSTREAM=http://127.0.0.1:8941 VHS_DATA_DIR=./data go run ./cmd/vhs-voice
  go run / go build 仅用标准库，无第三方依赖。
  仅监听回环地址，非 127.0.0.1 的请求一律拒绝；非回环绑定直接启动失败。
  默认端口 8950；默认上游 http://127.0.0.1:8941；默认数据目录 ./data。

二、环境变量
  VHS_VOICE_ADDR  监听地址，默认 127.0.0.1:8950
  VHS_UPSTREAM    主 harness 地址，默认 http://127.0.0.1:8941
  VHS_DATA_DIR    数据目录，默认 ./data
  VHS_TOKEN       鉴权占位令牌（可选，当前为占位实现）

三、HTTP 端点
  GET  /v1/health                    健康检查，返回 ok/service/upstream/sessions
  POST /v1/process                   主入口：JSON 请求/响应，内含 清洗→纠错→意图分类→处理
  POST /v1/voice/parse               口语去噪提意，出参 clean/actions/noise_removed
  POST /v1/voice/decompose           复合指令拆单任务，出参 tasks[seq,action,target]
  POST /v1/voice/dispatch            按序投递主 harness 并汇总语音友好结果
  GET  /v1/voice/health              适配层健康检查
  POST /v1/voice/feedback            反馈学习，✔/✘ 落盘 append-only
  GET  /v1/dict                      词典查询
  POST /v1/dict                      词典新增/删除条目
  POST /v1/tasks                     上游 POST /v1/tasks 的代理注册
  GET  /v1/tasks/{id}                上游任务状态查询代理

四、请求示例
  curl -s http://127.0.0.1:8950/v1/health
  curl -s -X POST http://127.0.0.1:8950/v1/voice/parse -d '{"text":"那个就是帮我跑一下测试吧"}'
  curl -s -X POST http://127.0.0.1:8950/v1/process -d '{"text":"...","conversation_id":"c1"}'

五、数据文件（append-only，UTF-8）
  <dataDir>/voice_sessions/<conversation_id>.jsonl   会话记忆，可审计
  <dataDir>/feedback.jsonl                           反馈学习记录
  <dataDir>/traces.jsonl                             处理轨迹
  <dataDir>/usage.jsonl                              调用计量
  目录不可写时启动即报错，不静默降级。

六、能力清单对照
  个性化词典：/v1/dict 增删查，匹配按最长优先，纠错仅在词典命中且不改变正常文本时替换
  文本纠错：clean 去填充词后再做词典纠错，正常文本原样透传
  意图分类：NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE 五类
  反馈学习：✔/✘ 写入 feedback.jsonl
  数据落盘：traces/usage 等 JSONL 独立数据目录