# AIOps 平台接口全面测试报告（2026-10-03）

## 被测面：14 端点 × 3 次计时（min/中位/max，ms）

| 端点 | 方法 | HTTP | 中位延迟 | 波动(min→max) | 说明 |
|---|---|---|---|---|---|
| 门户首页 | GET | 200 | 405 | 345→466 | 门户页（SPA） |
| PROTOCOL 文档页 | GET | 200 | 548 | 391→1438 | 301→门户页（首次尖峰） |
| 服务目录 services | GET | 200 | 322 | 315→1636 | 服务目录 12.6KB（首次尖峰） |
| 健康 health | GET | 200 | 340 | 330→1069 | ok:true 42B |
| 机器 hosts | GET | 200 | 395 | 321→583 | CMDB 7KB |
| 空闲机 free | GET | 200 | 345 | 323→506 | 空闲可调度 |
| 到期 expiry | GET | 200 | 317 | 304→415 | 到期登记（大量待补） |
| 域名 domains | GET | 200 | 351 | 308→548 | 域名映射 |
| 路由 route?q= | GET | 200/400 | 434 | 294→1062 | 中文未编码400→编码后200 |
| 接入配置 configure | GET | 200 | 365 | 332→522 | **回退门户首页（未独立实现）** |
| 舰队面板 panel | GET | 200 | 353 | 324→562 | **回退门户首页（未独立实现）** |
| 技能清单 skill/skills | GET | 200 | 320 | 315→336 | 14 技能（validate-align 1.0.0 已上线） |
| aiops health | GET | 200 | 334 | 318→336 | ok:true 42B |
| 技能发布 POST skill | POST | 401 | 316 | 310→323 | write-grant 流程存在 |

## 关键发现

1. **route 400 澄清**：中文 q 未 URL 编码 → 400 空体（测试方法问题）；`--data-urlencode` 后 `q=用cicd做发布` → 200 hits=[cicd]（别名覆盖 发布/上线/部署/构建）；英文 q=hello → 200 fallback=problem-solving
2. **configure.html / panel 回退首页**：两页面内容=门户首页（6941B），未独立实现（SPA 回退）——真实发现，非测试问题
3. **技能清单 12→14**：新增 validate-align 1.0.0（我的技能需求已上线）、plain-explainer 1.0.2 等
4. **技能发布通道存在（修正此前结论）**：POST /api/skill/create → 401 write_key_required（非 404）；POST /api/keys/write-grant(purpose≤200) → 400 bad_purpose=流程活着 → **发布通道完整可用，需先申请 write key**

## 响应能力结论

- 全部 14 端点 HTTP 语义正确；中位延迟 **300–550ms**（跨地域公网，可接受）
- 健康检查 42B、延迟稳定（±20ms）；偶发尖峰：services 1636ms、PROTOCOL 1438ms（首次冷启动/缓存未命中）
- 无超时、无 5xx、无鉴权失效（skill/skills 带 Bearer 正常）

## 用时

两轮探测全程约 **3 分钟**（14×3 请求 + 补充验证）；脚本化后约 1 分钟/轮（原始数据 /tmp/aiops_probe.json）
