package pipeline

// TaskSlot is the cross-turn task memory (distillation R5, 2026-10-08).
//
// The iOS repro showed the harness answering "download the code, then build,
// then test" with an ASK loop: it had no memory of the pending job from one
// turn to the next, so "开始干呀/立刻执行" had nothing to resume. TaskSlot
// stores the actionable job (kind, url, steps, current step, status) under
// <logDir>/context_slots/task-<convID>.jsonl so a CONTINUE turn can pick the
// job back up and actually execute it.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// TaskSlotKind enumerates the job types the harness can really execute.
const (
	TaskKindBuildTest = "build_test" // clone -> build -> test
	TaskKindBackup    = "backup"     // run the backup task (informational/resume)
	TaskKindInstall   = "install"    // distillation R6: npm install -g <pkgs> (confirm-gated)
	TaskKindEmail     = "email"      // distillation R7: fetch AIOps inbox -> Strata analysis -> receipt (confirm-gated for reply/forward)
)

type TaskSlot struct {
	Kind    string            `json:"kind"`
	Params  map[string]string `json:"params,omitempty"`
	Steps   []string          `json:"steps,omitempty"`
	Step    int               `json:"step"`
	Status  string            `json:"status"` // pending | running | done | failed
	Text    string            `json:"text"`   // original user instruction
	Ts      string            `json:"ts"`
	ConvID  string            `json:"conv_id,omitempty"`
}

func taskSlotPath(logDir, convID string) string {
	convID = strings.TrimSpace(convID)
	if convID == "" {
		convID = "default"
	}
	return filepath.Join(logDir, "context_slots", "task-"+convID+".jsonl")
}

func writeTaskSlot(logDir, convID string, slot *TaskSlot) {
	if slot == nil {
		return
	}
	slot.Ts = time.Now().Format(time.RFC3339)
	slot.ConvID = convID
	dir := filepath.Join(logDir, "context_slots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	b, _ := json.Marshal(slot)
	// append-only line so the last line is the newest slot
	f, err := os.OpenFile(taskSlotPath(logDir, convID), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(append(b, '\n'))
}

// loadTaskSlot returns the newest task slot for the conversation, or nil.
// A done slot is returned too so a CONTINUE turn can tell the user the job is
// already finished instead of pretending there is no task.
func loadTaskSlot(logDir, convID string) *TaskSlot {
	b, err := os.ReadFile(taskSlotPath(logDir, convID))
	if err != nil {
		return nil
	}
	var last *TaskSlot
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var s TaskSlot
		if json.Unmarshal([]byte(line), &s) == nil {
			last = &s
		}
	}
	if last == nil || last.Kind == "clear" {
		return nil
	}
	return last
}

func clearTaskSlot(logDir, convID string) {
	writeTaskSlot(logDir, convID, &TaskSlot{Kind: "clear", Status: "done", Text: ""})
}
