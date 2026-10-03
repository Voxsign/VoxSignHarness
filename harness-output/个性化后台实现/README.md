个性化后台实现 · 运行说明

一、环境与启动
依赖：Python 3.9+（仅标准库），无需额外安装。
启动：python app.py
指定数据目录：python app.py --data-dir ./data   （默认 ./data，首次启动自动创建）
指定端口：python app.py --port 8080
服务仅监听 127.0.0.1，绑定非回环地址（0.0.0.0/局域网 IP）会被直接拒绝启动。

二、HTTP 端点
1) 健康检查
GET /v1/health
返回：{"status":"ok","version":"...","data_dir":"...","uptime_s":N}

2) 主处理入口
POST /v1/process
请求头：X-Auth-Token: <token>（占位鉴权，默认 token 可用 --token 设置；留空则跳过校验）
请求体 JSON：
{
  "text": "待处理文本",
  "action": "process",        // process | dict_add | dict_del | dict_list | feedback
  "feedback": "ok"            // 仅 action=feedback 时使用：ok=✔ / bad=✘
}
响应 JSON：
{
  "ok": true,
  "corrected": "纠错后文本",
  "intent": "NOTE|QUERY|EDIT|COMMIT|ORCHESTRATE",
  "confidence": 0.0-1.0,
  "dict_hits": [{"term":"...","action":"..."}],
  "changed": true|false,
  "trace_id": "..."
}
文本纠错保证：未命中词典或清洗规则的正常文本原样返回，changed=false。

三、数据文件（全部 append-only JSONL，位于 data-dir 下）
data/feedback.jsonl    反馈学习记录：时间戳、trace_id、✔/✘、原始与纠错文本、意图
data/traces.jsonl      每次 /v1/process 的完整处理轨迹（入参、词典命中、纠错差异、意图）
data/usage.jsonl       调用量统计：时间戳、端点、耗时 ms、状态码
data/dict.json         个性化词典（唯一非 append 文件，增删改后整体重写，写入采用临时文件+原子替换）
日志文件只追加、不修改历史行；可用 grep/jq 直接消费。

四、词典操作
新增/覆盖：{"action":"dict_add","term":"误写","replace":"正确写法","match":"exact|contains"}
删除：{"action":"dict_del","term":"误写"}
查询：{"action":"dict_list"} 或 GET /v1/dict?q=关键词
匹配安全：默认仅在词边界/整词命中时替换，短词与高频词自动跳过，避免正常文本被改坏。

五、快速验证
curl http://127.0.0.1:8080/v1/health
curl -X POST http://127.0.0.1:8080/v1/process -H "Content-Type: application/json" -H "X-Auth-Token: dev" -d "{\"text\":\"帮我记一下明天开会\",\"action\":\"process\"}"
curl -X POST http://127.0.0.1:8080/v1/process -H "Content-Type: application/json" -d "{\"action\":\"feedback\",\"trace_id\":\"上一步返回的id\",\"feedback\":\"ok\"}"