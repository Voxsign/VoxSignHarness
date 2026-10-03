#!/usr/bin/env node
// accept-a1a12.mjs —— A1–A12 完整验收（交接 §5.2）。
//
// 依赖：Chrome --headless=new --remote-debugging-port + Node ≥22 原生 WebSocket（CDP）。
// **零 npm 依赖。** 无 Chrome/node ⇒ accept.sh 已显式 skip（这里假定前置已满足）。
//
// 环境：VHS_ACCEPT_PORT（服务端口）· VHS_ACCEPT_DATA（数据目录）
//      VHS_ACCEPT_CDP_PORT（Chrome 调试口）· VHS_ACCEPT_CHROME（Chrome 路径）· VHS_ACCEPT_SRV_PID（服务 pid）
//
// 纪律：每一条都真驱动页面（点击/填值/等待），并回读**页面输出与落盘文件**；
// A12 全程收集 Runtime.exceptionThrown，最终必须为空。

import { spawn } from 'node:child_process';
import { mkdtempSync, readFileSync, readdirSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

const PORT = process.env.VHS_ACCEPT_PORT || '8133';
const DATA = process.env.VHS_ACCEPT_DATA || '';
const CDP_PORT = process.env.VHS_ACCEPT_CDP_PORT || '9333';
const CHROME = process.env.VHS_ACCEPT_CHROME || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';
const SRV_PID = process.env.VHS_ACCEPT_SRV_PID ? Number(process.env.VHS_ACCEPT_SRV_PID) : 0;
const BASE = `http://127.0.0.1:${PORT}`;

const results = [];
function pass(a) { results.push({ a, ok: true }); console.log(`  ✅ ${a}`); }
function fail(a, why) { results.push({ a, ok: false, why }); console.log(`  ❌ ${a}：${why}`); }
function skip(a, why) { results.push({ a, ok: null, why }); console.log(`  ⚠️ SKIP ${a}：${why}`); }

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function waitFor(fn, timeoutMs = 8000, step = 150) {
  const t0 = Date.now();
  for (;;) {
    let v = false, err = null;
    try { v = await fn(); } catch (e) { err = e; }
    if (v) return v;
    if (Date.now() - t0 > timeoutMs) throw new Error(`waitFor 超时（${timeoutMs}ms）${err ? '：' + err : ''}`);
    await sleep(step);
  }
}

// ---- 最小 CDP 客户端（原生 WebSocket，零依赖）----
class CDP {
  constructor(wsUrl) {
    this.ws = new WebSocket(wsUrl);
    this.id = 0;
    this.pending = new Map();
    this.exceptions = [];
  }
  async open() {
    await new Promise((res, rej) => { this.ws.onopen = res; this.ws.onerror = rej; });
    this.ws.onmessage = (ev) => {
      const m = JSON.parse(ev.data);
      if (m.id && this.pending.has(m.id)) {
        const { resolve, reject } = this.pending.get(m.id);
        this.pending.delete(m.id);
        if (m.error) reject(new Error(JSON.stringify(m.error))); else resolve(m.result);
      } else if (m.method === 'Runtime.exceptionThrown') {
        this.exceptions.push(m.params);
      }
    };
  }
  send(method, params = {}) {
    const id = ++this.id;
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject });
      this.ws.send(JSON.stringify({ id, method, params }));
    });
  }
  async eval(expression) {
    const r = await this.send('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true });
    if (r.exceptionDetails) throw new Error('页面异常: ' + JSON.stringify(r.exceptionDetails));
    return r.result ? r.result.value : undefined;
  }
  close() { try { this.ws.close(); } catch {} }
}

// ---- 启动 Chrome ----
const userData = mkdtempSync(join(tmpdir(), 'vhs-a12-chrome-'));
const chrome = spawn(CHROME, [
  '--headless=new', `--remote-debugging-port=${CDP_PORT}`, `--user-data-dir=${userData}`,
  '--no-first-run', '--no-default-browser-check', '--disable-gpu', 'about:blank',
], { stdio: 'ignore' });
process.on('exit', () => { try { chrome.kill('SIGKILL'); } catch {} });

async function getPageTarget() {
  for (let i = 0; i < 60; i++) {
    try {
      const list = await (await fetch(`http://127.0.0.1:${CDP_PORT}/json/list`)).json();
      const page = list.find((t) => t.type === 'page');
      if (page && page.webSocketDebuggerUrl) return page.webSocketDebuggerUrl;
    } catch {}
    await sleep(300);
  }
  throw new Error('Chrome 调试口不可达（--remote-debugging-port）');
}

// ---- 页面脚本桩：浏览器原生录音（headless 无语音服务 ⇒ 用桩驱动 onresult 路径）----
// 同时给 window.fetch 计数，用于 A3「不自动提交」断言。
const FAKE_REC = `
(() => {
  if (window.__vhsA12Stub) return; window.__vhsA12Stub = true;
  window.__fetchCount = 0;
  const orig = window.fetch;
  window.fetch = function (...a) { window.__fetchCount++; return orig.apply(this, a); };
  class FakeRec {
    constructor() { window.__recs = window.__recs || []; window.__recs.push(this); }
    start() {
      const self = this;
      setTimeout(() => {
        if (self.onresult) self.onresult({ resultIndex: 0, results: [[{ transcript: '查一下库存' }]], length: 1 });
        if (self.onend) self.onend();
      }, 80);
    }
  }
  window.webkitSpeechRecognition = FakeRec;
})();
`;

const btns = `[...document.querySelectorAll('button')].map(b => (b.textContent||'').trim())`;
const clickBtn = (needle) => `(() => { const b = [...document.querySelectorAll('button')].find(b => (b.textContent||'').includes(${JSON.stringify(needle)})); if (!b) return false; b.click(); return true; })()`;
const stepsCount = `(() => { const rows = document.querySelectorAll('#planout table tr').length; return rows > 0 ? rows - 1 : 0; })()`;
const outText = `document.getElementById('out').innerHTML || ''`;
// 「纠错」行的内容（#out 里的第二个 <code>；原始/纠错/标点 三个 code 依次排列）。
const correctedText = `(() => { const cs = document.querySelectorAll('#out code'); return cs.length > 1 ? cs[1].textContent : ''; })()`;

function feedbackLineCount() {
  try {
    const t = readFileSync(join(DATA, 'feedback.jsonl'), 'utf8');
    return t.split('\n').filter((l) => l.trim()).length;
  } catch { return 0; }
}
function readLastFeedback() {
  try {
    const t = readFileSync(join(DATA, 'feedback.jsonl'), 'utf8');
    const lines = t.split('\n').filter((l) => l.trim());
    return JSON.parse(lines[lines.length - 1]);
  } catch { return null; }
}

async function main() {
  console.log('== A1–A12 完整验收（CDP 驱动，真实二进制）==');

  // A1：一条命令起服务 ⇒ 200
  try {
    const r = await fetch(`${BASE}/v1/health`);
    const ok = r.status === 200 && (await r.json()).status === 'ok';
    if (ok) pass('A1 一条命令起服务 ⇒ /v1/health 200'); else fail('A1', `status=${r.status}`);
  } catch (e) { fail('A1', String(e)); }

  // 连接页面
  const wsUrl = await getPageTarget();
  const cdp = new CDP(wsUrl);
  await cdp.open();
  await cdp.send('Page.enable');
  await cdp.send('Runtime.enable');
  await cdp.send('Page.addScriptToEvaluateOnNewDocument', { source: FAKE_REC });
  await cdp.send('Page.navigate', { url: BASE + '/' });
  await waitFor(() => cdp.eval(`document.readyState === 'complete' && !!document.getElementById('t')`));
  await sleep(300); // 等 log()/probe() 跑完

  // A2：页面含〔🎤说话〕〔处理〕〔上传〕〔规划〕〔✔〕〔✘〕
  try {
    const s = await cdp.eval(`(() => {
      const b = ${btns}.join('|');
      return {
        mic: b.includes('🎤'), handle: b.includes('处理'), plan: b.includes('规划'),
        ok: b.includes('✔'), no: b.includes('✘'),
        upload: !!document.querySelector('input[type=file]'),
      };
    })()`);
    const missing = ['mic', 'handle', 'plan', 'ok', 'no', 'upload'].filter((k) => !s[k]);
    if (missing.length === 0) pass('A2 页面含 🎤说话/处理/上传/规划/✔/✘');
    else fail('A2', `缺 ${missing.join('/')}：${JSON.stringify(s)}`);
  } catch (e) { fail('A2', String(e)); }

  // A3：录音桩 —— 点击 ⇒ start() ⇒ onresult 进 textarea（不自动提交）
  try {
    const baseFetch = await cdp.eval('window.__fetchCount');
    const clicked = await cdp.eval(`(() => { const b = document.getElementById('mic'); if (!b) return false; b.click(); return true; })()`);
    if (!clicked) throw new Error('mic 按钮不存在');
    await waitFor(() => cdp.eval(`document.getElementById('t').value.includes('查一下库存')`), 5000);
    const afterFetch = await cdp.eval('window.__fetchCount');
    const outAfter = await cdp.eval(outText);
    const ok = afterFetch === baseFetch && !outAfter.includes('处理中') && !outAfter.includes('处理失败');
    if (ok) pass('A3 录音桩：onresult 进 textarea、不自动提交');
    else fail('A3', `fetch ${baseFetch}→${afterFetch}，out=${outAfter.slice(0, 60)}`);
  } catch (e) { fail('A3', String(e)); }

  // A4：点〔处理〕⇒ #out 非空，含 原始：/纠错：/标点：/意图：
  try {
    await cdp.eval(`document.getElementById('t').value = '先改这个文件再提交'`);
    const okClick = await cdp.eval(clickBtn('处理'));
    if (!okClick) throw new Error('处理按钮不存在');
    // 注意：表达式必须整体加括号（`X || ''.includes(y)` 的优先级会把非空 #out 直接当 true）。
    await waitFor(() => cdp.eval(`(${outText}).includes('原始：')`));
    const t = await cdp.eval(outText);
    const parts = ['原始：', '纠错：', '标点：', '意图：'];
    const missing = parts.filter((p) => !t.includes(p));
    if (missing.length === 0) pass('A4 处理输出含 原始/纠错/标点/意图');
    else fail('A4', `缺 ${missing.join('/')}`);
  } catch (e) { fail('A4', String(e)); }

  // A5：上传短横线列表×34 + 任务含 TODO ⇒ 规划 steps > 2（分批）
  try {
    const doc34 = Array.from({ length: 34 }, (_, i) => `- item ${String(i).padStart(2, '0')}`).join('\n');
    await cdp.eval(`document.getElementById('doc').value = ${JSON.stringify(doc34)}`);
    await cdp.eval(`document.getElementById('task').value = '把这个文档里的 TODO 整理成一份计划'`);
    await cdp.eval(clickBtn('规划'));
    await waitFor(() => cdp.eval(`${stepsCount} > 0`));
    const n = await cdp.eval(stepsCount);
    const note = await cdp.eval(`document.getElementById('planout').innerHTML.includes('只规划')`);
    if (n > 2 && note) pass(`A5 34 条目文档 ⇒ steps=${n} > 2（分批；只规划不执行）`);
    else fail('A5', `steps=${n}（应>2）note=${note}`);
  } catch (e) { fail('A5', String(e)); }

  // A6：同任务无文档 ⇒ steps = 2
  try {
    await cdp.eval(`document.getElementById('doc').value = ''`);
    await cdp.eval(clickBtn('规划'));
    await waitFor(() => cdp.eval(`${stepsCount} > 0`));
    const n = await cdp.eval(stepsCount);
    if (n === 2) pass('A6 无文档 ⇒ steps=2（不误认分批）');
    else fail('A6', `steps=${n}（应=2）`);
  } catch (e) { fail('A6', String(e)); }

  // A7：TODO: 格式×34 ⇒ 也必须分批
  try {
    const docTodo = Array.from({ length: 34 }, (_, i) => `TODO: item ${String(i).padStart(2, '0')}`).join('\n');
    await cdp.eval(`document.getElementById('doc').value = ${JSON.stringify(docTodo)}`);
    await cdp.eval(clickBtn('规划'));
    await waitFor(() => cdp.eval(`${stepsCount} > 0`));
    const n = await cdp.eval(stepsCount);
    if (n > 2) pass(`A7 TODO 格式×34 ⇒ steps=${n} > 2（也分批）`);
    else fail('A7', `steps=${n}（应>2）`);
  } catch (e) { fail('A7', String(e)); }

  // A8：点〔✔ 对〕⇒ feedback.jsonl 多一条
  try {
    const before = feedbackLineCount();
    const okClick = await cdp.eval(`(() => { const b = document.getElementById('fbok'); if (!b) return false; b.click(); return true; })()`);
    if (!okClick) throw new Error('fbok 按钮不存在');
    await waitFor(() => feedbackLineCount() === before + 1);
    const last = readLastFeedback();
    if (last && last.accepted === true) pass('A8 ✔ ⇒ feedback.jsonl +1（accepted=true）');
    else fail('A8', `before=${before} after=${feedbackLineCount()} last=${JSON.stringify(last)}`);
  } catch (e) { fail('A8', String(e)); }

  // A9：点〔✘ 不对〕⇒ 多一条且带原因
  try {
    const before = feedbackLineCount();
    await cdp.eval(`document.getElementById('fbno').click()`);
    await waitFor(() => cdp.eval(`!document.getElementById('fbneg').hidden`), 4000);
    await cdp.eval(`document.getElementById('fbneg_reason').value = '这个不该改'`);
    await cdp.eval(clickBtn('提交 ✘ 原因'));
    await waitFor(() => feedbackLineCount() === before + 1);
    const last = readLastFeedback();
    if (last && last.accepted === false && last.reason === '这个不该改') pass('A9 ✘ ⇒ feedback.jsonl +1 且带原因');
    else fail('A9', `before=${before} after=${feedbackLineCount()} last=${JSON.stringify(last)}`);
  } catch (e) { fail('A9', String(e)); }

  // A10：〔这个改错了〕⇒ 黑名单生效 + 落盘
  try {
    // ① 教一个词（页面〔教一个词〕用的就是 /v1/observe）
    const ob = await fetch(`${BASE}/v1/observe`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ term: '哎欧劈艾斯', canonical: 'aiops' }),
    });
    if (ob.status !== 200) throw new Error('教词失败 HTTP ' + ob.status);
    // ② 处理 ⇒ 真的被改写（前置；用「纠错」行验证）
    await cdp.eval(`document.getElementById('t').value = '把哎欧劈艾斯接上'`);
    await cdp.eval(clickBtn('处理'));
    await waitFor(() => cdp.eval(`(${correctedText}).includes('把aiops接上')`), 6000);
    // ③ 点〔这个改错了〕⇒ 黑名单落盘 + 重新处理生效（纠错行回到原文、不再含 aiops）
    const blPath = join(DATA, 'blacklist.json');
    await cdp.eval(`document.getElementById('fbwrong').click()`);
    await waitFor(() => { try { return readFileSync(blPath, 'utf8').includes('哎欧劈艾斯'); } catch { return false; } }, 6000);
    await waitFor(() => cdp.eval(`(() => { const cs = document.querySelectorAll('#out code'); return cs.length > 1 && cs[1].textContent.includes('把哎欧劈艾斯接上') && !cs[1].textContent.includes('aiops'); })()`), 6000);
    const bl = readFileSync(blPath, 'utf8');
    if (bl.includes('哎欧劈艾斯')) {
      pass('A10 这个改错了 ⇒ 黑名单落盘 + 再处理不再改写');
    } else fail('A10', `blacklist=${bl.slice(0, 80)}`);
  } catch (e) { fail('A10', String(e)); }

  // A11：停服务 ⇒ 点〔处理〕⇒ 显示错误、不空白
  try {
    if (!SRV_PID) { skip('A11', '无服务 pid（环境未给 VHS_ACCEPT_SRV_PID）'); }
    else {
      process.kill(SRV_PID, 'SIGTERM');
      await sleep(800);
      const okClick = await cdp.eval(clickBtn('处理'));
      if (!okClick) throw new Error('处理按钮不存在');
      await waitFor(() => cdp.eval(`(${outText}).includes('处理失败')`), 8000);
      const t = await cdp.eval(outText);
      if (t.includes('处理失败') && t.includes('/v1/testpage')) pass('A11 停服务 ⇒ 处理失败可见、不空白');
      else fail('A11', t.slice(0, 120));
    }
  } catch (e) { fail('A11', String(e)); }

  // A12：全程 Runtime.exceptionThrown 为空
  try {
    if (cdp.exceptions.length === 0) pass('A12 全程无页面异常（exceptionThrown=0）');
    else fail('A12', `捕获 ${cdp.exceptions.length} 个异常：${JSON.stringify(cdp.exceptions[0])}`);
  } catch (e) { fail('A12', String(e)); }

  cdp.close();

  const passed = results.filter((r) => r.ok === true).length;
  const failed = results.filter((r) => r.ok === false).length;
  const skipped = results.filter((r) => r.ok === null).length;
  console.log(`\nA1–A12 汇总：PASS ${passed} / FAIL ${failed} / SKIP ${skipped}`);
  if (failed > 0) {
    console.log('未通过的条目：');
    results.filter((r) => r.ok === false).forEach((r) => console.log(`  - ${r.a}: ${r.why}`));
    process.exit(1);
  }
  process.exit(0);
}

main().catch((e) => { console.error('[accept] 运行失败：', e); process.exit(1); });
