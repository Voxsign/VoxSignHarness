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
	"encoding/json"
	"net/http"
)

// Server 持有管线，提供 HTTP 处理。
type Server struct {
	Pipe *Pipeline
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
	Traces           []Step         `json:"traces"`
}

type processRequest struct {
	Text      string `json:"text"`
	SessionID string `json:"session_id"`
	AudioMeta any    `json:"audio_meta"`
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
	// 第 2 批：真正的意图分类。当前按需求 4.6 兜底为低置信 ASK + 回问，绝不猜测。
	writeJSON(w, http.StatusOK, intentResponse{
		ContractVersion:  "1",
		Type:             "ASK",
		Path:             "",
		Params:           map[string]any{},
		Confidence:       0,
		NeedDisambiguate: true,
		DomainSuggestion: []string{},
		Control:          "",
		Traces:           res.Steps,
	})
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
