#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""vhs-validate.py — VoxSign Harness × iOS 自动验证器（validate-align R 通道落地）。

用法：
  python3 scripts/vhs-validate.py            # 默认连 129:8897，跑全部后台判据
  python3 scripts/vhs-validate.py --base http://192.168.8.129:8897 --token m7-token
  python3 scripts/vhs-validate.py --group remote   # 只跑远程控制组

判定：
  PASS = 任务 status==done 且回执命中断言关键字（need_ask 仅对设计回问判据算 OK）
  FAIL = 其余（running 超时 / need_ask 非预期 / 关键字缺失）
输出：对齐报告（PASS 率 / 偏差清单），判据↔证据链，JSON 一行可归档。
"""
import argparse
import json
import sys
import time
import urllib.request

BASE = "http://192.168.8.129:8897"
TOKEN = "m7-token"

# ── 后台能力判据（每条 = 需求 → 触发文本 → 期望断言）──
CRITERIA = [
    # 远程控制（J/K 系列：契约→真实执行器闭环）
    {"id": "R1", "group": "remote", "req": "能力询问命中执行器，不得回'无法控制'",
     "text": "能够控制后台的电脑吗", "expect": ["桌面共"], "ok_need_ask": False},
    {"id": "R2", "group": "remote", "req": "远程控制列真实桌面文件",
     "text": "远程控制电脑看看桌面上有什么", "expect": ["桌面共"], "ok_need_ask": False},
    {"id": "R3", "group": "remote", "req": "远程控制执行白名单只读命令",
     "text": "远程控制电脑运行 ls -la /tmp", "expect": ["命令「"], "ok_need_ask": False},
    {"id": "R4", "group": "remote", "req": "远程控制列出真实运行进程",
     "text": "远程控制电脑有哪些应用在跑", "expect": ["进程"], "ok_need_ask": False},
    {"id": "R5", "group": "remote", "req": "远程控制真实截屏并反馈路径大小",
     "text": "远程控制电脑截个图", "expect": ["已截图"], "ok_need_ask": False},
    # 意图/执行（I 系列：确认放行、越界、时间词）
    {"id": "R6", "group": "intent", "req": "写文件意图真实执行（把 XX 写到 /path）",
     "text": "把 自动验证器测试文件 写到 /tmp/vhs-va-确认放行.txt", "expect": ["writed"], "ok_need_ask": False},
    {"id": "R7", "group": "intent", "req": "写意图无路径时明确报错而非 UNKNOWN 回问",
     "text": "把内容写到没有注册的项目里", "expect": ["FAILED", "未识别要写入的文件路径"], "ok_need_ask": False},
    {"id": "R8", "group": "intent", "req": "时间查询触发词（现在几点了）",
     "text": "现在几点了", "expect": [], "ok_need_ask": False, "assert": "time"},
    # 话术/轨迹（H 系列 + dd6ff76）
    {"id": "R9", "group": "speech", "req": "抱怨/元反馈归人话回答，不回问",
     "text": "就为什么老是限制呢", "expect": [], "ok_need_ask": False, "assert": "human"},
    {"id": "R10", "group": "trace", "req": "early-return 出口补 final 轨迹（审计闭环）",
     "text": "自动验证器测试不存在的功能XYZ", "expect": [], "ok_need_ask": True, "assert": "final"},
    # 自举注册（09020a8：能力自迭代第一段）
    {"id": "R11", "group": "selfheal", "req": "语音能力自举注册落盘契约",
     "text": "你必须增加一个查看天气的能力", "expect": [], "ok_need_ask": False, "assert": "contract"},
]

TRACE = "/tmp/vhs-m7/log/trajectory-20261004.jsonl"
CONTRACTS = "/tmp/vhs-m7/log/contracts"


def post(base, path, body):
    req = urllib.request.Request(
        base + path, data=json.dumps(body).encode(),
        headers={"Content-Type": "application/json", "Authorization": f"Bearer {TOKEN}"})
    return json.loads(urllib.request.urlopen(req).read())


def get(base, path):
    req = urllib.request.Request(base + path, headers={"Authorization": f"Bearer {TOKEN}"})
    return json.loads(urllib.request.urlopen(req).read())


def run_criterion(c, base):
    r = post(base, "/v1/tasks", {"text": c["text"], "mode": "voice"})
    tid = r.get("task_id")
    if not tid:
        return {"pass": False, "evidence": f"无 task_id: {r}"}
    status, receipt = None, ""
    for _ in range(12):  # ≤ 24s
        time.sleep(2)
        d = get(base, f"/v1/tasks/{tid}")
        status = d.get("status")
        receipt = str(d.get("receipt", ""))
        if status in ("done", "need_ask", "failed"):
            break
    # 断言
    if status == "done":
        hit = all(e in receipt for e in c.get("expect", [])) if c.get("expect") else True
        return {"pass": hit, "evidence": f"done | {receipt[:120]}"}
    if status == "need_ask" and c.get("ok_need_ask"):
        return {"pass": True, "evidence": f"need_ask（设计回问）| {receipt[:80]}"}
    if status == "need_ask" and c.get("assert") == "final":
        # 轨迹 final 检查（审计判据单独走轨迹）
        return check_trace(c, tid)
    return {"pass": False, "evidence": f"{status} | {receipt[:120]}"}


def check_trace(c, tid):
    try:
        for line in open(TRACE, encoding="utf-8"):
            rec = json.loads(line)
            if rec.get("request_id") == tid and rec.get("kind") == "final":
                return {"pass": True, "evidence": "final 轨迹存在"}
    except FileNotFoundError:
        return {"pass": False, "evidence": "轨迹文件缺失"}
    return {"pass": False, "evidence": "无 final 轨迹记录"}


def contract_registered(name_kw):
    import glob, os
    for p in glob.glob(os.path.join(CONTRACTS, "*.contract.json")):
        try:
            if name_kw in json.load(open(p)).get("name", ""):
                return p
        except Exception:
            pass
    return None


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--base", default=BASE)
    ap.add_argument("--token", default=TOKEN)
    ap.add_argument("--group", default="all")
    args = ap.parse_args()
    crits = CRITERIA if args.group == "all" else [c for c in CRITERIA if c["group"] == args.group]

    results = []
    for c in crits:
        # 特判：R11 契约落盘 / R8 时间 / R9 人话
        if c.get("assert") == "contract":
            p = contract_registered("天气")
            results.append({"id": c["id"], "pass": bool(p), "evidence": p or "契约未落盘"})
            continue
        if c.get("assert") == "time":
            # 时间判据：done 即过（触发词命中即有效，不校验具体值）
            r = post(args.base, "/v1/tasks", {"text": c["text"], "mode": "voice"})
            tid = r.get("task_id")
            st = None
            for _ in range(8):
                time.sleep(2)
                st = get(args.base, f"/v1/tasks/{tid}").get("status")
                if st in ("done", "need_ask", "failed"):
                    break
            results.append({"id": c["id"], "pass": st == "done", "evidence": st})
            continue
        if c.get("assert") == "human":
            r = post(args.base, "/v1/tasks", {"text": c["text"], "mode": "voice"})
            tid = r.get("task_id")
            rc = ""
            for _ in range(12):
                time.sleep(2)
                d = get(args.base, f"/v1/tasks/{tid}")
                rc = str(d.get("receipt", ""))
                if d.get("status") in ("done", "need_ask", "failed"):
                    break
            human = any(k in rc for k in ("抱歉", "限制", "拆成", "回答"))
            results.append({"id": c["id"], "pass": d.get("status") == "done" and human,
                            "evidence": f"{d.get('status')} | {rc[:100]}"})
            continue
        results.append({"id": c["id"], **run_criterion(c, args.base)})

    total, passed = len(results), sum(1 for r in results if r["pass"])
    print(f"\n=== VoxSign 自动验证报告（validate-align R 通道）===")
    print(f"判据 {passed}/{total} 通过 | 对齐度 {round(passed / total * 100)}%")
    print(f"{'判据':<6} {'结果':<4} 证据")
    for r in results:
        mark = "✅" if r["pass"] else "❌"
        print(f"{r['id']:<6} {mark:<3} {r['evidence'][:110]}")
    fails = [r for r in results if not r["pass"]]
    if fails:
        print(f"\n偏差清单（{len(fails)} 项）：")
        for f in fails:
            print(f"  - {f['id']}: {f['evidence'][:110]}")
    print("\n判定口径：PASS=status done 且断言命中；need_ask 仅对设计回问判据算 OK。")
    print(json.dumps({"total": total, "passed": passed, "results": results}, ensure_ascii=False))


if __name__ == "__main__":
    main()
