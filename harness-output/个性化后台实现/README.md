个性化后台服务 —— 运行说明

一、环境与构建
需要 Go 1.21 及以上版本。在项目根目录执行：

go build -o p13n .

如需直接运行而不产出二进制：

go run .

二、启动命令
默认监听 127.0.0.1:8080，数据目录为 ./data：

./p13n

常用参数（环境变量同名，命令行优先）：

-port 8080          监听端口
-data-dir ./data    数据目录，不存在时自动创建
-token 123456       占位鉴权令牌，为空则不校验

示例：

./p13n -port 9000 -data-dir ./var/p13n -token mytoken

服务强制只监听回环地址。若绑定到非 127.0.0.1 的地址（含 0.0.0.0、局域网 IP），启动即失败并退出，不会降级为可访问状态。

三、鉴权
除 /v1/health 外，所有端点均需携带请求头：

Authorization: Bearer <token>

未配置 -token 时跳过校验（占位实现，仅供本地调试）。

四、HTTP 端点

1）健康检查
GET /v1/health
响应：{"ok":true,"version":"...","data_dir":"..."}
无需鉴权。

2）主处理端点
POST /v1/process
请求头：Content-Type: application/json
请求体示例：
{
  "text": "明天下午三点开会",
  "feedback": null,
  "trace_id": "可选，缺省自动生成"
}

字段说明：
text      必填，待处理原文本
intent    可选，显式指定则跳过分类
feedback  可选，"up"/"down"，为空表示无反馈
trace_id  可选，用于串联同一请求的落盘记录

响应体示例：
{
  "trace_id": "...",
  "intent": "NOTE",
  "text": "清洗并纠错后的文本",
  "origin": "原始输入",
  "corrected": true,
  "dict_hits": [{"from":"...","to":"..."}],
  "elapsed_ms": 3
}

intent 取值仅限五类：NOTE、QUERY、EDIT、COMMIT、ORCHESTRATE。无法判定时回落为 NOTE，不会返回空值或未知类别。

3）词典管理
GET    /v1/dict            列出全部条目
GET    /v1/dict?q=关键词   查询/模糊匹配条目
POST   /v1/dict            新增条目，体：{"from":"错词","to":"正词"}
DELETE /v1/dict?from=错词   删除条目

词典增删在落盘成功后才生效；重复 from 视为更新而非新增。

4）反馈回执（可选便捷入口，等价于 process 带 feedback）
POST /v1/feedback
体：{"trace_id":"...","feedback":"up"}

五、数据文件（全部位于 -data-dir 指定目录，JSONL，append-only）
dictionary.jsonl    词典条目变更流水（新增/删除），启动时回放重建内存词典
traces.jsonl        每次 /v1/process 的输入、意图、输出、耗时
usage.jsonl         端点调用计数与结果状态
feedback.jsonl      ✔/✘ 反馈记录，只追加、不改写、不删除

以上文件按行追加写入，每行一个完整 JSON 对象。文件不截断、不重写，历史可通过逐行回放还原。目录内另有一个内存词典快照文件，仅为加速启动，删除后可由 dictionary.jsonl 完整重建。

六、纠错与安全约束
1）纠错仅依据词典条目做精确替换，不做自由改写。
2）先清洗（去零宽字符、归一化空白）再纠错，顺序固定。
3）命中条目为空或替换后长度异常（超过原长 3 倍）时跳过该条，防止改坏正常文本。
4）未命中词典的文本原样返回，corrected 为 false。
5）词典删除后，该条不再参与后续纠错，但 traces 中的历史记录保持不变。

七、快速自检
启动后用以下顺序验证：

curl -s http://127.0.0.1:8080/v1/health
curl -s -X POST http://127.0.0.1:8080/v1/dict -d '{"from":"测试1","to":"测试一"}'
curl -s -X POST http://127.0.0.1:8080/v1/process -d '{"text":"测试1开始"}'

预期：健康检查返回 ok 为 true；process 返回的 text 为“测试一开始”，corrected 为 true，dict_hits 非空。随后检查 data-dir 下 traces.jsonl、usage.jsonl 均已新增对应行。

八、端口占用排查
启动报 bind 错误时，确认端口未被占用：

lsof -i :8080

换端口启动即可，无需改配置。