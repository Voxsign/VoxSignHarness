#!/usr/bin/env python3
# timing_sensitive_scan.py —— 找**依赖环境速度**的判据，并**区分 A/B 类**。
#
# ⚠️ 为什么有 .py 版本（2026-10-03）：
#   `.sh` 版本只看"短时长" ⇒ **把「轮询里的 50ms」与「固定等待的 50ms」混为一谈**
#   ⇒ 它报 **23 处**，而其中**大部分是正当轮询**（A 类）⇒ **数字被高估**，
#     我据此把 `asr/restart:48` 误判为 B 类（它其实是教科书式轮询）。
#   ⇒ **根因：判定 A/B 类必须看「外层有没有带 deadline 的循环」，而正则看不了。**
#   ⇒ 故本版**逐处看上层上下文**，给出 A/B 分类**建议**（仍需人读断言确认）。
#
# 分类口径：
#   A 类（正当）：外层有 `deadline` / `Before(deadline)` / `time.After(` ⇒ 轮询式，**不用改**
#   B 类（危险）：无外层 deadline ⇒ **固定等待** ⇒ 需再看断言语义 + 用"删掉跑 N 次"验证
#   ⚠️ 分类只是**建议** —— 「是否危险」最终取决于**断言语义**（要求"已…"还是"仍未…"）
#
# 用法：python3 scripts/timing_sensitive_scan.py
# 退出码：0 = 未发现 B 类；1 = **有 B 类建议**（列出）；2 = 装置不可用
import re
import subprocess
import sys

PATTERNS = [
    ("① 短 sleep（<200ms）", r"time\.Sleep\([0-9]+\s*\*\s*time\.(Milli|Micro)second"),
    ("② 短 TTL（<1s）传给 Open/New", r"(Open|New)\([^)]*[0-9]+\s*\*\s*time\.(Milli|Micro)second"),
    ("③ 耗时断言绝对阈值", r"(p99|p50|Latency|Elapsed|Duration)\s*[<>]=?\s*[0-9]+\s*\*\s*time\.(Milli|Micro)second"),
]
# ⚠️ 关键词表**不完整**：实测漏了「计数式有界循环」`for i := 0; i < 50; i++ { …; break }`
#   ⇒ 那是**教科书式轮询**（有界 + 条件退出 + 明确失败）⇒ **A 类**，却被判成 B 类
#   ⇒ 已补 `for i := 0; i < N` 与 `break` 两个线索；**但仍不完整**（正则看不了语义）
DEADLINE_HINT = re.compile(r"deadline|Before\(|time\.After\(|WaitGroup|<-ch|sync\.|for i := 0; i < [0-9]+|for attempt|break")
LOOKBACK = 25


def tracked_test_files():
    out = subprocess.run(["git", "ls-files", "*_test.go"], capture_output=True, text=True).stdout
    return [f for f in out.splitlines() if f]


# ⚠️ **白名单**：已人工判定为"慢只会帮它 / 余量已足够"的处，不再报红。
#   加白名单**必须附理由**（否则就是把"没修"藏起来）。
ALLOW = {
    # 原先 TTL=40ms 的修复；2.5s 与"塞 30 条"的耗时差 5 个数量级 ⇒ 余量足够，可不动。
    "cache/quad_test.go:61",
    # OBS-07 标定（2026-10-03 观察报告核对）：p99 5ms 为"量级判据"（实测噪声带 1.1-2.1ms，
    # 本地快路 40µs / 8 核压满 138µs，5ms = 2.4 倍噪声余量）。正确性由功能测试覆盖，
    # 本断言只防量级退化（>5ms 属量级异常）；慢 CI 需慢 35 倍以上才红，余量足够。
    "asr/engine_test.go:390",
}


def main():
    files = tracked_test_files()
    if not files:
        print("[timing] ❌ 无已跟踪的 *_test.go（或不在 git 仓）", file=sys.stderr)
        return 2

    hits = []  # (kind, path, lineno, text, is_a)
    for path in files:
        try:
            lines = open(path, encoding="utf-8", errors="ignore").read().splitlines()
        except OSError:
            continue
        for i, line in enumerate(lines):
            for kind, pat in PATTERNS:
                if re.search(pat, line):
                    ctx = "\n".join(lines[max(0, i - LOOKBACK):i + 1])
                    hits.append((kind, path, i + 1, line.strip(), bool(DEADLINE_HINT.search(ctx))))

    print("[timing] 扫描「依赖环境速度」的判据（逐处看上层上下文，给出 A/B 分类**建议**）")
    a = [h for h in hits if h[4]]
    b = [h for h in hits if not h[4]]

    print(f"\n── A 类（外层有 deadline / 同步原语）⇒ **正当轮询，不用改** —— {len(a)} 处")
    for kind, path, ln, text, _ in a:
        print(f"  {path}:{ln}  [{kind}]  {text[:80]}")

    allowed = [h for h in b if f"{h[1]}:{h[2]}" in ALLOW]
    b = [h for h in b if f"{h[1]}:{h[2]}" not in ALLOW]
    if allowed:
        print(f"\n── 白名单（已人工判定安全）—— {len(allowed)} 处")
        for _, path, ln, text, _ in allowed:
            print(f"  {path}:{ln}  {text[:70]}")
    print(f"\n── B 类（**无外层 deadline**）⇒ ⚠️ **固定等待，需逐个看断言语义** —— {len(b)} 处")
    for kind, path, ln, text, _ in b:
        print(f"  {path}:{ln}  [{kind}]  {text[:80]}")

    print(f"\n[timing] ---- 合计 {len(hits)} 处 · A 类 {len(a)} · **B 类 {len(b)}**")
    print("[timing] ⚠️ 分类只是**建议**：")
    print("         · A 类仍需确认它在**条件成立时会真的退出**（不是空转）")
    print("         · B 类要再看**断言语义**：要求「已…」⇒ 慢帮它（安全）；要求「仍未…」⇒ 慢害它（危险）")
    print("         · 危险的那些，**用「删掉它跑 N 次」验证**它掩盖了什么（见 tasks/VHS-TEST-001 §7）")
    if b:
        return 1
    print("[timing] ✅ 未发现 B 类（固定等待）")
    return 0


if __name__ == "__main__":
    sys.exit(main())
