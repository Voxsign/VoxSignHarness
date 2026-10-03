个性化后台 运行说明

环境
Go 1.21+。默认数据目录 ./data，可用 -data-dir 或环境变量 DATA_DIR 覆盖，进程启动时自动创建。

编译与启动
go build -o pbackend .
./pbackend -addr 127.0.0.1:8080 -data-dir ./data -token <可选占位令牌>

仅允许监听回环地址；传入非 127.0.0.1 的 -addr 将直接拒绝启动。鉴权为占位实现：配置 -token 后，请求需带 Authorization: Bearer <token>，未配置则不校验。

端点
GET  /v1/health              返回 {"status":"ok","version":"...","data_dir":"..."}
POST /v1/process             JSON 请求/响应，统一业务入口

/v1/process 请求示例
{"text":"帮我记一下明天十点开会","op":"process","feedback":null}

字段说明
text      待处理文本（必填）
op        可选：process（默认，清洗+纠错+意图分类）/dict_add /dict_del /dict_get /feedback
word      词典操作时的词条
intent    显式指定意图，缺省由分类器推断
feedback  "ok" 或 "bad"，写入反馈学习日志

响应示例
{"intent":"NOTE","corrected":"帮我记一下明天十点开会","changed":false,"matches":[{"word":"明天十点","correct":true}],"trace_id":"..."}

意图分类
NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE 五类，按关键词与句式规则打分，取最高分；无法判定时回落 NOTE 并在响应中标记 low_confidence。

纠错策略
先做清洗（去零宽字符、合并空白、全半角归一），再按个性化词典做替换与模糊纠正。命中词典的原词不重复改写，未命中且置信度低于阈值的片段保持原样，保证正常文本不被改坏。

数据文件（均在 -data-dir 下，全部 append-only JSONL，逐行 JSON，不重写历史）
dictionary.jsonl   词典增删记录，启动时重放得到当前词典
feedback.jsonl     反馈学习记录，✔/✘ 均落盘
traces.jsonl       每次 /v1/process 的请求、纠错结果、意图与耗时
usage.jsonl        端点调用计数与状态码

词典增删查示例
curl -s -X POST 127.0.0.1:8080/v1/process -H 'Content-Type: application/json' \
  -d '{"op":"dict_add","word":"十点","intent":"NOTE"}'
curl -s -X POST 127.0.0.1:8080/v1/process -H 'Content-Type: application/json' \
  -d '{"op":"dict_get","word":"十点"}'
curl -s -X POST 127.0.0.1:8080/v1/process -H 'Content-Type: application/json' \
  -d '{"op":"dict_del","word":"十点"}'

反馈示例
curl -s -X POST 127.0.0.1:8080/v1/process -H 'Content-Type: application/json' \
  -d '{"op":"feedback","trace_id":"<上一步返回的 trace_id>","feedback":"ok"}'

健康检查
curl -s 127.0.0.1:8080/v1/health

退出后直接查看落盘结果
tail -n 5 ./data/traces.jsonl
tail -n 5 ./data/feedback.jsonl