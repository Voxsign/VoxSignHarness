个性化后台实现 README

一、运行环境
Go 1.21+，仅需标准库，无外部依赖。

二、构建与启动
构建：
go build -o bin/personal-backend ./

启动（默认监听 127.0.0.1:8080，数据目录 ./data）：
./bin/personal-backend

指定端口与数据目录：
./bin/personal-backend -addr 127.0.0.1:8080 -data-dir ./data

可选环境变量（与命令行等价，命令行优先）：
PB_ADDR       监听地址，默认 127.0.0.1:8080
PB_DATA_DIR   数据目录，默认 ./data
PB_TOKEN      鉴权占位 token，默认空（空则不校验）

安全约束：服务只绑定回环地址。若 -addr 传入非 127.0.0.1/::1/localhost 的主机，启动即报错退出。除 /v1/health 外所有端点需携带 Authorization: Bearer <PB_TOKEN>（未配置 token 时跳过校验，仅作占位）。

三、HTTP 端点

1) 健康检查
GET /v1/health
响应：{"ok":true,"addr":"127.0.0.1:8080","data_dir":"./data","time":"..."}

2) 业务处理
POST /v1/process
Content-Type: application/json

请求字段：
text      必填，待处理文本
action    可选，dictionary_add | dictionary_del | dictionary_list | process | feedback，默认 process
term      可选，词典增删时的词条
rating    可选，反馈时取 "up" 或 "down"（对应 ✔ / ✘）
intent    可选，反馈时可显式指定被评估的意图

process 响应字段：
intent          意图分类结果，取值 NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE
corrected       清洗 + 词典纠错后的文本（正常文本原样返回）
changed         纠错是否发生变更
hits            命中的词典条目
trace_id        本次调用轨迹 ID

示例：
curl -s -X POST http://127.0.0.1:8080/v1/process -H "Content-Type: application/json" -d '{"text":"记一下明天开会"}'

四、数据落盘（全部 append-only，JSONL/JSON）
data/dictionary.json   个性化词典，含词条、别名、启用状态；增/删/查均写回
data/feedback.jsonl    反馈学习，每行一条 {time,trace_id,intent,rating,text}
data/traces.jsonl      处理轨迹，每行一条请求/响应摘要
data/usage.jsonl       用量统计，每行一条 {time,endpoint,intent,changed,elapsed_ms}

目录不存在时自动创建。所有写入使用追加模式加行锁，进程崩溃不破坏既有行；文件滚动可按日切割（traces-YYYYMMDD.jsonl）。

五、能力对应
个性化词典：dictionary_add / dictionary_del / dictionary_list，匹配带边界校验，纠错仅替换命中条目，未命中文本不被改写。
文本纠错：先做空白与全半角清洗，再做词典纠错，保证正常文本零改动。
意图分类：NOTE QUERY EDIT COMMIT ORCHESTRATE 五类，规则加权，词典命中可加权。
反馈学习：rating 为 up/down 时追加写 feedback.jsonl，用于后续加权。
鉴权与回环：见第二节。

六、常见问题
端口被占用：换 -addr 端口。
启动报非回环地址：改回 127.0.0.1 或 ::1。
想看历史数据：直接按行读取 data 目录下对应 JSONL 文件，每行均为独立 JSON 对象。
清空数据：停止服务后删除 data 目录，重启自动重建。