package pipeline

// Goal is the cross-turn durable objective (R9-D1, distilled from DeepSeek
// packages/goal + goal-round-driver).
//
// TaskSlot records "what this turn is doing"; Goal records "what the whole
// objective is, how far it got, and what comes next" — the persistence layer
// for persistent execution. One goal per conversation, stored as a single
// overwrite JSON file under <logDir>/context_slots/goal-<convID>.json so it
// survives restarts and resumes across iOS turns.
//
// Status machine: active -> paused | blocked | completed.
//   - active:    work continues; each turn bumps Round.
//   - paused:    user explicitly paused; a stale pause is surfaced proactively.
//   - blocked:   same failure streak >= 3 with a reason and options (DeepSeek:
//                difficulty != blocker, consecutive-blocked rule).
//   - completed: only when evidence shows the whole objective is achieved
//                (premature "done" is the #1 agent failure mode).

import (
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"voicesign-harness/contract"
)

// GoalStatus values.
const (
	GoalActive    = "active"
	GoalPaused    = "paused"
	GoalBlocked   = "blocked"
	GoalCompleted = "completed"
	GoalCanceled  = "canceled" // R11 (2026-10-09): user said 取消/算了/别发了 — honest stop
)

// MaxGoalBlockStreak is how many consecutive failing turns before the goal is
// reported blocked (DeepSeek: only a repeated blocker is a real blocker).
const MaxGoalBlockStreak = 3

// GoalStaleMinutes: an active goal untouched for this long is surfaced as
// "上次做到哪，是否继续" (proactive nudge, R9-D5).
const GoalStaleMinutes = 30

type Goal struct {
	Objective     string   `json:"objective"`               // original user instruction, verbatim
	IntentKind    string   `json:"intent_kind,omitempty"`   // executable intent type for resume
	Status        string   `json:"status"`                  // active|paused|blocked|completed
	Round         int      `json:"round"`                   // continuation rounds executed
	Steps         []string `json:"steps,omitempty"`         // executed steps so far
	LastProgress  string   `json:"last_progress,omitempty"` // what the last turn achieved
	NextStep      string   `json:"next_step,omitempty"`     // single next action
	BlockedReason string   `json:"blocked_reason,omitempty"`
	Options       []string `json:"options,omitempty"` // alternatives when blocked
	FailStreak    int      `json:"fail_streak"`
	FirstRetryOK  bool     `json:"first_retry_ok"` // R9-D4: first-retry-success indicator
	Queue         []string `json:"queue,omitempty"` // R12: pending multi-task steps to run automatically after the current one
	// QueueIDs holds the stable step id for each Queue entry at the same index.
	// A step id is assigned once when the step is enqueued and never reused for a
	// different step, so recovery can correlate a landed side effect to exactly
	// one (goal, step, attempt) instead of matching raw note text (P0 round 4).
	QueueIDs    []string `json:"queue_ids,omitempty"`
	GoalID      string   `json:"goal_id,omitempty"` // stable objective id, set at creation
	StepCounter int      `json:"step_counter,omitempty"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`

	// Crash-safety for the queued-step window (P0 round 3/4, 2026-10-11). The
	// drain is write-ahead: the head step's StepLease is durably persisted BEFORE
	// its side effect runs, and cleared only when the step is dequeued (completion
	// persisted in the same write). A lease still present on load means the prior
	// process died while that step was in flight — after the side effect but
	// before the dequeue landed (or while it ran). Recovery reconciles the lease
	// via stable (GoalID, StepID, Attempt) evidence instead of replaying blindly.
	StepSeq   int        `json:"step_seq,omitempty"`
	StepLease *StepLease `json:"step_lease,omitempty"`
	// HeadAttempt is the attempt count for the step currently at the queue head;
	// it survives a lease clear so a redo/re-run gets a fresh attempt number and
	// cannot be confused with the prior attempt's evidence. Reset on pop.
	HeadAttempt int `json:"head_attempt,omitempty"`
	// LeaseDecision is the operator's explicit resolution of an externally
	// uncertain lease: "" pending; "redo_requested" (operator asked to re-send but
	// has not yet passed the explicit risk confirmation); "redo_confirmed"
	// (explicitly confirmed after the risk warning — one re-run is authorized);
	// "accept" (operator attests the action already landed out-of-band).
	LeaseDecision string `json:"lease_decision,omitempty"`
	// LeaseManualAccept records an explicit human attestation for an accept
	// decision (timestamp + wording); an accepted external step is never treated
	// as system-proven success, only as human-verified.
	LeaseManualAccept string `json:"lease_manual_accept,omitempty"`
}

// StepLease is the durable in-flight marker for one queued step (write-ahead log
// row). GoalID/StepID identify the exact step, Attempt the current execution
// attempt; Text is the queued instruction and Intent its classified kind (drives
// the recovery class).
type StepLease struct {
	GoalID    string `json:"goal_id"`
	StepID    string `json:"step_id"`
	Attempt   int    `json:"attempt"`
	Seq       int    `json:"seq"`
	Text      string `json:"text"`
	Intent    string `json:"intent"`
	StartedAt string `json:"started_at"`
}

// Recovery classes for a write-ahead step lease left behind by a crashed process.
const (
	recoveryLocalNote  = "local_note"         // local side effect, reconcilable from disk -> exactly-once
	recoveryIdempotent = "idempotent_rerun"   // PROVEN read-only: no state change, safe to run again
	recoveryExternal   = "external_uncertain" // outcome unknown / any side-effecting action: hold for an operator decision
)

// leaseRecoveryClass decides how a crashed in-flight step is reconciled on the
// next process. Fail-safe by default: only intents whose contract is PROVABLY
// read-only (no file writes, no process launch, no remote control, no external
// state change) may be re-run automatically, and only the locally-reconcilable
// NOTE/REMINDER steps get exactly-once treatment from their stable token.
//
// IntentUnknown is NOT auto-replayed (its effect is unknown by definition), and
// neither are Test/BuildTest/Debug/AppLaunch: "test"/"debug"/"launch" can build
// artifacts, mutate the workspace, or start a process, so the intent label alone
// cannot prove idempotence.
//
// QUERY/ASK are ALSO held: although usually conversational, the executor routes
// some QUERY/ASK utterances to the remote-desktop / environment-control tool
// (see matchVoiceContract in execActions), which can operate a remote machine.
// A crash lease records only the intent, not whether that specific invocation
// was a pure answer or a remote action, so read-only-ness cannot be proven for
// the crashed attempt and it must hang pending a decision. A future refinement
// may record the actually-dispatched tool's read-only-ness on the lease and
// re-run only those; until then, default to holding.
//
// These, together with email/shell/install/edit/deploy/etc., fall through to
// external_uncertain and are held for an explicit redo (two-turn risk gate) or
// human accept. A duplicate action is far worse than asking once.
func leaseRecoveryClass(intent string) string {
	switch intent {
	case contract.IntentNote, contract.IntentReminder:
		return recoveryLocalNote
	case contract.IntentInfo,
		contract.IntentFileRead, contract.IntentFileList, contract.IntentTime:
		// Provably read-only / no state change and no remote-control routing.
		return recoveryIdempotent
	default:
		// Includes IntentUnknown, Query, Ask, Test, BuildTest, Debug, AppLaunch
		// and every state-changing intent: hold, never auto-replay.
		return recoveryExternal
	}
}

// Side-effect correlation token (P0 round 4). A local NOTE/REMINDER step appends
// this marker on its own line so recovery can prove that EXACT (goal, step,
// attempt) landed. Raw note text is never used as proof: an identical note from
// an earlier legitimate task, a second identical step in the same goal, or an
// identical note in a different goal must not count as this step's evidence.
//
//	- 2026-10-11 09:00 记一下 同步日志  {vhs g=g1a2 s=g1a2.s3 a=1}
const vhsTokenOpen = "{vhs "
const vhsTokenClose = "}"

// attemptToken renders the correlation marker for one step attempt.
func attemptToken(goalID, stepID string, attempt int) string {
	return fmt.Sprintf("%sg=%s s=%s a=%d%s", vhsTokenOpen, goalID, stepID, attempt, vhsTokenClose)
}

// parseAttemptToken extracts (goal, step, attempt) from a line's marker, if any.
func parseAttemptToken(line string) (goalID, stepID string, attempt int, ok bool) {
	i := strings.Index(line, vhsTokenOpen)
	if i < 0 {
		return "", "", 0, false
	}
	rest := line[i+len(vhsTokenOpen):]
	if j := strings.Index(rest, vhsTokenClose); j >= 0 {
		rest = rest[:j]
	}
	for _, f := range strings.Fields(rest) {
		kv := strings.SplitN(f, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "g":
			goalID = kv[1]
		case "s":
			stepID = kv[1]
		case "a":
			attempt, _ = strconv.Atoi(kv[1])
		}
	}
	ok = goalID != "" && stepID != "" && attempt > 0
	return goalID, stepID, attempt, ok
}

// noteAttemptLanded proves one specific step attempt's local side effect is on
// disk by matching the exact (goal, step, attempt) marker. It never matches on
// note text, so pre-existing identical notes cannot be mistaken for this step.
func noteAttemptLanded(logDir, goalID, stepID string, attempt int) bool {
	b, err := os.ReadFile(filepath.Join(logDir, "notes.md"))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(b), "\n") {
		g, s, a, ok := parseAttemptToken(line)
		if ok && g == goalID && s == stepID && a == attempt {
			return true
		}
	}
	return false
}

// newGoalID returns a process-unique stable goal id (time + crypto random, stdlib
// only — the module has zero third-party dependencies).
func newGoalID() string {
	rb := make([]byte, 3)
	_, _ = crand.Read(rb)
	return "g" + strconv.FormatInt(time.Now().UnixNano(), 36) + hex.EncodeToString(rb)
}

// ensureGoalID assigns a stable GoalID once.
func (g *Goal) ensureGoalID() {
	if g.GoalID == "" {
		g.GoalID = newGoalID()
	}
}

// ensureQueueIDs fills a stable id for every queue entry, preserving any ids
// already present. Legacy goals persisted without ids get deterministic ids so
// the mapping stays stable across reloads.
func (g *Goal) ensureQueueIDs() {
	g.ensureGoalID()
	if len(g.QueueIDs) == len(g.Queue) {
		return
	}
	ids := make([]string, len(g.Queue))
	for i := range g.Queue {
		if i < len(g.QueueIDs) && g.QueueIDs[i] != "" {
			ids[i] = g.QueueIDs[i]
		} else {
			ids[i] = fmt.Sprintf("%s.legacy%d", g.GoalID, i)
		}
	}
	g.QueueIDs = ids
}

// headID returns the stable id of the queue head.
func (g *Goal) headID() string {
	g.ensureQueueIDs()
	if len(g.QueueIDs) == 0 {
		return ""
	}
	return g.QueueIDs[0]
}

// prependQueued assigns fresh stable ids and prepends steps ahead of the current
// queue (execSequence enqueues later steps before older residual work).
func (g *Goal) prependQueued(texts []string) {
	g.ensureGoalID()
	newTexts := make([]string, 0, len(texts))
	newIDs := make([]string, 0, len(texts))
	for _, t := range texts {
		g.StepCounter++
		newIDs = append(newIDs, fmt.Sprintf("%s.s%d", g.GoalID, g.StepCounter))
		newTexts = append(newTexts, t)
	}
	g.ensureQueueIDs()
	g.Queue = append(newTexts, g.Queue...)
	g.QueueIDs = append(newIDs, g.QueueIDs...)
}

// popHead removes the queue head and its stable id together.
func (g *Goal) popHead() {
	if len(g.Queue) == 0 {
		return
	}
	g.Queue = g.Queue[1:]
	if len(g.QueueIDs) > 0 {
		g.QueueIDs = g.QueueIDs[1:]
	}
}

func goalPath(logDir, convID string) string {
	convID = strings.TrimSpace(convID)
	if convID == "" {
		convID = "default"
	}
	return filepath.Join(logDir, "context_slots", "goal-"+convID+".json")
}

// recentResumableGoal (R13, 2026-10-09): long-task persistence across sessions —
// scans every goal-<convID>.json under context_slots and returns the most recently
// updated goal that is still Active or Paused. Lets "继续上次的任务" resume a task
// that was started in another conversation (or after a restart) instead of asking
// "当前没有待执行的任务". Returns nil when nothing is resumable.
func recentResumableGoal(logDir string) *Goal {
	dir := filepath.Join(logDir, "context_slots")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var best *Goal
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "goal-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		b, rerr := os.ReadFile(filepath.Join(dir, name))
		if rerr != nil {
			continue
		}
		var g Goal
		if json.Unmarshal(b, &g) != nil {
			// Corrupt goal file: skip it for cross-session resume but never delete or
			// overwrite it — it may still carry a crash lease an execution path needs.
			log.Printf("[recentResumableGoal] skipping unreadable/corrupt goal file %s (left intact for manual inspection)", name)
			continue
		}
		if g.Status != GoalActive && g.Status != GoalPaused {
			continue
		}
		if best == nil || g.UpdatedAt > best.UpdatedAt {
			cp := g
			best = &cp
		}
	}
	return best
}

// loadGoalChecked loads the durable goal and distinguishes "no goal yet"
// (returns nil, nil) from "goal file exists but is unreadable/corrupt"
// (returns nil, err). Execution and goal-creation paths MUST use this and fail
// closed on error: a truncated/half-written goal must never be silently treated
// as "no goal", which would let a fresh goal overwrite the file and drop the
// surviving step lease (the protection against replaying an uncertain action).
func loadGoalChecked(logDir, convID string) (*Goal, error) {
	p := goalPath(logDir, convID)
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var g Goal
	if uerr := json.Unmarshal(b, &g); uerr != nil {
		return nil, fmt.Errorf("goal state is corrupt at %s: %w (refusing to overwrite or execute; manual inspection required)", p, uerr)
	}
	return &g, nil
}

// loadGoal is a best-effort, READ-ONLY convenience loader for display/guard
// paths. It returns nil for both "missing" and "corrupt". Any path that creates
// a goal, runs a side effect, or reconciles a crash lease must use
// loadGoalChecked and handle the corrupt-state error instead.
func loadGoal(logDir, convID string) *Goal {
	g, _ := loadGoalChecked(logDir, convID)
	return g
}

func saveGoal(logDir, convID string, g *Goal) {
	_ = saveGoalChecked(logDir, convID, g)
}

// saveGoalChecked atomically persists the goal: it writes a temp file in the
// SAME directory, fsyncs it, then renames it over the target. The destination
// is therefore always either the previous complete JSON or the new complete
// JSON — never a truncated/half-written file (a crash or disk-full mid-write
// leaves only an orphaned temp file, which loaders ignore).
//
// Durability scope: the file fsync plus a best-effort directory fsync cover the
// ordinary process-crash and power-loss cases on a journaling, same-filesystem
// setup. They cannot guarantee media-level atomicity on a failing disk or a
// cross-device rename (which is avoided by creating the temp file in place).
//
// The write-ahead drain must not run a side effect unless the preceding lease
// write succeeded, and must not claim completion if the dequeue write fails
// (P0 round 4: a swallowed persistence error causes false done / duplicate
// actions after restart; round 5: a non-atomic overwrite could truncate the
// only surviving lease).
func saveGoalChecked(logDir, convID string, g *Goal) error {
	if g == nil {
		return nil
	}
	g.UpdatedAt = time.Now().Format(time.RFC3339)
	if g.CreatedAt == "" {
		g.CreatedAt = g.UpdatedAt
	}
	dir := filepath.Join(logDir, "context_slots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return err
	}
	final := goalPath(logDir, convID)
	tmp, err := os.CreateTemp(dir, ".goal-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpName) // no orphaned half-file on any error path
		}
	}()
	if _, werr := tmp.Write(b); werr != nil {
		_ = tmp.Close()
		return werr
	}
	if ferr := tmp.Sync(); ferr != nil { // flush file contents (power durability)
		_ = tmp.Close()
		return ferr
	}
	if cerr := tmp.Close(); cerr != nil {
		return cerr
	}
	if merr := os.Chmod(tmpName, 0o644); merr != nil {
		return merr
	}
	// TEST-ONLY seam: hard-exit after the temp file is written and synced but
	// BEFORE the atomic rename, reproducing a crash/power loss in the write
	// window. The previous complete target file must be left intact.
	if crashBeforeGoalRename != nil && crashBeforeGoalRename() {
		os.Exit(CrashExitCode)
	}
	// Atomic same-directory replacement: readers never observe a partial file.
	if rerr := os.Rename(tmpName, final); rerr != nil {
		return rerr
	}
	committed = true
	// NOTE: we deliberately do NOT scan the directory to remove other
	// ".goal-*.tmp" files here. Concurrent saves for OTHER conversations create
	// their own live temp files in this same directory; a global sweep could
	// delete a temp file that another writer is about to rename (or that is mid
	// write). Each call only ever removes its OWN temp file (the deferred
	// cleanup above); orphaned temps from a genuinely crashed writer are ignored
	// by every loader and must be reclaimed only by a process that can prove
	// exclusive ownership/lifecycle (e.g. a single-instance startup lock), never
	// during normal concurrent operation.
	//
	// Advisory only: fsync the directory so rename metadata is more likely to
	// survive power loss. This error is intentionally not fatal (the rename
	// already succeeded) and is ignored, so it is NOT a power-loss durability
	// guarantee. The atomicity promised here covers process crash / SIGKILL /
	// disk-full-mid-write at the VFS-rename level; physical power loss and
	// failing media are not guaranteed.
	if d, oerr := os.Open(dir); oerr == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// crashBeforeGoalRename is a TEST-ONLY fault seam (nil in production) that
// hard-exits between temp-file sync and atomic rename.
var crashBeforeGoalRename func() bool

// saveGoalStage persists the goal at a labeled point, honoring the TEST-ONLY
// FailGoalSave fault seam. stage is "lease" (pre-side-effect WAL), "commit"
// (post-success dequeue/clear), or "reconcile" (crash recovery bookkeeping).
func (o *Options) saveGoalStage(logDir, convID string, g *Goal, stage string) error {
	if o != nil && o.FailGoalSave != nil && o.FailGoalSave(stage) {
		return fmt.Errorf("injected goal persistence failure at stage %s", stage)
	}
	return saveGoalChecked(logDir, convID, g)
}

// maybeCreateGoal establishes a durable goal for real-execution intents so a
// later "继续/接着干" turn resumes the whole objective, not just the slot.
// A corrupt existing goal is a hard fail-closed: it returns the error WITHOUT
// creating or overwriting anything, so a surviving crash lease is never lost
// under a freshly-created goal.
func (o *Options) maybeCreateGoal(it contract.Intent) (*Goal, error) {
	convID := strings.TrimSpace(o.ConvID)
	if convID == "" {
		convID = "default"
	}
	g, gerr := loadGoalChecked(o.logDir(), convID)
	if gerr != nil {
		return nil, gerr
	}
	obj := strings.TrimSpace(it.CorrectedText)
	if obj == "" {
		obj = strings.TrimSpace(it.RawText)
	}
	if obj == "" {
		return g, nil
	}
	if g != nil && g.Status == GoalActive && g.Objective == obj {
		return g, nil // same objective, keep going
	}
	if g != nil && g.Status == GoalCompleted && g.Objective == obj {
		return g, nil
	}
	// New objective (or the old one was paused/blocked and the user re-asks):
	// fresh active goal with a stable GoalID.
	ng := &Goal{
		Objective:  obj,
		IntentKind: it.Intent,
		Status:     GoalActive,
		Round:      0,
	}
	ng.ensureGoalID()
	if serr := saveGoalChecked(o.logDir(), convID, ng); serr != nil {
		// The goal is not durable: do not let the caller start executing work that
		// could never be recovered after a restart.
		return nil, serr
	}
	return ng, nil
}

// syncGoalFromTurn updates the goal after one executed turn:
//   - success (receipt present, no FAILED marker): round++, progress, streak=0
//   - failure: streak++, and blocked once the streak reaches the limit, with a
//     reason and options (never silently dropping the objective)
//   - completed slot: mark the goal completed (evidence-based done)
func (o *Options) syncGoalFromTurn(slotStatus string, receipt string, failed bool, reason string) *Goal {
	convID := strings.TrimSpace(o.ConvID)
	if convID == "" {
		convID = "default"
	}
	g := loadGoal(o.logDir(), convID)
	if g == nil || g.Status != GoalActive {
		return g
	}
	if slotStatus == "done" && !failed {
		g.Status = GoalCompleted
		g.LastProgress = truncateStr(receipt, 200)
		g.NextStep = ""
		g.FailStreak = 0
		saveGoal(o.logDir(), convID, g)
		return g
	}
	if failed {
		g.FailStreak++
		if g.FailStreak >= MaxGoalBlockStreak {
			g.Status = GoalBlocked
			g.BlockedReason = reason
			g.Options = []string{"换一种方式重试", "说明更精确的目标", "暂停这个目标"}
		}
		g.LastProgress = truncateStr(receipt, 200)
		g.NextStep = ""
		saveGoal(o.logDir(), convID, g)
		return g
	}
	g.Round++
	g.FailStreak = 0
	g.LastProgress = truncateStr(receipt, 200)
	saveGoal(o.logDir(), convID, g)
	return g
}

// isExecutableIntent reports whether the intent performs real work worth a
// durable goal (conversational intents ASK/QUERY/NOTE/UNKNOWN do not).
func isExecutableIntent(it contract.Intent) bool {
	switch it.Intent {
	case contract.IntentBuildTest, contract.IntentInstall, contract.IntentEmail,
		contract.IntentEdit, contract.IntentDebug, contract.IntentTest,
		contract.IntentCommit, contract.IntentDeploy, contract.IntentShell,
		contract.IntentFileWrite, contract.IntentRegisterTool, contract.IntentOrchestrate,
		contract.IntentSelfImprove:
		return true
	}
	return false
}

// goalRoundText renders the continuation prompt for one goal round (DeepSeek
// goal-round-driver wording, Chinese): inspect current state, make concrete
// progress, verify, gather evidence before claiming completion.
func goalRoundText(g *Goal) string {
	if g == nil || g.Status != GoalActive {
		return ""
	}
	return "<goal_round>\n" +
		"目标：" + g.Objective + "\n" +
		"轮次：" + strconv.Itoa(g.Round+1) + "\n\n" +
		"继续朝着目标推进（同一会话内）。以当前实际状态（工具结果、任务槽、文件）为准，" +
		"不要假设之前的叙述仍然成立。做出实质进展并验证结果。宣称完成之前，" +
		"先收集证据证明整个目标已达成；若还有工作，保持目标激活进入下一轮。" +
		"\n</goal_round>"
}

// goalStale returns a proactive nudge when an active goal has been untouched
// for longer than GoalStaleMinutes (R9-D5).
func goalStale(g *Goal) string {
	if g == nil || g.Status != GoalActive || g.UpdatedAt == "" {
		return ""
	}
	u, err := time.Parse(time.RFC3339, g.UpdatedAt)
	if err != nil {
		return ""
	}
	if time.Since(u) > GoalStaleMinutes*time.Minute {
		return "上次任务停在：" + g.Objective + "（进展：" + g.LastProgress + "）。要继续吗？"
	}
	return ""
}
