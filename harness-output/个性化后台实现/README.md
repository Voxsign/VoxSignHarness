个性化后台实现 — 运行说明

一、构建与启动
  go build -o pserver .
  ./pserver --addr=127.0.0.1:8080 --data-dir=./data

  开发模式可直接：go run . --data-dir=./data

启动参数
  --addr      监听地址，默认 127.0.0.1:8080；非回环地址一律拒绝启动
  --data-dir  数据目录，默认 ./data，不存在时自动创建
  --token     鉴权占位令牌，可选；设置后请求需带
              Authorization: Bearer <token>

二、HTTP 端点
  GET  /v1/health
       返回 {"status":"ok"}，不鉴权，用于存活探针

  POST /v1/process    Content-Type: application/json
       请求示例：
       {"text":"明天三点开个会","op":null,"feedback":null}
       {"op":"dict_add","term":"开个会","value":"开会"}
       {"op":"dict_del","term":"开个会"}
       {"op":"dict_list"}
       {"feedback":"up"}   /  {"feedback":"down"}

       响应示例：
       {"intent":"NOTE","corrected":"明天三点开个会",
        "matches":[],"feedback_logged":false}
       intent 取值：NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE

三、数据文件（位于 data-dir 下，全部 append-only JSONL）
  dictionary.jsonl  个性化词典条目，增/删/查均以追加记录体现
                    （删除写墓碑记录，不物理删行）
  traces.jsonl      每次 /v1/process 的处理轨迹
  usage.jsonl       调用与用量计量
  feedback.jsonl    ✔/✘ 反馈落盘，只追加、不覆盖

四、行为约定
  纠错仅在命中性词典条目时替换，未命中的正常文本原样返回
  反馈仅写入 feedback.jsonl，不回改历史记录
  进程重启后在原文件末尾继续追加，历史记录保留