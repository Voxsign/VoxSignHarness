个性化后台运行说明

一、环境与安装
Python 3.10+。首次运行前安装依赖：
pip install -r requirements.txt
（零依赖实现则可跳过）

二、启动
默认监听回环地址，仅允许 127.0.0.1 / ::1；若传入非回环地址，进程会直接拒绝启动并退出。

python -m app.server --host 127.0.0.1 --port 8080 --data-dir ./data --token dev-token

等价环境变量方式：
HOST=127.0.0.1 PORT=8080 DATA_DIR=./data AUTH_TOKEN=dev-token python -m app.server

参数说明
--host      默认 127.0.0.1，非回环值将被拒绝
--port      默认 8080
--data-dir  数据目录，默认 ./data，不存在时自动创建
--token     鉴权令牌（占位实现），默认从 AUTH_TOKEN 读取

鉴权：除 /v1/health 外，所有请求需带请求头
Authorization: Bearer <token>
未配置 token 时放行但会在 usage 中标记 auth=none。

三、端点
1) 健康检查
GET /v1/health
返回 {"status":"ok","version":"...","data_dir":"...","uptime_s":N}

2) 文本处理
POST /v1/process
Content-Type: application/json
请求体：
{
  "text": "把明天的会记一下",
  "session_id": "s-001",
  "feedback": null
}
响应体：
{
  "trace_id": "t-...",
  "intent": "NOTE",
  "corrected_text": "把明天的会记一下",
  "dictionary_hits": [{"term":"...","action":"keep"}],
  "changed": false,
  "notes": []
}
intent 取值：NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE。

3) 反馈（写回 feedback.jsonl）
沿用同一端点，提交时带 feedback 字段：
{
  "text": "把明天的会记一下",
  "trace_id": "t-...",
  "feedback": {"vote": "up", "comment": "可选"}
}
vote 取值 up(✔) 或 down(✘)。写入为 append-only，不覆盖历史。

4) 词典（增 / 删 / 查）
POST /v1/dict   {"op":"add","term":"飞书","aliases":["feishu"],"replace":"飞书"}
POST /v1/dict   {"op":"del","term":"飞书"}
GET  /v1/dict?q=飞书
纠错遵循安全原则：仅命中词典的确定性替换，未命中不做任何改写；长度、标点、大小写等无匹配时原样返回，changed=false。

curl 示例
curl -s http://127.0.0.1:8080/v1/health
curl -s -X POST http://127.0.0.1:8080/v1/process -H 'Authorization: Bearer dev-token' -H 'Content-Type: application/json' -d '{"text":"帮我查下上周的周报","session_id":"s-001"}'

四、数据文件
全部位于 --data-dir 指定目录（默认 ./data），均为 JSONL、append-only、逐行一个 JSON 对象，写入使用追加+fsync，不做原地修改：
data/traces.jsonl    每次 /v1/process 的输入、意图、纠错结果、trace_id、时间戳
data/usage.jsonl     端点调用、耗时、状态码、鉴权标记
data/feedback.jsonl  vote / comment / trace_id / 时间戳
data/dict.json       个性化词典（增删改时原子写：临时文件 + rename）
data/server.log       运行日志（可选）

清理与迁移：直接停服后移动或删除整个 data-dir 即可，服务下次启动会重建缺失文件。

五、快速自检
python -m app.server --host 127.0.0.1 --port 8080 &
curl -s http://127.0.0.1:8080/v1/health
curl -s -X POST http://127.0.0.1:8080/v1/process -H 'Content-Type: application/json' -d '{"text":"记一下：周五交材料"}'
tail -n 1 data/traces.jsonl
tail -n 1 data/usage.jsonl