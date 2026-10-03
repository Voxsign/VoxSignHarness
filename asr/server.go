// server.go —— 本地 HTTP 服务适配层（需求 §6 对外接口 / 验收 #1）。
//
// 定位：**传输适配**，不含任何业务判断。它调用 Pipeline / Dictionary，
// 把结果序列化成 JSON；不执行任务、不触碰自己数据目录之外的任何文件（红线 #1）。
//
// 本轮（第 1 批）只交付：/v1/health、/v1/correct、/v1/dictionary、/v1/process。
// /v1/process 的意图分类属第 2 批，当前返回**低置信 ASK + need_disambiguate**，
// 这是需求 4.6 明文的兜底行为（置信度低于阈值 → 回问/转 ASK），不是假装已实现意图解析。
package asr

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Server 持有管线，提供 HTTP 处理。
type Server struct {
	Pipe *Pipeline
	// IntentModel 是可选的**兜底**模型（只有本地低置信时才用；nil = 纯本地）。
	IntentModel   IntentModel
	IntentTimeout time.Duration
	// DataDir 是本机数据目录（画像等；空则视为无画像）。
	DataDir string
	// Teach 是"用户教一个词"的后端钩子（CACHE-001 G2）。
	// 路径用规范里已有的 `/v1/observe`（VHS-ASR-001 P3 端点清单），**不新造路径**。
	// 为空 ⇒ 该端点返回 503（不假装支持）。
	Teach func(term, canonical string) error
	// ClearTaught 清空"用户教的词"（**不得误清服务别名**）。
	ClearTaught func() int
}

// NewServer 构造服务。
func NewServer(p *Pipeline) *Server { return &Server{Pipe: p} }

// Handler 返回路由表。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/health", s.handleHealth)
	mux.HandleFunc("/v1/correct", s.handleCorrect)
	mux.HandleFunc("/v1/dictionary", s.handleDictionary)
	mux.HandleFunc("/v1/process", s.handleProcess)
	mux.HandleFunc("/v1/feedback", s.handleFeedback)
	mux.HandleFunc("/v1/observe", s.handleObserve)
	mux.HandleFunc("/v1/lexicon", s.handleLexicon)
	mux.HandleFunc("/v1/task", s.handleTask)
	mux.HandleFunc("/v1/testpage", s.handleTestRun)
	mux.HandleFunc("/v1/testlog", s.handleTestLog)
	mux.HandleFunc("/", s.handleTestPage)
	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "contract_version": "1"})
}

type correctRequest struct {
	Text      string   `json:"text"`
	SessionID string   `json:"session_id"`
	Context   []string `json:"context"`
}

type correctResponse struct {
	ContractVersion        string       `json:"contract_version"`
	Raw                    string       `json:"raw"`
	Text                   string       `json:"text"`
	Punctuated             string       `json:"punctuated"`
	Corrections            []Correction `json:"corrections"`
	PunctuationCorrections []Correction `json:"punctuation_corrections"`
	Candidates             []Candidate  `json:"candidates"`
	Steps                  []Step       `json:"steps"`
}

func (s *Server) handleCorrect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
		return
	}
	var req correctRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	res := s.Pipe.Process(req.Text, req.SessionID)
	writeJSON(w, http.StatusOK, correctResponse{
		ContractVersion:        "1",
		Raw:                    res.Raw,
		Text:                   res.Text,
		Punctuated:             res.Punctuated,
		Corrections:            res.Corrections,
		PunctuationCorrections: res.PunctuationCorrections,
		Candidates:             res.Candidates,
		Steps:                  res.Steps,
	})
}

// intentResponse 是**契约 v1** 的响应形状（需求 §6）。
// 字段集必须与 contracts/intent-v1.schema.json 一致（只增不改）。
type intentResponse struct {
	ContractVersion  string         `json:"contract_version"`
	Type             string         `json:"type"`
	Path             string         `json:"path"`
	Params           map[string]any `json:"params"`
	Confidence       float64        `json:"confidence"`
	NeedDisambiguate bool           `json:"need_disambiguate"`
	DomainSuggestion []string       `json:"domain_suggestion"`
	Control          string         `json:"control"`
	Confirmable      *confirmedRef  `json:"confirmable,omitempty"`
	// ContextSources 是**注入来源归因**（SCOPE-PROFILE-01：能否说清从哪来）。
	// 枚举：handwritten | zhiji | learned | project-map | none —— **none 是正规成员**（可穷举）。
	ContextSources []string `json:"context_sources"`
	// ContextSourcesDetails 放**原因**（如 no_profile），不塞进枚举值。
	ContextSourcesDetails string `json:"context_sources_details,omitempty"`
	// ConfirmPatternMiss：确认话术**落空留痕**（匹配不上时必须可见，否则命中率无从得知）。
	ConfirmPatternMiss bool   `json:"confirm_pattern_miss,omitempty"`
	Degraded           bool   `json:"degraded,omitempty"`
	DegradedReason     string `json:"degraded_reason,omitempty"`
	Traces             []Step `json:"traces"`
}

// SourceNone 是 context_sources 的**正规枚举成员**：确实没有来源。
const SourceNone = "none"

type processRequest struct {
	Text      string `json:"text"`
	SessionID string `json:"session_id"`
	AudioMeta any    `json:"audio_meta"`
	// Context：**调用方携带的上下文**（选项 C：会话记忆由调用方带回，服务架构不变）。
	Context []string `json:"context,omitempty"`
	// Confirmed：**调用方带回的确认结构**（服务据此复用，但自己不记得）。
	Confirmed *confirmedRef `json:"confirmed,omitempty"`
}

// confirmedRef 是可携带的确认结构（第一次响应给出，第二次由调用方带回）。
type confirmedRef struct {
	Mention   string `json:"mention"`
	Canonical string `json:"canonical"`
}

// pickPathFromContext 从调用方给的上下文里取一个可用的路径（不猜：只认显式形态）。
func pickPathFromContext(ctx []string) string {
	for _, c := range ctx {
		c = strings.TrimSpace(c)
		for _, prefix := range []string{"打开 ", "打开", "open "} {
			if strings.HasPrefix(c, prefix) {
				return strings.TrimSpace(strings.TrimPrefix(c, prefix))
			}
		}
		if strings.ContainsAny(c, "/.") && !strings.Contains(c, " ") {
			return c
		}
	}
	return ""
}

// profileSources 返回画像注入来源（归因，不保证内容）。
//
// 要求（SCOPE-PROFILE-01）：**永远非 nil**；无画像文件时给**显式值**（不是缺字段）；
// 且**绝不做全量历史加载**（需求 4.5）。
// ⚠️ L3 的 zhiji / learned / project-map 三类**尚未实现**（等真值来源裁决）。
//
// `none` 是**正规枚举成员**（不是 "none:no_profile" 这种哨兵字符串）——
// 哨兵在枚举之外，等于"一个表示没有来源的值看起来像一个来源"，按枚举穷举的消费方会漏掉它。
// **"没有"与"有"必须分列**；原因（no_profile）放 details，不塞进枚举值。
func profileSources(dataDir string) (sources []string, details string) {
	if dataDir == "" {
		return []string{SourceNone}, "no_data_dir"
	}
	p := filepath.Join(dataDir, "profile", "handwritten.json")
	if b, err := os.ReadFile(p); err == nil && len(bytes.TrimSpace(b)) > 0 {
		return []string{"handwritten"}, ""
	}
	return []string{SourceNone}, "no_profile"
}

// detectConfirmation 识别"确认，就是 X"形态并给出可携带结构（服务不保存它）。
func detectConfirmation(text string) *confirmedRef {
	if !strings.Contains(text, "确认") {
		return nil
	}
	for _, sep := range []string{"就是", "指的是", "是"} {
		if i := strings.Index(text, sep); i >= 0 {
			canon := strings.TrimSpace(strings.Trim(text[i+len(sep):], "。，,. ！!"))
			if canon != "" {
				return &confirmedRef{Mention: "那个模块", Canonical: canon}
			}
		}
	}
	return nil
}

func (s *Server) handleProcess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
		return
	}
	var req processRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	res := s.Pipe.Process(req.Text, req.SessionID)
	// 本地规则解析意图（红线 #6：核心路径本地）；低置信时才由模型兜底，失败即降级。
	var cs []string
	var csDetails string
	cs, csDetails = profileSources(s.DataDir)
	var confirmable *confirmedRef
	missConfirm := false
	ir := ClassifyIntentWith(r.Context(), res.Text, s.IntentModel, s.IntentTimeout)
	// 选项 C：**服务不持久化会话态**；消解所需的上下文/确认全部由调用方携带。
	var path string
	if req.Confirmed != nil && req.Confirmed.Canonical != "" {
		path = req.Confirmed.Canonical
		ir.NeedDisambiguate = false
	} else if p := pickPathFromContext(req.Context); p != "" {
		path = p
		ir.NeedDisambiguate = false
	}
	if ref := detectConfirmation(res.Text); ref != nil {
		confirmable = ref
	} else if strings.Contains(res.Text, "确认") {
		// 用户想确认，但话术没匹配上 ⇒ **落空留痕**（不静默）。
		missConfirm = true
	}
	writeJSON(w, http.StatusOK, intentResponse{
		ContractVersion:       "1",
		Type:                  ir.Type,
		Path:                  path,
		Params:                map[string]any{},
		Confidence:            ir.Confidence,
		NeedDisambiguate:      ir.NeedDisambiguate,
		DomainSuggestion:      ir.DomainSuggestion,
		Control:               ir.Control,
		Degraded:              ir.Degraded,
		DegradedReason:        ir.DegradedReason,
		Confirmable:           confirmable,
		ContextSources:        cs,
		ContextSourcesDetails: csDetails,
		ConfirmPatternMiss:    missConfirm,
		Traces:                res.Steps,
	})
}

// handleObserve 是"用户教一个词"的入口（G2：Peter 原话「最近我说的词」）。
// 来源固定标 user_taught（可审计）；空词/同值一律拒绝（否则缓存被污染成"什么都能命中"）。
func (s *Server) handleObserve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
		return
	}
	if s.Teach == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "teaching disabled"})
		return
	}
	var req struct {
		Term      string `json:"term"`
		Canonical string `json:"canonical"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if strings.TrimSpace(req.Term) == "" || strings.TrimSpace(req.Canonical) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "term/canonical 不得为空"})
		return
	}
	if strings.TrimSpace(req.Term) == strings.TrimSpace(req.Canonical) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "term 与 canonical 相同，无需教"})
		return
	}
	if err := s.Teach(req.Term, req.Canonical); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "source": "user_taught"})
}

// handleLexicon 是词表管理入口（路径取自规范 VHS-ASR-001 P3 的端点清单）。
// 当前支持 {"op":"clear_taught"}：**只清用户教的词**，不碰服务别名。
func (s *Server) handleLexicon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
		return
	}
	var req struct {
		Op string `json:"op"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	switch req.Op {
	case "clear_taught":
		if s.ClearTaught == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "lexicon disabled"})
			return
		}
		n := s.ClearTaught()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "removed": n, "scope": "user_taught"})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unknown op: " + req.Op})
	}
}

// handleFeedback 回传一次反馈：登记为**候选词典条目**（source/created_at 可审计）。
// 注意：按 ASR-MODEL-02 L2，用户显式反馈属 user_explicit，不过 learn 通道；
// 隐式推断才必须过 learn（本轮未实现）。
func (s *Server) handleFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
		return
	}
	var fb struct {
		TextRaw   string `json:"text_raw"`
		TextFinal string `json:"text_final"`
		Accepted  bool   `json:"accepted"`
		Source    string `json:"source"`
	}
	if err := readJSON(r, &fb); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if s.Pipe.Dict == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "dictionary disabled"})
		return
	}
	if fb.Accepted && fb.TextRaw != "" && fb.TextFinal != "" && fb.TextRaw != fb.TextFinal {
		src := fb.Source
		if src == "" {
			src = "user_edit"
		}
		if err := s.Pipe.Dict.Add(DictionaryEntry{RawSpeech: fb.TextRaw, Target: fb.TextFinal, Source: src}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type dictionaryRequest struct {
	Text      string `json:"text"` // 语音指令原文
	Op        string `json:"op"`   // add | update | delete | list
	RawSpeech string `json:"raw_speech"`
	Target    string `json:"target"`
	Scope     string `json:"scope"`
	Priority  int    `json:"priority"`
	Source    string `json:"source"`
	Term      string `json:"term"`
	Confirm   bool   `json:"confirm"`
}

func (s *Server) handleDictionary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
		return
	}
	if s.Pipe.Dict == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "dictionary disabled"})
		return
	}
	var req dictionaryRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}

	// 语音指令优先：先 Add，再 Delete（都是高风险以外的路径有明确护栏）。
	if req.Text != "" {
		if e, ok := ParseVoiceAdd(req.Text); ok {
			if err := s.Pipe.Dict.Add(e); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "entry": e, "version": s.Pipe.Dict.Version()})
			return
		}
		if term, ok := ParseVoiceDelete(req.Text); ok {
			if !req.Confirm {
				writeJSON(w, http.StatusOK, map[string]any{"need_confirm": true, "term": term})
				return
			}
			_, _ = s.Pipe.Dict.Delete(term, true)
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": true, "term": term, "version": s.Pipe.Dict.Version()})
			return
		}
	}

	switch req.Op {
	case "add", "update":
		e := DictionaryEntry{RawSpeech: req.RawSpeech, Target: req.Target, Scope: req.Scope, Priority: req.Priority, Source: req.Source}
		if e.Source == "" {
			e.Source = "manual"
		}
		if err := s.Pipe.Dict.Add(e); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "entry": e, "version": s.Pipe.Dict.Version()})
	case "delete":
		// 红线 #3：无确认不执行，且**不落盘**（文件必须原样）。
		if !req.Confirm {
			writeJSON(w, http.StatusOK, map[string]any{"need_confirm": true, "term": req.Term})
			return
		}
		deleted, err := s.Pipe.Dict.Delete(req.Term, true)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": deleted, "term": req.Term, "version": s.Pipe.Dict.Version()})
	case "list":
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "entries": s.Pipe.Dict.List(), "version": s.Pipe.Dict.Version()})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unknown op"})
	}
}
