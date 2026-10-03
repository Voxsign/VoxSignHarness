# 风险说明：`token_set` 与两条线的鉴权边界（2026-10-03）

> **触发**：目标 ④「`/v1/voice` 的 ③ `token_set` 风险说明」· Peter v2.3 §8 工程轴「安全 ❌ **ASR 无鉴权**」。
> **方法**：读代码（C 证据）+ **真跑对照**（R 证据）—— 按 `skills/validate-align/SKILL.md` 双证。

---

## 1. 结论（一句话）

> **线 A（harness）有鉴权；线 B（vhs-asr）没有。**
> ⇒ Peter v2.3 的「安全 ❌」**只在 `cmd/vhs-asr` 成立**，而 `server/` 侧是受控的。

---

## 2. 线 A（`server/server.go`）—— **有鉴权**

**C 证据**：
```
server/server.go:219-233   func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc
    if s.cfg.Server.Token == "" {
        if !isLoopback(r.RemoteAddr) { ⇒ 403 "缺 token 且非本机访问" }
    } else {
        tok := Bearer(r.Authorization) 或 X-Token
        if tok != s.cfg.Server.Token { ⇒ 401 "token 无效" }
    }
config/config.go:346-349   token 为空 且 bind 非本机 ⇒ **追加 Warning**（不是拒绝启动）
server/server.go:531       /v1/health 回显 "token_set": bool   ⇒ **可观测**
```

**R 证据（真跑对照，绑 `0.0.0.0:8765`）**：
```
从回环 127.0.0.1:8765/v1/health      ⇒ 200
从 LAN  192.168.8.129:8765/v1/health ⇒ **403** {"error":"缺 token 且非本机访问"}
```
**⇒ 非回环且未配 token ⇒ 被挡住。行为与代码一致。**

---

## 3. 线 B（`cmd/vhs-asr`）—— **无鉴权**

**C 证据**：
```
cmd/vhs-asr/main.go  ⇒ grep Token|auth|Authorization|Bearer|isLoopback ⇒ **零命中**
asr/*.go（非测试）    ⇒ 无 auth / 无 Token
```

**R 证据（真跑，绑 `0.0.0.0:8123`）**：
```
从回环 127.0.0.1:8123/v1/health      ⇒ 200
从 LAN  192.168.8.129:8123/v1/health ⇒ **200** {"contract_version":"1","status":"ok"}
```
**⇒ 任何能连到该端口的人都能调用 ASR 服务（`/v1/process` `/v1/dictionary` `/v1/feedback` …）。**

---

## 4. 风险评级（我的判断，待 Peter 裁）

| 线 | 鉴权 | 暴露面 | 风险 |
|---|---|---|---|
| **A** `server/` | ✅ token + 回环豁免 | 绑非回环时需 token | **受控**（但 token 未配时仅 Warning，**不拒绝启动** ⇒ 见 §5） |
| **B** `cmd/vhs-asr` | ❌ 无 | 绑非回环 ⇒ LAN 全开 | **高**：可被内网任意调用；产出落盘到 `$DATA`（含 `feedback.jsonl` 等） |

**⚠️ 而默认绑定是 `127.0.0.1`** ⇒ **默认配置下两条线都不出本机** ⇒
**风险只在"显式绑到非回环"时成立**（我的真跑正是显式绑 `0.0.0.0`）。

---

## 5. 附带发现（线 A 的一处**软约束**）

```
config/config.go:346-349
  token 为空 且 bind 非本机 ⇒ **只追加 Warning**，**不拒绝启动**
⇒ 即：**配置错了会警告，但照样起来** ⇒ 依赖人看见 Warning。
⚠️ 建议（待裁）：非回环 + 无 token ⇒ **拒绝启动**（fail-closed），而非仅警告。
   依据：本项目第一原则「域门禁默认拒绝」—— 网络暴露边界应与之一致。
```

---

## 6. 复现命令

```bash
LAN=$(ipconfig getifaddr en0)

# 线 B（无鉴权）
go build -o /tmp/vhs-B ./cmd/vhs-asr
VHS_ASR_ADDR="0.0.0.0:8123" /tmp/vhs-B &
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8123/v1/health   # 200
curl -s -o /dev/null -w '%{http_code}\n' http://$LAN:8123/v1/health        # **200** ← 无鉴权

# 线 A（有鉴权）
go build -o /tmp/vhs-A .
VHS_ADDR="0.0.0.0:8765" /tmp/vhs-A serve &
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8765/v1/health   # 200
curl -s -o /dev/null -w '%{http_code}\n' http://$LAN:8765/v1/health        # **403** ← 被挡
```

---

## 7. 未确认 / 未做（如实）

```
· 线 A 的对照**第一次失败**（用 `VHS_SERVER_BIND` 绑定被忽略 ⇒ LAN 返回 000 而非 403）
  ⇒ 我**没有**据此下结论；改用正确的 `VHS_ADDR` 后对照才成立。**如实记录这次失败。**
· **只测了 `/v1/health`**（一条 GET）⇒ 其它端点（`/v1/run` `/v1/voice` …）**未逐条测鉴权**
· `.env` 里**无 `VHS_TOKEN`** ⇒ **"有 token 时校验 Bearer/X-Token"这条路径未真跑**
  ⇒ 仅 C 证据（代码），**R 证据缺**
· 未评估：ASR 服务被内网调用后的**数据面风险**（产出落盘、可读性、是否含用户文本）
· 未与 Peter 对齐**修复优先级**（v2.3 把它列为 P2）
```

*DSH · 2026-10-03 · 双证：C（文件:行）+ R（真跑对照）*
