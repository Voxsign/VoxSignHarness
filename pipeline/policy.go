package pipeline

// R13 (2026-10-09): Governance — declarative constitution (distilled from
// Agent Harness Engineering §9.4 AutoHarness YAML constitution + Kim risk
// taxonomy). Each gated tool declares {risk, confirm}:
//   confirm=always -> confirmation gate every time (high risk)
//   confirm=once   -> confirmation gate once per session+target, then allowed
//                     (medium risk; allow-once recurrence policy, H4)
//   confirm=never  -> no gate (low risk)
// The default constitution is embedded; an optional <logDir>/constitution.json
// overrides it (declarative policy, auditable, no code change required).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type toolPolicy struct {
	Risk    string `json:"risk"`
	Confirm string `json:"confirm"` // never | once | always
}

type constitution struct {
	Version string                `json:"version"`
	Tools   map[string]toolPolicy `json:"tools"`
}

func defaultConstitution() *constitution {
	return &constitution{
		Version: "1",
		Tools: map[string]toolPolicy{
			"install":       {Risk: "high", Confirm: "always"},
			"email:reply":   {Risk: "medium", Confirm: "once"},
			"email:forward": {Risk: "medium", Confirm: "once"},
		},
	}
}

func constitutionPath(logDir string) string {
	return filepath.Join(logDir, "constitution.json")
}

func loadConstitution(logDir string) *constitution {
	c := defaultConstitution()
	b, err := os.ReadFile(constitutionPath(logDir))
	if err != nil {
		return c
	}
	var over constitution
	if json.Unmarshal(b, &over) == nil && over.Tools != nil {
		// merge: overrides replace per-tool entries, defaults fill the rest.
		for k, v := range over.Tools {
			c.Tools[k] = v
		}
	}
	return c
}

// confirmMode returns the confirmation mode for a tool key ("install",
// "email:reply", "email:forward"). Unknown keys default to "always" — safe by
// default: new gated actions never silently drop a confirmation.
func (c *constitution) confirmMode(toolKey string) string {
	if c == nil || c.Tools == nil {
		return "always"
	}
	if p, ok := c.Tools[toolKey]; ok && p.Confirm != "" {
		return p.Confirm
	}
	return "always"
}

// --- session-level allow-once memory (mirror of deny.go) ---

type allowDB struct {
	Since map[string]string `json:"since"`
}

func allowedPath(logDir, convID string) string {
	if strings.TrimSpace(convID) == "" {
		convID = "default"
	}
	return filepath.Join(logDir, "context_slots", "allowed-"+convID+".json")
}

func loadAllows(logDir, convID string) map[string]string {
	m := map[string]string{}
	b, err := os.ReadFile(allowedPath(logDir, convID))
	if err != nil {
		return m
	}
	var db allowDB
	if json.Unmarshal(b, &db) == nil && db.Since != nil {
		return db.Since
	}
	return m
}

func saveAllows(logDir, convID string, m map[string]string) {
	db := allowDB{Since: m}
	if b, err := json.MarshalIndent(db, "", "  "); err == nil {
		dir := filepath.Dir(allowedPath(logDir, convID))
		if err := os.MkdirAll(dir, 0o755); err == nil {
			_ = os.WriteFile(allowedPath(logDir, convID), b, 0o644)
		}
	}
}

func isAllowed(logDir, convID, key string) bool {
	if key == "" {
		return false
	}
	_, ok := loadAllows(logDir, convID)[key]
	return ok
}

func recordAllow(logDir, convID, key string) {
	if key == "" {
		return
	}
	m := loadAllows(logDir, convID)
	m[key] = time.Now().Format(time.RFC3339)
	saveAllows(logDir, convID, m)
}
