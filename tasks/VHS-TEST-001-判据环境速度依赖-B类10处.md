# VHS-TEST-001 · 判据的「环境速度依赖」—— B 类 10 处待修

> **来源**：2026-10-03 CI 红了 3 次，**没有一次是产品缺陷**，全部是判据自身依赖"环境速度"。
> **工具**：`sh scripts/timing_sensitive_scan.sh`（扫出 23 处）
> **分类**：见 §2 —— **扫描器只能缩小范围；"要不要改"取决于断言语义。**

---

## 1. 判定标准（**先读这一节，再决定改不改**）

```
· 断言要求「**已**过期 / **已**完成」   ⇒ **慢只会帮它** ⇒ ✅ 安全，**不用改**
· 断言要求「**仍未**过期 / **仍在**窗口内」 ⇒ **慢会害它**   ⇒ ⚠️ **危险，必须改**

依据：`cache/quad.go:225` 的 `Set` **每次都调** `evictLocked()`，
      后者**无条件先 `sweepExpiredLocked()`** ⇒ 条目在写入过程中就可能被清。
```

**⚠️ 正则看不出语义** ⇒ 扫出的 23 处**必须逐处读断言**才能定性（我一度把 C 类那处
误判为"改漏了"，实际它是"越慢越容易通过"）。

---

## 2. 三类（已逐个看过）

### A 类 · 轮询式等待 —— ✅ 正当（**有 deadline + 条件判断**）
```
hotcache/cache_criteria_test.go:164    for calls < 2 && time.Now().Before(deadline) { sleep(10ms) }
recog/l2assemble_criteria_test.go:222  for …Before(deadline) && !contains(logs,"L2") { sleep(50ms) }
recog/realbinary_criteria_test.go:125  同上
server/asrsim_http_test.go:96                       ⚠️ 疑似（未逐个确认）
server/server_test.go:134/201/219/250/282/324       ⚠️ 疑似（20ms × 6 · 未逐个确认）
```

### B 类 · 固定 sleep 等异步 —— ⚠️ **危险，本卡的目标（10 处）**
```
server/server_test.go:382   // 等任务 done          sleep(200ms)   ← 注释自证
server/server_test.go:414                          sleep(200ms)
server/server_test.go:464                          sleep(200ms)
server/server_test.go:497                          sleep(200ms)
server/server_test.go:667   // 等任务完成            sleep(300ms)   ← 注释自证
server/server_test.go:739   go func(){ sleep(100ms); postJSON(…cancel) }()   ← 时序耦合
asr/restart_criteria_test.go:48                    sleep(50ms)
asr/scopecriteria_test.go:166                      sleep(100ms)
recog/l2assemble_criteria_test.go:111              sleep(50ms)
recog/realbinary_criteria_test.go:93               sleep(50ms)
```

### C 类 · 要求"已过期" ⇒ ✅ 安全（**不用改**）
```
cache/r04_capacity_criteria_test.go:50  TestR04ExpiredEntriesAreSwept
  TTL=20ms · 塞 50 条 · sleep(60ms) · 断言 `Len()==0`
  ⇒ 慢机器上更多条目在塞的过程中就过期，而 60ms > 20ms ⇒ 最后一条也必过期
  ⇒ **越慢越容易通过** ⇒ **不是危险形态**（我曾误判为"改漏了"）
cache/quad_test.go:61 · cache/r04_capacity_criteria_test.go:88    已放大到 2.5s（余量足够）
```

---

## 3. 修法（**不是放大 sleep，而是取消固定等待**）

```
server_test.go:382/414/464/497/667 ⇒ **轮询**任务状态到 `done`
    形如：deadline := time.Now().Add(5*time.Second)
          for time.Now().Before(deadline) { if 条件成立 { break }; time.Sleep(10*time.Millisecond) }
          // 超时 ⇒ **明确 t.Fatal("超时未 done")**（而不是让断言莫名失败）
server_test.go:739 ⇒ 用**同步原语**（channel / WaitGroup）代替"后台 sleep 后发"
asr/* 2 处 · recog/* 2 处 ⇒ **先读断言语义**再决定（可能是 A 类）
⇒ 共同点：**把"等多久"从常量改成"等条件成立 + 超时上限"**
   ⇒ 快机器立刻过 · 慢机器等到条件成立 ⇒ **对速度免疫**
```

---

## 4. 已修（**先例，可照抄**）

```
d303a5c  cache/r04_capacity_criteria_test.go  TTL **50ms → 2s** · sleep **70ms → 2.5s**
         ⇒ 它曾在**纯文档提交**上红过（判据缺陷，非实现缺陷）
d6d0355  cache/quad_test.go  TTL **40ms → 2s** · sleep **80ms → 2.5s**
         ⇒ `timing_sensitive_scan.sh` 第一次跑就扫出它（**同类第二例**）
⚠️ 而这两处是"放大余量"的做法 ⇒ **代价：cache 包测试从 ~0.4s 变 5.958s**（门禁变慢）
   ⇒ 若嫌慢 ⇒ 改为**注入可控时钟**（需改产品接口）⇒ **待 Peter 定**
```

---

## 5. ⚠️ 不能过早接入门禁

```
`timing_sensitive_scan.sh` 现在报 **23 处** ⇒ **接进 `gate.sh` 会重演"已知红弄红共享门禁"**
⇒ **正确顺序**：先按 §2 分类 ⇒ 修掉 B 类 ⇒ 再接入
⇒ （我今天的第 25 条教训：**判据先红 ≠ 共享门禁**）
```

---

## 6. 未确认（如实）

```
· ⚠️ **判定标准本身只在 C 类 1 处验证过** ⇒ 未在其余 22 处逐条套用
· ⚠️ A 类里 6 处 `server_test.go` 的 20ms **未逐个确认是轮询**（只看了上下文两行）
· ⚠️ B 类 10 处**未逐个看断言语义** ⇒ 其中可能有"要求已过期"的（那就不危险）
· ⚠️ 扫描器**是模式匹配** ⇒ `time.Sleep(d)` 用变量时**会漏**
· ⚠️ **修完是否真能让 CI 稳定，仍未验证**（本地复现不了慢；`test.14` 前 3 次 CI 绿只是方向一致）
```


---

## 7. ⭐ **方法修正（2026-10-03 实测后）：先"删掉跑 N 次"，再谈修法**

### 实验（`server/server_test.go:382`）
```
原版（含 `time.Sleep(200ms) // 等任务 done`）跑 **100 次** ⇒ `ok  21.524s` ⇒ **100/100 通过**
删掉那句 sleep 跑 1 次 ⇒ **第 1 次就失败**：
  `testing.go:1464: TempDir RemoveAll cleanup: unlinkat …/001: **directory not empty**`
```

### ⇒ 由此得到三点（其中一点推翻了我先前的推断）
```
① **那句 sleep 是必要的** ⇒ 我据"去重语义（提交即去重）"推断"它多余" ⇒ **错**
   错因：**去重不需要 done ≠ 测试不需要等 done**（它还要等**磁盘写入**结束）
② ⭐ **它掩盖了一个真实竞态**：测试**没等任务结束**就退出 ⇒ **后台 goroutine 仍在写盘**
   ⇒ 与 `t.TempDir()` 的自动清理**竞争**（失败形态就是"目录非空"）
③ ⚠️ **本地 100 次跑不出来**（200ms 在本地绰绰有余）⇒ **但慢 CI 上 200ms 可能不够**
   ⇒ **同一个失败会出现** ⇒ **所以它确实是 B 类（危险），只是失败率 <1%**
```

### ⇒ 所以修法要**修正**（我先前写的是"轮询到 `done`"）
```
⚠️ **`status=done` 可能不够** —— 失败是"**目录非空**"，说明**盘上仍有写者**
⇒ 正确条件应是「**任务真正结束且不再写盘**」
   ⇒ 而这需要**先读服务端的任务生命周期**（done 与 goroutine 退出的关系）⇒ **我未读**
⇒ 故修法应写成：**等到"可观测的终态" + 兜底超时**，并在条件不明时**先读生命周期再改**
```

### ⭐ 而这一节最重要的是一条**方法**
```
**"删掉它跑 N 次" 比 "读代码猜它是否必要" 有力得多。**
⇒ 我上一轮据"去重语义"推断"它多余" ⇒ **错**
⇒ 删掉跑一次 ⇒ **立刻看到它掩盖着什么**（`directory not empty`）
⇒ ⇒ **这就是"判据先红"用在"判据自身"上的样子。**
⇒ **建议**：其余 9 处 B 类**都按此法办**（临时删掉 ⇒ 跑 N 次 ⇒ 看露出什么），
   而不是逐个"读断言语义"猜。
```

### 未确认（如实）
```
· ⚠️ **未核** `status=done` 是否等价于"不再写盘"（修法的关键）
· ⚠️ 失败率**未量化**（本地 <1/100）⇒ 无法估计 CI 上的概率
· ⚠️ **未核**其余 9 处是否也有这种"掩盖竞态"的性质（**可能不止是"等得不够"**）
· **未改任何代码** ⇒ 实验后已恢复（工作区干净）
```


---

## 8. ⚠️ **分类修正**：`asr/restart_criteria_test.go:48` 是 **A 类**，不是 B 类

### 证据（看**外层循环**才看得出）
```go
asr/restart_criteria_test.go:40-50
deadline := time.Now().Add(10 * time.Second)
for time.Now().Before(deadline) {                 // ← **deadline 循环**
    if resp, err := http.Get(base + "/v1/health"); err == nil {
        if resp.StatusCode == 200 { return cmd }   // ← **条件成立即返回**
    }
    time.Sleep(50 * time.Millisecond)              // ← **扫描器命中的就是这一行**
}
_ = cmd.Process.Kill()
t.Fatal("真进程未在 10s 内就绪")                    // ← **有明确超时失败**
```
**⇒ 教科书式轮询（deadline + 条件 + 明确超时）⇒ **A 类，正当，不用改** ✅**

### 我错在哪
```
我在本卡 §2 用 `sed -n "$((L-1)),$((L+1))p"` 看**上下文两行**⇒ **看不到外层 deadline 循环**
⇒ 于是把 A 类判成了 B 类
⇒ ⇒ **教训：判定 A/B 类必须看到"外层有没有带 deadline 的循环"，两行不够。**
```

## 9. ⚠️⚠️ 由此得出**扫描器的根本局限**（重要）
```
`timing_sensitive_scan.sh` 只看"**短时长**"，**看不到"是否在带 deadline 的循环里"**
⇒ 它把「**轮询里的 50ms**」与「**固定等待的 50ms**」**混为一谈**
⇒ ⇒ **它报的 23 处被高估** —— 其中**大部分可能是 A 类（正当轮询）的循环内 sleep**
⇒ ⇒ **所以它的价值只是"缩小范围"，不是"给结论"**；
   **"是否危险"必须读上下文（外层循环 + 断言语义），而正则做不到。**
```

## 10. 修正后的**待办口径**
```
**不要**按"23 处"去做
⇒ 正确做法：
   ① 对每处**看外层循环**：有 deadline ⇒ A 类，**跳过**
   ② 无外层循环 ⇒ 再看**断言语义**：要求"已…"⇒ 安全；要求"仍未…"⇒ B 类
   ③ B 类再**用 §7 的方法**（删掉跑 N 次）看它掩盖了什么
⇒ **已确认的 B 类**（看过断言/注释的）：
   · `server/server_test.go:382`（**已用 §7 方法验证**：删掉即失败，TempDir 非空）
   · `server/server_test.go:667`（注释自证"等任务完成"）· `:739`（`go{sleep;cancel}`）
   · `server/server_test.go:414 / 464 / 497`（同族，**未逐个验证**）
⇒ **未确认类别的**：`asr/scopecriteria_test.go:166` · `recog/l2assemble:111` · `recog/realbinary:93`
   （⚠️ 其中**可能也有 A 类** —— 与 `asr/restart:48` 同族）
```

## 11. 未确认（如实）
```
· ⚠️ **§2 的分类已知有一处错**（`asr/restart:48`）⇒ **其余同类错可能存在**
· ⚠️ 我**未逐个用"看外层循环"重分类那 23 处** ⇒ **B 类真实数量未知**（远少于 10）
· ⚠️ **未改任何代码**
```


---

## 12. ✅ **收敛结果（最终口径）：B 类 = 7 处**（用 `timing_sensitive_scan.py`）

### 工具（**可复现**）
```bash
python3 scripts/timing_sensitive_scan.py     # 退出码 1（有 B 类）
⇒ 合计 23 处 · **A 类 16 · B 类 7**
```

### **B 类 7 处（本卡的真实目标）**
```
cache/quad_test.go:61                    2500ms   ← **我自己放的**（余量足够，可不动）
server/server_test.go:382                 200ms   ← **§7 已用"删掉跑 N 次"验证**（删掉即失败）
server/server_test.go:414                 200ms
server/server_test.go:464                 200ms
server/server_test.go:497                 200ms
server/server_test.go:667                 300ms   ← 注释自证"等任务完成"
server/server_test.go:739                 100ms   ← `go { sleep(100ms); cancel }`
⇒ **净待看：6 处**（`cache/quad_test.go:61` 余量已足够）
```

### A 类 16 处（**正当轮询，不用改**）
```
含 `asr/restart_criteria_test.go:48`（**deadline 循环**）·
   `server/server_test.go:134/201/219/250/282/324`（**计数式有界循环** `for i := 0; i < 50; i++` + break）
   · `hotcache/cache_criteria_test.go:164` · `recog/l2assemble:222` · `recog/realbinary:125` · `server/asrsim_http:96` …
```

### ⚠️ 而"7"这个数字**也是工具给的**，仍需人确认
```
· **A 类**仍需确认「条件成立时会真的退出」（不是空转）
· **B 类**仍需：① 看**断言语义**（要求"已…"⇒ 慢帮它 ⇒ 安全；"仍未…"⇒ 慢害它 ⇒ 危险）
              ② 用 **§7 的方法**（删掉跑 N 次）验证它掩盖了什么
⇒ ⚠️ 关键词表**仍不完整**（我补了 3 条线索才把 6 处误报归位）⇒ **可能还有别的轮询写法没覆盖**
⇒ ⚠️ lookback **25 行**可能不够（外层循环更远 ⇒ 漏判为 B 类）
```

### ⭐ 这个数字被修正了**三次**（每次靠多读一段代码，没有一次靠更好的推理）
```
23 处 ⇒（补 deadline/同步原语关键词）⇒ **A 8 · B 15** ⇒（认出"计数式有界循环"也是轮询）⇒ **A 16 · B 7**
⇒ 而我**手工核过 3 处**都**与工具一致**：
   `:382`（§7 删掉跑 N 次 · 删掉即失败）· `:134`（计数式轮询）· `asr/restart:48`（deadline 轮询）
```

### 未确认（如实）
```
· ⚠️ **A 类 16 处未逐个确认**「条件成立会退出」· **B 类 7 处未逐个看断言语义**
· ⚠️ 工具**仍未接入 gate**（B 类未清 ⇒ 接进去会重演"已知红弄红门禁"）
· ⚠️ `cache/quad_test.go:61` 的 2.5s **让 cache 包从 0.4s 变 ~6s** ⇒ 若嫌慢需注入可控时钟（待 Peter 定）
· **未改任何代码**
```
