// Package server ismobile client HTTP API face(M2 #10 / M3 INTERACT-v1   ): 
// internal listening + token auth, pipe pipeline.Run   givemobile. 
//
// posformendpoint set(INTERACT-v1, 2026-10-02   ): 
//
//	POST /v1/tasks                 {text, space?}             task -> 202 {task_id,status}
//	GET  /v1/tasks/{id}                                     poll {status,question?,options?,receipt?,attribution?,error?}
//	POST /v1/tasks/{id}/answer     {answer}                 answercurbeforeuniquedecision point(clarification  /confirmword)
//	POST /v1/tasks/{id}/rollback                            from <log_dir>/backups rollback(onlyreversibletask)
//	GET  /v1/status                                         {version,uptime,ok}
//
// compatlegacy endpoint(keep ,   tgt deprecated): /v1/run /v1/task/{id} /v1/confirm /v1/cancel /v1/health. 
//
// statusword(INTERACT-v1 § ): need_ask(low-confidenceclarification)/ need_confirm( confirm)/ running / done. 
// auth(#32): cfg.Server.Token / VHS_TOKEN  empty -> Bearer; token empty -> only 127.0.0.1. 
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

// version and main.go same (M3 INTERACT-v1 endpoint set). 
const version = "0.2.0"

// task state machinegetvalue(INTERACT-v1 § state transition + M2 compat). 
const (
	stRunning     = "running"
	stWaiting     = "waiting_confirm" // compatlegacy field(legacy endpointuse)
	stNeedAsk     = "need_ask"        // low-confidenceclarification(question    )
	stNeedConfirm = "need_confirm"    //  confirm(  )
	stDone        = "done"
	stCanceled    = "canceled"
	stRevoked     = "revoked"     //    v1 §6.2:  backalreadydonetask(alreadytgt ,   use    has rollback)
	stPending     = "pending"     //   :    openstart(  call  use)
	stInterrupted = "interrupted" // M4-1 ③: heavystart    donetask,    continue 
)

//     (M5-3): pipeline 13 stage -> Planner / Executor / Verifier. 
//
// [pseudocode logic layer](      decide  ): 
//
//	planner  = classify/domain decide/risk grading/confirm /clarification(decide and  )--to  need_ask/need_confirm/initstart running. 
//	executor =       (NOTE/EDIT/COMMIT/QUERY)--running in  seg. 
//	verifier = verify verify/attribution/back --done endstate( after segisverify+attribution+back ). 
//	canceled/interrupted stage  keep  in statusbefore  after  . 
const (
	RolePlanner  = "planner"
	RoleExecutor = "executor"
	RoleVerifier = "verifier"
)

// roleForStatus pipetaskstatus  ascurbefore  (endstatekeep  after     ). 
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

// stepName pipestatus  asin stagename. 
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

// taskState is  diff task finish status(  rollback  need reversible/    ). 
type taskState struct {
	Document  string               // needrequire  safety ( nowclasstask; continue /answertime  Options.Document back )
	ID        string               `json:"task_id"`
	RequestID string               `json:"request_id,omitempty"` // M4-1 ① heavy  heavy 
	Text      string               `json:"text,omitempty"`       // M4-1 ② need_ask continue orig 
	Mode      string               `json:"mode,omitempty"`       // voice|text; voice confirm    
	Status    string               `json:"status"`
	Role      string               `json:"role,omitempty"` // M5-3: curbefore  
	Question  string               `json:"question,omitempty"`
	Options   []pipeline.AskOption `json:"options,omitempty"`

	// askedQuestions / askRounds(   G8 fix ): already ed      +     . 
	//
	//    S1: only to"  on  "  Q1->Q2->Q1->Q2   time   in,   nolimit  . 
	// modifyas  **    **and   onlimit,    ini.e. recv   . 
	// note(   S5):    charseg  out,   and JSON  listize;  process  after    , 
	//  already limitrestrict--curbefore server  tasktableisinstorestate, heavystart  path has interrupted handle. 
	askedQuestions map[string]bool
	askRounds      int
	Outcome        *pipeline.Outcome `json:"outcome,omitempty"`
	Err            string            `json:"error,omitempty"`

	// rollback  numdata
	Reversible bool   `json:"reversible,omitempty"`
	BackupPath string `json:"backup_path,omitempty"`
	TargetPath string `json:"target_path,omitempty"`

	//    v1 §6   : priority 0-255(default 50), same first by  timetime. 
	Priority int `json:"priority,omitempty"`

	startedAt time.Time

	confirmCh chan bool
	cancel    context.CancelFunc
	// doneCh  taskto endstate(done/failed/canceled)timeclose . C2 call   worker   sem periodtime
	// <-doneCh waittask poscloseendagain   ; default  pathno wait,   aschangeize. 
	doneCh chan struct{}

	// M6-1 SSE: event   +     er. 
	eventSeq  int
	events    []sseEvent
	listeners []chan sseEvent
}

// sseEvent is   SSE event(seq  call add). Data be    end JSON(  : data    seq). 
type sseEvent struct {
	Seq  int            `json:"seq"`
	Type string         `json:"-"`
	Data map[string]any `json:"-"`
}

// flattened returnback {seq, ...Data}. 
func (e sseEvent) flattened() map[string]any {
	out := map[string]any{"seq": e.Seq, "type": e.Type}
	for k, v := range e.Data {
		out[k] = v
	}
	return out
}

// Server keephas  , pipeline   andtasktable. 
// platformAIOpsBase     in (and config/model-center.json gateway.base_url   ). 
//  keep VHS_PLATFORM_BASE overwrite(2026-10-05: peterzou.com domainname SNI/  andserveservice   , already  aiops.voxsign.ai). 
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

	// cloud  end form(VHS_MODE=cloud):     / user/  ; nil=basely form. 
	cloud *cloudAuth
	// devices   note table(    code restrict); basely/ end formall  . 
	devices *deviceRegistry
	// relayHub diff  send:   code ->    SSE revtolinkconnect(portbyserveservicesplit,  by  split). 
	relay *relayHub

	// out calluse  ity  (   v1 §7):   Server  example  ,     user/    disconnect  . 
	asrBH *bulkhead
	asrCB *circuitBreaker

	mu    sync.Mutex
	tasks map[string]*taskState
	byReq map[string]string // request_id -> task_id(M4-1 ①)

	// activityVer taskstatus baseid(   v1 §5): markStatus      add; 
	//     data sendnowstatuschangeizeand i.e.patchsend(prevent sec status    ). 
	activityVer int64

	// sched C2 processincall  (VHS_USE_SCHEDULER=true time  ; nil=   path charnode change). 
	sched *Scheduler
}

// New    Server andfrom <log_dir>/tasks     task(M4-1 ③). 
func New(cfg *config.Config, o *pipeline.Options) *Server {
	// C0 accept serial: to[  ]Options initstartize    serial gate. aftercontinue task `o := *tmpl`    
	//   restrict o.mu refer  ->  hastask  same pipe ,  taskserialoccur (fix  before   mu=nil,  task
	// Run in  new           bug).  see pipeline.Options.EnableSerialGate note . 
	// but C2 call  connectmanageandsendtime(VHS_USE_SCHEDULER=true and VHS_MAX_CONCURRENT>1),  again  serial--
	// andsendonlimitbycall   sem control(  Runner andsend=objtgtstate); default   / call  =1  keep serial gate bot. 
	if o != nil && !(cfg.Server.UseScheduler && cfg.Server.MaxConcurrent > 1) {
		o.EnableSerialGate()
	}
	s := &Server{cfg: cfg, tmpl: o, boot: time.Now(), tasks: map[string]*taskState{}, byReq: map[string]string{}}
	s.asrBH = newBulkhead(2)
	s.asrCB = &circuitBreaker{}
	// C2: VHS_USE_SCHEDULER=true(  , default false)time  processincall  connectmanagetask  ; 
	// exec     runPipeline and <-doneCh etctask poscloseend,   sem     andsend. 
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
		//   note tableonly end form  (  note is end  ; basely form  endpointby deviceToken
		//   returnback 501"   startuse",  serviceendpoint    token     s.devices==nil time ed). 
		s.devices = newDeviceRegistry(cfg.Global.LogDir)
	}
	s.relay = newRelayHub()
	s.restore()
	return s
}

// Start   cfg.ServerBind() onraise HTTP serveservice. 
// Handler returnbackfinish routeby(**Start and data usesame    ** --   " data    !=  start    "). 
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	// INTERACT-v1 posformendpoint
	mux.HandleFunc("/v1/tasks", s.auth(s.handleTasksPost))
	mux.HandleFunc("/v1/tasks/", s.auth(s.handleTasksSub))
	mux.HandleFunc("/v1/status", s.auth(s.handleStatus))
	mux.HandleFunc("/v1/roles", s.auth(s.handleRoles)) // M5-3
	// compatlegacy endpoint(deprecated, keep )
	mux.HandleFunc("/v1/run", s.auth(s.handleRun))
	mux.HandleFunc("/v1/voice", s.auth(s.handleVoice)) // line A connectlangaudio:  base -> line B intent ->   
	mux.HandleFunc("/v1/task/", s.auth(s.handleTaskGet))
	mux.HandleFunc("/v1/confirm", s.auth(s.handleConfirm))
	mux.HandleFunc("/v1/cancel", s.auth(s.handleCancel))
	//    v1 §6 controlface:  disconnect /  back(   ,  i.e.occur ; basely formbase   ,  endneed   JWT). 
	mux.HandleFunc("/v1/interrupt", s.auth(s.handleInterrupt))
	mux.HandleFunc("/v1/withdraw", s.auth(s.handleWithdraw))
	// health    ( end    /        name  ). 
	mux.HandleFunc("/v1/health", s.public(s.handleHealth))
	// ASR  approve(iOS  audio ->    model-center   ):   see docs/ASRconnect   -20261004.md. 
	mux.HandleFunc("/v1/asr", s.auth(s.handleASR))
	//  end form(VHS_MODE=cloud):      +  user/    . 
	mux.HandleFunc("/v1/auth/google", s.public(s.handleAuthGoogle))
	mux.HandleFunc("/v1/me", s.auth(s.handleMe))
	//     note table(  code restrict): register/heartbeat need VHS_TOKEN; lookup    (  codei.e.  ). 
	mux.HandleFunc("/v1/devices/lookup", s.public(s.handleDevicesLookup))
	mux.HandleFunc("/v1/devices/register", s.deviceToken(s.handleDevicesRegister))
	mux.HandleFunc("/v1/devices/heartbeat", s.deviceToken(s.handleDevicesHeartbeat))
	// diff  send(relay): 443 uniquein , by  coderouteby.      token   ,    s.auth. 
	mux.HandleFunc("/v1/relay/connect", s.handleRelayConnect) // Mac agent    SSE out 
	mux.HandleFunc("/v1/relay/respond", s.handleRelayRespond) // Mac agent back   
	mux.HandleFunc("/v1/relay/", s.handleRelayForward)        // clientuserend sendin  /v1/relay/{code}/{path...}
	//    stateserveservice(  back ): /screenshots/<file> -> <log_dir>/screenshots/<file>. 
	// only provide .png; path.Base preventobj   (onlygetfilename), auth protect. 
	mux.HandleFunc("/screenshots/", s.auth(s.handleScreenshot))
	return logRequests(mux)
}

// logRequests   day middle :    method/path/status/ time( disconnectand  use; 
// only    tasktrace,       nowtrace). 
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("req rid=%s %s %s %d %s", requestIDFromReq(r), r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
	})
}

// requestIDFromReq getclientuserend    request_id(X-Request-Id head);   returnback "-". 
// P0-4a/b: in day  ↔ trace/selfheal/ASR day usesame  rid    chain. 
func requestIDFromReq(r *http.Request) string {
	if id := strings.TrimSpace(r.Header.Get("X-Request-Id")); id != "" {
		return id
	}
	return "-"
}

// statusRecorder     statuscode, provide  day  out. 
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Flush  now http.Flusher: SSE(/v1/tasks/{id}/events)dependency Flush     , 
// onlyin  ResponseWriter connect      Flush,    form send,  thendisconnectlang  returnback 500. 
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// handleASR connectrecv iOS  audio(multipart file=WAV), base64 after send   model-center   /api/model/asr
// (     ASR, AIOPS_KEY   , and chat   same     change ).   see docs/ASRconnect   -20261004.md. 
func (s *Server) handleASR(w http.ResponseWriter, r *http.Request) {
	asrStart := time.Now()
	r.Body = http.MaxBytesReader(w, r.Body, 15<<20)
	if err := r.ParseMultipartForm(15 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"ok": "false", "code": "bad_audio", "error": "audio parse failed: " + err.Error()})
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"ok": "false", "code": "bad_audio", "error": "missing file field (multipart audio)"})
		return
	}
	defer file.Close()
	audio, err := io.ReadAll(file)
	if err != nil || len(audio) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"ok": "false", "code": "bad_audio", "error": "audio empty or unreadable"})
		return
	}
	key := strings.TrimSpace(os.Getenv("AIOPS_KEY"))
	if key == "" {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"ok": "false", "code": "asr_channel_not_ready",
			"error": "平台 model-center 未提供 /api/model/asr（AIOPS_KEY 未设置）"})
		return
	}
	//    ASR endpoint: and model-center same  base, path /api/model/asr. 
	asrURL := platformAIOpsBase + "/api/model/asr"
	// v2  ityize word: ASR serveservice    resolvedecide ityize  ( name/ lang/ useword  ,  dependency iOS basely). 
	//  end formby user   word (tenants/<sub>/hotwords.json); basely formuseglobal personal/. 
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
	//    v1 §7: ASR   calluse     --hasboundaryandsend(2) +  disconnect + split  time(tMid). 
	// ASR  sendas POST,   etc(heavy calluse      ),    heavy , only disconnect  . 
	status, data, rerr := robustJSONHdr(ctx, s.asrBH, s.asrCB,
		http.MethodPost, asrURL, payload, tMid, false,
		map[string]string{"Authorization": "Bearer " + key})
	if rerr != nil {
		code := "asr_channel_not_ready"
		msg := "platform model-center unreachable: " + rerr.Error()
		if err == ctx.Err() || ctx.Err() != nil {
			code = "asr_timeout"
			msg = "platform model-center timed out (" + tMid.String() + "）"
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
		//     now endpointtimereturnback not_found ->     as  semantic asr_channel_not_ready. 
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
	// empty baseis  close ( audio/  audio),     error; only JSON   or ok=false only . 
	if err := json.Unmarshal(data, &pr); err != nil || !pr.OK {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"ok": "false", "code": "asr_bad_response", "error": "platform returned error: " + pr.Err})
		return
	}
	log.Printf("ASR: rid=%s calibrated model=%s duration_ms=%d text_len=%d wall_ms=%d",
		requestIDFromReq(r), pr.Model, pr.DurMs, len(pr.Text), time.Since(asrStart).Milliseconds())
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "text": pr.Text, "duration_ms": pr.DurMs, "model": pr.Model,
		"hotwords_used": len(hotwords)})
}

// loadASRHotwords read ityize word  <log_dir>/personal/hotwords.json( store thenwritekind word). 
//   timeuseuser pos wordby voice chainroute  ( heavy, onlimit 100 word,  word <=20 char ). 
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
	// kind word(fromuseuser  langaudiorev  get  name/ userefer word). 
	seeds := []string{"VoxSign", "VoiceSign", "Harness", "aiops", "PeterZou", "季总", "截个图", "远程控制", "查看天气", "校准"}
	b, _ := json.MarshalIndent(seeds, "", "  ")
	_ = os.WriteFile(p, b, 0o644)
	log.Printf("ASR: init personalized hot-word lib %s（%d 词）", p, len(seeds))
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

// handleScreenshot  provide  control  (  back ). iOS enduse <base>/screenshots/<name>  connect  . 
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

// ---------- authmiddle (#32) ----------

// cors middle (M7): to has /v1/*   CORS head; OPTIONS    routereturnback. 
//     : back  Origin,  allow Authorization/Content-Type/X-Token. 
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

// public no  endpoint(only end form   and    ). 
func (s *Server) public(next http.HandlerFunc) http.HandlerFunc {
	return s.cors(func(w http.ResponseWriter, r *http.Request) {
		next(w, r)
	})
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return s.cors(func(w http.ResponseWriter, r *http.Request) {
		//      first: Bearer <  token>  inbase  devices.json note table -> notein      . 
		// overwrite /v1/status, /v1/tasks, /v1/voice, /v1/me etc serviceendpoint(iOS    code lookup to  token  link). 
		// orighas  (   JWT,  state VHS_TOKEN)keepkeep:   in   token only underface  /base   . 
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
		//  end form: verify   JWT andnotein user; basely form use m7-token / base   . 
		if s.cloud != nil {
			tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if tok == "" {
				tok = r.Header.Get("X-Token")
			}
			claims, err := s.cloud.verifyJWT(tok)
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{
					"error": "session invalid or expired, please sign in again", "code": "unauthorized"})
				return
			}
			ctx := withTenant(r.Context(), claims.Sub)
			// task       (    limit 429). 
			if r.Method == http.MethodPost && (r.URL.Path == "/v1/tasks" || r.URL.Path == "/v1/run") {
				left, err := s.cloud.checkAndConsume(claims.Sub)
				if err == quotaExceededErr {
					writeJSON(w, http.StatusTooManyRequests, map[string]string{
						"error": "daily free quota exhausted; upgrade to VoiceSign Prime for unlimited",
						"code":  "quota_exceeded",
					})
					return
				}
				_ = left
			}
			next(w, r.WithContext(ctx))
			return
		}
		// base   : token asemptytimeonly allowback ly . 
		if s.cfg.Server.Token == "" {
			if !isLoopback(r.RemoteAddr) {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "missing token and non-loopback access"})
				return
			}
		} else {
			// has token -> verify Authorization: Bearer <token> or X-Token. 
			tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if tok == "" {
				tok = r.Header.Get("X-Token")
			}
			if tok != s.cfg.Server.Token {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
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

// ---------- endpoint ----------

type runReq struct {
	Text string `json:"text"`
	Mode string `json:"mode,omitempty"` // voice|text; voice  closeconfirm    (2026-10-04 useuser  "blocksafety  ")
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST only"})
		return
	}
	var req runReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request body must be JSON {text}"})
		return
	}

	ts := &taskState{
		ID:        fmt.Sprintf("task-%d", time.Now().UnixNano()),
		Status:    stRunning,
		confirmCh: make(chan bool, 1),
	}
	// taskoccur  period  at HTTP  require(  returnbackafter require ctx  becancel). 
	ctx, cancel := context.WithCancel(context.Background())
	ts.cancel = cancel

	s.mu.Lock()
	s.tasks[ts.ID] = ts
	s.mu.Unlock()

	//      Options, noteinbasetask  ConfirmFn( connecttomobile /v1/confirm). 
	o := *s.tmpl
	// P0-4b:  in  /v1/run also  clientuserend X-Request-Id(nothenempty->pipeline  occurbecome, toaftercompat). 
	if rid := strings.TrimSpace(r.Header.Get("X-Request-Id")); rid != "" {
		o.RequestID = rid
	}
	o.ConfirmFn = func(taskID, question string) (bool, error) {
		// 2026-10-04 useuser  " hasblock  ": voice  closeconfirm    ,   raisewait
		//( then iOS noconfirm   -> task   waiting, useuser  "  "). 
		// confirm   today provide  ; text  formkeep humanconfirm . 
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
			return false, errors.New("task canceled")
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

// voiceReq is /v1/voice  in : **onlyrecv base**(  audiofreq resolvecode). 
type voiceReq struct {
	Text string `json:"text"`
}

// asrEndpoint returnbackline B ly (   VHS_ASR_ENDPOINT; defaultbase  8787). 
// P0-3: by     ASR  ityizeserveservice(lineB)  vhs-asr listenportasapprove. 
//   close : vhs-asr(cmd/vhs-asr)defaultlisten 127.0.0.1:8787 and   /v1/process; 
// vhs-voice(:8950)islangaudio   ( send  harness :8941),  lineB.  default 8123 as    . 
func asrEndpoint() string {
	if v := os.Getenv("VHS_ASR_ENDPOINT"); v != "" {
		return v
	}
	return "http://127.0.0.1:8787"
}

// handleVoice: **line A connectlangaudio in **. 
//
//  boundary(Lead 2026-10-03): 
//
//	① line A ** in line B  code**, only  HTTP(keepkeep "ASR   ,     "); 
//	② line B     ⇒ **     + degraded**, ** allow    become" connect  orig "**; 
//	③  line: intentas ASK / need_disambiguate ⇒ **    **, pipe   back; 
//	④ its   has pipeline.Run path( domain forbidand reversibleconfirm). 
func (s *Server) handleVoice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST only"})
		return
	}
	var req voiceReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Text) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request body must be JSON {text} (ASR transcript)"})
		return
	}
	// P0-4b:    ASR    id  connect--clientuserend  X-Session-Id   timeuseof,     "voice"(    has schema). 
	sid := r.Header.Get("X-Session-Id")
	intent, err := s.callASRProcess(r.Context(), req.Text, sid)
	if err != nil {
		// ② **     **:    503 + degraded, and**     task**
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error":    "ASR service unreachable (no silent fallback to raw text): " + err.Error(),
			"degraded": true,
			"endpoint": asrEndpoint(),
		})
		return
	}
	// ③  line: ASK / need_disambiguate ⇒     
	needAsk, _ := intent["need_disambiguate"].(bool)
	typ, _ := intent["type"].(string)
	ask, _ := intent["ask"].(string)
	if needAsk || strings.EqualFold(typ, "ASK") || ask != "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"executed": false, "reason": "intent needs clarification (ASK/low confidence) -- red line: never execute when Ask != ''",
			"intent": intent,
		})
		return
	}
	// ④   has  path( domain forbid /  reversibleconfirm)
	ts := &taskState{ID: fmt.Sprintf("task-%d", time.Now().UnixNano()), Status: stRunning, confirmCh: make(chan bool, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	ts.cancel = cancel
	s.mu.Lock()
	s.tasks[ts.ID] = ts
	s.mu.Unlock()
	o := *s.tmpl
	// P0-4b:  in  /v1/voice also  clientuserend X-Request-Id(nothenempty->pipeline  occurbecome, toaftercompat). 
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
			return false, errors.New("task canceled")
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

// callASRProcess callline B   POST /v1/process(**only  HTTP**). 
// sessionID asemptytime   "voice"(P0-4b:   connect     id). 
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
		return nil, fmt.Errorf("line B HTTP %d", resp.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Server) handleTaskGet(w http.ResponseWriter, r *http.Request) {
	// path e.g. /v1/task/{id}
	id := strings.TrimPrefix(r.URL.Path, "/v1/task/")
	s.mu.Lock()
	ts, ok := s.tasks[id]
	s.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown task_id"})
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
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST only"})
		return
	}
	var req confirmReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request body must be JSON {task_id,approved}"})
		return
	}
	s.mu.Lock()
	ts, ok := s.tasks[req.TaskID]
	s.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown task_id"})
		return
	}
	select {
	case ts.confirmCh <- req.Approved:
	default:
		writeJSON(w, http.StatusConflict, map[string]any{"error": "task not waiting for confirmation"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type cancelReq struct {
	TaskID string `json:"task_id"`
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST only"})
		return
	}
	var req cancelReq
	_ = json.NewDecoder(r.Body).Decode(&req)
	s.mu.Lock()
	ts, ok := s.tasks[req.TaskID]
	s.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown task_id"})
		return
	}
	if ts.cancel != nil {
		ts.cancel()
	}
	s.mu.Lock()
	applied := []string{}
	notApplied := []string{"task canceled, no action executed"}
	if ts.Outcome != nil && len(ts.Outcome.Receipts) > 0 {
		applied = []string{"see receipt for executed actions"}
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

// ---------- controlface:  disconnect /  back(   v1 §6) ----------

type interruptReq struct {
	TaskID string `json:"task_id,omitempty"` //   =global stop has  task
	Force  bool   `json:"force,omitempty"`   //   :   link pending   task raise 
}

// isActiveTask  disconnecttaskis "  in"( be disconnect):   in / waitconfirm / waitanswer. 
func isActiveTask(status string) bool {
	switch status {
	case stRunning, stWaiting, stNeedConfirm, stNeedAsk:
		return true
	}
	return false
}

// handleInterrupt global stop/refer  disconnect:  connect cancel   task  context(cancel  chain
//  disconnect end), tgt  canceled andsendevent. controlface   betask    . 
func (s *Server) handleInterrupt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST only"})
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
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown task_id 或无活动任务可打断"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "interrupted": ids})
}

type withdrawReq struct {
	TaskID string `json:"task_id"`
	Scope  string `json:"scope,omitempty"` // pending|running|done;    running
}

// handleWithdraw  back(  ): bytaskstagesplit handle(   v1 §6.2). 
//   - pending:  openstart/ raisein(need_ask/need_confirm/waiting)-> cancel,    use; 
//   - running:   in -> endstop +    out(cancel    disconnect end); 
//   - done: alreadydone -> tgt  revoked(  use    has /v1/tasks/{id}/rollback). 
func (s *Server) handleWithdraw(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST only"})
		return
	}
	var req withdrawReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TaskID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request body must be JSON {task_id, scope}"})
		return
	}
	if req.Scope == "" {
		req.Scope = "running"
	}
	switch req.Scope {
	case "pending", "running", "done":
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "scope must be pending|running|done"})
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	ts, ok := s.tasks[req.TaskID]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown task_id"})
		return
	}

	switch req.Scope {
	case "pending":
		//  raiseclass(   finish/ openstart):  connectcancel. 
		if !isActiveTask(ts.Status) {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "task not in revocable state", "status": ts.Status})
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
			writeJSON(w, http.StatusConflict, map[string]any{"error": "task not running", "status": ts.Status})
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
			writeJSON(w, http.StatusConflict, map[string]any{"error": "task unfinished, cannot withdraw as done", "status": ts.Status})
			return
		}
		s.markStatus(ts, stRevoked)
		s.emitEvent(ts, "withdrawn", map[string]any{
			"scope":      "done",
			"reversible": ts.Reversible,
			"note":       "for side-effect cleanup use /v1/tasks/{id}/rollback",
		})
		s.persist(ts)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "scope": "done", "revoked": true})
	}
}

// ActivityState returnback Harness   status(   v1 §5): decision(has decide /clarification,   )
// > busy(has  in)> idle(no  ). pending=  tasknum. version=status baseid
// (    useat  changeize i.e.patchsend, prevent sec status    ). 
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

// ---------- INTERACT-v1 posformendpoint ----------

type tasksPostReq struct {
	Text      string `json:"text"`
	Space     string `json:"space,omitempty"`
	RequestID string `json:"request_id,omitempty"` // M4-1 ① heavy  heavy 
	Mode      string `json:"mode,omitempty"`       // voice|text; voice confirm    (2026-10-04 useuser  "blocksafety  ")
	Document  string `json:"document,omitempty"`   // needrequire  safety ( nowclasstask; execImplement  data /LLM occurbecome use)
	Priority  int    `json:"priority,omitempty"`   //    v1 §6: 0-255    first , default 50
}

// clampPriority pipe priority recv to [0,255];   /   -> 50(   v1 §6). 
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

// spawnTask raise  task(text asfinish orig ; spaceHint  emptytimebefore "  <space>"). 
// requestID  emptytime   byReq useatheavy  heavy. status  after   persist. 
// returnback (ts, accepted): sched  listfulltime accepted=false(-> HTTP 429)and  note ; default    true. 
func (s *Server) spawnTask(text, spaceHint, requestID, mode, document string, priority int) (*taskState, bool) {
	// call  connectmanagetimeinitstartstatus pending(   openstart); default    running( charnode change). 
	initStatus := stRunning
	if s.sched != nil {
		initStatus = stPending
	}
	ts := &taskState{
		ID:        fmt.Sprintf("task-%d", time.Now().UnixNano()),
		RequestID: requestID,
		Text:      text,
		Mode:      mode, // voice->confirm    (2026-10-04 useuser  "blocksafety  ")
		Document:  document,
		Status:    initStatus,
		Role:      RolePlanner, // M5-3: initstart rule stage
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

	// C2 call path: in (byReq note andin same boundary out  , preventsame id heavy in ). 
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

// runPipeline  give  ts on    pipeline(first   or need_ask continue  use). 
// taskMaxDeadline task  deadline  onlimit(   v1 §7.1 task ):  ed restrictinterrupt, 
// prevent"byas 1 sec   1  time" out calluse  taskand process. 
const taskMaxDeadline = 10 * time.Minute

func (s *Server) runPipeline(ts *taskState, ctx context.Context, text, spaceHint, document string) {
	// task  onlimit: out calluseagain also   ed taskMaxDeadline. 
	// note : task  goroutine indiff   , cancel   by goroutine keephasand itsreturnbacktime  , 
	//     numbodysame returnbacktimetriggersend( thentask be i.e.cancel). 
	ctx, cancel := context.WithTimeout(ctx, taskMaxDeadline)
	o := *s.tmpl
	o.Document = document
	o.RequestID = ts.RequestID // P0-4b: in  request_id   to pipeline trace(emptythen pipeline  occurbecome)
	// §7.4 S3: pipeline      event   SSE(kind:"internal",  diffat markStatus   transition). 
	o.ProgressObserver = func(stage, detail string) { s.bridgeFromBus(ts, stage, detail) }
	o.ConfirmFn = func(taskID, question string) (bool, error) {
		// 2026-10-04 useuser  " hasblock  ": voice  formconfirm    (  raisewait, 
		//  then iOS noconfirm   -> task   need_confirm, useuser  "  "). confirm  day provide  . 
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
			return false, errors.New("task canceled")
		}
	}

	fullText := text
	if spaceHint != "" {
		fullText = "在 " + spaceHint + " " + text
	}
	safeGo("pipeline:"+ts.ID, func() {
		defer cancel() // taskcloseend(  time/cancel)time   deadline  time 
		// C2: task goroutine  outtimeclose  doneCh(endstate/ raiseall ), providecall   worker    . select prevent close(need_ask continue  again   num). 
		defer func() {
			if ts.doneCh != nil {
				select {
				case <-ts.doneCh:
				default:
					close(ts.doneCh)
				}
			}
		}()
		// C1: pipeline.Run recv  Runner(ob=nil: SSE/CLI    event   o.ProgressObserver=bridgeFromBus,   aschangeize). 
		runStart := time.Now()
		rn := NewRunner(ts, &o, nil)
		out, err := rn.Run(ctx, fullText)
		loopMs := time.Since(runStart).Milliseconds()
		// close ize timeday (mobileside  use,     ): total_ms = classify+LLM+  safetyseg. 
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
			// langaudio scenario(iOS by   , no  rev   ): intent UNKNOWN time**  raiserev **, 
			//  connectcloseendandback  show,   task   need_ask -> App pollnoendstate -> useuser" rev ". 
			if ts.Mode == "" || ts.Mode == "voice" {
				heard := strings.TrimSpace(text)
				if len(heard) > 40 {
					heard = heard[:40] + "…"
				}
				receipt := "you said [" + heard + "] -- I am not sure what you meant." +
					"be explicit, e.g. check Beijing weather, note an idea, run the tests."
				// mobilesidebypoll GET /v1/tasks/{id}   receipt asapprove,   modify View.Result,  modify event  occur . 
				out.View.Action = "(needs clarification)"
				out.View.Result = receipt
				ts.Outcome = &out
				s.markStatus(ts, stDone)
				s.emitEvent(ts, "done", map[string]any{
					"receipt":     contract.RenderReceipt(out.View),
					"attribution": out.Attribution,
				})
				s.persist(ts)
				return
			}
			//    G8 fix :   no recv time    , but isnolimitclarification. 
			//    S1:    datais**     +   onlimit**,  is"andon   char same"
			// -- afterer  Q1->Q2->Q1->Q2   time    in. 
			if ts.askedQuestions == nil {
				ts.askedQuestions = map[string]bool{}
			}
			if ts.askedQuestions[out.Ask] || ts.askRounds >= maxAskRounds {
				ts.Err = "clarification not converged: same question repeated or round limit reached;" +
					"please restate as a full command (name the object explicitly)"
				s.markStatus(ts, stCanceled)
				s.emitEvent(ts, "canceled", map[string]any{
					"reason": "ask_not_converging", "rounds": ts.askRounds,
				})
				s.persist(ts)
				return
			}
			// clarificationexit:  raiseas need_ask, wait /answer continue (M4-1 ②). 
			ts.askedQuestions[out.Ask] = true
			ts.askRounds++
			s.markStatus(ts, stNeedAsk)
			ts.Question = out.Ask
			ts.Options = out.Options // M4-3 ① close ize [{id,label}]
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
	}, func(r any) { s.onTaskPanic(ts, &o, r) })
}

// maxAskRounds issame task allow       (   G8 fix ). 
//  edi.e. "   recv "andcancel,  againnolimitclarification. 
const maxAskRounds = 3

// askAnaphora iscontinue time curbe        coreferenceword( word before,   "  file"be"  " first split). 
var askAnaphora = []string{
	"那个文件", "这个文件", "那个页面", "这个页面", "那个项目", "这个项目",
	"那个", "这个", "它",
}

// pronounIndex returnback trig   text in **charnode**undertgt;   inreturnback -1. 
//
//    S2: in  hasword boundary,   char worduse strings.Contains    word   split
// ("its "    " ", "by "    " "). thusto char word before   . 
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
			case '其', '由': // its  / by  -- isword   split,  is word
				continue
			}
		}
		return len(string(runes[:i]))
	}
	return -1
}

// stripOptionPrefix pipe  by  id(dict:xxx / rec:xxx)alsoorigbecome read bodyname. 
func stripOptionPrefix(s string) string {
	s = strings.TrimSpace(s)
	for _, p := range []string{"dict:", "rec:"} {
		if strings.HasPrefix(s, p) {
			return strings.TrimSpace(strings.TrimPrefix(s, p))
		}
	}
	return s
}

// resolveClarified use      origsent  coreferenceword,  to   continue  base(   G8 fix ). 
//
// orig nowonlypipe     senttail(`orig  + "   : " +   `), origsent  "  /  "  , 
// refer      to   -> again writeoutsame sent Ask -> mobileon    out   . 
//
//   afteruse id    ,  classify   extractPath   connect out formto : 
//
//	"pipe  modify under" + "main.go" -> "pipe“main.go”modify under",  againneedneedcoreference resolution. 
//
//  " id be out"   by server      TestResolveClarifiedExtractsObject   (   S3). 
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

// resumeAsk pipe need_ask taskuse answer continue ; answer asemptythen**reject**andkeepkeepwaitstate. 
// returnback false tableshow accept (calluse  back 400), task stop  need_ask, useuser byagain   . 
func (s *Server) resumeAsk(ts *taskState, answer string) bool {
	if strings.TrimSpace(answer) == "" {
		//    S7: empty   is"   recv ", origbecause same,     . 
		return false
	}
	//    P3:      clarification     useuser" cancel",     give    semantic, 
	// but is    "   recv " errorpath. 
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
	// note :   ** **heavy  askedQuestions / askRounds --   need  keep , 
	// only  diff"same    againbe   "(   S1). 

	//    G5-P2-7:    clarification    becur **new   refer **  . 
	//
	// origsentbase     , pipe  back (`orig  + "   : " +   `)only again triggersend     , 
	//  become    .  giveclarification     "first needfirst   "  **  semantic**: 
	// useuser   sent, thenisneed      . 
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
		// no   coreferenceword  botpath:  need     id before ,     pathhandle   (   S4). 
		substituted = " 澄清：" + stripOptionPrefix(answer)
	}
	s.runPipeline(ts, ctx, prefix+substituted, "", ts.Document)
	return true
}

func (s *Server) handleTasksPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST only"})
		return
	}
	var req tasksPostReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Text) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request body must be JSON {text}"})
		return
	}

	// M4-1 ①: same request_id heavy    -> returnback hastask,  heavy   /writetrace. 
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
		// C2: call   listfull -> 429 Too Many Requests. 
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "task queue full, retry later"})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"task_id": ts.ID, "status": ts.Status})
}

// handleTasksSub routeby /v1/tasks/{id} and /v1/tasks/{id}/answer, /rollback. 
func (s *Server) handleTasksSub(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/v1/tasks/")
	parts := strings.Split(strings.TrimSuffix(rest, "/"), "/")
	id := parts[0]

	s.mu.Lock()
	ts, ok := s.tasks[id]
	s.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown task_id"})
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
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "unsupported method/path"})
	}
}

// handleEvents  now SSE  (M6-1): ?after=<lastSeq> heavy , howeverafter raise   toendstate. 
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
	// heavy  seq > after  event
	lastTerminal := false
	for _, ev := range ts.events {
		if ev.Seq > after {
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, mustJSON(ev.flattened()))
			if ev.Type == "done" || ev.Type == "failed" || ev.Type == "canceled" {
				lastTerminal = true
			}
		}
	}
	// alreadyendstatethen connectclose 
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

// handleCancelSub routeby POST /v1/tasks/{id}/cancel(INTERACT-v1 new state). 
func (s *Server) handleCancelSub(w http.ResponseWriter, r *http.Request, ts *taskState) {
	if ts.cancel != nil {
		ts.cancel()
	}
	s.mu.Lock()
	applied := []string{}
	notApplied := []string{"task canceled, no action executed"}
	if ts.Outcome != nil && len(ts.Outcome.Receipts) > 0 {
		applied = []string{"see receipt for executed actions"}
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

// writeTaskView by INTERACT-v1  status  (receipt=  , attribution=  ). 
//
// race fix : ts  charsegby runPipeline after  goroutine keep s.mu write(Outcome/Status/Role…), 
// base numis GET /v1/tasks/{id} pollreadend. fast    same pipe s.mu indone, again outwrite HTTP, 
//  thenread ts.Outcome/ts.Status andafter writeandsend -> DATA RACE(-race  ). 
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
	}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, body)
}

type answerReq struct {
	Answer string `json:"answer"`
}

// handleAnswer:     decision point. need_confirm -> confirm ; need_ask -> by  continue (M4-1 ②). 
func (s *Server) handleAnswer(w http.ResponseWriter, r *http.Request, ts *taskState) {
	var req answerReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request body must be JSON {answer}"})
		return
	}
	s.mu.Lock()
	st := ts.Status
	s.mu.Unlock()

	// need_ask: by   basecontinue same task. 
	if st == stNeedAsk {
		s.mu.Lock()
		accepted := s.resumeAsk(ts, req.Answer)
		s.mu.Unlock()
		if !accepted {
			// empty  : rejectaccept , task stop  need_ask, useuser byagain   (   S7). 
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "clarification answer cannot be empty; task still awaiting answer", "status": st,
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "resumed": true})
		return
	}

	if st != stNeedConfirm {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "no pending decision point", "status": st})
		return
	}
	// confirmword:   /y/yes/true/1 ->   ; its  -> reject. 
	approved := isConfirmWord(req.Answer)
	select {
	case ts.confirmCh <- approved:
	default:
		writeJSON(w, http.StatusConflict, map[string]string{"error": "decision point expired"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "approved": approved})
}

// handleRollback: from    . 
//
// [pseudocode logic layer]( writemodule: reversibleity   +      + rejectpath)
//
// control flow: 
//
//	if ts.Status != done: 409 "task done,   rollback". 
//	if !isReversibleIntent(intent): 409 " reversibletask(COMMIT/DEPLOY etc)forbidstoprollback". 
//	if ts.BackupPath == "" || file store : 404 "no     ". 
//	target = ts.TargetPath; ifempty -> 404(   alsoorigto ). 
//	copy(BackupPath -> target)(os.ReadFile+WriteFile, 0644). 
//	become  -> 200 {ok:true, restored:target}. 
//
// error: copy    -> 500; server heavystartafterinstorestate  (M3 connectaccept,   tgtnote). 
func (s *Server) handleRollback(w http.ResponseWriter, ts *taskState) {
	s.mu.Lock()
	st := ts.Status
	rev := ts.Reversible
	bak := ts.BackupPath
	tgt := ts.TargetPath
	s.mu.Unlock()

	if st != stDone {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "task unfinished, cannot roll back", "status": st})
		return
	}
	if !rev {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "irreversible tasks (COMMIT/DEPLOY) cannot roll back"})
		return
	}
	if bak == "" || tgt == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no revocable backup or unknown target"})
		return
	}
	data, err := os.ReadFile(bak)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "backup file does not exist"})
		return
	}
	if err := os.WriteFile(tgt, data, 0o644); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "restore failed: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "restored": tgt})
}

// handleStatus: GET /v1/status(INTERACT-v1). 
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"version": version,
		"uptime":  time.Since(s.boot).Seconds(),
		"tasks":   len(s.tasks),
	})
}

// handleRoles(M5-3): returnback   define + curbeforetask  . 
func (s *Server) handleRoles(w http.ResponseWriter, r *http.Request) {
	all := []map[string]any{
		{"id": RolePlanner, "label": "Planner（规划）"},
		{"id": RoleExecutor, "label": "Executor（执行）"},
		{"id": RoleVerifier, "label": "Verifier（校验）"},
	}
	// get     endstatetask   ; nothensafety inactive. 
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

// ---------- M4-1: keep ize /  heavy / ask continue  ----------
//
// [pseudocode logic layer]( writemodule: ①request_id  etc heavy + ②need_ask  raisecontinue  + ③heavystart  )
//
// POST /v1/tasks: 
//   if body.request_id  emptyand s.byReq[request_id] store :
//       ts =  hastask; if ts.status∈{running,need_confirm,need_ask}:
//           return 202 {task_id, status}( andwait,  open   )
//       else: return 200 {task_id, status: hasclose }( heavy   /writetrace)
//   else: new  ts, spawnTask(orig  ). 
//
// POST /v1/tasks/{id}/answer: 
//   if ts.status == need_ask:
//       fullText = ts.Text + "   : " + answer   //   continue =by  heavy 
//       ts.status = running; spawnTask   goroutine heavy  pipeline( use ts.ID)
//       return 200 {ok:true, resumed:true}
//   elif ts.status == need_confirm:  orig confirmCh  . 
//   else: 409. 
//
// keep ize:   status  after persist(ts) -> <log_dir>/tasks/<id>.json(tmp+rename orig ). 
// restore()(New time):    tasks/*.json, done/canceled origkindintable; 
//   running/need_confirm/need_ask -> tgt interrupted( heavy  confirmCh/cancel, prevent etc). 
// error: tasks obj   write -> onlyinstore  (M4-1  connectaccept); restore  file ed. 

func (s *Server) tasksDir() string {
	return filepath.Join(s.cfg.Global.LogDir, "tasks")
}

// persist orig   taskstatus(tmp + rename). 
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

// restore start time    task;  donestatustgt interrupted. 
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
			ts.Err = "task interrupted by restart, please resubmit"
		}
		s.tasks[ts.ID] = &ts
		if ts.RequestID != "" {
			s.byReq[ts.RequestID] = ts.ID
		}
	}
}

// emitEvent      SSE event(seq  add)and  give    er. 
//
// [pseudocode logic layer](M6-1: event emit time /heavy / disconnect  ): 
//
//	ts.eventSeq++; ev = {seq: ts.eventSeq, type, data}
//	ts.events = append(ts.events, ev)(andtaskkeep izesameoccur  period)
//	for ch in ts.listeners: non-blocking send ev(fullthen ed, prevent    )
//	endstateevent(done/failed/canceled)send after close  listener--by handler   endstateafter out. 
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

// bridgeFromBus pipe pipeline       event   SSE(   §7.4). 
//
//	kind:"internal" =    in stage(pipeline ProgressObserver backcall); 
//	 diffat markStatus send  kind:"transition"    status  (phase). 
//
//  use ts.eventSeq++  id(     id), event append   ts.events -> ?after= heavy   overwrite. 
// by pipeline goroutine backcall,    keep s.mu(emitEvent readwrite ts.eventSeq/events/listeners). 
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

// markStatus modifystatusand  (calluse alreadykeep mu or i.e.  ). 
func (s *Server) markStatus(ts *taskState, status string) {
	s.activityVer++ // status   ->  base add(    data  i.e.patchsend,    v1 §5)
	ts.Status = status
	ts.Role = roleForStatus(status) // M5-3
	s.emitEvent(ts, "stage", map[string]any{
		"role": ts.Role, "phase": status, "step": stepName(status),
	})
	s.persist(ts)
}

// onTaskPanic isafter task goroutine be safeGo recover after   recvtail(P0-2): 
//  1. pipetasktgt  canceled(panic  aserrorendstop,     stRunning   ); 
//  2. pipe panic writeastrace error kind(best-effort, provide afterheavy /attribution). 
//
//  timecalluse  goroutine   `defer mu.Unlock()` already  unwind stagefirst   finish (body defer in first , 
// safeGo   recover defer out after ), thus   safesafetyheavynew  . 
func (s *Server) onTaskPanic(ts *taskState, o *pipeline.Options, r any) {
	if ts != nil {
		s.mu.Lock()
		ts.Err = fmt.Sprintf("panic: %v", r)
		s.markStatus(ts, stCanceled)
		s.mu.Unlock()
	}
	rid := ""
	if ts != nil {
		// S0/P0-4b: panic trace  request_id and pipeline same   (ts.RequestID); emptythen ts.ID  bot, 
		//   panic     join   requirechain(in ->trace->day ). 
		rid = ts.RequestID
		if rid == "" {
			rid = ts.ID
		}
	}
	if o != nil && o.Trace != nil {
		_ = o.Trace.Write(trajectory.Entry{
			RequestID: rid,
			Kind:      trajectory.KindError,
			Content:   fmt.Sprintf("background task goroutine panic: %v", r),
		})
	}
}

// ---------- rollback   (reversibleity/   get) ----------

//  reversibleintentwordtable(COMMIT/DEPLOY etc). 
var irreversibleIntents = map[string]bool{
	contract.IntentCommit: true,
	contract.IntentDeploy: true,
}

func isReversibleIntent(intent string) bool {
	return !irreversibleIntents[intent]
}

// pickBackupPath use C deliver close izetgt resolve     path(M4-3). 
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
		// NOTE   to <log_dir>/notes.md(planVerify same ). 
		return "" // bycalluse by log_dir  --server  keephas log_dir,  empty->404 safesafety. 
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

// newestBackup get <log_dir>/backups under new  .bak(NOTE back    bodypathtime  bot). 
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

// isCancelAnswer     answeris as"cancel/  "(   P3). 
//
//      clarification   useuser" cancel",       has  semantic: 
//   lycanceltask, but isbecurbecomeagain      . 
func isCancelAnswer(answer string) bool {
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "取消", "不用了", "算了", "不做了", "不要了", "cancel", "no", "n":
		return true
	}
	return false
}
