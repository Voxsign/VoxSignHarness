#!/usr/bin/env python3
"""V0 启动延迟采样：30 次运行 dist/vhs-darwin-arm64 version，输出 min/median/max。"""
import subprocess, time, statistics

bin_path = "./dist/vhs-darwin-arm64"
ts = []
for _ in range(30):
    t0 = time.perf_counter()
    subprocess.run([bin_path, "version"], stdout=subprocess.DEVNULL)
    ts.append((time.perf_counter() - t0) * 1000)
print(f"startup n=30  min={min(ts):.1f}ms  median={statistics.median(ts):.1f}ms  max={max(ts):.1f}ms")
