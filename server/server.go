// Package server 是手机端 HTTP API 面（M2 #10 / M3 INTERACT-v1 扩展）：
// 内网监听 + token 认证，把 pipeline.Run 暴露给手机。
//
// 正式端点集（INTERACT-v1，2026-10-02 定稿）：
//
//	POST /v1/tasks                 {text, space?}           提交任务 → 202 {task_id,status}
//	GET  /v1/tasks/{id}                                     轮询 {status,question?,options?,receipt?,attribution?,error?}
//	POST /v1/tasks/{id}/answer     {answer}                 回答当前唯一决策点（回问选项/确认词）
//	POST /v1/tasks/{id}/rollback                            从 <log_dir>/backups 回滚（仅可逆任务）
//	GET  /v1/status                                         {version,uptime,ok}
//
// 兼容旧端点（保留，文档标 deprecated）：/v1/run /v1/task/{id} /v1/confirm /v1/cancel /v1/health。
//
// 状态词（INTERACT-v1 §三）：need_ask（低置信回问）/ need_confirm（强确认）/ running / done。
// 认证（#32）：cfg.Server.Token / VHS_TOKEN 非空 → Bearer；token 空 → 仅 127.0.0.1。
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bytes"
	"voicesign-harness/config"
	"voicesign-harness/contract"
	"voicesign-harness/pipeline"
	"voicesign-harness/tools"
)

// version 与 main.go 同步（M3 INTERACT-v1 端点集）。
const version = "0.2.0"

// 任务状态机取值（INTERACT-v1 §三状态流转 + M2 兼容）。
const (
	stRunning     = "running"
	stWaiting     = "waiting_confirm" // 兼容旧字段（旧端点用）
	stNeedAsk     = "need_ask"        // 低置信回问（question 带选项）
	stNeedConfirm = "need_confirm"    // 强确认（红条）
	stDone        = "done"
	stCanceled    = "canceled"
	stInterrupted = "interrupted" // M4-1 ③：重启恢复的未完成任务，不自动续跑
)

// 角色映射（M5-3）：pipeline 13 阶段 → Planner / Executor / Verifier。
//
// 【伪代码逻辑层】（角色映射属裁决逻辑）：
//
//	planner  = 分类/域裁决/风险分级/确认闸/回问（决策与计划）——对应 need_ask/need_confirm/初始 running。
//	executor = 工具动作执行（NOTE/EDIT/COMMIT/QUERY）——running 中执行段。
//	verifier = verify 校验/归因/回执——done 终态（最后一段是校验+归因+回执）。
//	canceled/interrupted 阶段角色保留进入该状态前的最后角色。
const (
	RolePlanner  = "planner"
	RoleExecutor = "executor"
	RoleVerifier = "verifier"
)

// roleForStatus 把任务状态映射为当前角色（终态保留最后角色的近似）。
func roleForStatus(status string) string {
	switch status {
	case stNeedAsk, stNeedConfirm, stWaiting:
		return RolePlanner
	case stDone:
		return RoleVerifier
	case stRunning:
		return RoleExecutor
	default:
		return RolePlanner
	}
}

// stepName 把状态映射为中文阶段名。
func stepName(status string) string {
	switch status {
	case stRunning:
		return "执行"
	case stNeedAsk:
		return "回问"
	case stNeedConfirm:
		return "确认闸"
	case stDone:
		return "校验/归因"
	case stCanceled:
		return "取消"
	case stInterrupted:
		return "中断"
	default:
		return status
	}
}

// taskState 是一次异步任务的完整状态（含 rollback 所需的可逆/备份信息）。
type taskState struct {
	ID        string               `json:"task_id"`
	RequestID string               `json:"request_id,omitempty"` // M4-1 ① 重试去重键
	Text      string               `json:"text,omitempty"`       // M4-1 ② need_ask 续跑原文
	Status    string               `json:"status"`
	Role      string               `json:"role,omitempty"` // M5-3：当前角色
	Question  string               `json:"question,omitempty"`
	Options   []pipeline.AskOption `json:"options,omitempty"`

	// askedQuestions / askRounds（缺口 G8 修复）：已问过的问题集合 + 澄清轮次。
	//
	// 评审 S1：只比对"紧邻上一轮"在 Q1→Q2→Q1→Q2 交替时永不命中，仍会无限循环。
	// 改为记录**问题集合**并加轮次上限，任一命中即判收敛失败。
	// 注（评审 S5）：这两个字段未导出、不参与 JSON 序列化；跨进程恢复后守卫失效，
	// 属已知限制——当前 server 的任务表是内存态，重启恢复路径另有 interrupted 处理。
	askedQuestions map[string]bool
	askRounds      int
	Outcome        *pipeline.Outcome `json:"outcome,omitempty"`
	Err            string            `json:"error,omitempty"`

	// rollback 元数据
	Reversible bool   `json:"reversible,omitempty"`
	BackupPath string `json:"backup_path,omitempty"`
	TargetPath string `json:"target_path,omitempty"`

	startedAt time.Time

	confirmCh chan bool
	cancel    context.CancelFunc

	// M6-1 SSE：事件记录 + 活跃订阅者。
	eventSeq  int
	events    []sseEvent
	listeners []chan sseEvent
}

// sseEvent 是一条 SSE 事件（seq 单调递增）。Data 被平铺进最终 JSON（契约：data 必含 seq）。
type sseEvent struct {
	Seq  int            `json:"seq"`
	Type string         `json:"-"`
	Data map[string]any `json:"-"`
}

// flattened 返回 {seq, ...Data}。
func (e sseEvent) flattened() map[string]any {
	out := map[string]any{"seq": e.Seq, "type": e.Type}
	for k, v := range e.Data {
		out[k] = v
	}
	return out
}

// Server 持有配置、pipeline 模板与任务表。
type Server struct {
	cfg  *config.Config
	tmpl *pipeline.Options

	boot time.Time

	mu    sync.Mutex
	tasks map[string]*taskState
	byReq map[string]string // request_id → task_id（M4-1 ①）
}

// New 构造 Server 并从 <log_dir>/tasks 恢复历史任务（M4-1 ③）。
func New(cfg *config.Config, o *pipeline.Options) *Server {
	s := &Server{cfg: cfg, tmpl: o, boot: time.Now(), tasks: map[string]*taskState{}, byReq: map[string]string{}}
	s.restore()
	return s
}

// Start 在 cfg.ServerBind() 上起 HTTP 服务。
// Handler 返回完整路由（**Start 与判据共用同一份装配** —— 避免"判据的装配 ≠ 真启动的装配"）。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	// INTERACT-v1 正式端点
	mux.HandleFunc("/v1/tasks", s.auth(s.handleTasksPost))
	mux.HandleFunc("/v1/tasks/", s.auth(s.handleTasksSub))
	mux.HandleFunc("/v1/status", s.auth(s.handleStatus))
	mux.HandleFunc("/v1/roles", s.auth(s.handleRoles)) // M5-3
	// 兼容旧端点（deprecated，保留）
	mux.HandleFunc("/v1/run", s.auth(s.handleRun))
	mux.HandleFunc("/v1/voice", s.auth(s.handleVoice)) // 线 A 接语音：文本 → 线 B 意图 → 执行
	mux.HandleFunc("/v1/task/", s.auth(s.handleTaskGet))
	mux.HandleFunc("/v1/confirm", s.auth(s.handleConfirm))
	mux.HandleFunc("/v1/cancel", s.auth(s.handleCancel))
	mux.HandleFunc("/v1/health", s.auth(s.handleHealth))
	return mux
}

func (s *Server) Start() error {
	mux := s.Handler()
	addr := s.cfg.ServerBind()
	fmt.Printf("vhs server listening on %s (token_set=%v)\n", addr, s.cfg.Server.Token != "")
	return http.ListenAndServe(addr, mux)
}

// ---------- 认证中间件（#32） ----------

// cors 中间件（M7）：对所有 /v1/* 加 CORS 头；OPTIONS 预检短路返回。
// 简单胶水：回显 Origin、允许 Authorization/Content-Type/X-Token。
func (s *Server) cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Token")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return s.cors(func(w http.ResponseWriter, r *http.Request) {
		// 本机豁免：token 为空时只允许回环地址。
		if s.cfg.Server.Token == "" {
			if !isLoopback(r.RemoteAddr) {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "缺 token 且非本机访问"})
				return
			}
		} else {
			// 有 token → 校验 Authorization: Bearer <token> 或 X-Token。
			tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if tok == "" {
				tok = r.Header.Get("X-Token")
			}
			if tok != s.cfg.Server.Token {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "token 无效"})
				return
			}
		}
		next(w, r)
	})
}

func isLoopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// ---------- 端点 ----------

type runReq struct {
	Text string `json:"text"`
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "仅 POST"})
		return
	}
	var req runReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体应为 JSON {text}"})
		return
	}

	ts := &taskState{
		ID:        fmt.Sprintf("task-%d", time.Now().UnixNano()),
		Status:    stRunning,
		confirmCh: make(chan bool, 1),
	}
	// 任务生命周期独立于 HTTP 请求（响应返回后请求 ctx 会被取消）。
	ctx, cancel := context.WithCancel(context.Background())
	ts.cancel = cancel

	s.mu.Lock()
	s.tasks[ts.ID] = ts
	s.mu.Unlock()

	// 克隆模板 Options，注入本任务的 ConfirmFn（桥接到手机 /v1/confirm）。
	o := *s.tmpl
	o.ConfirmFn = func(taskID, question string) (bool, error) {
		s.mu.Lock()
		ts.Status = stWaiting
		ts.Question = question
		s.mu.Unlock()

		select {
		case approved := <-ts.confirmCh:
			s.mu.Lock()
			ts.Status = stRunning
			ts.Question = ""
			s.mu.Unlock()
			return approved, nil
		case <-ctx.Done():
			return false, errors.New("任务被取消")
		}
	}

	go func() {
		out, err := pipeline.Run(ctx, &o, req.Text)
		s.mu.Lock()
		ts.Outcome = &out
		if err != nil {
			ts.Err = err.Error()
			ts.Status = stCanceled
		} else if ctx.Err() != nil {
			ts.Status = stCanceled
		} else {
			ts.Status = stDone
		}
		s.mu.Unlock()
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"task_id": ts.ID})
}

// voiceReq 是 /v1/voice 的入参：**只收文本**（不碰音频编解码）。
type voiceReq struct {
	Text string `json:"text"`
}

// asrEndpoint 返回线 B 地址（可配；默认本机 8123）。
func asrEndpoint() string {
	if v := os.Getenv("VHS_ASR_ENDPOINT"); v != "" {
		return v
	}
	return "http://127.0.0.1:8123"
}

// handleVoice：**线 A 接语音的入口**。
//
// 边界（Lead 2026-10-03）：
//
//	① 线 A **不内嵌线 B 代码**，只走 HTTP（保持 "ASR 独立、多消费方"）；
//	② 线 B 不可达 ⇒ **明确报错 + degraded**，**不许静默降级成"直接执行原文"**；
//	③ 红线：意图为 ASK / need_disambiguate ⇒ **绝不执行**，把问题交回；
//	④ 其余走既有 pipeline.Run 路径（含域门禁与不可逆确认）。
func (s *Server) handleVoice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "仅 POST"})
		return
	}
	var req voiceReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Text) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体应为 JSON {text}（ASR 识别文本）"})
		return
	}
	intent, err := s.callASRProcess(r.Context(), req.Text)
	if err != nil {
		// ② **不静默降级**：明确 503 + degraded，且**不创建任何任务**
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error":    "ASR 服务不可达（不静默降级为直接执行原文）: " + err.Error(),
			"degraded": true,
			"endpoint": asrEndpoint(),
		})
		return
	}
	// ③ 红线：ASK / need_disambiguate ⇒ 绝不执行
	needAsk, _ := intent["need_disambiguate"].(bool)
	typ, _ := intent["type"].(string)
	ask, _ := intent["ask"].(string)
	if needAsk || strings.EqualFold(typ, "ASK") || ask != "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"executed": false, "reason": "意图需要澄清（ASK / 低置信）—— 红线：Ask != '' 绝不执行",
			// 判据 ④（VHS-VOICE-001）：响应**必带** intent_source ∈ {asr-intent, text-fallback}
			// ⚠️ 2026-10-03 真装配级真跑发现：此前响应里**一次都没出现过**该字段，
			//    而 `trajectory.KindIntentSource` 其常量虽已登记、**从无写入点**。
			"intent_source": "asr-intent",
			"intent":        intent,
		})
		return
	}
	// ④ 走既有执行路径（含域门禁 / 不可逆确认）
	ts := &taskState{ID: fmt.Sprintf("task-%d", time.Now().UnixNano()), Status: stRunning, confirmCh: make(chan bool, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	ts.cancel = cancel
	s.mu.Lock()
	s.tasks[ts.ID] = ts
	s.mu.Unlock()
	o := *s.tmpl
	o.ConfirmFn = func(taskID, question string) (bool, error) {
		s.mu.Lock()
		ts.Status = stWaiting
		ts.Question = question
		s.mu.Unlock()
		select {
		case approved := <-ts.confirmCh:
			s.mu.Lock()
			ts.Status = stRunning
			ts.Question = ""
			s.mu.Unlock()
			return approved, nil
		case <-ctx.Done():
			return false, errors.New("任务被取消")
		}
	}
	go func() {
		out, err := pipeline.Run(ctx, &o, req.Text)
		s.mu.Lock()
		ts.Outcome = &out
		if err != nil {
			ts.Err = err.Error()
			ts.Status = stCanceled
		} else if ctx.Err() != nil {
			ts.Status = stCanceled
		} else {
			ts.Status = stDone
		}
		s.mu.Unlock()
	}()
	// 判据 ④：执行路径同样必带 intent_source（本次意图来自线 B ⇒ asr-intent）
	writeJSON(w, http.StatusAccepted, map[string]any{
		"task_id": ts.ID, "intent": intent, "intent_source": "asr-intent",
	})
}

// callASRProcess 调线 B 的 POST /v1/process（**只走 HTTP**）。
func (s *Server) callASRProcess(ctx context.Context, text string) (map[string]any, error) {
	body, _ := json.Marshal(map[string]string{"text": text, "session_id": "voice"})
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodPost, strings.TrimRight(asrEndpoint(), "/")+"/v1/process", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("线 B HTTP %d", resp.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Server) handleTaskGet(w http.ResponseWriter, r *http.Request) {
	// 路径形如 /v1/task/{id}
	id := strings.TrimPrefix(r.URL.Path, "/v1/task/")
	s.mu.Lock()
	ts, ok := s.tasks[id]
	s.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "未知 task_id"})
		return
	}
	writeJSON(w, http.StatusOK, ts)
}

type confirmReq struct {
	TaskID   string `json:"task_id"`
	Approved bool   `json:"approved"`
}

func (s *Server) handleConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "仅 POST"})
		return
	}
	var req confirmReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体应为 JSON {task_id,approved}"})
		return
	}
	s.mu.Lock()
	ts, ok := s.tasks[req.TaskID]
	s.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "未知 task_id"})
		return
	}
	select {
	case ts.confirmCh <- req.Approved:
	default:
		writeJSON(w, http.StatusConflict, map[string]any{"error": "任务不在等待确认状态"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type cancelReq struct {
	TaskID string `json:"task_id"`
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "仅 POST"})
		return
	}
	var req cancelReq
	_ = json.NewDecoder(r.Body).Decode(&req)
	s.mu.Lock()
	ts, ok := s.tasks[req.TaskID]
	s.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "未知 task_id"})
		return
	}
	if ts.cancel != nil {
		ts.cancel()
	}
	s.mu.Lock()
	applied := []string{}
	notApplied := []string{"任务已取消，未执行动作"}
	if ts.Outcome != nil && len(ts.Outcome.Receipts) > 0 {
		applied = []string{"已执行动作见回执"}
	}
	s.emitEvent(ts, "interrupt", map[string]any{
		"applied":     applied,
		"notApplied":  notApplied,
		"canRollback": ts.Reversible,
	})
	s.emitEvent(ts, "canceled", map[string]any{})
	ts.Status = stCanceled
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"bind":      s.cfg.ServerBind(),
		"tasks":     len(s.tasks),
		"token_set": s.cfg.Server.Token != "",
	})
}

// ---------- INTERACT-v1 正式端点 ----------

type tasksPostReq struct {
	Text      string `json:"text"`
	Space     string `json:"space,omitempty"`
	RequestID string `json:"request_id,omitempty"` // M4-1 ① 重试去重键
}

// spawnTask 起一个任务（text 为完整原文；spaceHint 非空时前置"在 <space>"）。
// requestID 非空时登记 byReq 用于重试去重。状态迁移后自动 persist。
func (s *Server) spawnTask(text, spaceHint, requestID string) *taskState {
	ts := &taskState{
		ID:        fmt.Sprintf("task-%d", time.Now().UnixNano()),
		RequestID: requestID,
		Text:      text,
		Status:    stRunning,
		Role:      RolePlanner, // M5-3：初始在规划阶段
		startedAt: time.Now(),
		confirmCh: make(chan bool, 1),
	}
	ctx, cancel := context.WithCancel(context.Background())
	ts.cancel = cancel

	s.mu.Lock()
	s.tasks[ts.ID] = ts
	if requestID != "" {
		s.byReq[requestID] = ts.ID
	}
	s.persist(ts)
	s.mu.Unlock()

	s.runPipeline(ts, ctx, text, spaceHint)
	return ts
}

// runPipeline 在给定 ts 上跑一次 pipeline（首次提交或 need_ask 续跑共用）。
func (s *Server) runPipeline(ts *taskState, ctx context.Context, text, spaceHint string) {
	o := *s.tmpl
	o.ConfirmFn = func(taskID, question string) (bool, error) {
		s.mu.Lock()
		s.markStatus(ts, stNeedConfirm)
		ts.Question = question
		s.emitEvent(ts, "need_confirm", map[string]any{"question": question})
		s.mu.Unlock()
		select {
		case approved := <-ts.confirmCh:
			s.mu.Lock()
			s.markStatus(ts, stRunning)
			ts.Question = ""
			s.mu.Unlock()
			return approved, nil
		case <-ctx.Done():
			return false, errors.New("任务被取消")
		}
	}

	fullText := text
	if spaceHint != "" {
		fullText = "在 " + spaceHint + " " + text
	}
	go func() {
		out, err := pipeline.Run(ctx, &o, fullText)
		s.mu.Lock()
		defer s.mu.Unlock()
		ts.Outcome = &out
		switch {
		case err != nil:
			ts.Err = err.Error()
			s.markStatus(ts, stCanceled)
		case ctx.Err() != nil:
			s.markStatus(ts, stCanceled)
		case out.Ask != "":
			// 缺口 G8 修复：澄清无法收敛时明确失败，而不是无限回问。
			// 评审 S1：判定依据是**问题集合 + 轮次上限**，不是"与上一轮逐字相同"
			// —— 后者在 Q1→Q2→Q1→Q2 交替时永远不命中。
			if ts.askedQuestions == nil {
				ts.askedQuestions = map[string]bool{}
			}
			if ts.askedQuestions[out.Ask] || ts.askRounds >= maxAskRounds {
				ts.Err = "澄清未收敛：同一个问题被反复问到，或已达澄清轮次上限；" +
					"请直接用完整指令再说一遍（明确说出对象）"
				s.markStatus(ts, stCanceled)
				s.emitEvent(ts, "canceled", map[string]any{
					"reason": "ask_not_converging", "rounds": ts.askRounds,
				})
				s.persist(ts)
				return
			}
			// 回问出口：挂起为 need_ask，等待 /answer 续跑（M4-1 ②）。
			ts.askedQuestions[out.Ask] = true
			ts.askRounds++
			s.markStatus(ts, stNeedAsk)
			ts.Question = out.Ask
			ts.Options = out.Options // M4-3 ① 结构化 [{id,label}]
			s.emitEvent(ts, "need_ask", map[string]any{
				"question": out.Ask, "options": out.Options,
			})
			s.persist(ts)
		default:
			s.markStatus(ts, stDone)
			ts.Reversible = isReversibleIntent(out.Intent.Intent)
			ts.BackupPath = pickBackupPath(out.Receipts, out.View.Undo)
			ts.TargetPath = pickTargetPath(out.Intent)
			if out.Intent.Intent == contract.IntentNote {
				ts.TargetPath = filepath.Join(s.cfg.Global.LogDir, "notes.md")
				if ts.BackupPath == "" {
					ts.BackupPath = newestBackup(s.cfg.Global.LogDir)
				}
			}
			s.emitEvent(ts, "done", map[string]any{
				"receipt":     contract.RenderReceipt(out.View),
				"attribution": out.Attribution,
				"reversible":  ts.Reversible,
				"role":        ts.Role,
			})
			s.persist(ts)
		}
	}()
}

// maxAskRounds 是同一任务允许的最大澄清轮次（缺口 G8 修复）。
// 超过即判"澄清未收敛"并取消，不再无限回问。
const maxAskRounds = 3

// askAnaphora 是续跑时应当被澄清答案替换掉的指代词（长词在前，避免"那个文件"被"那个"抢先切分）。
var askAnaphora = []string{
	"那个文件", "这个文件", "那个页面", "这个页面", "那个项目", "这个项目",
	"那个", "这个", "它",
}

// pronounIndex 返回 trig 在 text 中的**字节**下标；未命中返回 -1。
//
// 评审 S2：中文没有词边界，裸单字代词用 strings.Contains 会误伤词的一部分
// （"其它" 里的 "它"、"由它" 里的 "它"）。故对单字代词做前文排除。
func pronounIndex(text, trig string) int {
	runes := []rune(text)
	tr := []rune(trig)
	if len(tr) == 0 || len(tr) > len(runes) {
		return -1
	}
	if len(tr) > 1 {
		return strings.Index(text, trig)
	}
	for i, r := range runes {
		if r != tr[0] {
			continue
		}
		if i > 0 {
			switch runes[i-1] {
			case '其', '由': // 其它 / 由它 —— 是词的一部分，不是代词
				continue
			}
		}
		return len(string(runes[:i]))
	}
	return -1
}

// stripOptionPrefix 把候选按钮 id（dict:xxx / rec:xxx）还原成可读实体名。
func stripOptionPrefix(s string) string {
	s = strings.TrimSpace(s)
	for _, p := range []string{"dict:", "rec:"} {
		if strings.HasPrefix(s, p) {
			return strings.TrimSpace(strings.TrimPrefix(s, p))
		}
	}
	return s
}

// resolveClarified 用澄清答案替换原句里的指代词，得到自洽的续跑文本（缺口 G8 修复）。
//
// 原实现只把答案追加在句尾（`原文 + " 澄清：" + 答案`），原句里的"这个/那个"仍在，
// refer 层依旧找不到候选 → 再次写出同一句 Ask → 手机上永远走不出这一屏。
//
// 替换后用引号包裹答案，使分类器的 extractPath 能直接抽出显式对象：
//
//	"把这个改一下" + "main.go" → "把“main.go”改一下"，不再需要指代消解。
//
// 该"引号可被抽出"的假设由 server 包的单测 TestResolveClarifiedExtractsObject 锁定（评审 S3）。
func resolveClarified(text, answer string) string {
	a := stripOptionPrefix(answer)
	if a == "" {
		return text
	}
	for _, trig := range askAnaphora {
		if i := pronounIndex(text, trig); i >= 0 {
			return text[:i] + "“" + a + "”" + text[i+len(trig):]
		}
	}
	return text
}

// resumeAsk 把 need_ask 任务用 answer 续跑；answer 为空则**拒绝**并保持等待态。
// 返回 false 表示未受理（调用方应回 400），任务仍停在 need_ask，用户可以再答一次。
func (s *Server) resumeAsk(ts *taskState, answer string) bool {
	if strings.TrimSpace(answer) == "" {
		// 评审 S7：空答案不是"澄清未收敛"，原因不同，不能混报。
		return false
	}
	// 评审 P3：否定仲裁的回问文案明确让用户「说取消」，这里必须给它确定的语义，
	// 而不是让它落进"澄清未收敛"的错误路径。
	if isCancelAnswer(answer) {
		_, cancel := context.WithCancel(context.Background())
		ts.cancel = cancel
		ts.Question = ""
		ts.Options = nil
		ts.Err = ""
		s.markStatus(ts, stCanceled)
		s.emitEvent(ts, "canceled", map[string]any{"reason": "user_canceled_at_ask"})
		s.persist(ts)
		return true
	}
	ctx, cancel := context.WithCancel(context.Background())
	ts.cancel = cancel
	s.markStatus(ts, stRunning)
	ts.Question = ""
	ts.Options = nil
	// 注意：这里**不**重置 askedQuestions / askRounds —— 它们要跨轮保留，
	// 才能识别"同一个问题又被问了一遍"（评审 S1）。

	// 评审 G5-P2-7：多动作回问的答案应被当作**新的单条指令**执行。
	//
	// 原句本身含多动作，把它拼回去（`原文 + " 澄清：" + 答案`）只会再次触发多动作检测，
	// 形成澄清循环。这给回问文案承诺的「先说要先做哪个」一个**确定语义**：
	// 用户说的那句，就是要执行的那一件。
	if ts.Outcome != nil && ts.Outcome.Intent.Conflict == contract.ConflictMultiAction {
		s.runPipeline(ts, ctx, strings.TrimSpace(answer), "")
		return true
	}

	prefix := ""
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "edit":
		prefix = "改文件 "
	case "query":
		prefix = "查代码 "
	case "note":
		prefix = "记一下 "
	case "commit":
		prefix = "提交 "
	}
	substituted := resolveClarified(ts.Text, answer)
	if substituted == ts.Text {
		// 无可替换指代词的兜底路径：仍要剥离候选 id 前缀，避免两条路径处理不一致（评审 S4）。
		substituted = " 澄清：" + stripOptionPrefix(answer)
	}
	s.runPipeline(ts, ctx, prefix+substituted, "")
	return true
}

func (s *Server) handleTasksPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "仅 POST"})
		return
	}
	var req tasksPostReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Text) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体应为 JSON {text}"})
		return
	}

	// M4-1 ①：同 request_id 重复提交 → 返回既有任务，不重复执行/写轨迹。
	s.mu.Lock()
	if req.RequestID != "" {
		if existingID, ok := s.byReq[req.RequestID]; ok {
			if existing, ok2 := s.tasks[existingID]; ok2 {
				s.mu.Unlock()
				writeJSON(w, http.StatusOK, map[string]string{
					"task_id": existing.ID, "status": existing.Status, "deduped": "true",
				})
				return
			}
		}
	}
	s.mu.Unlock()

	ts := s.spawnTask(req.Text, req.Space, req.RequestID)
	writeJSON(w, http.StatusAccepted, map[string]string{"task_id": ts.ID, "status": ts.Status})
}

// handleTasksSub 路由 /v1/tasks/{id} 与 /v1/tasks/{id}/answer、/rollback。
func (s *Server) handleTasksSub(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/v1/tasks/")
	parts := strings.Split(strings.TrimSuffix(rest, "/"), "/")
	id := parts[0]

	s.mu.Lock()
	ts, ok := s.tasks[id]
	s.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "未知 task_id"})
		return
	}

	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		s.writeTaskView(w, ts)
	case len(parts) == 2 && parts[1] == "events" && r.Method == http.MethodGet:
		s.handleEvents(w, r, ts)
	case len(parts) == 2 && parts[1] == "cancel" && r.Method == http.MethodPost:
		s.handleCancelSub(w, r, ts)
	case len(parts) == 2 && parts[1] == "answer" && r.Method == http.MethodPost:
		s.handleAnswer(w, r, ts)
	case len(parts) == 2 && parts[1] == "rollback" && r.Method == http.MethodPost:
		s.handleRollback(w, ts)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "不支持的方法/路径"})
	}
}

// handleEvents 实现 SSE 流（M6-1）：?after=<lastSeq> 重放，然后挂起订阅直到终态。
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request, ts *taskState) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	after := 0
	if a := r.URL.Query().Get("after"); a != "" {
		fmt.Sscanf(a, "%d", &after)
	}

	s.mu.Lock()
	// 重放 seq > after 的事件
	lastTerminal := false
	for _, ev := range ts.events {
		if ev.Seq > after {
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, mustJSON(ev.flattened()))
			if ev.Type == "done" || ev.Type == "failed" || ev.Type == "canceled" {
				lastTerminal = true
			}
		}
	}
	// 已终态则直接关闭
	if lastTerminal || ts.Status == stDone || ts.Status == stCanceled || ts.Status == stInterrupted {
		s.mu.Unlock()
		flusher.Flush()
		return
	}
	ch := make(chan sseEvent, 16)
	ts.listeners = append(ts.listeners, ch)
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		for i, l := range ts.listeners {
			if l == ch {
				ts.listeners = append(ts.listeners[:i], ts.listeners[i+1:]...)
				break
			}
		}
		s.mu.Unlock()
		close(ch)
	}()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-ch:
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, mustJSON(ev.flattened()))
			flusher.Flush()
			if ev.Type == "done" || ev.Type == "failed" || ev.Type == "canceled" {
				return
			}
		}
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// handleCancelSub 路由 POST /v1/tasks/{id}/cancel（INTERACT-v1 新形态）。
func (s *Server) handleCancelSub(w http.ResponseWriter, r *http.Request, ts *taskState) {
	if ts.cancel != nil {
		ts.cancel()
	}
	s.mu.Lock()
	applied := []string{}
	notApplied := []string{"任务已取消，未执行动作"}
	if ts.Outcome != nil && len(ts.Outcome.Receipts) > 0 {
		applied = []string{"已执行动作见回执"}
	}
	s.emitEvent(ts, "interrupt", map[string]any{
		"applied": applied, "notApplied": notApplied, "canRollback": ts.Reversible,
	})
	s.emitEvent(ts, "canceled", map[string]any{})
	ts.Status = stCanceled
	s.persist(ts)
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// writeTaskView 按 INTERACT-v1 形状渲染（receipt=四行，attribution=六格）。
func (s *Server) writeTaskView(w http.ResponseWriter, ts *taskState) {
	body := map[string]any{"task_id": ts.ID, "status": ts.Status, "role": ts.Role}
	if ts.Question != "" {
		body["question"] = ts.Question
	}
	if len(ts.Options) > 0 {
		body["options"] = ts.Options
	}
	if ts.Err != "" {
		body["error"] = ts.Err
	}
	if ts.Outcome != nil {
		body["receipt"] = contract.RenderReceipt(ts.Outcome.View)
		body["attribution"] = ts.Outcome.Attribution
		if ts.Reversible {
			body["reversible"] = true
		}
	}
	writeJSON(w, http.StatusOK, body)
}

type answerReq struct {
	Answer string `json:"answer"`
}

// handleAnswer：一屏一个决策点。need_confirm → 确认桥；need_ask → 以澄清续跑（M4-1 ②）。
func (s *Server) handleAnswer(w http.ResponseWriter, r *http.Request, ts *taskState) {
	var req answerReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体应为 JSON {answer}"})
		return
	}
	s.mu.Lock()
	st := ts.Status
	s.mu.Unlock()

	// need_ask：以澄清文本续跑同一任务。
	if st == stNeedAsk {
		s.mu.Lock()
		accepted := s.resumeAsk(ts, req.Answer)
		s.mu.Unlock()
		if !accepted {
			// 空答案：拒绝受理，任务仍停在 need_ask，用户可以再答一次（评审 S7）。
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "澄清答案不能为空；任务仍在等待作答", "status": st,
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "resumed": true})
		return
	}

	if st != stNeedConfirm {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "当前无待回答的决策点", "status": st})
		return
	}
	// 确认词：执行/y/yes/true/1 → 放行；其余 → 拒绝。
	approved := isConfirmWord(req.Answer)
	select {
	case ts.confirmCh <- approved:
	default:
		writeJSON(w, http.StatusConflict, map[string]string{"error": "决策点已过期"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "approved": approved})
}

// handleRollback：从备份恢复。
//
// 【伪代码逻辑层】（必写模块：可逆性判定 + 备份恢复 + 拒绝路径）
//
// 控制流：
//
//	if ts.Status != done: 409 "任务未完成，不可回滚"。
//	if !isReversibleIntent(intent): 409 "不可逆任务（COMMIT/DEPLOY 等）禁止回滚"。
//	if ts.BackupPath == "" || 文件不存在: 404 "无可撤销备份"。
//	target = ts.TargetPath；若空 → 404（不知道还原到哪）。
//	copy(BackupPath → target)（os.ReadFile+WriteFile，0644）。
//	成功 → 200 {ok:true, restored:target}。
//
// 异常：copy 失败 → 500；server 重启后内存态丢失（M3 接受，文档标注）。
func (s *Server) handleRollback(w http.ResponseWriter, ts *taskState) {
	s.mu.Lock()
	st := ts.Status
	rev := ts.Reversible
	bak := ts.BackupPath
	tgt := ts.TargetPath
	s.mu.Unlock()

	if st != stDone {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "任务未完成，不可回滚", "status": st})
		return
	}
	if !rev {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "不可逆任务（COMMIT/DEPLOY）禁止回滚"})
		return
	}
	if bak == "" || tgt == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "无可撤销备份或目标未知"})
		return
	}
	data, err := os.ReadFile(bak)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "备份文件不存在"})
		return
	}
	if err := os.WriteFile(tgt, data, 0o644); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "还原失败: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "restored": tgt})
}

// handleStatus：GET /v1/status（INTERACT-v1）。
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"version": version,
		"uptime":  time.Since(s.boot).Seconds(),
		"tasks":   len(s.tasks),
	})
}

// handleRoles（M5-3）：返回三角色定义 + 当前任务角色。
func (s *Server) handleRoles(w http.ResponseWriter, r *http.Request) {
	all := []map[string]any{
		{"id": RolePlanner, "label": "Planner（规划）"},
		{"id": RoleExecutor, "label": "Executor（执行）"},
		{"id": RoleVerifier, "label": "Verifier（校验）"},
	}
	// 取最近一个非终态任务的角色；无则全 inactive。
	s.mu.Lock()
	var current *taskState
	for _, ts := range s.tasks {
		if ts.Status == stRunning || ts.Status == stNeedAsk || ts.Status == stNeedConfirm || ts.Status == stWaiting {
			current = ts
			break
		}
	}
	s.mu.Unlock()

	resp := map[string]any{"roles": all}
	if current != nil {
		resp["task_id"] = current.ID
		resp["role"] = current.Role
	}
	writeJSON(w, http.StatusOK, resp)
}

// ---------- M4-1：持久化 / 去重 / ask 续跑 ----------
//
// 【伪代码逻辑层】（必写模块：①request_id 幂等去重 + ②need_ask 挂起续跑 + ③重启恢复）
//
// POST /v1/tasks：
//   if body.request_id 非空且 s.byReq[request_id] 存在:
//       ts = 既有任务；if ts.status∈{running,need_confirm,need_ask}:
//           return 202 {task_id, status}（合并等待，不开第二份）
//       else: return 200 {task_id, status:既有结果}（不重复执行/写轨迹）
//   else: 新建 ts，spawnTask（原逻辑）。
//
// POST /v1/tasks/{id}/answer：
//   if ts.status == need_ask:
//       fullText = ts.Text + " 澄清：" + answer   // 最简续跑=以澄清重跑
//       ts.status = running；spawnTask 的 goroutine 重跑 pipeline（复用 ts.ID）
//       return 200 {ok:true, resumed:true}
//   elif ts.status == need_confirm: 走原 confirmCh 桥。
//   else: 409。
//
// 持久化：每次状态迁移后 persist(ts) → <log_dir>/tasks/<id>.json（tmp+rename 原子）。
// restore()（New 时）：遍历 tasks/*.json，done/canceled 原样入表；
//   running/need_confirm/need_ask → 标 interrupted（不重建 confirmCh/cancel，防死等）。
// 异常：tasks 目录不可写 → 仅内存运行（M4-1 可接受）；restore 坏文件跳过。

func (s *Server) tasksDir() string {
	return filepath.Join(s.cfg.Global.LogDir, "tasks")
}

// persist 原子落盘任务状态（tmp + rename）。
func (s *Server) persist(ts *taskState) {
	dir := s.tasksDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	b, err := json.MarshalIndent(ts, "", "  ")
	if err != nil {
		return
	}
	final := filepath.Join(dir, ts.ID+".json")
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, final)
}

// restore 启动时加载历史任务；未完成状态标 interrupted。
func (s *Server) restore() {
	dir := s.tasksDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var ts taskState
		if json.Unmarshal(data, &ts) != nil {
			continue
		}
		switch ts.Status {
		case stRunning, stNeedAsk, stNeedConfirm, stWaiting:
			ts.Status = stInterrupted
			ts.Err = "任务在重启时被中断，请重新提交"
		}
		s.tasks[ts.ID] = &ts
		if ts.RequestID != "" {
			s.byReq[ts.RequestID] = ts.ID
		}
	}
}

// emitEvent 追加一条 SSE 事件（seq 自增）并广播给活跃订阅者。
//
// 【伪代码逻辑层】（M6-1：事件 emit 时序/重放/打断广播）：
//
//	ts.eventSeq++; ev = {seq: ts.eventSeq, type, data}
//	ts.events = append(ts.events, ev)（与任务持久化同生命周期）
//	for ch in ts.listeners: non-blocking send ev（满则跳过，防背压阻塞）
//	终态事件（done/failed/canceled）发送后不关闭 listener——由 handler 检测终态后退出。
func (s *Server) emitEvent(ts *taskState, typ string, data map[string]any) {
	ts.eventSeq++
	ev := sseEvent{Seq: ts.eventSeq, Type: typ, Data: data}
	ts.events = append(ts.events, ev)
	for _, ch := range ts.listeners {
		select {
		case ch <- ev:
		default:
		}
	}
}

// markStatus 改状态并落盘（调用方已持 mu 或立即释放）。
func (s *Server) markStatus(ts *taskState, status string) {
	ts.Status = status
	ts.Role = roleForStatus(status) // M5-3
	s.emitEvent(ts, "stage", map[string]any{
		"role": ts.Role, "phase": status, "step": stepName(status),
	})
	s.persist(ts)
}

// ---------- rollback 辅助（可逆性/备份抽取） ----------

// 不可逆意图词表（COMMIT/DEPLOY 等）。
var irreversibleIntents = map[string]bool{
	contract.IntentCommit: true,
	contract.IntentDeploy: true,
}

func isReversibleIntent(intent string) bool {
	return !irreversibleIntents[intent]
}

// pickBackupPath 用 C 交付的结构化标记解析精确备份路径（M4-3）。
func pickBackupPath(receipts []contract.Receipt, _ string) string {
	for _, r := range receipts {
		if p := tools.ParseBackupPath(r.Stdout); p != "" {
			return p
		}
	}
	return ""
}

func pickTargetPath(it contract.Intent) string {
	switch it.Intent {
	case contract.IntentNote:
		// NOTE 追加到 <log_dir>/notes.md（planVerify 同款）。
		return "" // 由调用方按 log_dir 拼——server 不持有 log_dir，留空→404 安全。
	}
	if it.Params != nil {
		if p := it.Params["object"]; p != "" {
			return p
		}
	}
	return ""
}

func isConfirmWord(ans string) bool {
	switch strings.ToLower(strings.TrimSpace(ans)) {
	case "执行", "确认", "y", "yes", "true", "1", "ok":
		return true
	}
	return false
}

// newestBackup 取 <log_dir>/backups 下最新的 .bak（NOTE 回执未带具体路径时的兜底）。
func newestBackup(logDir string) string {
	entries, err := os.ReadDir(filepath.Join(logDir, "backups"))
	if err != nil {
		return ""
	}
	var newest string
	var newestMod time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".bak") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		if newest == "" || fi.ModTime().After(newestMod) {
			newest = filepath.Join(logDir, "backups", e.Name())
			newestMod = fi.ModTime()
		}
	}
	return newest
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// isCancelAnswer 报告澄清回答是否为"取消/不做"（评审 P3）。
//
// 否定仲裁的回问文案让用户「说取消」，这个选项必须有确定语义：
// 干净地取消任务，而不是被当成又一轮澄清答案。
func isCancelAnswer(answer string) bool {
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "取消", "不用了", "算了", "不做了", "不要了", "cancel", "no", "n":
		return true
	}
	return false
}
