#!/usr/bin/env node
/*
 * VoxSign iOS   ·        ( dependency, node in  assert)
 *   : node test.js
 * overwrite:   status  / back   resolve  /   by  decide /  tgt    /   decision pointrouteby /      /  disconnectstatus . 
 */
'use strict';
const L = require('./logic.js');

let pass = 0, fail = 0;
function eq(actual, expected, name) {
  const a = JSON.stringify(actual), e = JSON.stringify(expected);
  if (a === e) { pass++; console.log('  ✓ ' + name); }
  else { fail++; console.error('  ✗ ' + name + '\n      expected: ' + e + '\n      actual:   ' + a); }
}
function ok(cond, name) { eq(!!cond, true, name); }

console.log('== 1. 终态/决策点状态机 ==');
eq(L.isTerminal('done'), true, 'done 是终态');
eq(L.isTerminal('canceled'), true, 'canceled 是终态');
eq(L.isTerminal('interrupted'), true, 'interrupted 是终态');
eq(L.isTerminal('running'), false, 'running 不是终态');
eq(L.isTerminal('need_ask'), false, 'need_ask 不是终态（挂起等决策）');
eq(L.isDecision('need_confirm'), true, 'need_confirm 是决策点');
eq(L.isDecision('done'), false, 'done 不是决策点');

console.log('== 2. 回执四行解析 ==');
const receipt = '动作：NOTE 追加一行\n文件：notes.md\n结果：OK 已追加\n撤销：从备份 notes.md.20261002T1530.bak 恢复';
const r = L.parseReceipt(receipt);
eq(r.action, 'NOTE 追加一行', '动作行解析');
eq(r.files, 'notes.md', '文件行解析');
eq(r.result, 'OK 已追加', '结果行解析');
eq(r.undo, '从备份 notes.md.20261002T1530.bak 恢复', '撤销行解析');
const r2 = L.parseReceipt('');
eq(r2, { action: '', files: '', result: '', undo: '' }, '空回执不炸');
const r3 = L.parseReceipt('动作：COMMIT 提交\n撤销：不可撤销（不可逆，已人工确认）');
eq(r3.result, '', '缺行补空');
eq(r3.undo, '不可撤销（不可逆，已人工确认）', '半角/全角兼容撤销行');

console.log('== 3. 撤销按钮裁决 ==');
eq(L.extractUndo(r, true), { show: true, backup: 'notes.md.20261002T1530.bak', irreversible: false }, '可逆+有.bak → 显示撤销按钮并提取备份名');
eq(L.extractUndo(L.parseReceipt('动作：COMMIT\n撤销：不可撤销（不可逆，已人工确认）'), true).show, false, '撤销行声明不可撤销 → 不给按钮');
eq(L.extractUndo(r, false).show, false, 'server 未标 reversible → 不给按钮');
eq(L.extractUndo(L.parseReceipt('撤销：VHS_BACKUP_PATH: notes.md.20261002T.bak'), true).backup, 'notes.md.20261002T.bak', '兼容 VHS_BACKUP_PATH: 契约前缀');

console.log('== 4. 轻标签压缩 ==');
const badges = L.compressBadges({ status: 'running', receipt: receipt, reversible: true });
ok(badges.some(b => b.label === '执行中' && b.tone === 'blue'), 'running → 状态徽章"执行中"');
ok(badges.some(b => b.label === '笔记' && b.kind === 'intent'), 'NOTE 动作 → 意图徽章"笔记"');
ok(badges.some(b => b.label === '笔记域'), 'notes.md → 域徽章"笔记域"');
ok(badges.some(b => b.label === '可逆' && b.tone === 'green'), 'reversible → 风险徽章"可逆"');
const b2 = L.compressBadges({ status: 'need_confirm', question: '人工放行' });
ok(b2.some(b => b.label === '高风险·待放行' && b.tone === 'red'), 'need_confirm → 高风险红徽章');
const b3 = L.compressBadges({ status: 'done', receipt: '动作：COMMIT 提交', reversible: false });
ok(b3.some(b => b.label === '不可逆' && b.tone === 'red'), 'done 且不可逆 → 红徽章"不可逆"');

console.log('== 5. 一屏一个决策点路由 ==');
eq(L.nextDecisionPoint({ status: 'need_confirm', question: '人工放行（不可逆）：git commit' }).kind, 'confirm', 'need_confirm → 红色确认条');
const askDP = L.nextDecisionPoint({ status: 'need_ask', question: '你说的"那个文件"指哪个？', options: [{ id: 'f1', label: 'notes.md' }, { id: 'f2', label: 'main.go' }] });
eq(askDP.kind, 'ask', 'need_ask → 候选按钮');
eq(askDP.options.length, 2, '候选按钮带 options[{id,label}]');
eq(L.nextDecisionPoint({ status: 'done', receipt: receipt, reversible: true }).kind, 'receipt', 'done → 回执卡');
eq(L.nextDecisionPoint({ status: 'running' }).kind, 'running', 'running → 执行卡');
eq(L.nextDecisionPoint({ status: 'canceled', error: '任务被取消' }).kind, 'error', 'canceled → 错误条');
eq(L.nextDecisionPoint({ status: 'interrupted' }).kind, 'error', 'interrupted → 错误条');

console.log('== 6. 角色折叠（M5-3）==');
eq(L.roleForStatus('need_ask'), 'planner', '回问 → Planner');
eq(L.roleForStatus('need_confirm'), 'planner', '强确认 → Planner');
eq(L.roleForStatus('running'), 'executor', '执行 → Executor');
eq(L.roleForStatus('done'), 'verifier', '完成 → Verifier');
eq(L.ROLE_LABELS.verifier, 'Verifier', '角色 label 映射齐全');

console.log('== 7. 打断状态机（说"停" → 红色系统条）==');
const barRunning = L.interruptSystemBar({ status: 'running' });
ok(barRunning.active[0].indexOf('尚未产生文件变更') >= 0, 'running 无 receipt → 已生效=尚未变更');
eq(barRunning.actions, ['继续'], 'running 无备份 → 只有"继续"');
const barDone = L.interruptSystemBar({ status: 'done', receipt: receipt, reversible: true });
ok(barDone.active[0].indexOf('NOTE 追加一行') >= 0, 'done → 已生效列具体动作');
ok(barDone.actions.indexOf('撤销') >= 0 && barDone.actions.indexOf('继续') >= 0, '可逆 done → 可撤销/可继续');
eq(L.interruptSystemBar({}).actions, [], '空 view → 无进行中任务，无动作');

console.log('== 8. request_id 幂等键 ==');
eq(typeof L.genRequestId(), 'string', 'genRequestId 返回字符串');
ok(L.genRequestId() !== L.genRequestId(), '两次生成不同（幂等键唯一）');
ok(L.EXEC_STAGES.length >= 6, '执行卡阶段链非空');

console.log('== 9. SSE 线协议解析（M6） ==');
var block1 = 'event: stage\ndata: {"seq":1,"role":"planner","phase":"classify","step":"意图分类"}';
var ev1 = L.parseSSEBlock(block1);
eq(ev1.event, 'stage', 'SSE event: 名解析');
eq(ev1.data.seq, 1, 'SSE data JSON 解析');
eq(ev1.data.step, '意图分类', 'SSE step 中文字段');
var evDefault = L.parseSSEBlock('data: {"seq":2}');
eq(evDefault.event, 'message', 'event 缺省 = message');
var evBad = L.parseSSEBlock('data: {not json');
ok(evBad.data && evBad.data._raw !== undefined, '坏 JSON 不崩，落 _raw');
var evComment = L.parseSSEBlock(': keepalive\nevent: done\ndata: {"seq":9,"receipt":"动作：A"}');
eq(evComment.event, 'done', '行首冒号注释行忽略');

console.log('== 10. 重连幂等：seq 去重 ==');
var seen = {};
var evs = [
  L.parseSSEBlock('event: stage\ndata: {"seq":1,"step":"意图分类"}'),
  L.parseSSEBlock('event: stage\ndata: {"seq":2,"step":"执行"}'),
  L.parseSSEBlock('event: stage\ndata: {"seq":1,"step":"意图分类"}') // heavy heavy 
];
var fresh1 = L.filterNew(seen, evs);
eq(fresh1.length, 2, '首次只放行 seq 1,2（重复 1 去重）');
// disconnectlineheavylink ?after=2: server heavy  seq>2
var evs2 = [L.parseSSEBlock('event: stage\ndata: {"seq":3,"step":"校验"}')];
var fresh2 = L.filterNew(seen, evs2);
eq(fresh2.length, 1, '重连后只收到 seq 3');
eq(seen[1] && seen[2] && seen[3], 1, 'seen 集合累积 1/2/3');

console.log('== 11. 打断三语义（SSE interrupt 事件 → 红条） ==');
var bar = L.interruptBarFromEvent({ applied: ['NOTES 已追加一行'], notApplied: ['git commit'], canRollback: true });
ok(bar.active[0].indexOf('已生效：NOTES') >= 0, 'applied → 已生效行');
ok(bar.blocked[0].indexOf('未执行：git commit') >= 0, 'notApplied → 未执行行');
ok(bar.actions[0] === '撤销' && bar.actions.indexOf('继续') >= 0, 'canRollback → 撤销在前+继续');
var barNone = L.interruptBarFromEvent({ applied: [], notApplied: [], canRollback: false });
ok(barNone.active[0].indexOf('尚未产生文件变更') >= 0, '空 applied → 兜底文案');
eq(barNone.actions, ['继续'], 'canRollback=false → 只有继续');

console.log('== 12. 角色实时（GET /v1/roles） ==');
eq(L.activeRole([{ id: 'planner', label: 'Planner', active: true }, { id: 'executor' }]), 'planner', 'active 角色提取');
eq(L.activeRole([]), '', '空数组 → 空');
eq(L.activeRole(null), '', 'null 容错');
eq(L.stageIndex('执行'), 4, 'stage 中文名 → 执行卡下标');
eq(L.stageIndex('不存在的阶段'), -1, '未知阶段不跳（-1）');

console.log('== 13. 语音识别两态探测（webkitSpeechRecognition mock） ==');
eq(L.detectRecognition({ SpeechRecognition: function () {} }), 'native', '有标准 SpeechRecognition');
eq(L.detectRecognition({ webkitSpeechRecognition: function () {} }), 'webkit', '有 webkit 前缀');
eq(L.detectRecognition({}), 'none', '两态都无 → 键盘兜底');

console.log('\n----------------------------------------');
console.log('结果: ' + pass + ' 通过, ' + fail + ' 失败');
process.exit(fail === 0 ? 0 : 1);
