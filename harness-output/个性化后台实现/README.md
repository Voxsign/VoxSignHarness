个性化后台实现 · 运行说明

一、环境与启动
依赖：Go 1.21 及以上，无外部数据库。
构建：go build -o personal-server ./cmd/server
启动：./personal-server --addr 127.0.0.1:8787 --data-dir ./data --token dev-token
开发态：go run ./cmd/server --data-dir ./data
参数：
--addr 监听地址，默认 127.0.0.1:8787；非回环地址（非 127.0.0.1/::1）启动即拒绝并退出。
--data-dir 数据目录，默认 ./data，不存在时自动创建。
--token 鉴权占位令牌；为空时仍要求携带 Authorization 头，仅不校验取值。

二、HTTP 端点
GET /v1/health
返回 JSON：status、version、data_dir、uptime。
POST /v1/process
请求体 JSON，字段：text（原始输入）、action（可选，默认 auto）、user_id（可选）、feedback（可选，取值 ok/bad，用于反馈学习）。
响应体 JSON：cleaned（清洗后文本）、corrected（词典纠错后文本）、changed（是否发生改动，正常文本保证 false）、intent（NOTE/QUERY/EDIT/COMMIT/ORCHESTRATE 之一）、matched（命中的词典条目）、trace_id。
词典管理（可合理扩展）：POST /v1/dict 增删查条目，action 取 add/del/list，字段 word、replacement、note。
鉴权：请求头 Authorization: Bearer <token>，缺失或格式错误返回 401。

三、数据文件（全部 append-only，写入后不改写不重写）
data/dictionary.json 个性化词典条目，支撑增/删/查与匹配纠错。
data/feedback.jsonl 反馈学习记录，每行一条 ✔/✘ 回馈。
data/traces.jsonl /v1/process 调用轨迹，每行一条含 trace_id、输入输出摘要、耗时。
data/usage.jsonl 用量统计，每行一条含时间、意图、命中数。

四、行为约定
纠错只作用于词典命中的确定性条目，未命中文本原样返回，避免改坏正常文本。
清洗仅做空白归一与不可见字符处理，不改变语义。
意图分类按关键词与句式打分取最高分，无法判定时归入 NOTE。
启动时校验数据目录可写，写入失败记录到 traces.jsonl 并返回 500。