用户姓名记忆服务 — 运行说明

一、构建与启动
go build -o name-server .
./name-server
也可以直接：go run .
服务监听 127.0.0.1:8099（仅回环，非回环地址拒绝）。纯 Go 标准库，无第三方依赖。
数据目录默认当前工作目录，可用 -data-dir 指定。

二、端点
POST /v1/set_name    请求体 JSON {"name":"周勇明","breakdown":"勇敢的勇，明天的明"}，写入 user_profile.json，返回已保存的姓名。
GET  /v1/get_name    返回当前记住的姓名；未设置时返回 {"name":"","hint":"未设置"}。
GET  /v1/health      返回 {"status":"ok"}。
POST /v1/process     文本处理主入口，JSON 请求/响应，覆盖动作识别、复合指令拆解、指代消解等能力。
                      - 动作词表命中即返回非空 actions，不返回空数组。
                      - 复合指令按标点/连接词切分，返回非空 tasks。
                      - 含指代词且该会话无历史时返回 {"resolved":false,"target":"unresolved","pending_resolve":true}；
                        有历史时从最近记录补全 target，不使用默认值。

三、调用示例
curl -X POST http://127.0.0.1:8099/v1/set_name -H "Content-Type: application/json" -d "{\"name\":\"周勇明\",\"breakdown\":\"勇敢的勇，明天的明\"}"
curl http://127.0.0.1:8099/v1/get_name
curl http://127.0.0.1:8099/v1/health

四、数据文件
user_profile.json        当前工作目录（或 -data-dir），存姓名与拆字说明，重启不丢失。
feedback.jsonl           append-only，✔/✘ 反馈落盘。
traces/*.jsonl           append-only，请求与处理轨迹。
usage/*.jsonl            append-only，调用用量统计。
会话历史同样落盘于数据目录，供指代消解查最近记录。

五、配置与鉴权
-data-dir     指定数据目录，默认当前目录。
-addr         默认 127.0.0.1:8099，非回环地址直接拒绝启动。
鉴权头为占位实现，未配置时不做拦截。