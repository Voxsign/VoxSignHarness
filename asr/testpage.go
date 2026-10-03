// testpage.go —— 本地测试页（Peter 要"马上能测"）：零依赖 HTML + 真实台账。
//
// 全部意义在**台账落盘**：每用一次就产生一条**真实样本**，
// 让"L0 比例 / 升级率 / 升级后是否变好"从"无法判定"变成"可算"。
package asr

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// RealLogRecord 是一条真实使用样本（append-only）。
type RealLogRecord struct {
	At          string  `json:"at"`
	Raw         string  `json:"raw"`
	Corrected   string  `json:"corrected"`
	Intent      string  `json:"intent"`
	AskBack     bool    `json:"ask_back"`
	Degraded    bool    `json:"degraded"`
	Level       string  `json:"level"`
	Ms          float64 `json:"ms"`
	TaughtHit   bool    `json:"taught_hit"`
	Corrections int     `json:"corrections"`
}

func (s *Server) realLogPath() string {
	if s.DataDir == "" {
		return ""
	}
	return filepath.Join(s.DataDir, "reallog.jsonl")
}

// appendRealLog 追加一条真实样本（append-only；失败不阻断页面）。
func (s *Server) appendRealLog(rec RealLogRecord) error {
	p := s.realLogPath()
	if p == "" {
		return fmt.Errorf("no datadir")
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return err
}

// handleTestRun 是页面唯一的数据入口：纠错 → 意图 → 落**真实台账**。
func (s *Server) handleTestRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	start := time.Now()
	res := s.Pipe.Process(req.Text, "testpage")
	ir := ClassifyIntentWith(r.Context(), res.Text, s.IntentModel, s.IntentTimeout)
	ms := float64(time.Since(start).Microseconds()) / 1000.0

	level := "L0"
	if ir.Degraded {
		level = "L0(fallback)"
	}
	taughtHit := false
	for _, c := range res.Corrections {
		if strings.Contains(c.Evidence, "user_taught") {
			taughtHit = true
		}
	}
	rec := RealLogRecord{
		At: time.Now().UTC().Format(time.RFC3339Nano), Raw: req.Text, Corrected: res.Text,
		Intent: ir.Type, AskBack: ir.NeedDisambiguate, Degraded: ir.Degraded,
		Level: level, Ms: ms, TaughtHit: taughtHit, Corrections: len(res.Corrections),
	}
	logErr := ""
	if err := s.appendRealLog(rec); err != nil {
		logErr = err.Error() // 记账失败**留痕**，不静默、不阻断
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"raw": rec.Raw, "corrected": rec.Corrected, "intent": rec.Intent,
		"ask_back": rec.AskBack, "degraded": rec.Degraded, "degraded_reason": ir.DegradedReason,
		"level": rec.Level, "ms": rec.Ms, "taught_hit": rec.TaughtHit,
		"corrections": res.Corrections, "candidates": ir.DomainSuggestion,
		"log_error": logErr, "log_path": s.realLogPath(),
	})
}

// handleTestLog 返回最近 N 条 + **三个考察点**的聚合。
func (s *Server) handleTestLog(w http.ResponseWriter, r *http.Request) {
	n := 20
	if v := r.URL.Query().Get("n"); v != "" {
		if k, err := strconv.Atoi(v); err == nil && k > 0 && k <= 500 {
			n = k
		}
	}
	p := s.realLogPath()
	var recs []RealLogRecord
	if p != "" {
		if f, err := os.Open(p); err == nil {
			sc := bufio.NewScanner(f)
			sc.Buffer(make([]byte, 1<<20), 1<<20)
			for sc.Scan() {
				var rec RealLogRecord
				if json.Unmarshal(sc.Bytes(), &rec) == nil {
					recs = append(recs, rec)
				}
			}
			_ = f.Close()
		}
	}
	total := len(recs)
	l0, askBack, degraded := 0, 0, 0
	for _, rec := range recs {
		if strings.HasPrefix(rec.Level, "L0") {
			l0++
		}
		if rec.AskBack {
			askBack++
		}
		if rec.Degraded {
			degraded++
		}
	}
	rate := func(k int) float64 {
		if total == 0 {
			return 0
		}
		return float64(k) / float64(total)
	}
	// 末尾 N 条（倒序）
	recent := recs
	if len(recent) > n {
		recent = recent[len(recent)-n:]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path": p, "total": total, "recent": recent,
		"metrics": map[string]any{
			"l0_share": rate(l0), "ask_back_rate": rate(askBack), "degraded_rate": rate(degraded),
			"note": "样本 <10 时只报数字、不下结论（与 route.Report 同口径）",
		},
	})
}
