#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""e2e-ios-link.py — 模拟 iOS 客户端与 VoxSign Harness 的完整语音链路连调。

链路（与 App 内 APIClient/SSEClient 完全一致）：
  1. POST /v1/tasks            (text=语音识别结果, request_id)
  2. GET  /v1/tasks/{id}       轮询视图（App 主通道）
  3. 可选：SSE /v1/tasks/{id}/events 订阅阶段事件（App 的 ExecCard 数据源）
  4. 收尾：done → 回执；need_ask/need_confirm → 决策点（App 朗读/按钮）

用法：python3 e2e-ios-link.py "<语音命令文本>" [--events]
退出码：0=链路通(收到 done 回执)；1=链路异常。
"""
import json
import sys
import time
import urllib.request

BASE = "http://127.0.0.1:8897"
TOKEN = "m7-token"


def req(method, path, body=None, timeout=15):
    url = BASE + path
    data = json.dumps(body).encode() if body is not None else None
    r = urllib.request.Request(url, data=data, method=method)
    r.add_header("Authorization", f"Bearer {TOKEN}")
    if body is not None:
        r.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(r, timeout=timeout) as resp:
            return resp.status, json.loads(resp.read().decode())
    except Exception as e:
        return -1, {"error": str(e)}


def main():
    if len(sys.argv) < 2:
        print("用法: e2e-ios-link.py \"<语音命令>\" [--events]")
        sys.exit(1)
    text = sys.argv[1]
    want_events = "--events" in sys.argv

    print(f"[1/3] POST /v1/tasks  text={text!r}")
    rid = f"ios-e2e-{int(time.time()*1000)}"
    code, j = req("POST", "/v1/tasks", {"text": text, "request_id": rid})
    if code not in (200, 201, 202) or "task_id" not in j:
        print(f"  ✗ 提交失败 code={code} body={j}")
        sys.exit(1)
    tid = j["task_id"]
    print(f"  ✓ task_id={tid} status={j.get('status')} deduped={j.get('deduped')}")

    if want_events:
        print(f"[2/3] SSE /v1/tasks/{tid}/events（5s 快照，验证 ExecCard 数据源）")
        try:
            r = urllib.request.Request(BASE + f"/v1/tasks/{tid}/events")
            r.add_header("Authorization", f"Bearer {TOKEN}")
            with urllib.request.urlopen(r, timeout=5) as resp:
                chunk = resp.read(2048).decode(errors="replace")
                print(f"  ✓ SSE 收到: {chunk[:300]}")
        except Exception as e:
            print(f"  ~ SSE 快照结束（正常，非阻塞）: {e}")

    print("[3/3] 轮询视图（App 同款 fetchTask，最长 60s）")
    t0 = time.time()
    last = ""
    while time.time() - t0 < 60:
        code, v = req("GET", f"/v1/tasks/{tid}")
        if code != 200:
            print(f"  ✗ 轮询失败 code={code} {v}")
            sys.exit(1)
        st = v.get("status", "?")
        q = (v.get("question") or "")[:60]
        rc = (v.get("receipt") or "")[:120]
        line = f"  status={st}"
        if q:
            line += f"  question={q}"
        if rc:
            line += f"  receipt={rc}"
        if line != last:
            print(line)
            last = line
        if st in ("done", "need_ask", "need_confirm", "canceled", "interrupted", "error"):
            if st in ("done",):
                print(f"  ✓ 链路完成：done（回执={v.get('receipt')}）")
                sys.exit(0)
            if st in ("need_ask", "need_confirm"):
                print(f"  ✓ 链路到达决策点：{st} question={v.get('question')} options={v.get('options')}")
                sys.exit(0)
            print(f"  ✗ 链路异常终止：{st} {v.get('message', '')}")
            sys.exit(1)
        time.sleep(1)
    print("  ✗ 超时未完成")
    sys.exit(1)


if __name__ == "__main__":
    main()
