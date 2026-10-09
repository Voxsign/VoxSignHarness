package pipeline

// R13 (2026-10-09): Observability layer (ETCLOVG "O", distillation from
// AutoHarness per-tool-call JSONL audit records + Agent Harness Engineering §9.5).
// Every executed turn appends one structured record to
// context_slots/trace-<convID>.jsonl so the harness owns a replayable
// diagnosis/audit trail: request_id, intent/action, per-receipt tool outcome,
// ask, goal state, attribution, and latency. Cost fields are reserved for the
// model-hub billing integration (R13 P2).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type traceRec struct {
	RequestID   string         `json:"request_id"`
	TS          string         `json:"ts"`
	ConvID      string         `json:"conv_id"`
	Intent      string         `json:"intent"`
	Action      string         `json:"action"`
	Receipts    []traceReceipt `json:"receipts"`
	Ask         string         `json:"ask,omitempty"`
	GoalStatus  string         `json:"goal_status,omitempty"`
	GoalQueue   []string       `json:"goal_queue,omitempty"`
	Attribution string         `json:"attribution,omitempty"`
	LoopMs      int64          `json:"loop_ms,omitempty"`
	NetMs       int64          `json:"net_ms,omitempty"`
}

type traceReceipt struct {
	Tool      string `json:"tool"`
	OK        bool   `json:"ok"`
	Confirm   bool   `json:"confirm,omitempty"`
	Err       string `json:"err,omitempty"`
	StdoutLen int    `json:"stdout_len,omitempty"`
}

func appendTrace(logDir, convID string, rec traceRec) {
	if logDir == "" || rec.RequestID == "" {
		return
	}
	if strings.TrimSpace(convID) == "" {
		convID = "default" // same fallback as goal/slot files
	}
	dir := filepath.Join(logDir, "context_slots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "trace-"+convID+".jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(b, '\n'))
}

func newTraceRec(out Outcome, o *Options) traceRec {
	rec := traceRec{
		RequestID:   out.RequestID,
		TS:          time.Now().Format(time.RFC3339),
		ConvID:      o.ConvID,
		Intent:      out.View.Action,
		Action:      out.View.Action,
		Ask:         out.Ask,
		Attribution: out.Attribution.Class + "/" + out.Attribution.Detail,
		LoopMs:      out.LoopMs,
		NetMs:       out.NetMs,
	}
	for _, r := range out.Receipts {
		rec.Receipts = append(rec.Receipts, traceReceipt{
			Tool:      r.Tool,
			OK:        r.OK,
			Confirm:   r.ConfirmAsk,
			Err:       truncateStr(r.Err, 160),
			StdoutLen: len(r.Stdout),
		})
	}
	if g := loadGoal(o.logDir(), o.ConvID); g != nil {
		rec.GoalStatus = string(g.Status)
		rec.GoalQueue = g.Queue
	}
	return rec
}
