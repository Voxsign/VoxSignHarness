/*
 * VoxSign iOS 壳 · app.js（M6-1b：SSE 流式消费 + 角色实时 + 打断即时 + 轮询兜底）
 * ------------------------------------------------------------------
 * 纯判断/状态机集中在 logic.js（VSLogic）；本文件做 DOM 渲染、HTTP/SSE 对接、事件编排。
 *
 * SSE 说明：契约要求 Authorization: Bearer，原生 EventSource 不能自定义请求头，
 *   故用 fetch + ReadableStream 手写 SSE 客户端（可带头、可手动 ?after=<lastSeq> 重连）。
 */
(function () {
  'use strict';
  var L = window.VSLogic;

  /* ================= 设置 ================= */
  var LS_KEY = 'vhs-web-settings';
  var settings = loadSettings();
  function loadSettings() {
    try { return Object.assign({ base: 'http://127.0.0.1:8765', token: '' }, JSON.parse(localStorage.getItem(LS_KEY) || '{}')); }
    catch (e) { return { base: 'http://127.0.0.1:8765', token: '' }; }
  }
  function saveSettings() {
    settings.base = (document.getElementById('serverUrl').value || '').replace(/\/+$/, '');
    settings.token = document.getElementById('serverToken').value || '';
    localStorage.setItem(LS_KEY, JSON.stringify(settings));
  }

  function authHeaders(extra) {
    var h = Object.assign({ 'Content-Type': 'application/json' }, extra || {});
    if (settings.token) h['Authorization'] = 'Bearer ' + settings.token;
    return h;
  }
  function api(path, opts) {
    opts = opts || {};
    opts.headers = authHeaders(opts.headers);
    return fetch(settings.base + path, opts).then(function (res) {
      return res.text().then(function (txt) {
        var body; try { body = JSON.parse(txt); } catch (e) { body = { raw: txt }; }
        if (!res.ok) { var err = new Error(body.error || ('HTTP ' + res.status)); err.status = res.status; err.body = body; throw err; }
        return body;
      });
    });
  }

  /* ================= DOM 句柄 ================= */
  var $ = function (id) { return document.getElementById(id); };
  var chat = $('chat'), decisionZone = $('decisionZone'),
      systemBar = $('systemBar'), input = $('textInput');

  /* ================= 对话流渲染 ================= */
  function scrollBottom() { chat.scrollTop = chat.scrollHeight; }
  function el(tag, cls, html) {
    var d = document.createElement(tag);
    if (cls) d.className = cls;
    if (html != null) d.innerHTML = html;
    return d;
  }
  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"]/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c];
    });
  }
  function badgeHtml(b) { return '<span class="badge ' + b.kind + ' ' + b.tone + '">' + b.label + '</span>'; }

  function appendUserBubble(text, fromVoice) {
    var row = el('div', 'row user');
    var b = el('div', 'bubble'); b.textContent = text;
    if (fromVoice) { var w = el('span', 'wave'); w.innerHTML = '<i></i><i></i><i></i><i></i>'; b.appendChild(w); }
    row.appendChild(b); chat.appendChild(row); scrollBottom();
  }
  function appendHarnessBubble(text, view) {
    var row = el('div', 'row harness');
    var b = el('div', 'bubble'); b.textContent = text;
    row.appendChild(b);
    var badges = el('div', 'badges');
    L.compressBadges(view || {}).forEach(function (bg) { badges.insertAdjacentHTML('beforeend', badgeHtml(bg)); });
    if (badges.children.length) row.appendChild(badges);
    chat.appendChild(row); scrollBottom();
  }
  function appendTyping() {
    var row = el('div', 'row harness');
    var b = el('div', 'bubble');
    b.innerHTML = '<span class="typing"><i></i><i></i><i></i></span>';
    row.appendChild(b); chat.appendChild(row); scrollBottom();
    return row;
  }

  /* ================= 执行卡（M6：SSE stage 事件真实驱动，不再模拟推进） ================= */
  var execCard = null;
  function ensureExecCard() {
    if (execCard) return execCard;
    var row = el('div', 'row harness');
    execCard = el('div', 'exec-card');
    execCard.innerHTML = '<div class="exec-title">正在处理…</div>';
    L.EXEC_STAGES.forEach(function (s) {
      var st = el('div', 'exec-step', '<span class="tick"></span><span class="t"></span>');
      st.querySelector('.t').textContent = s;
      execCard.appendChild(st);
    });
    row.appendChild(execCard); chat.appendChild(row); scrollBottom();
    return execCard;
  }
  // 真实 stage 事件：把 step 之前（含）画成 done，step 自身画 active。
  function paintExecTo(stepName) {
    ensureExecCard();
    var idx = L.stageIndex(stepName);
    var steps = execCard.querySelectorAll('.exec-step');
    steps.forEach(function (s, i) {
      s.classList.remove('done', 'active');
      s.querySelector('.tick').textContent = '';
      if (idx < 0) return;
      if (i < idx) { s.classList.add('done'); s.querySelector('.tick').textContent = '✓'; }
      else if (i === idx) { s.classList.add('active'); s.querySelector('.tick').textContent = '•'; }
    });
  }
  function completeExec() {
    if (!execCard) return;
    var steps = execCard.querySelectorAll('.exec-step');
    steps.forEach(function (s) { s.classList.add('done'); s.classList.remove('active'); s.querySelector('.tick').textContent = '✓'; });
  }
  function closeExecCard() { execCard = null; }

  /* ================= 回执卡（done 事件渲染） ================= */
  function appendReceiptCardFromView(view) {
    var r = L.parseReceipt(view.receipt);
    var und = L.extractUndo(r, view.reversible);
    var row = el('div', 'row harness');
    var card = el('div', 'receipt-card');
    card.innerHTML =
      '<div class="receipt-title">✓ 完成</div>' +
      '<div class="receipt-line"><b>动作</b>　' + esc(r.action) + '</div>' +
      '<div class="receipt-line"><b>文件</b>　' + esc(r.files) + '</div>' +
      '<div class="receipt-line"><b>结果</b>　' + esc(r.result) + '</div>';
    var undoRow = el('div', 'receipt-undo-row');
    undoRow.innerHTML = '<span class="receipt-line"><b>撤销</b>　' + esc(r.undo) + '</span>';
    if (und.show) {
      var btn = el('button', 'btn-undo', '撤销');
      btn.onclick = function () { doRollback(view.task_id, btn); };
      undoRow.appendChild(btn);
    }
    card.appendChild(undoRow); row.appendChild(card);
    var badges = el('div', 'badges');
    L.compressBadges(view).forEach(function (bg) { badges.insertAdjacentHTML('beforeend', badgeHtml(bg)); });
    row.appendChild(badges); chat.appendChild(row); scrollBottom();
  }
  function doRollback(taskId, btn) {
    btn.disabled = true; btn.textContent = '撤销中…';
    api('/v1/tasks/' + encodeURIComponent(taskId) + '/rollback', { method: 'POST' })
      .then(function (res) { appendHarnessBubble('已撤销：' + (res.restored || '已恢复'), { status: 'done', reversible: false }); btn.textContent = '已撤销'; })
      .catch(function (err) { appendHarnessBubble('撤销失败：' + err.message, { status: 'canceled' }); btn.disabled = false; btn.textContent = '撤销'; });
  }

  /* ================= 决策点区（need_ask/need_confirm 事件渲染，一屏一个） ================= */
  function renderAsk(question, options) {
    decisionZone.innerHTML = '';
    var box = el('div', 'ask-box');
    box.innerHTML = '<div class="ask-q">' + esc(question) + '</div>';
    var opts = el('div', 'ask-opts');
    (options || []).forEach(function (opt) {
      var b = el('button', 'ask-opt', esc(opt.label));
      b.onclick = function () { answerAndResume(currentTaskId, opt.id); };
      opts.appendChild(b);
    });
    box.appendChild(opts); decisionZone.appendChild(box);
  }
  function renderConfirm(question) {
    decisionZone.innerHTML = '';
    var bar = el('div', 'confirm-bar');
    bar.innerHTML = '<div class="confirm-q">⚠ ' + esc(question) + '</div>';
    var btns = el('div', 'confirm-btns');
    var ok = el('button', 'btn-confirm', '执行');
    ok.onclick = function () { answerAndResume(currentTaskId, '执行'); };
    var no = el('button', 'btn-reject', '拒绝');
    no.onclick = function () { answerAndResume(currentTaskId, '拒绝'); };
    btns.appendChild(ok); btns.appendChild(no); bar.appendChild(btns); decisionZone.appendChild(bar);
  }
  function renderError(msg) { decisionZone.innerHTML = ''; decisionZone.appendChild(el('div', 'error-bar', esc(msg))); }
  function clearDecision() { decisionZone.innerHTML = ''; }

  /* ================= 红色系统条（interrupt 事件即时渲染） ================= */
  function showSystemBar(bar) {
    $('sbTitle').textContent = bar.title;
    $('sbActive').textContent = bar.active.join('；');
    $('sbBlocked').textContent = bar.blocked.join('；');
    var acts = $('sbActions'); acts.innerHTML = '';
    bar.actions.forEach(function (a) {
      var b = el('button', a === '撤销' ? 'btn-undo' : 'btn-confirm');
      b.textContent = a;
      b.onclick = function () { if (a === '撤销' && currentTaskId) doRollback(currentTaskId, b); hideSystemBar(); };
      acts.appendChild(b);
    });
    systemBar.classList.remove('hidden');
  }
  function hideSystemBar() { systemBar.classList.add('hidden'); }
  $('sbClose').onclick = hideSystemBar;

  /* ================= 角色条实时（M6：GET /v1/roles + stage.role 事件） ================= */
  function setRoleUI(roleId) {
    if (!roleId || !L.ROLE_LABELS[roleId]) return;
    $('roleLabel').textContent = L.ROLE_LABELS[roleId];
    roleBar.querySelectorAll('.role-chip').forEach(function (c) {
      c.classList.toggle('active', c.getAttribute('data-role') === roleId);
    });
  }
  function loadRoles() {
    // GET /v1/roles 未实现时静默降级为本地静态（M5 行为）。
    api('/v1/roles').then(function (roles) {
      var active = L.activeRole(roles);
      if (active) setRoleUI(active);
    }).catch(function () { /* 本地静态态即可 */ });
  }

  /* ================= SSE 客户端（fetch + ReadableStream） =================
   *
   * 【伪代码逻辑层】（必写：SSE 消费 / 重连幂等 / 兜底裁决）
   *   openSSE(id):
   *     url = base + /v1/tasks/{id}/events + (lastSeq ? '?after='+lastSeq : '')
   *     fetch(url, headers=Bearer) → res.body.getReader()
   *       逐块 decode → 按 "\n\n" 切完整 SSE 块 → L.parseSSEBlock → L.filterNew(seenSeqs) 去重
   *       → handleEvent(ev) 按 event 类型路由。
   *   断线/异常：
   *     若已收到过至少一个事件 → 1.5s 后用 ?after=<lastSeq> 重连（server 重放 seq>after，filterNew 去重）。
   *     若首连即失败（/events 404/405/网络）→ 判定 server 未升级 SSE → 回退 startPolling(id)。
   *   终态（done/failed/canceled）→ 关闭 reader，不再重连。
   */
  var esReader = null, esBuffer = '', esClosed = false, sseUsed = false;
  var seenSeqs = {}, lastSeq = 0;
  var reconnectTimer = null;
  var currentTaskId = null, currentView = null;

  function openSSE(id) {
    closeSSEStream();
    esClosed = false;
    var qs = lastSeq > 0 ? '?after=' + lastSeq : '';
    fetch(settings.base + '/v1/tasks/' + encodeURIComponent(id) + '/events' + qs, { headers: authHeaders() })
      .then(function (res) {
        if (!res.ok || !res.body || !res.body.getReader) throw new Error('SSE 不支持 HTTP ' + res.status);
        sseUsed = true;
        var reader = res.body.getReader(); esReader = reader;
        var dec = new TextDecoder('utf-8');
        function pump() {
          reader.read().then(function (r) {
            if (r.done) { onStreamEnd(id); return; }
            esBuffer += dec.decode(r.value, { stream: true });
            var idx;
            while ((idx = esBuffer.indexOf('\n\n')) >= 0) {
              var block = esBuffer.slice(0, idx);
              esBuffer = esBuffer.slice(idx + 2);
              var ev = L.parseSSEBlock(block);
              if (ev) L.filterNew(seenSeqs, [ev]).forEach(handleEvent);
            }
            pump();
          }).catch(function () { onStreamError(id); });
        }
        pump();
      })
      .catch(function () {
        // 首连失败：server 未升级 SSE → 回退轮询兜底。
        if (!sseUsed) { startPolling(id); }
        else { scheduleReconnect(id); }
      });
  }
  function closeSSEStream() {
    if (reconnectTimer) { clearTimeout(reconnectTimer); reconnectTimer = null; }
    if (esReader) { try { esReader.cancel(); } catch (e) {} esReader = null; }
    esBuffer = '';
  }
  function onStreamEnd(id) {
    esReader = null;
    if (!esClosed) scheduleReconnect(id); // 非预期关闭（非终态）→ 重连
  }
  function onStreamError(id) {
    esReader = null;
    if (esClosed) return;
    if (sseUsed) scheduleReconnect(id);
    else startPolling(id);
  }
  function scheduleReconnect(id) {
    if (esClosed) return;
    if (reconnectTimer) return;
    reconnectTimer = setTimeout(function () { reconnectTimer = null; openSSE(id); }, 1500);
  }

  /* SSE 事件路由（契约 7 类） */
  function handleEvent(ev) {
    var d = ev.data || {};
    if (d.seq != null && d.seq > lastSeq) lastSeq = d.seq;
    switch (ev.event) {
      case 'stage':
        paintExecTo(d.step);
        if (d.role) setRoleUI(d.role);
        break;
      case 'need_ask':
        renderAsk(d.question, d.options);
        break;
      case 'need_confirm':
        renderConfirm(d.question);
        break;
      case 'done':
        esClosed = true; closeSSEStream(); completeExec(); closeExecCard(); clearDecision();
        appendReceiptCardFromView({ task_id: currentTaskId, receipt: d.receipt, reversible: d.reversible });
        if (d.role) setRoleUI(d.role);
        currentView = { status: 'done', receipt: d.receipt, reversible: d.reversible };
        break;
      case 'failed':
        esClosed = true; closeSSEStream(); closeExecCard();
        renderError(d.error || '任务失败');
        break;
      case 'interrupt':
        // 打断即时：以 interrupt 三语义事件为信号渲染红条（优先于轮询感知）
        showSystemBar(L.interruptBarFromEvent(d));
        break;
      case 'canceled':
        esClosed = true; closeSSEStream(); closeExecCard(); clearDecision();
        break;
      default: break;
    }
  }

  /* ================= 轮询兜底（SSE 不可用 / 断线多次失败） ================= */
  var pollTimer = null;
  function startPolling(id) {
    stopPolling();
    pollTimer = setInterval(function () { pollTick(id); }, 900);
    pollTick(id);
  }
  function stopPolling() { if (pollTimer) { clearInterval(pollTimer); pollTimer = null; } }
  function pollTick(id) {
    api('/v1/tasks/' + encodeURIComponent(id)).then(function (view) {
      view.task_id = id; currentView = view;
      if (L.isDecision(view.status)) {
        stopPolling(); clearDecision();
        if (view.status === 'need_ask') renderAsk(view.question, view.options);
        else renderConfirm(view.question);
      } else if (L.isTerminal(view.status)) {
        stopPolling(); clearDecision();
        if (view.status === 'done') appendReceiptCardFromView(view);
        else renderError(view.error || '任务结束');
      } else {
        // running：兜底模式下仍按节奏推执行卡（SSE 缺席时的降级演示）
        ensureExecCard(); paintExecTo(L.EXEC_STAGES[Math.min(lastSeq, L.EXEC_STAGES.length - 1)]);
      }
    }).catch(function () { /* 静默，下轮重试 */ });
  }

  /* ================= 提交 / 应答 / 打断 =================
   *
   * 【伪代码逻辑层】（必写：提交→SSE 主路 / 轮询兜底；answer 续跑；说停→cancel+SSE interrupt）
   *   submit(text):
   *     reqId = genRequestId()（客户端幂等，重试复传 → server deduped）
   *     POST /v1/tasks {text,request_id} → {task_id}
   *     重置 seenSeqs/lastSeq，openSSE(id)；若 SSE 首连失败自动 startPolling。
   *   answerAndResume(id, ans):
   *     POST /v1/tasks/{id}/answer {answer}
   *     SSE 主路：连接在挂起期间保持，answer 后 server 继续推流，无需重连；
   *     兜底轮询：重新 startPolling(id)。
   *   说"停"：POST /v1/tasks/{id}/cancel（契约端点；legacy /v1/cancel 兼容）
   *     → server 立即推 interrupt 事件 → handleEvent 即时渲染红条。
   */
  function submit(text) {
    var reqId = L.genRequestId();
    appendUserBubble(text, false);
    var typing = appendTyping();
    api('/v1/tasks', { method: 'POST', body: JSON.stringify({ text: text, request_id: reqId }) })
      .then(function (res) {
        typing.remove();
        currentTaskId = res.task_id;
        seenSeqs = {}; lastSeq = 0; esClosed = false; sseUsed = false;
        openSSE(res.task_id);
      })
      .catch(function (err) {
        typing.remove();
        appendHarnessBubble('提交失败：' + err.message + '（检查 ⚙ server 地址/token）', { status: 'canceled' });
      });
  }

  function answerAndResume(id, ans) {
    clearDecision();
    api('/v1/tasks/' + encodeURIComponent(id) + '/answer', { method: 'POST', body: JSON.stringify({ answer: ans }) })
      .then(function () {
        if (sseUsed) { /* SSE 流仍在，继续等事件 */ }
        else startPolling(id); // 兜底模式续跑
      })
      .catch(function (err) { appendHarnessBubble('应答失败：' + err.message, { status: 'canceled' }); });
  }

  function maybeInterrupt(text) {
    if (!/^(停|停止|停下|stop)$/i.test(text.trim())) return false;
    if (!currentTaskId) return false;
    if (currentView && L.isTerminal(currentView.status)) return false;
    // 契约：POST /v1/tasks/{id}/cancel；legacy /v1/cancel 兼容。SSE interrupt 事件为即时信号。
    api('/v1/tasks/' + encodeURIComponent(currentTaskId) + '/cancel', { method: 'POST' })
      .catch(function () { return api('/v1/cancel', { method: 'POST', body: JSON.stringify({ task_id: currentTaskId }) }); })
      .catch(function () { /* 网络失败不阻塞 UI */ });
    stopPolling();
    return true;
  }

  /* ================= 多角色折叠条 UI ================= */
  var roleToggle = $('roleToggle'), roleBar = $('roleBar');
  roleToggle.onclick = function () {
    var open = roleBar.classList.toggle('hidden');
    roleToggle.setAttribute('aria-expanded', String(!open));
  };
  roleBar.querySelectorAll('.role-chip').forEach(function (chip) {
    chip.onclick = function () {
      roleBar.querySelectorAll('.role-chip').forEach(function (c) { c.classList.remove('active'); });
      chip.classList.add('active');
      setRoleUI(chip.getAttribute('data-role'));
      roleBar.classList.add('hidden');
    };
  });

  /* ================= 设置面板 ================= */
  $('settingsBtn').onclick = function () {
    $('serverUrl').value = settings.base;
    $('serverToken').value = settings.token;
    $('statusLine').textContent = '';
    $('settingsPanel').classList.toggle('hidden');
  };
  $('saveSettings').onclick = function () {
    saveSettings();
    $('statusLine').textContent = '已保存 ✓';
    setTimeout(function () { $('settingsPanel').classList.add('hidden'); }, 600);
  };
  $('testStatus').onclick = function () {
    saveSettings();
    $('statusLine').textContent = '连接中…';
    api('/v1/status').then(function (s) {
      $('statusLine').textContent = 'OK · v' + s.version + ' · tasks=' + s.tasks;
    }).catch(function (err) { $('statusLine').textContent = '失败：' + err.message; });
  };

  /* ================= 麦克风（webkitSpeechRecognition 两态探测） ================= */
  var rec = null, recording = false;
  var micBtn = $('micBtn');
  var recogKind = L.detectRecognition(window); // 'native'|'webkit'|'none'
  if (recogKind !== 'none') {
    var Ctor = window.SpeechRecognition || window.webkitSpeechRecognition;
    rec = new Ctor();
    rec.lang = 'zh-CN'; rec.interimResults = false; rec.maxAlternatives = 1;
    rec.onresult = function (e) { input.value = e.results[0][0].transcript; };
    rec.onend = function () { setRecording(false); };
    rec.onerror = function () { setRecording(false); };
    micBtn.onclick = function () { recording ? rec.stop() : rec.start(); setRecording(!recording); };
  } else {
    micBtn.title = '浏览器不支持语音识别，用键盘输入';
    micBtn.onclick = function () { input.focus(); };
  }
  function setRecording(on) {
    recording = on;
    micBtn.classList.toggle('recording', on);
    micBtn.querySelector('.wave').classList.toggle('hidden', !on);
    micBtn.querySelector('.mic-icon').classList.toggle('hidden', on);
  }

  /* ================= 发送 ================= */
  function doSend() {
    var text = input.value.trim();
    if (!text) return;
    input.value = '';
    if (maybeInterrupt(text)) return;
    submit(text);
  }
  $('sendBtn').onclick = doSend;
  input.addEventListener('keydown', function (e) { if (e.key === 'Enter') doSend(); });

  // 启动：拉一次角色实时（失败则本地静态）
  setRoleUI('planner');
  loadRoles();
})();
