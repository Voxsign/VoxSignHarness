# NF-1 性能债 · 索引化 `lookupInternal`（侦察记录 2026-10-03）

> **状态**：判据已立（`hotcache/nf1_alloc_criteria_test.go`，**当前 SKIP = 已知未修**）· **实现未做**。
> **依据**：Peter `docs/校准报告-产品与实现-L01.md` §6 修订项 1（NF-1 性能）。

## 1. 已坐实的根因（三条证据链闭合）

```
① 长度扫描（端到端 P50 随输入长度线性增长，约 110ms/字；32 字 ⇒ 3530ms）
② traces（`/v1/process` 的 `hotcache` step = **743.468ms**，占 99.99%；其余四步合计 0.044ms）
③ 代码：
   `recog/rewriter.go:90 pass1`：每位置试 **5 个窗口长度**（6,5,4,3,2）⇒ **5n 次** `LookupForRewrite`
   `hotcache/cache_impl.go:82 lookupInternal`：**4 遍全量别名遍历**
      · :100  精确      遍历 s.aliases
      · :106  别名      遍历 s.aliases
      · :116  拼音近音  遍历 s.aliases
      · :129  混排      遍历 s.aliases **且每条算 2 次 `MixedKey`**
⇒ 合计 **5n × 4 × 别名数**（207 条）⇒ 8 字 743ms / 32 字 3530ms
```

## 2. 侦察结果（索引化的改动面）

**`aliases` 的写点（索引必须同步）**：
```
① hotcache/cache_impl.go:62   更新已有别名   s.aliases[i] = Alias{...}
② hotcache/cache_impl.go:66   追加新别名     s.aliases = append(...)
③ hotcache/cache_impl.go:282  批量替换       s.aliases[i] = a     ← 加载/刷新路径
```

**遍历点（要改成索引查找）**：
```
· :100  精确（a.Canonical == term）
· :106  别名（a.Alias == term）
· :116  拼音近音（a.PinyinKey == key）
· :129  混排（MixedKey(a.Alias) / MixedKey(a.Canonical)）← 每条算 2 次
· :177  另一处 —— **未看**（下一轮第一件事）
```

## 3. 建议的实现形状（**未做**）

```
**不在写点手工维护索引**（3 个写点容易漏同步）⇒ 用**懒构建 + 失效标记**：
  · `Store` 加 `idx *aliasIndex` 与 `idxDirty bool`
  · 三个写点末尾置 `idxDirty = true`（各一行，不易漏）
  · `lookupInternal` 开头：若 `idxDirty` ⇒ 重建 `aliasIndex`（207 条，成本 ≪ 5n 次扫描）
  · `aliasIndex` 内容：`byCanonical map[string]Alias` · `byAlias map[string]Alias` ·
    `byPinyin map[string][]Alias` · `byMixed map[string][]Alias`（**预计算 MixedKey，去掉每条 2 次计算**）
⇒ 预期：743ms → **亚毫秒量级**（⚠️ **推算，未实测**）
⇒ 修好后 **删掉 `nf1_alloc_criteria_test.go` 里的 SKIP 分支** ⇒ 判据自动转真断言
```

## 4. 红线（沿用 today's 纪律）

```
· **判据先红不得弄红共享门禁** ⇒ 修好前保持 SKIP（含数字与根因），不许改回 t.Errorf
· 修完必须**真装配级验证**（起真进程，`/v1/process` 端到端 + traces 里 hotcache 的 ms）
· **`MixedKey` 的语义**（混排规范化）须先确认可预计算（它是否依赖外部状态）
```

## 5. 未确认（如实）

```
· `:177` 那处遍历**未看** ⇒ 可能还有第 5 遍
· `MixedKey` 可否预计算**未核**（若它依赖全局状态，索引化要改设计）
· 别名**并发写**（`PutAlias` 在锁内）与懒构建的交互**未评估**
· "743ms → 亚毫秒"是**推算**，未实测
· 别名条数**真实峰值未知**（我只见到 207 条）
```
