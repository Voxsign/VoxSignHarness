# 域（Space）设计提案 v0.1 —— 权限与边界的实施单元

- 日期：2026-10-02 · 目的：供 Codex / Cursor / Claude 三个高级模型评审
- 背景：OPC 一人公司，人少盯不过来；权限/边界不能散成"路径+权限列表"，需要**域**作为边界单元

---

## 一、核心思想
**域 = 受边界约束的工作空间（namespace 演进版）。一句话"在哪个域干活"，自动套上整套边界。**

类比：软件模块 = 代码的边界单元；**域 = 执行的边界单元**。模型在域内自由，域外拦截（BOUNDARY_VIOLATION）。

## 二、域定义（space manifest，代码可执行配置）
```json
{
  "name": "voicesign-harness",
  "type": "project",
  "scope": ["/path/voicesign-harness/**"],
  "tools": ["search","file","git","test","verify"],
  "perms": { "read": true, "write": true, "exec": ["go build","go test"] },
  "contracts": ["file@1.0","git@1.0"],
  "context": ["project-map:harness","decisions:harness"],
  "risk_default": "auto",
  "acceptance": ["go test ./..."]
}
```

## 三、域类型（五类起步）
| 类型 | 权限 | 场景 | 一句话 |
|---|---|---|---|
| global | 只读兜底 | 默认域 | "查一下 X" |
| project | 可读可写+契约+认知切片 | 开发 | "在 voicesign-harness 里改 X" |
| sandbox | 临时隔离+自动清理 | 实验 | "搭个 demo 试试" |
| vault | 只读/追加 | 想法库/凭证 | "记一条想法" |
| external | 永远强确认 | 部署/发布/发消息 | "发布到服务器" |

## 四、域与已有机制的结合（不推翻已有设计，域是其容器）
- **边界五轴** = 域 manifest 的五个字段（scope/non_goals/perms/acceptance/context）
- **风险分级三信号**：域 risk_default + 可逆性 + 影响面
- **三元组缓存**：{意图, 域, 权限}——"不再问"按域记忆，换域重新问
- **认知注入**：只注入本域 context 切片（解决注入膨胀）
- **工具契约注册**：工具声明"允许哪些域调用"，出域调用拦截
- **权限判断由代码做**：域 manifest 是可执行配置；模型只建议域选择，不决定权限

## 五、一句话用法（意图识别）
"在 voicesign-harness 里把错误提示改中文"
→ 意图解析出 space=voicesign-harness → 自动带 scope/工具白名单/perms/context/acceptance → 边界=域定义

## 六、待评审问题
1. 域作为边界单元，比"路径+权限列表"好在哪？坑在哪？
2. 域的数量/粒度怎么控制？OPC 一人怎么管域（太多=负担，太少=模糊）？
3. 域与工具契约、认知切片、风险分级结合的最干净方式？
4. "在 X 域干活"的一句话意图解析怎么做？域识别错的风险与兜底？
5. 实施位置建议：放在详细设计 v2 的哪一节？M2 哪个模块先落地（建议：space 注册表 + 意图解析加 space 字段 + 执行前域校验）？
