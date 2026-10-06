/*
 * VoxSign iOS   ·     (logic.js)
 * ------------------------------------------------------------------
 *  dependency, no DOM, no  :      window.VSLogic, node   module.exports. 
 *  has" disconnect/ decide/status " in   , thenat node disconnectlang  andaftercontinue WKWebView  use. 
 *    /kindform  (   file). 
 */
(function (global) {
  'use strict';

  /* ================= endstate / decision pointstatusword(to  INTERACT-v1) ================= */

  // server statusword: running -> need_ask/need_confirm -> running … -> done/canceled; 
  // interrupted = heavystartbefore done(  after   continue ). 
  var TERMINAL = { done: 1, canceled: 1, interrupted: 1 };
  var DECISION = { need_ask: 1, need_confirm: 1 };

  // isTerminal: pollis  stop. 
  function isTerminal(status) { return !!TERMINAL[status]; }

  // isDecision: is  raise "  decision point"on( time stoppoll, etcuseuser answer). 
  function isDecision(status) { return !!DECISION[status]; }

  /* ================= request_id(M4  etc ) ================= */

  // genRequestId: clientuserendoccurbecome, heavy same tasktime   -> server  heavy(deduped:true). 
  function genRequestId() {
    return 'req-' + Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 8);
  }

  /* ================= back   resolve (contract.RenderReceipt  revtoresolve ) =================
   *
   * server    form    : 
   *     : <action>\nfile: <files>\nclose : <result>\n  : <undo>
   *   :     / safety   id /     / beforeafterempty  all  , by firsttgt   . 
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

  /* =================    resolve (back    4   ->   by ) =================
   *
   * [pseudocode logic layer]( write:   by     decide)
   *   show = server.reversible===true  and  undo    "    / reversible/forbidstoprollback". 
   *   backup: undo    get .bak filename(compat VHS_BACKUP_PATH: before   ). 
   *   error: undo asempty -> show=false, backup=''(    ,  giveby ). 
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

  /* ================= intentclose word ->  tgt (from receipt      ) ================= */
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

  /* =================  tgt   (intent/domain/risk/status) =================
   *
   * [pseudocode logic layer]( write:   from"status/back /attribution"  , in  node   )
   *    in view = GET /v1/tasks/{id}    (  finish  Outcome, onlyhas receipt/attribution/reversible). 
   *    out badges[] = 2~4     ,    {kind,label,tone}: 
   *     state   ← status  connect  (  in/ clarification/ confirm/done/alreadycancel/alreadyinterrupt)
   *     intent  ← receipt.   close word(  /modifyfile/  /  /  )
   *     domain  ← receipt.file   : notes.md->  domain; has  to path-> objdomain; no->  
   *     risk    ← need_confirm-> risk·   ; reversible->reversible; done and !reversible-> reversible
   *   origthen:    show   splitnum/ASR orig / pos  --only useuserneedneed    . 
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

  /* =================     decision point(  routeby) =================
   *
   * [pseudocode logic layer]( write: decision point first ,   only    )
   *    first   -> : 
   *     1. need_confirm -> {kind:'confirm', question}              confirm (answer:"  ")
   *     2. need_ask     -> {kind:'ask', question, options[]}       by (pt  answer:option.id)
   *     3. canceled/interrupted -> {kind:'error', message}         error 
   *     4. done         -> {kind:'receipt', receipt, undo}          back  (  by by undo.show)
   *     5. running      -> {kind:'running'}                              
   *     6. its /idle    -> {kind:'idle'}
   *   origthen: on   done  back   as      to   , bot  again     decide   . 
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

  /* =================     (M5-3,    server.roleForStatus) =================
   *
   * [pseudocode logic layer]( write: stage->   decide)
   *   planner  = classify/domain decide/risk grading/confirm /clarification(decide )-> need_ask/need_confirm
   *   executor =                                     -> running
   *   verifier = verify/attribution/back                             -> done
   *   its /canceled/interrupted ->  back planner. 
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

  /* =================  disconnectstatus :  "stop" ->       =================
   *
   * [pseudocode logic layer]( write: stopstop->alreadyoccur /   / continuecontinueor  )
   *    in view = curbeforetask  (   running/need_ask/need_confirm/done). 
   *   control flow: 
   *     hasEffect = alreadyhas receipt and  /file empty(done beforealready     close ). 
   *     active   = hasEffect ? ['alreadyoccur : <action>(<files>)']
   *                          : ['alreadyoccur :   produceoccurfilechangechange']
   *     blocked  = ['   : aftercontinuestagealreadyinstop']
   *     actions  = ['continuecontinue'] + (undo.show ? ['  '] : [])   //     before
   *   error: view asempty -> active=' has  in task', actions=[]. 
   *   note :    onlyis UI  now;  pos stopstopby app.js call legacy /v1/cancel done. 
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

  /* =================    stagechain(M6: by SSE stage event    ,  as  table) ================= */
  var EXEC_STAGES = ['意图分类', '域裁决', '风险分级', '确认闸', '执行', '校验', '归因'];

  // stageIndex: SSE step(in stagename)-> EXEC_STAGES undertgt;   namereturnback -1(     ). 
  function stageIndex(stepName) { return EXEC_STAGES.indexOf(stepName); }

  /* ================= SSE line  resolve (M6, to  SSE-v1   ) =================
   *
   * [pseudocode logic layer]( write: SSE  resolve     decide)
   *  in  byempty closeend finish  SSE  (    event:/data:   ): 
   *   event: stage\n
   *   data: {"seq":1,...}\n\n
   * rule: 
   *   -  first ':' = note /keepalive,   ; 
   *   - 'event:'    -> 'message'; 
   *   - 'data:'    -> use \n  connectafter JSON.parse; resolve    -> data={_raw:orig },   ; 
   *   - id:/retry: baseclientuserend  (heavylinkuse ?after=<lastSeq>, see  ). 
   */
  function parseSSEBlock(blockText) {
    var ev = { event: 'message', data: null };
    var dataLines = [];
    String(blockText).split(/\r?\n/).forEach(function (line) {
      if (line === '') return;              //  endempty bycalluse  split
      if (line.charAt(0) === ':') return;   // note /  
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

  /* ================= heavylink etc: by seq  heavy =================
   *
   * [pseudocode logic layer]( write: disconnectlineheavylink ?after=<lastSeq>  heavy   )
   *   seen = {seq:1}(already  ed  id  ). 
   *   filterNew(seen, events):   event; 
   *     seq    ->  connect  (prevent ity,   event); 
   *     seq already  seen ->  ed(heavy  heavy); 
   *      then in seen and  . 
   *   returnbackneweventnum (seen origlychangenew). 
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

  /* =================  disconnect semantic: SSE interrupt event ->       =================
   *
   * [pseudocode logic layer]( write: interrupt eventasi.e.timesignal,  firstatpoll  )
   *   data = {seq, applied:[...], notApplied:[...], canRollback:bool}
   *   active   = applied.map('alreadyoccur : '+x); empty -> 'alreadyoccur :   produceoccurfilechangechange'
   *   blocked  = notApplied.map('   : '+x); empty -> '   : aftercontinuestagealreadyinstop'
   *   actions  = ['continuecontinue'] + (canRollback ? ['  '] before  : [])
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

  /* =================    time(M6: GET /v1/roles [{id,label,active}]) ================= */
  function activeRole(roles) {
    if (!Array.isArray(roles)) return '';
    for (var i = 0; i < roles.length; i++) {
      if (roles[i] && roles[i].active) return roles[i].id;
    }
    return '';
  }

  /* ================= langaudio diff  (webkitSpeechRecognition  state) =================
   * returnback 'native' | 'webkit' | 'none'. node     mock global  state. 
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
