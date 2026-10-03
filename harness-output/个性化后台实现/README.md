个性化后台 运行说明

一、启动
1. 直接运行
   go run . --addr 127.0.0.1:8080 --data-dir ./data

2. 编译后运行
   go build -o server .
   ./server --addr 127.0.0.1:8080 --data-dir ./data

默认参数
   --addr     监听地址，仅允许 127.0.0.1，默认 127.0.0.1:8080
   --data-dir 数据目录，默认 ./data
   --token    可选占位鉴权 token，默认空

二、端点
1. 健康检查
   GET /v1/health
   响应示例
   {"status":"ok","addr":"127.0.0.1:8080","data_dir":"./data"}

2. 业务处理
   POST /v1/process
   Content-Type: application/json
   可选鉴权头
   Authorization: Bearer <token>
   请求示例
   {"text":"帮我记一下明天开会","action":"classify","feedback":null}
   响应示例
   {"ok":true,"intent":"NOTE","corrected":"帮我记一下明天开会","changed":false}

3. 词典操作
   通过 /v1/process 的 action 字段区分
   action=dict_add   新增词条
   action=dict_del   删除词条
   action=dict_get   查询词条
   action=correct    文本清洗与词典纠错
   action=classify   意图分类
   action=feedback   提交 ✔/✘ 反馈

4. 意图分类取值
   NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE

三、数据文件
数据目录由 --data-dir 指定，所有 JSONL 均为 append-only。

   data/dict.json       个性化词典持久化文件
   data/feedback.jsonl  ✔/✘ 反馈记录，每行一条
   data/traces.jsonl    处理轨迹，每行一条
   data/usage.jsonl     调用用量，每行一条

四、请求示例
1. 文本纠错
   curl -s -X POST http://127.0.0.1:8080/v1/process \
     -H 'Content-Type: application/json' \
     -d '{"text":"帮我记一下明天开会","action":"correct"}'

2. 意图分类
   curl -s -X POST http://127.0.0.1:8080/v1/process \
     -H 'Content-Type: application/json' \
     -d '{"text":"帮我记一下明天开会","action":"classify"}'

3. 新增词典条目
   curl -s -X POST http://127.0.0.1:8080/v1/process \
     -H 'Content-Type: application/json' \
     -d '{"action":"dict_add","word":"开会","replacement":"会议"}'

4. 提交反馈
   curl -s -X POST http://127.0.0.1:8080/v1/process \
     -H 'Content-Type: application/json' \
     -d '{"action":"feedback","trace_id":"t-123","feedback":"✔"}'

五、安全与限制
   服务仅监听 127.0.0.1，非回环地址拒绝启动。
   Token 鉴权为占位实现，生产环境需替换为真实校验。
   所有落盘文件按 append-only 方式写入，不覆盖历史数据。