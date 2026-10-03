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
