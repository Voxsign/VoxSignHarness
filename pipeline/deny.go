package pipeline

// R13 (2026-10-09): Governance — session-level deny-once, distilled from
// Agent Harness Engineering §9.2 human-in-the-loop hooks (recurrence policy:
// allow-once vs allow-always). When the user explicitly rejects a gated action
// ("算了不装了 / 不用了 / 别回了"), the harness records {kind:target} in
// context_slots/denied-<convID>.json and will NOT re-ask the same gate again in
// this session (no nagging loops, R11 fix extended). A revive phrase
// ("还是要 / 改成做 / 重新做 / 算了做吧") clears the deny so a genuine change
// of mind always stays possible — rejection is remembered, not deafening.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"voicesign-harness/contract"
)

type denyDB struct {
	Since map[string]string `json:"since"`
}

func denyPath(logDir, convID string) string {
	if strings.TrimSpace(convID) == "" {
		convID = "default" // same fallback as goal/slot files
	}
	return filepath.Join(logDir, "context_slots", "denied-"+convID+".json")
}

func loadDenies(logDir, convID string) map[string]string {
	m := map[string]string{}
	b, err := os.ReadFile(denyPath(logDir, convID))
	if err != nil {
		return m
	}
	var db denyDB
	if json.Unmarshal(b, &db) == nil && db.Since != nil {
		return db.Since
	}
	return m
}

func saveDenies(logDir, convID string, m map[string]string) {
	db := denyDB{Since: m}
	if b, err := json.MarshalIndent(db, "", "  "); err == nil {
		dir := filepath.Dir(denyPath(logDir, convID))
		if err := os.MkdirAll(dir, 0o755); err == nil {
			_ = os.WriteFile(denyPath(logDir, convID), b, 0o644)
		}
	}
}

func isDenied(logDir, convID, key string) bool {
	if key == "" {
		return false
	}
	_, ok := loadDenies(logDir, convID)[key]
	return ok
}

func recordDeny(logDir, convID, key string) {
	if key == "" {
		return
	}
	m := loadDenies(logDir, convID)
	m[key] = time.Now().Format(time.RFC3339)
	saveDenies(logDir, convID, m)
}

func clearDeny(logDir, convID, key string) {
	if key == "" {
		return
	}
	m := loadDenies(logDir, convID)
	if _, ok := m[key]; ok {
		delete(m, key)
		saveDenies(logDir, convID, m)
	}
}

var reviveTriggers = []string{"还是要", "还是装", "还是回", "还是发", "改成做", "重新做", "算了做吧", "要的", "还是要做"}

func isRevive(text string) bool {
	if text == "" {
		return false
	}
	low := strings.ToLower(text)
	for _, t := range reviveTriggers {
		if strings.Contains(low, strings.ToLower(t)) {
			return true
		}
	}
	return false
}

// denyKeyForIntent derives the session-level deny key for a gated intent.
// install:<packages> / email:reply:<target> (empty target = the bare "写回复" gate).
func denyKeyForIntent(it contract.Intent) string {
	switch it.Intent {
	case contract.IntentInstall:
		pkgs := strings.TrimSpace(it.Params["packages"])
		if pkgs == "" {
			pkgs = strings.TrimSpace(it.Params["pkg"])
		}
		return "install:" + pkgs
	case contract.IntentEmail:
		action := it.Params["action"]
		if action == "reply" || action == "forward" {
			return "email:" + action + ":" + it.Params["target"]
		}
	}
	return ""
}

func denyKeyFromSlot(s *TaskSlot) string {
	if s == nil || s.Params == nil {
		return ""
	}
	switch s.Kind {
	case TaskKindInstall:
		return "install:" + s.Params["packages"]
	case TaskKindEmail:
		action := s.Params["action"]
		if action == "reply" || action == "forward" {
			return "email:" + action + ":" + s.Params["target"]
		}
	}
	return ""
}
