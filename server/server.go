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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bytes"
	"voicesign-harness/config"
	"voicesign-harness/contract"
	"voicesign-harness/pipeline"
	"voicesign-harness/tools"
	"voicesign-harness/trajectory"
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
	stRevoked     = "revoked"     // 架构 v1 §6.2：撤回已完成任务（已标记，副作用清理走既有 rollback）
	stPending     = "pending"     // 预留：排队未开始（未来调度器用）
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
	Document  string               // 需求文档全文（实现类任务；续跑/回答时随 Options.Document 回传）
	ID        string               `json:"task_id"`
	RequestID string               `json:"request_id,omitempty"` // M4-1 ① 重试去重键
	Text      string               `json:"text,omitempty"`       // M4-1 ② need_ask 续跑原文
	Mode      string               `json:"mode,omitempty"`       // voice|text；voice 确认自动放行
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

	// 架构 v1 §6 插队：priority 0-255（默认 50），同优先级按提交时间。
	Priority int `json:"priority,omitempty"`

	startedAt time.Time

	confirmCh chan bool
	cancel    context.CancelFunc
	// doneCh 在任务到达终态（done/failed/canceled）时关闭。C2 调度器 worker 占 sem 期间
	// <-doneCh 等待任务真正结束再释放槽；默认直跑路径无人等待，零行为变化。
	doneCh chan struct{}

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
// platformAIOpsBase 自建平台入口（与 config/model-center.json gateway.base_url 一致）。
// 支持 VHS_PLATFORM_BASE 覆盖（2026-10-05：peterzou.com 域名 SNI/证书与服务器不符，已切 aiops.voxsign.ai）。
var platformAIOpsBase = func() string {
	if v := os.Getenv("VHS_PLATFORM_BASE"); v != "" {
		return v
	}
	return "https://aiops.voxsign.ai"
}()

type Server struct {
	cfg  *config.Config
	tmpl *pipeline.Options

	boot time.Time

	// cloud 云端模式（VHS_MODE=cloud）：谷歌登录/租户/配额；nil=本地模式。
	cloud *cloudAuth
	// devices 设备注册表（云道机器码机制）；本地/云端模式都创建。
	devices *deviceRegistry
	// relayHub 异网转发：机器码 → 活动 SSE 反向连接（端口按服务分、不按机器分）。
	relay *relayHub

	// 外部调用健壮性通道（架构 v1 §7）：每 Server 实例独立，避免跨租户/跨测试熔断污染。
	asrBH *bulkhead
	asrCB *circuitBreaker

	mu    sync.Mutex
	tasks map[string]*taskState
	byReq map[string]string // request_id → task_id（M4-1 ①）

	// activityVer 任务状态版本号（架构 v1 §5）：markStatus 每次迁移递增；
	// 心跳循环据此发现状态变化并立即补发（防毫秒级状态闪烁漏报）。
	activityVer int64

	// sched C2 进程内调度器（VHS_USE_SCHEDULER=true 时创建；nil=旧直跑路径逐字节不变）。
	sched *Scheduler
}

// New 构造 Server 并从 <log_dir>/tasks 恢复历史任务（M4-1 ③）。
func New(cfg *config.Config, o *pipeline.Options) *Server {
	// C0 受控串行：对【模板】Options 初始化一次共享串行闸。后续每任务 `o := *tmpl` 浅拷贝
	// 会复制 o.mu 指针 → 所有任务共享同一把锁，跨任务串行生效（修复此前模板 mu=nil、每任务
	// Run 内各建新锁导致闸失效的浅拷贝 bug）。详见 pipeline.Options.EnableSerialGate 注释。
	// 但 C2 调度器接管并发时（VHS_USE_SCHEDULER=true 且 VHS_MAX_CONCURRENT>1），不再锁死串行——
	// 并发上限由调度器 sem 控制（多 Runner 并发=目标态）；默认直跑 / 调度器=1 仍保留串行闸兜底。
	if o != nil && !(cfg.Server.UseScheduler && cfg.Server.MaxConcurrent > 1) {
		o.EnableSerialGate()
	}
	s := &Server{cfg: cfg, tmpl: o, boot: time.Now(), tasks: map[string]*taskState{}, byReq: map[string]string{}}
	s.asrBH = newBulkhead(2)
	s.asrCB = &circuitBreaker{}
	// C2：VHS_USE_SCHEDULER=true（灰度，默认 false）时创建进程内调度器接管任务排队；
	// exec 闭包跑 runPipeline 并 <-doneCh 等任务真正结束，使 sem 绑住真实并发。
	if cfg.Server.UseScheduler {
		sc := NewScheduler(cfg.Server.MaxConcurrent, 64)
		sc.exec = func(t *schedTask) {
			s.runPipeline(t.ts, t.ctx, t.text, t.spaceHint, t.document)
			<-t.ts.doneCh
		}
		s.sched = sc
		sc.Start()
	}
	if cfg.Global.CloudMode {
		s.cloud = newCloudAuth(cfg)
		// 设备注册表仅云端模式创建（设备注册是云端能力；本地模式设备端点由 deviceToken
		// 守卫返回 501「功能未启用」，业务端点的设备 token 放行在 s.devices==nil 时跳过）。
		s.devices = newDeviceRegistry(cfg.Global.LogDir)
	}
	s.relay = newRelayHub()
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
	mux.HandleFunc("/v1/orgs", s.auth(s.handleOrgs)) // 独立部署组织目录（iOS 自动拉取）
	mux.HandleFunc("/v1/roles", s.auth(s.handleRoles)) // M5-3
	// 兼容旧端点（deprecated，保留）
	mux.HandleFunc("/v1/run", s.auth(s.handleRun))
	mux.HandleFunc("/v1/voice", s.auth(s.handleVoice)) // 线 A 接语音：文本 → 线 B 意图 → 执行
	mux.HandleFunc("/v1/task/", s.auth(s.handleTaskGet))
	mux.HandleFunc("/v1/confirm", s.auth(s.handleConfirm))
	mux.HandleFunc("/v1/cancel", s.auth(s.handleCancel))
	// 架构 v1 §6 控制面：打断 / 撤回（不排队、立即生效；本地模式本机豁免、云端需会话 JWT）。
	mux.HandleFunc("/v1/interrupt", s.auth(s.handleInterrupt))
	mux.HandleFunc("/v1/withdraw", s.auth(s.handleWithdraw))
	// health 免鉴权（云端负载均衡/健康检查必须可匿名探测）。
	mux.HandleFunc("/v1/health", s.public(s.handleHealth))
	// ASR 校准（iOS 录音 → 平台 model-center 千问）：契约见 docs/ASR接口契约-20261004.md。
	mux.HandleFunc("/v1/asr", s.auth(s.handleASR))
	// 云端模式（VHS_MODE=cloud）：谷歌登录 + 租户/配额查询。
	mux.HandleFunc("/v1/auth/google", s.public(s.handleAuthGoogle))
	mux.HandleFunc("/v1/me", s.auth(s.handleMe))
	// 云道设备注册表（机器码机制）：register/heartbeat 需 VHS_TOKEN；lookup 免鉴权（机器码即凭证）。
	mux.HandleFunc("/v1/devices/lookup", s.public(s.handleDevicesLookup))
	mux.HandleFunc("/v1/devices/register", s.deviceToken(s.handleDevicesRegister))
	mux.HandleFunc("/v1/devices/heartbeat", s.deviceToken(s.handleDevicesHeartbeat))
	// 异网转发（relay）：443 唯一入口，按机器码路由。自带设备 token 鉴权，不走 s.auth。
	mux.HandleFunc("/v1/relay/connect", s.handleRelayConnect) // Mac agent 常驻 SSE 出站
	mux.HandleFunc("/v1/relay/respond", s.handleRelayRespond) // Mac agent 回报响应
	mux.HandleFunc("/v1/relay/", s.handleRelayForward)        // 客户端转发入口 /v1/relay/{code}/{path...}
	// 截图静态服务（图片回执）：/screenshots/<file> → <log_dir>/screenshots/<file>。
	// 仅提供 .png；path.Base 防目录穿越（只取文件名），auth 保护。
	mux.HandleFunc("/screenshots/", s.auth(s.handleScreenshot))
	return logRequests(mux)
}

// logRequests 访问日志中间件：记录 method/path/status/耗时（诊断与排障用；
// 仅打印不落任务轨迹，避免污染可复现轨迹）。
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("req rid=%s %s %s %d %s", requestIDFromReq(r), r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
	})
}

// requestIDFromReq 取客户端透传的 request_id（X-Request-Id 头）；缺省返回 "-"。
// P0-4a/b：入口日志 ↔ 轨迹/selfheal/ASR 日志用同一 rid 贯串通链。
func requestIDFromReq(r *http.Request) string {
	if id := strings.TrimSpace(r.Header.Get("X-Request-Id")); id != "" {
		return id
	}
	return "-"
}

// statusRecorder 记录响应状态码，供访问日志输出。
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Flush 实现 http.Flusher：SSE（/v1/tasks/{id}/events）依赖 Flush 逐条推流，
// 仅内嵌 ResponseWriter 接口不会提升 Flush，必须显式转发，否则断言失败返回 500。
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// handleASR 接收 iOS 录音（multipart file=WAV），base64 后转发平台 model-center 的 /api/model/asr
// （阿里千问 ASR，AIOPS_KEY 鉴权，与 chat 通道同一密钥环境变量）。契约见 docs/ASR接口契约-20261004.md。
func (s *Server) handleASR(w http.ResponseWriter, r *http.Request) {
	asrStart := time.Now()
	r.Body = http.MaxBytesReader(w, r.Body, 15<<20)
	if err := r.ParseMultipartForm(15 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"ok": "false", "code": "bad_audio", "error": "音频解析失败：" + err.Error()})
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"ok": "false", "code": "bad_audio", "error": "缺少 file 字段（multipart 音频）"})
		return
	}
	defer file.Close()
	audio, err := io.ReadAll(file)
	if err != nil || len(audio) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"ok": "false", "code": "bad_audio", "error": "音频为空或读取失败"})
		return
	}
	key := strings.TrimSpace(os.Getenv("AIOPS_KEY"))
	if key == "" {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"ok": "false", "code": "asr_channel_not_ready",
			"error": "平台 model-center 未提供 /api/model/asr（AIOPS_KEY 未设置）"})
		return
	}
	// 平台 ASR 端点：与 model-center 同一 base，路径 /api/model/asr。
	asrURL := platformAIOpsBase + "/api/model/asr"
	// v2 个性化热词：ASR 服务器这一层解决个性化问题（专名/口语/常用词提升，不依赖 iOS 本地）。
	// 云端模式按租户隔离热词库（tenants/<sub>/hotwords.json）；本地模式用全局 personal/。
	hotwords := loadASRHotwords(s.cfg.Global.LogDir)
	if sub := ctxTenant(r.Context()); sub != "" && s.cloud != nil {
		tenantDir := filepath.Join(s.cfg.Global.LogDir, "tenants", sanitizeSub(sub))
		hotwords = loadASRHotwords(tenantDir)
	}
	payload, _ := json.Marshal(map[string]any{
		"model":        "qwen-audio-asr",
		"audio_base64": base64.StdEncoding.EncodeToString(trimWAVSilence(audio)),
		"mime":         "audio/wav",
		"language":     "zh",
		"hotwords":     hotwords,
	})
	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
	defer cancel()
	// 架构 v1 §7：ASR 平台调用走健壮通道——有界并发(2) + 熔断 + 分级超时(tMid)。
	// ASR 转发为 POST，非幂等（重复调用消耗平台资源），不自动重试，仅熔断累计。
	status, data, rerr := robustJSONHdr(ctx, s.asrBH, s.asrCB,
		http.MethodPost, asrURL, payload, tMid, false,
		map[string]string{"Authorization": "Bearer " + key})
	if rerr != nil {
		code := "asr_channel_not_ready"
		msg := "平台 model-center 不可达：" + rerr.Error()
		if err == ctx.Err() || ctx.Err() != nil {
			code = "asr_timeout"
			msg = "平台 model-center 超时（" + tMid.String() + "）"
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"ok": "false", "code": code, "error": msg})
		return
	}
	log.Printf("ASR: rid=%s 平台 %s → HTTP %d（text_len 见响应）resp=%s", requestIDFromReq(r), asrURL, status, truncate(string(data), 300))
	if status != http.StatusOK {
		var perr map[string]any
		_ = json.Unmarshal(data, &perr)
		code, _ := perr["code"].(string)
		// 平台未实现该端点时返回 not_found → 统一映射为契约语义 asr_channel_not_ready。
		if code == "" || code == "not_found" {
			code = "asr_channel_not_ready"
		}
		msg, _ := perr["error"].(string)
		if msg == "" {
			msg = "平台 model-center 的 /api/model/asr 返回 HTTP " + http.StatusText(status)
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"ok": "false", "code": code, "error": msg})
		return
	}
	var pr struct {
		OK    bool   `json:"ok"`
		Text  string `json:"text"`
		DurMs int    `json:"duration_ms"`
		Model string `json:"model"`
		Err   string `json:"error"`
		Code  string `json:"code"`
	}
	// 空文本是合法结果（静音/环境音），不算平台异常；仅 JSON 失败或 ok=false 才算。
	if err := json.Unmarshal(data, &pr); err != nil || !pr.OK {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"ok": "false", "code": "asr_bad_response", "error": "平台返回异常：" + pr.Err})
		return
	}
	log.Printf("ASR: rid=%s 校准成功 model=%s duration_ms=%d text_len=%d wall_ms=%d",
		requestIDFromReq(r), pr.Model, pr.DurMs, len(pr.Text), time.Since(asrStart).Milliseconds())
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "text": pr.Text, "duration_ms": pr.DurMs, "model": pr.Model,
		"hotwords_used": len(hotwords)})
}

// loadASRHotwords 读个性化热词库 <log_dir>/personal/hotwords.json（不存在则写入种子词）。
// 运行时用户纠正的词由 voice 链路追加（去重、上限 100 词、每词 ≤20 字符）。
func loadASRHotwords(logDir string) []string {
	dir := filepath.Join(logDir, "personal")
	_ = os.MkdirAll(dir, 0o755)
	p := filepath.Join(dir, "hotwords.json")
	data, err := os.ReadFile(p)
	if err == nil {
		var ws []string
		if json.Unmarshal(data, &ws) == nil && len(ws) > 0 {
			return trimHotwords(ws)
		}
	}
	// 种子词（从用户历史语音反馈提取的专名/常用指令词）。
	seeds := []string{"VoxSign", "VoiceSign", "Harness", "aiops", "PeterZou", "季总", "截个图", "远程控制", "查看天气", "校准"}
	b, _ := json.MarshalIndent(seeds, "", "  ")
	_ = os.WriteFile(p, b, 0o644)
	log.Printf("ASR: 初始化个性化热词库 %s（%d 词）", p, len(seeds))
	return trimHotwords(seeds)
}

func trimHotwords(ws []string) []string {
	out := make([]string, 0, len(ws))
	seen := map[string]bool{}
	for _, w := range ws {
		w = strings.TrimSpace(w)
		if w == "" || len([]rune(w)) > 20 || seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
		if len(out) >= 100 {
			break
		}
	}
	return out
}

// handleScreenshot 提供远程控制截图（图片回执）。iOS 端用 <base>/screenshots/<name> 直接渲染。
func (s *Server) handleScreenshot(w http.ResponseWriter, r *http.Request) {
	name := path.Base(strings.TrimPrefix(r.URL.Path, "/screenshots/"))
	if name == "." || name == "/" || !strings.HasSuffix(name, ".png") {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	p := filepath.Join(s.cfg.Global.LogDir, "screenshots", name)
	if fi, err := os.Stat(p); err != nil || fi.IsDir() {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	http.ServeFile(w, r, p)
}

func (s *Server) Start() error {
	mux := s.Handler()
	addr := s.cfg.ServerBind()
	fmt.Printf("vhs server listening on %s (token_set=%v) 实际ASR端点=%s\n", addr, s.cfg.Server.Token != "", asrEndpoint())
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

// public 无鉴权端点（仅云端模式的登录与健康检查）。
func (s *Server) public(next http.HandlerFunc) http.HandlerFunc {
	return s.cors(func(w http.ResponseWriter, r *http.Request) {
		next(w, r)
	})
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return s.cors(func(w http.ResponseWriter, r *http.Request) {
		// 设备凭证优先：Bearer <设备token> 命中本机 devices.json 注册表 → 注入机器身份放行。
		// 覆盖 /v1/status、/v1/tasks、/v1/voice、/v1/me 等业务端点（iOS 拿机器码 lookup 到的 token 直连）。
		// 原有鉴权（会话 JWT、静态 VHS_TOKEN）保持：未命中设备 token 才走下面的云/本机逻辑。
		dTok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if dTok == "" {
			dTok = r.Header.Get("X-Token")
		}
		if s.devices != nil && dTok != "" {
			if rec := s.devices.findByToken(dTok); rec != nil {
				next(w, r.WithContext(withMachine(r.Context(), rec.MachineCode)))
				return
			}
		}
		// 云端模式：校验会话 JWT 并注入租户；本地模式沿用 m7-token / 本机豁免。
		if s.cloud != nil {
			tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if tok == "" {
				tok = r.Header.Get("X-Token")
			}
			claims, err := s.cloud.verifyJWT(tok)
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{
					"error": "会话无效或已过期，请重新登录", "code": "unauthorized"})
				return
			}
			ctx := withTenant(r.Context(), claims.Sub)
			// 任务提交做配额检查（免费档超限 429）。
			if r.Method == http.MethodPost && (r.URL.Path == "/v1/tasks" || r.URL.Path == "/v1/run") {
				left, err := s.cloud.checkAndConsume(claims.Sub)
				if err == quotaExceededErr {
					writeJSON(w, http.StatusTooManyRequests, map[string]string{
						"error": "今日免费额度已用完，升级 VoiceSign Prime 解锁不限量",
						"code":  "quota_exceeded",
					})
					return
				}
				_ = left
			}
			next(w, r.WithContext(ctx))
			return
		}
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
	Mode string `json:"mode,omitempty"` // voice|text；voice 网关确认自动放行（2026-10-04 用户拍板"拦截全去掉"）
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
	// P0-4b：旧入口 /v1/run 也透传客户端 X-Request-Id（无则空→pipeline 自生成，向后兼容）。
	if rid := strings.TrimSpace(r.Header.Get("X-Request-Id")); rid != "" {
		o.RequestID = rid
	}
	o.ConfirmFn = func(taskID, question string) (bool, error) {
		// 2026-10-04 用户拍板"所有拦截去掉"：voice 网关确认自动放行，不挂起等待
		//（否则 iOS 无确认交互 → 任务永久 waiting，用户实测"卡住"）。
		// 确认题记录到日志供审计；text 模式保留人工确认桥。
		if req.Mode == "" || req.Mode == "voice" {
			fmt.Printf("confirm-auto-approve task=%s q=%s\n", taskID, question)
			return true, nil
		}
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

	safeGo("handleRun:"+ts.ID, func() {
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
	}, func(r any) { s.onTaskPanic(ts, &o, r) })

	writeJSON(w, http.StatusAccepted, map[string]string{"task_id": ts.ID})
}

// voiceReq 是 /v1/voice 的入参：**只收文本**（不碰音频编解码）。
type voiceReq struct {
	Text string `json:"text"`
}

// asrEndpoint 返回线 B 地址（可配 VHS_ASR_ENDPOINT；默认本机 8787）。
// P0-3：以实际承担 ASR 个性化服务（线B）的 vhs-asr 监听端口为准。
// 查证结论：vhs-asr（cmd/vhs-asr）默认监听 127.0.0.1:8787 并暴露 /v1/process；
// vhs-voice（:8950）是语音适配层（转发主 harness :8941），非线B。旧默认 8123 为配置漂移。
func asrEndpoint() string {
	if v := os.Getenv("VHS_ASR_ENDPOINT"); v != "" {
		return v
	}
	return "http://127.0.0.1:8787"
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
	// P0-4b：真实 ASR 会话 id 桥接——客户端经 X-Session-Id 透传时用之，缺省仍 "voice"（不破坏既有 schema）。
	sid := r.Header.Get("X-Session-Id")
	intent, err := s.callASRProcess(r.Context(), req.Text, sid)
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
			"intent": intent,
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
	// P0-4b：旧入口 /v1/voice 也透传客户端 X-Request-Id（无则空→pipeline 自生成，向后兼容）。
	if rid := strings.TrimSpace(r.Header.Get("X-Request-Id")); rid != "" {
		o.RequestID = rid
	}
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
	safeGo("voice:"+ts.ID, func() {
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
	}, func(r any) { s.onTaskPanic(ts, &o, r) })
	writeJSON(w, http.StatusAccepted, map[string]any{"task_id": ts.ID, "intent": intent})
}

// callASRProcess 调线 B 的 POST /v1/process（**只走 HTTP**）。
// sessionID 为空时缺省 "voice"（P0-4b：可桥接真实会话 id）。
func (s *Server) callASRProcess(ctx context.Context, text, sessionID string) (map[string]any, error) {
	if sessionID == "" {
		sessionID = "voice"
	}
	body, _ := json.Marshal(map[string]string{"text": text, "session_id": sessionID})
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

// ---------- 控制面：打断 / 撤回（架构 v1 §6） ----------

type interruptReq struct {
	TaskID string `json:"task_id,omitempty"` // 缺省=全局急停所有活动任务
	Force  bool   `json:"force,omitempty"`   // 预留：未来连 pending 排队任务一起清
}

// isActiveTask 判断任务是否"活动中"（可被打断）：执行中 / 等待确认 / 等待回答。
func isActiveTask(status string) bool {
	switch status {
	case stRunning, stWaiting, stNeedConfirm, stNeedAsk:
		return true
	}
	return false
}

// handleInterrupt 全局急停/指定打断：直接 cancel 活动任务的 context（取消传播链
// 掐断远端），标记 canceled 并发事件。控制面命令不被任务执行阻塞。
func (s *Server) handleInterrupt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "仅 POST"})
		return
	}
	var req interruptReq
	_ = json.NewDecoder(r.Body).Decode(&req)

	s.mu.Lock()
	defer s.mu.Unlock()
	var ids []string
	for id, ts := range s.tasks {
		if req.TaskID != "" && id != req.TaskID {
			continue
		}
		if !isActiveTask(ts.Status) {
			continue
		}
		if ts.cancel != nil {
			ts.cancel()
		}
		s.markStatus(ts, stCanceled)
		s.emitEvent(ts, "interrupted", map[string]any{
			"by":       "interrupt",
			"priority": ts.Priority,
		})
		s.persist(ts)
		ids = append(ids, id)
	}
	if req.TaskID != "" && len(ids) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "未知 task_id 或无活动任务可打断"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "interrupted": ids})
}

type withdrawReq struct {
	TaskID string `json:"task_id"`
	Scope  string `json:"scope,omitempty"` // pending|running|done；缺省 running
}

// handleWithdraw 撤回（说错了）：按任务阶段分层处理（架构 v1 §6.2）。
//   - pending：未开始/挂起中（need_ask/need_confirm/waiting）→ 取消，零副作用；
//   - running：执行中 → 终止 + 丢弃输出（cancel 传播掐断远端）；
//   - done：已完成 → 标记 revoked（副作用清理走既有 /v1/tasks/{id}/rollback）。
func (s *Server) handleWithdraw(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "仅 POST"})
		return
	}
	var req withdrawReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TaskID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体应为 JSON {task_id, scope}"})
		return
	}
	if req.Scope == "" {
		req.Scope = "running"
	}
	switch req.Scope {
	case "pending", "running", "done":
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "scope 应为 pending|running|done"})
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	ts, ok := s.tasks[req.TaskID]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "未知 task_id"})
		return
	}

	switch req.Scope {
	case "pending":
		// 挂起类（未执行完/未开始）：直接取消。
		if !isActiveTask(ts.Status) {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "任务不在可撤回状态", "status": ts.Status})
			return
		}
		if ts.cancel != nil {
			ts.cancel()
		}
		s.markStatus(ts, stCanceled)
		s.emitEvent(ts, "withdrawn", map[string]any{"scope": "pending"})
		s.persist(ts)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "scope": "pending"})
	case "running":
		if !isActiveTask(ts.Status) {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "任务不在执行中", "status": ts.Status})
			return
		}
		if ts.cancel != nil {
			ts.cancel()
		}
		s.markStatus(ts, stCanceled)
		s.emitEvent(ts, "withdrawn", map[string]any{"scope": "running", "dropped": true})
		s.persist(ts)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "scope": "running"})
	case "done":
		if ts.Status != stDone {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "任务未完成，无法按 done 撤回", "status": ts.Status})
			return
		}
		s.markStatus(ts, stRevoked)
		s.emitEvent(ts, "withdrawn", map[string]any{
			"scope":      "done",
			"reversible": ts.Reversible,
			"note":       "副作用清理请走 /v1/tasks/{id}/rollback",
		})
		s.persist(ts)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "scope": "done", "revoked": true})
	}
}

// ActivityState 返回 Harness 心跳状态（架构 v1 §5）：decision（有待决策/回问，最高）
// > busy（有执行中）> idle（无活动）。pending=活动任务数。version=状态版本号
// （心跳循环用于检测变化立即补发，防毫秒级状态闪烁漏报）。
func (s *Server) ActivityState() (state string, pending int, version int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	decision, busy := 0, 0
	for _, ts := range s.tasks {
		switch ts.Status {
		case stNeedConfirm, stWaiting, stNeedAsk:
			decision++
		case stRunning:
			busy++
		}
	}
	if decision > 0 {
		return "decision", decision, s.activityVer
	}
	if busy > 0 {
		return "busy", busy, s.activityVer
	}
	return "idle", 0, s.activityVer
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
	Mode      string `json:"mode,omitempty"`       // voice|text；voice 确认自动放行（2026-10-04 用户拍板"拦截全去掉"）
	Document  string `json:"document,omitempty"`   // 需求文档全文（实现类任务；execImplement 证据门/LLM 生成共用）
	Priority  int    `json:"priority,omitempty"`   // 架构 v1 §6：0-255 插队优先级，默认 50
}

// clampPriority 把 priority 收拢到 [0,255]；非法/缺省 → 50（架构 v1 §6）。
func clampPriority(p int) int {
	if p == 0 {
		return 50
	}
	if p < 0 {
		return 0
	}
	if p > 255 {
		return 255
	}
	return p
}

// spawnTask 起一个任务（text 为完整原文；spaceHint 非空时前置"在 <space>"）。
// requestID 非空时登记 byReq 用于重试去重。状态迁移后自动 persist。
// 返回 (ts, accepted)：sched 队列满时 accepted=false（→ HTTP 429）并撤销注册；默认直跑恒 true。
func (s *Server) spawnTask(text, spaceHint, requestID, mode, document string, priority int) (*taskState, bool) {
	// 调度器接管时初始状态 pending（排队未开始）；默认直跑仍 running（逐字节不变）。
	initStatus := stRunning
	if s.sched != nil {
		initStatus = stPending
	}
	ts := &taskState{
		ID:        fmt.Sprintf("task-%d", time.Now().UnixNano()),
		RequestID: requestID,
		Text:      text,
		Mode:      mode, // voice→确认自动放行（2026-10-04 用户拍板"拦截全去掉"）
		Document:  document,
		Status:    initStatus,
		Role:      RolePlanner, // M5-3：初始在规划阶段
		Priority:  clampPriority(priority),
		startedAt: time.Now(),
		confirmCh: make(chan bool, 1),
		doneCh:    make(chan struct{}),
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

	// C2 调度路径：入队（byReq 注册与入队同临界区外撤销，防同 id 重复入队）。
	if s.sched != nil {
		ok := s.sched.Enqueue(&schedTask{ts: ts, ctx: ctx, text: text, spaceHint: spaceHint, document: document})
		if !ok {
			s.mu.Lock()
			delete(s.tasks, ts.ID)
			if requestID != "" {
				delete(s.byReq, requestID)
			}
			s.mu.Unlock()
			cancel()
			return ts, false
		}
		return ts, true
	}

	s.runPipeline(ts, ctx, text, spaceHint, document)
	return ts, true
}

// runPipeline 在给定 ts 上跑一次 pipeline（首次提交或 need_ask 续跑共用）。
// taskMaxDeadline 任务总 deadline 硬上限（架构 v1 §7.1 任务层）：超过强制中断，
// 防"以为 1 秒实际 1 小时"的外部调用拖死任务与主进程。
const taskMaxDeadline = 10 * time.Minute

func (s *Server) runPipeline(ts *taskState, ctx context.Context, text, spaceHint, document string) {
	// 任务层硬上限：外部调用再卡也不会超过 taskMaxDeadline。
	// 注意：任务在 goroutine 中异步执行，cancel 必须由 goroutine 持有并在其返回时释放，
	// 不能在函数体同步返回时触发（否则任务会被立即取消）。
	ctx, cancel := context.WithTimeout(ctx, taskMaxDeadline)
	o := *s.tmpl
	o.Document = document
	o.RequestID = ts.RequestID // P0-4b：入口 request_id 贯通到 pipeline 轨迹（空则 pipeline 自生成）
	// §7.4 S3：pipeline 细粒度进度事件桥进 SSE（kind:"internal"，区别于 markStatus 的 transition）。
	o.ProgressObserver = func(stage, detail string) { s.bridgeFromBus(ts, stage, detail) }
	o.ConfirmFn = func(taskID, question string) (bool, error) {
		// 2026-10-04 用户拍板"所有拦截去掉"：voice 模式确认自动放行（不挂起等待，
		// 否则 iOS 无确认交互 → 任务永久 need_confirm，用户实测"卡住"）。确认题记日志供审计。
		if ts.Mode == "" || ts.Mode == "voice" {
			fmt.Printf("confirm-auto-approve task=%s q=%s\n", taskID, question)
			return true, nil
		}
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
	safeGo("pipeline:"+ts.ID, func() {
		defer cancel() // 任务结束（含超时/取消）时释放 deadline 定时器
		// C2：任务 goroutine 退出时关闭 doneCh（终态/挂起都算），供调度器 worker 释放槽。select 防双关（need_ask 续跑会再进此函数）。
		defer func() {
			if ts.doneCh != nil {
				select {
				case <-ts.doneCh:
				default:
					close(ts.doneCh)
				}
			}
		}()
		// C1：pipeline.Run 收进 Runner（ob=nil：SSE/CLI 细粒度事件仍走 o.ProgressObserver=bridgeFromBus，零行为变化）。
		runStart := time.Now()
		rn := NewRunner(ts, &o, nil)
		out, err := rn.Run(ctx, fullText)
		loopMs := time.Since(runStart).Milliseconds()
		// 结构化计时日志（手机侧定位用，一行一条）：total_ms = 分类+LLM+工具全段。
		log.Printf("[timing] task=%s total_ms=%d loop_ms=%d intent=%s text=%q",
			ts.ID, loopMs, out.LoopMs, out.Intent.Intent, strings.TrimSpace(text))
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
			// 语音场景（iOS 按住说话，无键盘反问交互）：意图 UNKNOWN 时**不挂起反问**，
			// 直接结束并回复提示，避免任务永久 need_ask → App 轮询无终态 → 用户"没反应"。
			if ts.Mode == "" || ts.Mode == "voice" {
				heard := strings.TrimSpace(text)
				if len(heard) > 40 {
					heard = heard[:40] + "…"
				}
				receipt := "你说的是「" + heard + "」——我没太确定要做什么。" +
					"可以直接说清楚对象，比如「查一下北京的天气」「记一个想法：…」「跑一下测试」。"
				// 手机侧以轮询 GET /v1/tasks/{id} 的 receipt 为准，必须改 View.Result，光改 event 不生效。
				out.View.Action = "（待澄清）"
				out.View.Result = receipt
				out.Reply = receipt // Phase 1：done 必带自然语言回答（此处 pipeline 出口时 View.Result 尚不是友好文案）
				ts.Outcome = &out
				s.markStatus(ts, stDone)
				s.emitEvent(ts, "done", map[string]any{
					"receipt":     contract.RenderReceipt(out.View),
					"attribution": out.Attribution,
					"reply":       out.Reply,
				})
				s.persist(ts)
				return
			}
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
				"receipt":            contract.RenderReceipt(out.View),
				"attribution":        out.Attribution,
				"reversible":         ts.Reversible,
				"role":               ts.Role,
				"reply":              out.Reply, // Phase 1：自然语言回答（D0 契约）
				"termination_reason": out.TerminationReason,
			})
			s.persist(ts)
		}
	}, func(r any) { s.onTaskPanic(ts, &o, r) })
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
		s.runPipeline(ts, ctx, strings.TrimSpace(answer), "", ts.Document)
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
	s.runPipeline(ts, ctx, prefix+substituted, "", ts.Document)
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

	ts, accepted := s.spawnTask(req.Text, req.Space, req.RequestID, req.Mode, req.Document, req.Priority)
	if !accepted {
		// C2：调度器队列满 → 429 Too Many Requests。
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "task queue full, retry later"})
		return
	}
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
//
// race 修复：ts 各字段由 runPipeline 后台 goroutine 持 s.mu 写（Outcome/Status/Role…），
// 本函数是 GET /v1/tasks/{id} 轮询读端。快照必须在同一把 s.mu 内完成，再锁外写 HTTP，
// 否则读 ts.Outcome/ts.Status 与后台写并发 → DATA RACE（-race 门）。
func (s *Server) writeTaskView(w http.ResponseWriter, ts *taskState) {
	s.mu.Lock()
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
		// Phase 1（D0 契约）：done 必带自然语言回答 reply；生成失败时带可读终止原因。
		if ts.Outcome.Reply != "" {
			body["reply"] = ts.Outcome.Reply
		}
		if ts.Outcome.TerminationReason != "" {
			body["termination_reason"] = ts.Outcome.TerminationReason
		}
	}
	s.mu.Unlock()
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

// handleOrgs：独立部署组织目录（iOS「独立部署」区自动拉取，与 OrgEntry 结构对齐）。
func (s *Server) handleOrgs(w http.ResponseWriter, r *http.Request) {
	orgs := make([]map[string]any, 0, len(s.cfg.Orgs))
	for _, o := range s.cfg.Orgs {
		orgs = append(orgs, map[string]any{
			"orgId":    o.OrgID,
			"orgName":  o.OrgName,
			"base":     o.Base,
			"viaRelay": o.ViaRelay,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"orgs": orgs})
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

// bridgeFromBus 把 pipeline 的细粒度进度事件桥进 SSE（设计 §7.4）。
//
//	kind:"internal" = 细粒度内部阶段（pipeline ProgressObserver 回调）；
//	区别于 markStatus 发的 kind:"transition" 粗粒度状态迁移（phase）。
//
// 复用 ts.eventSeq++ 编号（桥层不另编号），事件 append 进 ts.events → ?after= 重放自动覆盖。
// 由 pipeline goroutine 回调，须自己持 s.mu（emitEvent 读写 ts.eventSeq/events/listeners）。
func (s *Server) bridgeFromBus(ts *taskState, stage, detail string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emitEvent(ts, "stage", map[string]any{
		"kind":   "internal",
		"stage":  stage,
		"detail": detail,
		"trace":  ts.RequestID,
	})
}

// markStatus 改状态并落盘（调用方已持 mu 或立即释放）。
func (s *Server) markStatus(ts *taskState, status string) {
	s.activityVer++ // 状态迁移 → 版本递增（心跳循环据此立即补发，架构 v1 §5）
	ts.Status = status
	ts.Role = roleForStatus(status) // M5-3
	s.emitEvent(ts, "stage", map[string]any{
		"role": ts.Role, "phase": status, "step": stepName(status),
	})
	s.persist(ts)
}

// onTaskPanic 是后台任务 goroutine 被 safeGo recover 后的统一收尾（P0-2）：
//  1. 把任务标记 canceled（panic 视为异常终止，绝不留 stRunning 假活）；
//  2. 把 panic 写为轨迹 error kind（best-effort，供事后重放/归因）。
//
// 此时调用方 goroutine 的 `defer mu.Unlock()` 已在 unwind 阶段先行执行完毕（body defer 内层先跑，
// safeGo 的 recover defer 外层后跑），故这里可安全重新加锁。
func (s *Server) onTaskPanic(ts *taskState, o *pipeline.Options, r any) {
	if ts != nil {
		s.mu.Lock()
		ts.Err = fmt.Sprintf("panic: %v", r)
		s.markStatus(ts, stCanceled)
		s.mu.Unlock()
	}
	rid := ""
	if ts != nil {
		// S0/P0-4b：panic 轨迹的 request_id 与 pipeline 同一来源（ts.RequestID）；空则 ts.ID 兜底，
		// 使 panic 记录能 join 进请求链（入口→轨迹→日志）。
		rid = ts.RequestID
		if rid == "" {
			rid = ts.ID
		}
	}
	if o != nil && o.Trace != nil {
		_ = o.Trace.Write(trajectory.Entry{
			RequestID: rid,
			Kind:      trajectory.KindError,
			Content:   fmt.Sprintf("后台任务 goroutine panic: %v", r),
		})
	}
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
