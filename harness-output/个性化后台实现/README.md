个性化后台服务 运行说明

一、环境

Python 3.9+，无外部服务依赖。安装依赖后可直接启动，所有数据写入本地数据目录。

  pip install -r requirements.txt

二、启动命令

  python -m app.server --host 127.0.0.1 --port 8080 --data-dir ./data

参数说明
  --host      仅允许 127.0.0.1 / localhost / ::1，传入其他地址将启动失败
  --port      监听端口，默认 8080
  --data-dir  数据目录，默认 ./data，不存在时自动创建
  --token     鉴权占位令牌，也可用环境变量 APP_TOKEN 提供

环境变量等价写法
  APP_HOST=127.0.0.1 APP_PORT=8080 APP_DATA_DIR=./data APP_TOKEN=dev-token python -m app.server

启动成功会打印监听地址与数据目录路径。

三、HTTP 端点

1) 健康检查
   GET /v1/health
   响应: {"status":"ok","version":"0.1.0","data_dir":"./data"}

2) 文本处理（纠错 + 意图分类 + 词典匹配）
   POST /v1/process
   Header: Authorization: Bearer <token>
   Body: {"text":"...","session_id":"s1","user_id":"u1"}
   响应: {
     "intent":"NOTE|QUERY|EDIT|COMMIT|ORCHESTRATE",
     "corrected_text":"...",
     "changed": false,
     "dictionary_hits": [],
     "trace_id":"..."
   }
   说明：无把握时 corrected_text 与原文一致，changed=false，保证正常文本不被改坏。

3) 词典管理
   GET    /v1/dictionary            列出条目
   POST   /v1/dictionary            新增 {"term":"...","aliases":[],"replacement":"..."}
   DELETE /v1/dictionary/{term}     删除条目

4) 反馈学习
   POST /v1/feedback
   Body: {"trace_id":"...","verdict":"ok|bad","note":"..."}
   以 append-only 方式追加，不覆盖历史。

鉴权：除 /v1/health 外均需 Authorization 头，占位令牌校验失败返回 401。

四、数据文件（均位于 --data-dir，JSONL 追加写入，不重写）

  data/dictionary.jsonl   个性化词典条目，一行一条
  data/feedback.jsonl     ✔/✘ 反馈记录
  data/traces.jsonl       每次 /v1/process 的请求、纠错结果与意图
  data/usage.jsonl        端点调用计数与耗时

文件只追加不截断，可直接用 tail -f 观察，也可随时离线分析。

五、快速自检

  curl http://127.0.0.1:8080/v1/health
  curl -X POST http://127.0.0.1:8080/v1/process -H "Content-Type: application/json" -H "Authorization: Bearer dev-token" -d "{\"text\":\"明天下班前把周报提交\"}"
  tail -n 1 data/traces.jsonl

六、停止

前台运行时按 Ctrl+C；日志与数据文件保留，重启后续接同一 --data-dir 即可继承词典与反馈。