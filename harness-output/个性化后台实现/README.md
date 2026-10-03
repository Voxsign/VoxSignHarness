# 个性化后台实现

本服务提供个性化词典、文本纠错、意图分类、反馈学习与数据落盘能力，仅监听回环地址，供本机调用。

## 一、环境准备

Python 3.10+，无外部依赖时使用标准库即可运行。

安装依赖（如有）：
pip install -r requirements.txt

## 二、启动命令

默认启动（监听 127.0.0.1:8000，数据目录 ./data）：
python -m app.server

指定数据目录与端口：
python -m app.server --data-dir /var/lib/personalize --host 127.0.0.1 --port 8000

说明：--host 仅接受 127.0.0.1、::1、localhost。传入其他地址（如 0.0.0.0）将直接拒绝启动。

鉴权为占位实现：请求头需带 X-Auth-Token（默认值 dev-token），缺省或错误返回 401；可在启动参数 --token 修改，留空则关闭校验（仅限本机调试）。

## 三、HTTP 端点

GET /v1/health
返回服务状态与数据目录：
{"status":"ok","data_dir":"./data","uptime_s":12}

POST /v1/process
请求体 JSON：
{"text":"明天下午三点提醒我开会","user_id":"u1","feedback":null}

响应体 JSON：
{"intent":"NOTE","corrected_text":"明天下午三点提醒我开会","hits":[{"term":"开会","action":"keep"}],"trace_id":"t-20250101-0001"}

字段说明：
intent 取值 NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE 五类之一。
corrected_text 为清洗与词典纠错后的文本；未命中词典时原样返回，保证正常文本不被改坏。
hits 为词典匹配明细，含匹配项与纠错动作，便于人工核对安全边界。

反馈提交（可选）：请求体带 feedback 字段，值 "up" 或 "down"，用于写入 feedback.jsonl。

## 四、数据文件

所有文件位于 data-dir 目录下，均为 append-only，不覆盖不改写历史。

data/dictionary.json   个性化词典条目，支持增/删/查，结构为 {"term":"原词","replacement":"替换词","enabled":true}
data/traces.jsonl      每次 /v1/process 的完整请求与响应轨迹，一行一条
data/usage.jsonl       调用用量，含时间戳、意图、耗时、命中数，一行一条
data/feedback.jsonl    反馈记录，含 trace_id、评价（up/down）、时间戳，一行一条

词典的增删查通过服务内部接口完成，写入后立即生效，无需重启。

## 五、运行自检

查看健康状态：
curl -s http://127.0.0.1:8000/v1/health

发起一次处理：
curl -s -X POST http://127.0.0.1:8000/v1/process -H "Content-Type: application/json" -H "X-Auth-Token: dev-token" -d "{\"text\":\"帮我记一下明天交周报\"}"

确认落盘：
tail -n 1 data/traces.jsonl
tail -n 1 data/usage.jsonl

## 六、注意事项

1. 服务只监听回环地址，非回环绑定会被拒绝，避免误暴露到局域网。
2. 纠错遵循保守策略：无词典命中且不符合清洗规则时，文本原样返回。
3. JSONL 文件按日或按需轮转需自行处理，服务本身只做追加。
4. 迁移数据目录只需复制整个 data 目录，路径通过 --data-dir 指定。