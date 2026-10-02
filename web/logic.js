/*
 * VoxSign iOS 壳 · 纯逻辑层（logic.js）
 * ------------------------------------------------------------------
 * 零依赖、无 DOM、无网络：浏览器挂 window.VSLogic，node 挂 module.exports。
 * 所有"判断/裁决/状态机"集中在这里，便于 node 断言测试与后续 WKWebView 复用。
 * 纯渲染/样式豁免（不在此文件）。
 */
(function (global) {
  'use strict';

  /* ================= 终态 / 决策点状态词（对齐 INTERACT-v1） ================= */

  // server 状态词：running → need_ask/need_confirm → running … → done/canceled；
  // interrupted = 重启前未完成（恢复后不自动续跑）。
  var TERMINAL = { done: 1, canceled: 1, interrupted: 1 };
  var DECISION = { need_ask: 1, need_confirm: 1 };

  // isTerminal：轮询是否该停。
  function isTerminal(status) { return !!TERMINAL[status]; }

  // isDecision：是否挂起在"一个决策点"上（此时暂停轮询，等用户 answer）。
  function isDecision(status) { return !!DECISION[status]; }

  /* ================= request_id（M4 幂等键） ================= */

  // genRequestId：客户端生成，重试同一任务时复传 → server 去重（deduped:true）。
  function genRequestId() {
    return 'req-' + Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 8);
  }

  /* ================= 回执四行解析（contract.RenderReceipt 的反向解析） =================
   *
   * server 渲染格式恰好四行：
   *   动作：<action>\n文件：<files>\n结果：<result>\n撤销：<undo>
   * 容错：行缺失 / 全半角冒号 / 多余行 / 前后空白 都不炸，按行首标签归位。
   */
  function parseReceipt(text) {
    var out = { action: '', files: '', result: '', undo: '' };
    if (!text) return out;
    String(text).split(/\r?\n/).forEach(function (ln) {
      var m = ln.match(/^\s*(动作|文件|结果|撤销)\s*[:：]\s*(.*)$/);
      if (!m) return;
      var v = m[2].trim();
      switch (m[1]) {
        case '动作': out.action = v; break;
        case '文件': out.files = v; break;
        case '结果': out.result = v; break;
        case '撤销': out.undo = v; break;
      }
    });
    return out;
  }

  /* ================= 撤销行解析（回执卡第 4 行 → 撤销按钮） =================
   *
   * 【伪代码逻辑层】（必写：撤销按钮显隐属裁决）
   *   show = server.reversible===true  且  undo 行不含"不可撤销/不可逆/禁止回滚"。
   *   backup：undo 行里提取 .bak 文件名（兼容 VHS_BACKUP_PATH: 前缀契约）。
   *   异常：undo 为空 → show=false，backup=''（不可撤销，不给按钮）。
   */
  function extractUndo(receiptObj, reversible) {
    var undo = (receiptObj && receiptObj.undo) || '';
    var irreversible = /不可撤销|不可逆|禁止回滚/.test(undo);
    var show = !!reversible && !irreversible;
    var backup = '';
    var m = undo.match(/(?:VHS_BACKUP_PATH\s*[:：]\s*)?([^\s，,]+\.bak)/);
    if (m) backup = m[1];
    return { show: show, backup: backup, irreversible: irreversible };
  }

  /* ================= 意图关键词 → 轻标签（从 receipt 动作行压缩） ================= */
  var INTENT_WORDS = [
    { re: /NOTE|记一下|笔记/i, label: '笔记' },
    { re: /EDIT|改文件|编辑|删除/i, label: '改文件' },
    { re: /QUERY|查代码|查询|看看/i, label: '查询' },
    { re: /COMMIT|提交|commit/i, label: '提交' },
    { re: /DEPLOY|部署/i, label: '部署' }
  ];
  function intentBadge(actionText) {
    for (var i = 0; i < INTENT_WORDS.length; i++) {
      if (INTENT_WORDS[i].re.test(actionText || '')) return INTENT_WORDS[i].label;
    }
    return '';
  }

  function toneForStatus(s) {
    if (s === 'done') return 'green';
    if (s === 'need_confirm') return 'red';
    if (s === 'need_ask') return 'amber';
    if (s === 'canceled' || s === 'interrupted') return 'gray';
    return 'blue';
  }

  /* ================= 轻标签压缩（意图/域/风险/状态） =================
   *
   * 【伪代码逻辑层】（必写：徽章从"状态/回执/归因"压缩，内部细节不放大）
   *   输入 view = GET /v1/tasks/{id} 的响应（不含完整 Outcome，只有 receipt/attribution/reversible）。
   *   输出 badges[] = 2~4 个小徽章，每个 {kind,label,tone}：
   *     state   ← status 直接映射（执行中/待回问/待确认/完成/已取消/已中断）
   *     intent  ← receipt.动作行关键词（笔记/改文件/查询/提交/部署）
   *     domain  ← receipt.文件行压缩：notes.md→笔记域；有真实对象路径→项目域；无→省略
   *     risk    ← need_confirm→高风险·待放行；reversible→可逆；done 且 !reversible→不可逆
   *   原则：绝不展示置信度分数/ASR 原文/纠正明细——只留用户需要的掌控感。
   */
  function compressBadges(view) {
    view = view || {};
    var badges = [];
    var stateMap = {
      running: '执行中', need_ask: '待回问', need_confirm: '待确认',
      done: '完成', canceled: '已取消', interrupted: '已中断'
    };
    if (stateMap[view.status]) {
      badges.push({ kind: 'state', label: stateMap[view.status], tone: toneForStatus(view.status) });
    }
    var r = parseReceipt(view.receipt);
    var it = intentBadge(r.action);
    if (it) badges.push({ kind: 'intent', label: it, tone: 'blue' });
    if (/notes\.md|笔记/.test(r.files) || /笔记|NOTE/.test(r.action)) {
      badges.push({ kind: 'domain', label: '笔记域', tone: 'gray' });
    } else if (r.files && r.files !== '—') {
      badges.push({ kind: 'domain', label: '项目域', tone: 'gray' });
    }
    if (view.status === 'need_confirm') {
      badges.push({ kind: 'risk', label: '高风险·待放行', tone: 'red' });
    } else if (view.reversible) {
      badges.push({ kind: 'risk', label: '可逆', tone: 'green' });
    } else if (view.status === 'done' && !view.reversible) {
      badges.push({ kind: 'risk', label: '不可逆', tone: 'red' });
    }
    return badges;
  }

  /* ================= 一屏一个决策点（渲染路由） =================
   *
   * 【伪代码逻辑层】（必写：决策点优先级，一次只渲染一个）
   *   优先级 高→低：
   *     1. need_confirm → {kind:'confirm', question}            红色确认条（answer:"执行"）
   *     2. need_ask     → {kind:'ask', question, options[]}     候选按钮（点选 answer:option.id）
   *     3. canceled/interrupted → {kind:'error', message}       系统错误条
   *     4. done         → {kind:'receipt', receipt, undo}        绿色回执卡（撤销按钮按 undo.show）
   *     5. running      → {kind:'running'}                       执行卡滚动步骤
   *     6. 其他/idle    → {kind:'idle'}
   *   原则：上一个 done 的回执卡作为历史气泡留在对话流里，底部不再叠加第二个决策控件。
   */
  function nextDecisionPoint(view) {
    view = view || {};
    switch (view.status) {
      case 'need_confirm':
        return { kind: 'confirm', question: view.question || '人工放行（不可逆）操作' };
      case 'need_ask':
        return { kind: 'ask', question: view.question || '你想让我做什么？', options: view.options || [] };
      case 'canceled':
      case 'interrupted':
        return { kind: 'error', message: view.error || (view.status === 'interrupted' ? '任务已中断，请重新提交' : '任务被取消') };
      case 'done':
        return { kind: 'receipt', receipt: parseReceipt(view.receipt), undo: extractUndo(parseReceipt(view.receipt), view.reversible) };
      case 'running':
        return { kind: 'running' };
      default:
        return { kind: 'idle' };
    }
  }

  /* ================= 角色映射（M5-3，镜像 server.roleForStatus） =================
   *
   * 【伪代码逻辑层】（必写：阶段→角色裁决）
   *   planner  = 分类/域裁决/风险分级/确认闸/回问（决策）→ need_ask/need_confirm
   *   executor = 工具动作执行                              → running
   *   verifier = 校验/归因/回执                            → done
   *   其他/canceled/interrupted → 落回 planner。
   */
  function roleForStatus(status) {
    switch (status) {
      case 'need_ask':
      case 'need_confirm':
        return 'planner';
      case 'done':
        return 'verifier';
      case 'running':
        return 'executor';
      default:
        return 'planner';
    }
  }
  var ROLE_LABELS = { planner: 'Planner', executor: 'Executor', verifier: 'Verifier' };

  /* ================= 打断状态机：说"停" → 红色系统条 =================
   *
   * 【伪代码逻辑层】（必写：停止→已生效/未执行/可继续或撤销）
   *   输入 view = 当前任务视图（可能 running/need_ask/need_confirm/done）。
   *   控制流：
   *     hasEffect = 已有 receipt 且动作/文件非空（done 前已落盘的执行结果）。
   *     active   = hasEffect ? ['已生效：<action>（<files>）']
   *                          : ['已生效：尚未产生文件变更']
   *     blocked  = ['未执行：后续阶段已中止']
   *     actions  = ['继续'] + (undo.show ? ['撤销'] : [])   // 撤销在最前
   *   异常：view 为空 → active='没有进行中的任务'，actions=[]。
   *   注意：系统条只是 UI 呈现；真正的停止由 app.js 调 legacy /v1/cancel 完成。
   */
  function interruptSystemBar(view) {
    view = view || {};
    if (!view.status && !view.receipt) {
      return { title: '停止', active: ['没有进行中的任务'], blocked: [], actions: [], closable: true };
    }
    var r = parseReceipt(view.receipt);
    var hasEffect = !!view.receipt && !!(r.action || r.files);
    var active = hasEffect
      ? ['已生效：' + (r.action || '执行已落盘') + (r.files && r.files !== '—' ? '（' + r.files + '）' : '')]
      : ['已生效：尚未产生文件变更'];
    var blocked = ['未执行：后续阶段已中止'];
    var actions = ['继续'];
    var und = extractUndo(r, view.reversible);
    if (und.show) actions.unshift('撤销');
    return { title: '已按下停止', active: active, blocked: blocked, actions: actions, closable: true };
  }

  /* ================= 执行卡阶段链（M6：由 SSE stage 事件真实驱动，此为索引表） ================= */
  var EXEC_STAGES = ['意图分类', '域裁决', '风险分级', '确认闸', '执行', '校验', '归因'];

  // stageIndex：SSE step（中文阶段名）→ EXEC_STAGES 下标；未知名返回 -1（执行卡不跳）。
  function stageIndex(stepName) { return EXEC_STAGES.indexOf(stepName); }

  /* ================= SSE 线协议解析（M6，对齐 SSE-v1 契约） =================
   *
   * 【伪代码逻辑层】（必写：SSE 块解析属协议裁决）
   * 输入一个以空行结束的完整 SSE 块（可能含 event:/data: 多行）：
   *   event: stage\n
   *   data: {"seq":1,...}\n\n
   * 规则：
   *   - 行首 ':' = 注释/keepalive，忽略；
   *   - 'event:' 缺省 → 'message'；
   *   - 'data:' 多行 → 用 \n 拼接后 JSON.parse；解析失败 → data={_raw:原文}，不崩；
   *   - id:/retry: 本客户端忽略（重连用 ?after=<lastSeq>，见契约）。
   */
  function parseSSEBlock(blockText) {
    var ev = { event: 'message', data: null };
    var dataLines = [];
    String(blockText).split(/\r?\n/).forEach(function (line) {
      if (line === '') return;              // 块末空行由调用方切分
      if (line.charAt(0) === ':') return;   // 注释/心跳
      var idx = line.indexOf(':');
      var field = idx < 0 ? line : line.slice(0, idx);
      var val = idx < 0 ? '' : line.slice(idx + 1).replace(/^ /, '');
      if (field === 'event') ev.event = val;
      else if (field === 'data') dataLines.push(val);
    });
    if (!dataLines.length) return null;
    var raw = dataLines.join('\n');
    try { ev.data = JSON.parse(raw); } catch (e) { ev.data = { _raw: raw }; }
    return ev;
  }

  /* ================= 重连幂等：按 seq 去重 =================
   *
   * 【伪代码逻辑层】（必写：断线重连 ?after=<lastSeq> 不重复渲染）
   *   seen = {seq:1}（已渲染过的序号集合）。
   *   filterNew(seen, events)：遍历事件；
   *     seq 缺失 → 直接放行（防御性，不丢事件）；
   *     seq 已在 seen → 跳过（重放去重）；
   *     否则记入 seen 并放行。
   *   返回新事件数组（seen 原地更新）。
   */
  function filterNew(seen, events) {
    var fresh = [];
    (events || []).forEach(function (ev) {
      if (!ev) return;
      var seq = ev.data && ev.data.seq;
      if (seq == null) { fresh.push(ev); return; }
      if (seen[seq]) return;
      seen[seq] = 1;
      fresh.push(ev);
    });
    return fresh;
  }

  /* ================= 打断三语义：SSE interrupt 事件 → 红色系统条 =================
   *
   * 【伪代码逻辑层】（必写：interrupt 事件为即时信号，优先于轮询感知）
   *   data = {seq, applied:[...], notApplied:[...], canRollback:bool}
   *   active   = applied.map('已生效：'+x)；空 → '已生效：尚未产生文件变更'
   *   blocked  = notApplied.map('未执行：'+x)；空 → '未执行：后续阶段已中止'
   *   actions  = ['继续'] + (canRollback ? ['撤销'] 前置 : [])
   */
  function interruptBarFromEvent(ev) {
    ev = ev || {};
    var applied = (ev.applied || []).map(function (x) { return '已生效：' + x; });
    var notApplied = (ev.notApplied || []).map(function (x) { return '未执行：' + x; });
    if (!applied.length) applied = ['已生效：尚未产生文件变更'];
    if (!notApplied.length) notApplied = ['未执行：后续阶段已中止'];
    var actions = ['继续'];
    if (ev.canRollback) actions.unshift('撤销');
    return { title: '已按下停止', active: applied, blocked: notApplied, actions: actions, closable: true };
  }

  /* ================= 角色实时（M6：GET /v1/roles [{id,label,active}]） ================= */
  function activeRole(roles) {
    if (!Array.isArray(roles)) return '';
    for (var i = 0; i < roles.length; i++) {
      if (roles[i] && roles[i].active) return roles[i].id;
    }
    return '';
  }

  /* ================= 语音识别探测（webkitSpeechRecognition 两态） =================
   * 返回 'native' | 'webkit' | 'none'。node 测试可 mock global 两态。
   */
  function detectRecognition(g) {
    g = g || {};
    if (g.SpeechRecognition) return 'native';
    if (g.webkitSpeechRecognition) return 'webkit';
    return 'none';
  }

  var api = {
    isTerminal: isTerminal,
    isDecision: isDecision,
    genRequestId: genRequestId,
    parseReceipt: parseReceipt,
    extractUndo: extractUndo,
    intentBadge: intentBadge,
    compressBadges: compressBadges,
    nextDecisionPoint: nextDecisionPoint,
    roleForStatus: roleForStatus,
    ROLE_LABELS: ROLE_LABELS,
    interruptSystemBar: interruptSystemBar,
    EXEC_STAGES: EXEC_STAGES,
    stageIndex: stageIndex,
    parseSSEBlock: parseSSEBlock,
    filterNew: filterNew,
    interruptBarFromEvent: interruptBarFromEvent,
    activeRole: activeRole,
    detectRecognition: detectRecognition
  };

  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else global.VSLogic = api;
})(typeof window !== 'undefined' ? window : globalThis);
