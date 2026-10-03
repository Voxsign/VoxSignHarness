# 线 C 接线 · 判据先红记录（2026-10-03）

> **依据**：`docs/测试方案-线C-技能层.md`（判据定稿 C-01..C-04，前置依赖「接线，实现方 deepseek」）
> **本文是该方案的第一步：「判据先红」的实测记录。**

## 任务输入（技能类长程任务）

```
POST /v1/run  {"text":"调用校准与对齐技能，对 VoxSign 做一次校准，输出校准报告"}
```
**真装配**：线 A（`main.go serve`，:8765）+ 线 B（`cmd/vhs-asr`，:8123）同时活。

## 观测（**正确的观测点**）

```
POST /v1/run  ⇒ 202 + {"task_id":"task-1791010462168153000"}   ← **只是受理，不含结果**
GET  /v1/tasks/task-1791010462168153000  ⇒ 5s 后 status=**done**
   顶层键 = ['attribution','receipt','role','status','task_id']   （440 bytes）
```
⚠️ **注意**：若只在 `/v1/run` 的响应上判，会误以为"响应里没有就是红" ——
   而那是**异步受理**，结果在 `/v1/tasks/{id}`。**观测点错了会得出错误的红/绿。**

## 四态判定（全部先红）

| 判据 | 方案定义 | 实测 | 判定 |
|---|---|---|---|
| **C-01 技能发现** | 规划/元数据中出现「技能域匹配」记录（含技能域名+置信） | 无 `skill` / `技能` / `domain` 字样 | **✗ 红** |
| **C-02 技能选择** | 选中技能 ID 与任务语义匹配，清单来源=技能系统 | 无选中技能 ID | **✗ 红** |
| **C-03 技能调用** | 执行段**实际调用技能（非降级规则式）** | 走了普通路径；方案红线：**降级不算** | **✗ 红** |
| **C-04 证据留痕** | traces/日志含 发现→选择→调用→产出 | `traces` / `trajectory` **均不在 task 详情里** | **✗ 红** |

## 关键输入（第 11 章不触发）

```
AIOPS_KEY ⇒ 有（长度 46）· Bearer ✅ · X-AIops-Key ✅（两种都通）
GET https://aiops.peterzou.com/api/skill/skills ⇒ **200** {"ok":true,"count":**13**}
```

## 顺带发现（文档与实现不一致，待核）

```
skill/fetch.go:34                Authorization: Bearer <key>
skill/knowhow_fixture_test.go:3  …（Header **X-AIops-Key**）
⇒ 实测两种都通 ⇒ 不是 bug，但**注释与实现不一致，会误导后来者**
```

## 接线前的代码现状（C 证据）

```
`skill/` 包完整：select.go · store.go · fetch.go · judgement.go · evidence.go · basis_map.go
                + 8 个 *_criteria_test.go（判据齐全）
而**没有任何非测试文件 import 或引用它**：
  git grep '"voicesign-harness/skill'  ⇒ 零
  git grep 'skill\.'      (非测试)     ⇒ 0
⇒ **层做完了，线没接**（这是已登记的待办，不是新发现的缺口）
```

## 下一步（接线）

```
1. `tools/skill` → `pipeline` 执行路径（fetch → select → execute → evidence 链）
2. 技能清单**必须来自技能系统**（已实测可达，13 条）
3. 红线：**不许桩注入**（真技能、真产出）· **降级不算**
4. 接线后**用本文同一任务重放** ⇒ 四条应转绿（**同一把尺子**）
```
