package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"voicesign-harness/contract"
	"voicesign-harness/provider"
)

// R15/v0.6.0 (2026-10-09): SELF_IMPROVE — the channel that lets the harness run an
// open-ended long task AUTONOMOUSLY: research a reference (e.g. ~/.codex / ~/.claude),
// distil improvements, edit its OWN backend source, and verify with go build/go test.
//
// Before R15 an open task like "研究 Claude Code 怎么执行任务然后改进你自己的后端，
// 步骤 1)…2)…" was shredded by the multi-task splitter into a SEQUENCE queue and the
// first fragment landed on a placeholder "action pending model tool loop" receipt —
// nothing ever ran. execSelfImprove is the real model tool loop (ReAct): the local
// Qwen model picks one tool action per turn, the harness executes it and feeds the
// observation back, until the model declares done or the step budget is exhausted.
//
// Safety: every write is confined to the harness's own repo (VHS_SELF_REPO, default
// cwd); front-end/iOS file types are refused; dangerous shell commands and git push
// are denied; the build/test gate runs with LLM keys unset (CI-like, see R14).

const (
	selfImproveMaxSteps  = 14 // model tool-loop turns
	selfImproveMaxFix    = 2  // extra repair turns after a failed build/test gate
	selfImproveObsCap    = 1800
	selfImproveBuildSecs = 180
)

// selfImproveRepo resolves the working copy the loop is allowed to edit.
func selfImproveRepo() string {
	repo := strings.TrimSpace(os.Getenv("VHS_SELF_REPO"))
	if repo == "" {
		repo = "."
	}
	abs, err := filepath.Abs(repo)
	if err != nil {
		return repo
	}
	return abs
}

// wantsCodeChange reports whether the objective explicitly requires editing backend
// source (research-only tasks may legitimately finish without touching Go files).
func wantsCodeChange(objective string) bool {
	for _, kw := range []string{
		"改进", "修改", "改你", "改自", "优化你", "升级你", "重构", "实现", "修复",
		"改代码", "改后端", "改 pipeline", "改pipeline", "落地",
		"improve", "modify", "implement", "refactor", "fix ", "edit your", "change your",
	} {
		if strings.Contains(strings.ToLower(objective), strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

// reactAction is one model-chosen tool call.
type reactAction struct {
	Thought string          `json:"thought"`
	Done    bool            `json:"done"`
	Summary string          `json:"summary"`
	Tool    string          `json:"tool"`
	Command []string        `json:"command"`
	Path    string          `json:"path"`
	Content string          `json:"content"`
	Old     string          `json:"old"`
	New     string          `json:"new"`
	Changed []string        `json:"changed"`
	Steps   []reactPlanStep `json:"steps"`
}

func parseReactAction(raw string) (reactAction, error) {
	var a reactAction
	var parseErr error
	s := normalizeModelJSON(stripCodeFence(raw))
	for _, obj := range extractJSONObjects(s) {
		var candidate reactAction
		if err := json.Unmarshal([]byte(obj), &candidate); err != nil {
			parseErr = err
			continue
		}
		if candidate.Tool == "plan" {
			if err := (&reactThread{}).updatePlan(candidate.Steps); err != nil {
				parseErr = err
				continue
			}
		}
		return candidate, nil
	}
	if strings.Contains(s, "{") && len(s) > 900 {
		return a, fmt.Errorf("动作 JSON 不完整或过大（可能在输出整个文件时被截断）。请改用 replace 做小步修改，单次 old/new 各不超过 40 行，不要一次性输出整个大文件")
	}
	if parseErr != nil {
		return a, fmt.Errorf("invalid action JSON: %w", parseErr)
	}
	return a, fmt.Errorf("no JSON object in model output")
}

// normalizeModelJSON repairs common small-model quirks: Chinese full-width quotes,
// smart quotes, zero-width chars; the model is still asked to emit strict JSON.
func normalizeModelJSON(s string) string {
	rep := map[rune]rune{
		'“': '"', '”': '"', '‘': '\'', '’': '\'',
		'｛': '{', '｝': '}', '［': '[', '］': ']', '：': ':', '，': ',',
	}
	var b strings.Builder
	for _, r := range s {
		if nr, ok := rep[r]; ok {
			b.WriteRune(nr)
			continue
		}
		if r == '\u200b' || r == '\ufeff' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// extractJSONObjects returns every balanced top-level {...} candidate, honoring
// string literals and escape sequences so nested braces do not split an object.
func extractJSONObjects(s string) []string {
	var out []string
	for i := 0; i < len(s); i++ {
		if s[i] != '{' {
			continue
		}
		depth := 0
		inStr := false
		var esc bool
		for j := i; j < len(s); j++ {
			c := s[j]
			if inStr {
				if esc {
					esc = false
				} else if c == '\\' {
					esc = true
				} else if c == '"' {
					inStr = false
				}
				continue
			}
			switch c {
			case '"':
				inStr = true
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					out = append(out, s[i:j+1])
					i = j
					j = len(s)
				}
			}
		}
	}
	return out
}

func selfImproveSystemPrompt(repo, objective string) string {
	return "你是 VoxSignHarness 的后端本体，正在自主执行一个「研究 → 蒸馏 → 自我改进」的长任务。\n" +
		"你的源码仓库（唯一允许修改的目录，也是所有命令的工作目录）：" + repo + "\n" +

		"每一轮你只能输出一个最小 JSON 动作（不要 Markdown、不要解释、不要多个动作、不要 thought 字段）：\n" +
		"- 运行命令研究（command 为字符串数组，工作目录锁定仓库；读仓库外资料用绝对路径或 sh -c，例如 [\"sh\",\"-c\",\"ls -la $HOME/.codex\"]）：\n" +
		"  {\"tool\":\"run\",\"command\":[\"sh\",\"-c\",\"grep -rn SELF_IMPROVE --include=*.go . | head -30\"]}\n" +
		"- 读文件：{\"tool\":\"read\",\"path\":\"pipeline/pipeline.go\"}\n" +
		"- 精准修改（首选，old 必须与文件内容逐字一致，先用 read 取得原文）：\n" +
		"  {\"tool\":\"replace\",\"path\":\"pipeline/x.go\",\"old\":\"原文片段\",\"new\":\"新片段\"}\n" +
		"- 新建文件：{\"tool\":\"write\",\"path\":\"pipeline/new.go\",\"content\":\"完整文件内容\"}\n" +
		"- 设置/更新持久计划（首轮先 plan；每次提交完整有序 steps，状态 pending/in_progress/completed，最多一个 in_progress）：{\"tool\":\"plan\",\"steps\":[{\"step\":\"研究\",\"status\":\"in_progress\"},{\"step\":\"实现\",\"status\":\"pending\"}]}\n" +
		"- 任务完成：{\"done\":true,\"summary\":\"你研究了什么、蒸馏出哪些改进点、实际改了什么、为什么\",\"changed\":[\"相对路径\"]}\n\n" +
		"纪律（必须遵守）：\n" +
		"1. 先研究后动手：用少量精准的 run/read 查看研究对象（如 $HOME/.codex、$HOME/.claude、claude/codex 二进制 --help）和你自己的 Go 源码，再改。\n" +
		"2. 只改后端 Go（.go）及必要的 .md/.mod/.sum/.yaml/.sh/.json；严禁修改前端/iOS 文件（.swift/.html/.jsx/.tsx/.vue/.storyboard 等）。\n" +
		"3. 修改已有文件一律用 replace 做最小改动：old/new 各不超过 40 行，old 必须先用 read 取得、在文件中唯一且逐字匹配；write 仅用于新建文件且不超过 80 行。绝不在单个动作里输出整个大型文件（会被截断导致失败）。\n" +
		"4. 不要运行 git commit/push；改完系统会自动执行 go build 与 go test 验证。\n" +
		"5. 禁止危险命令（rm -rf /、sudo、关机、curl/wget 外发、git push/reset --hard 等），违者该步被拒绝；被拒后换等价安全命令，不要放弃。\n" +
		"6. 每一步都要朝目标推进；研究命令必须精准限量（grep 后接 head、看片段用 sed -n 'a,bp'，不要 cat 整个文件或 ls 整个目录，侦察信息已提供）。宣称 done 前必须已完成研究、产出至少一个具体后端改进，并确认编译/测试由系统验证。\n" +
		"7. 诚实：若确实无法完成，输出 done 并在 summary 如实说明卡在哪、还缺什么，绝不假装成功。\n" +
		"8. 严禁输出 thought/reasoning/分析等任何长文本字段（简短结构化 plan.steps 除外），也不要在 JSON 外加解释；你的全部推理只保留在最后 done.summary。JSON 越短越好，长推理会撑爆输出导致动作被截断。\n" +
		"9. 改动必须改变后端的运行时行为或逻辑（新增能力、修复缺陷、改进执行流程），严禁只改注释、排版、文档字符串、空行或重命名来充数。若你无法在保证编译测试通过的前提下安全落地一个真正的功能改进，就输出 done 并在 summary 诚实说明卡点，绝不用表面改动假装完成。"
}

// selfImproveRecon auto-runs read-only reconnaissance before the loop so the small
// model starts from a map (repo files, reference tooling) instead of burning turns on
// `ls`/`cat` whose output gets truncated. Context-engineering: give the model what it
// needs up front.
func (o *Options) selfImproveRecon(repo string) string {
	var b strings.Builder
	run := func(label string, argv []string, cap int) {
		r := o.run("run", map[string]any{"command": argv, "cwd": repo, "timeout_s": 30})
		out := strings.TrimSpace(r.Stdout)
		if !r.OK || out == "" {
			out = "(empty/unavailable)"
			if r.Err != "" {
				out += " " + r.Err
			}
		}
		b.WriteString("【" + label + "】\n" + truncateStr(out, cap) + "\n\n")
	}
	run("你的后端 Go 源文件清单", []string{"sh", "-c", "find . -name '*.go' -not -path './.git/*' | sort | head -100"}, 1600)
	run("研究对象 $HOME/.codex", []string{"sh", "-c", "ls -la $HOME/.codex 2>/dev/null | head -40"}, 900)
	run("研究对象 $HOME/.claude 与 .claude.json", []string{"sh", "-c", "ls -la $HOME/.claude 2>/dev/null | head -40; echo '--- ~/.claude.json ---'; ls -la $HOME/.claude.json 2>/dev/null"}, 1100)
	run("claude / codex 可执行与版本", []string{"sh", "-c",
		"ls \"$HOME/Library/Application Support/DoubaoWork/sandbox_runtime/bases/\"*/bin/claude \"$HOME/Library/Application Support/DoubaoWork/sandbox_runtime/bases/\"*/bin/codex 2>/dev/null; " +
			"(command -v claude >/dev/null && claude --version 2>/dev/null); (command -v codex >/dev/null && codex --version 2>/dev/null)"}, 600)
	return b.String()
}

// execSelfImprove runs the autonomous ReAct loop and then the build/test gate.
func (o *Options) execSelfImprove(ctx context.Context, it contract.Intent, logDir string) []contract.Receipt {
	repo := selfImproveRepo()
	var recv []contract.Receipt
	seq := 0
	add := func(tool string, ok bool, out, errStr string) {
		seq++
		r := contract.Receipt{Seq: seq, Tool: tool, OK: ok, Stdout: truncateStr(out, selfImproveObsCap)}
		if errStr != "" {
			r.Err = truncateStr(errStr, selfImproveObsCap)
		}
		recv = append(recv, r)
	}

	if _, err := os.Stat(filepath.Join(repo, "go.mod")); err != nil {
		add("selfimprove", false, "", "自改目标不是 Go 仓库（"+repo+" 下找不到 go.mod）。请设置 VHS_SELF_REPO 指向 VoxSignHarness 源码根目录。")
		return recv
	}

	objective := it.CorrectedText
	if it.Params != nil && strings.TrimSpace(it.Params["objective"]) != "" {
		objective = strings.TrimSpace(it.Params["objective"])
	}
	objective = truncateStr(objective, 2000)
	thread := &reactThread{
		goal:   objective,
		system: contract.Message{Role: "system", Content: selfImproveSystemPrompt(repo, objective)},
		recon: contract.Message{Role: "user", Content: "系统预先收集的侦察信息如下（无需再重复 ls 整个目录或 cat 整个大文件；定位代码用 " +
			"`grep -rn \"词\" --include=*.go . | head -30`、看片段用 `sed -n '100,160p' 文件`）：\n\n" +
			o.selfImproveRecon(repo) +
			"\n基于以上地图开始：先做少量精准研究，然后用 replace 小步在后端 .go 落地一个具体改进。现在只输出第一个 JSON 动作。"},
	}
	add("selfimprove.recon", true, "已自动收集仓库地图与研究对象清单（上下文工程：开局即给全局视图）", "")

	touchedGo := map[string]bool{}
	codeSnap := map[string]string{} // abs path -> original Go source (lazy, before first edit)
	badTurns := 0
	earlyDoneRejects := 0
	planDoneRejects := 0
	requiresEdit := wantsCodeChange(objective)
	finalSummary := ""
	completed := false

	for step := 1; step <= selfImproveMaxSteps; step++ {
		resp, err := o.chatWithFallback(ctx, []string{"fast"}, provider.ChatRequest{
			Messages:       thread.messages(),
			MaxTokens:      8000,
			ResponseFormat: noJSON(),
		})
		if err != nil {
			add("selfimprove.llm", false, "", "模型调用失败: "+err.Error())
			break
		}
		o.turnTokens += int64(resp.Usage.TotalTokens)
		raw := resp.Content

		act, perr := parseReactAction(raw)
		if perr != nil {
			badTurns++
			obs := "无法解析你的动作 JSON（" + perr.Error() + "）。请只输出一个最小 JSON 对象，例如 {\"tool\":\"run\",\"command\":[\"sh\",\"-c\",\"grep -rn X --include=*.go . | head\"]}；不要输出解释或整个文件。"
			add("selfimprove.parse", false, truncateStr(raw, 400), obs+" ||RAW|| "+singleLinePreview(raw, 260))
			thread.push(
				contract.Message{Role: "assistant", Content: raw},
				contract.Message{Role: "user", Content: "OBSERVATION: " + obs},
				fmt.Sprintf("#%d 动作无法解析（已要求重发最小 JSON）", step))
			if badTurns >= 3 {
				break
			}
			continue
		}
		badTurns = 0

		if act.Done {
			if err := thread.planCompletionError(); err != nil {
				add("selfimprove.done_guard", false, "", err.Error())
				planDoneRejects++
				if planDoneRejects >= 2 {
					break
				}
				thread.push(contract.Message{Role: "assistant", Content: raw},
					contract.Message{Role: "user", Content: "OBSERVATION: " + err.Error()},
					fmt.Sprintf("#%d done rejected: unfinished plan", step))
				continue
			}
			// Anti-premature-completion: a code-change task is not done until at least
			// one backend Go file was actually edited. Small models tend to give up after
			// a single rejection/parse error and declare done with zero changes.
			if requiresEdit && len(touchedGo) == 0 && earlyDoneRejects < 2 {
				earlyDoneRejects++
				obs := "你还没有修改任何后端 Go 代码，目标（先研究、再蒸馏、并在后端落地至少一个具体改进）尚未达成，不能 done。" +
					"请继续：用 run/read 完成精准研究，然后用 replace（首选，old 逐字一致）或 write 在 .go 文件里实现一个具体改进。" +
					"遇到命令被拒就换一条等价的安全命令，不要放弃。"
				add("selfimprove.done_guard", false, truncateStr(act.Summary, 300), obs)
				thread.push(
					contract.Message{Role: "assistant", Content: raw},
					contract.Message{Role: "user", Content: "OBSERVATION: " + obs},
					fmt.Sprintf("#%d 过早 done 被驳回（零代码改动）", step))
				continue
			}
			finalSummary = strings.TrimSpace(act.Summary)
			completed = true
			add("selfimprove.done", true, fmt.Sprintf("模型宣布完成（第 %d 步）：%s", step, finalSummary), "")
			break
		}

		obs, ok, touched := o.executeThreadAction(repo, logDir, act, codeSnap, thread)
		for _, f := range touched {
			touchedGo[f] = true
		}
		toolName := "selfimprove." + strings.TrimSpace(act.Tool)
		if toolName == "selfimprove." {
			toolName = "selfimprove.step"
		}
		add(toolName, ok, obs.Obs, obs.Err)
		thread.push(
			contract.Message{Role: "assistant", Content: raw},
			contract.Message{Role: "user", Content: "OBSERVATION:\n" + obs.Obs +
				" ；ok=" + fmt.Sprintf("%v", ok) +
				"\n继续下一步（只输出一个 JSON 动作；若已全部完成输出 done）。"},
			stepSummary(step, act, ok))
	}

	// Build/test gate — but only when Go source was actually touched.
	var changed []string
	for f := range touchedGo {
		changed = append(changed, f)
	}
	gate := o.selfImproveBuildTestGate(ctx, repo, logDir, changed, requiresEdit, codeSnap, thread, &recv, &seq)

	if !completed {
		gate.passed = false
		gate.detail += "; 主循环未宣布完成（异常退出或预算耗尽）"
	}
	if !gate.passed {
		// honest failure: do not claim success even if the model said done.
		recv = append(recv, contract.Receipt{Seq: seq + 1, Tool: "selfimprove", OK: false,
			Err: "自主任务最终判定失败：" + gate.detail +
				"（模型完成标记=" + fmt.Sprintf("%v", completed) + "，改动文件=" + strings.Join(changed, ", ") + "）"})
		return recv
	}

	summary := finalSummary
	if summary == "" {
		summary = "模型未提供总结。"
	}
	recv = append(recv, contract.Receipt{Seq: seq + 1, Tool: "selfimprove", OK: true,
		Stdout: fmt.Sprintf(
			"自主长任务结束。步骤=%d 改动Go文件=%d（%s）；验证结果：%s。\n模型总结：%s",
			seq, len(changed), strings.Join(changed, ", "), gate.detail, summary)})
	return recv
}

// --- sliding-window ReAct conversation (context engineering for small models) ---

const selfImproveWindowK = 5 // keep last K action/observation pairs verbatim

type reactStep struct {
	asst    contract.Message
	user    contract.Message
	summary string
}

type reactThread struct {
	goal      string
	plan      []reactPlanStep
	system    contract.Message
	recon     contract.Message
	summaries []string
	recent    []reactStep
}

// push records one (model action, observation) pair; pairs older than the window are
// collapsed into one-line summaries so the prompt never grows past the small model's
// effective attention span.
func (t *reactThread) push(asst, user contract.Message, summary string) {
	t.recent = append(t.recent, reactStep{asst: asst, user: user, summary: summary})
	if len(t.recent) > selfImproveWindowK {
		old := t.recent[0]
		t.recent = t.recent[1:]
		t.summaries = append(t.summaries, old.summary)
	}
}

func (t *reactThread) messages() []contract.Message {
	recon := t.recon
	if t.goal != "" || len(t.plan) > 0 {
		recon.Content += "\n\n" + t.renderPlan()
	}
	if len(t.summaries) > 0 {
		recon.Content += "\n此前已完成步骤：\n" + strings.Join(t.summaries, "\n")
	}
	m := []contract.Message{t.system, recon}
	for _, step := range t.recent {
		m = append(m, step.asst, step.user)
	}
	return m
}

func stepSummary(n int, a reactAction, ok bool) string {
	var detail string
	switch strings.TrimSpace(a.Tool) {
	case "run":
		detail = "run " + strings.Join(a.Command, " ")
	case "read":
		detail = "read " + a.Path
	case "write":
		detail = "write " + a.Path
	case "replace":
		detail = "replace " + a.Path
	default:
		detail = a.Tool
	}
	return fmt.Sprintf("#%d %s → ok=%v（%s）", n, strings.TrimSpace(a.Tool), ok, truncateStr(detail, 80))
}

// reactObs is a normalized tool observation handed back to the model.
type reactObs struct {
	Obs string
	Err string
}

// executeReactAction performs one model-chosen action confined to the repo.
// snap lazily captures the original Go source of each file before its first edit,
// so the gate can later reject comment-only/whitespace-only "cosmetic" changes.
func (o *Options) executeReactAction(repo, logDir string, a reactAction, snap map[string]string) (reactObs, bool, []string) {
	var touched []string
	snapshotGo := func(p string) {
		if snap == nil {
			return
		}
		if _, seen := snap[p]; seen {
			return
		}
		if b, err := os.ReadFile(p); err == nil {
			snap[p] = string(b)
		} else {
			snap[p] = "" // new file
		}
	}
	switch strings.TrimSpace(a.Tool) {
	case "run":
		if len(a.Command) == 0 {
			return reactObs{Err: "run 缺少 command 数组"}, false, nil
		}
		if bad := dangerousCommand(a.Command); bad != "" {
			return reactObs{Err: bad}, false, nil
		}
		r := o.run("run", map[string]any{
			"command":   a.Command,
			"cwd":       repo,
			"timeout_s": 120,
		})
		out := strings.TrimSpace(r.Stdout)
		if r.Stderr != "" {
			out = out + "\n[stderr] " + strings.TrimSpace(r.Stderr)
		}
		if !r.OK && r.Err != "" {
			out = out + "\n[error] " + r.Err
		}
		return reactObs{Obs: out, Err: gateErr(r.OK, r.Err)}, r.OK, nil
	case "read":
		p, ok := withinRepo(repo, a.Path)
		if !ok {
			return reactObs{Err: "路径越界或非法（只允许读仓库内）: " + a.Path}, false, nil
		}
		r := o.run("file", map[string]any{"action": "read", "path": p})
		return reactObs{Obs: r.Stdout, Err: gateErr(r.OK, r.Err)}, r.OK, nil
	case "write":
		p, ok := withinRepo(repo, a.Path)
		if !ok {
			return reactObs{Err: "路径越界或非法（只允许写仓库内）: " + a.Path}, false, nil
		}
		if err := editSafetyError(p); err != "" {
			return reactObs{Err: err}, false, nil
		}
		if strings.HasSuffix(p, ".go") {
			snapshotGo(p)
		}
		r := o.run("file", map[string]any{
			"action": "write", "path": p, "content": a.Content, "log_dir": logDir,
		})
		if r.OK && strings.HasSuffix(p, ".go") {
			touched = append(touched, relPath(repo, p))
		}
		return reactObs{Obs: r.Stdout, Err: gateErr(r.OK, r.Err)}, r.OK, touched
	case "replace":
		p, ok := withinRepo(repo, a.Path)
		if !ok {
			return reactObs{Err: "路径越界或非法（只允许改仓库内）: " + a.Path}, false, nil
		}
		if err := editSafetyError(p); err != "" {
			return reactObs{Err: err}, false, nil
		}
		if strings.TrimSpace(a.Old) == "" {
			return reactObs{Err: "replace 缺少 old（必须提供与文件逐字一致的原文片段）"}, false, nil
		}
		if strings.HasSuffix(p, ".go") {
			snapshotGo(p)
		}
		r := o.run("file", map[string]any{
			"action": "replace", "path": p, "old": a.Old, "new": a.New, "log_dir": logDir,
		})
		if r.OK && strings.HasSuffix(p, ".go") && !strings.Contains(r.Stdout, "no occurrence") {
			touched = append(touched, relPath(repo, p))
		}
		if strings.Contains(r.Stdout, "no occurrence") {
			return reactObs{Obs: r.Stdout, Err: "old 片段未在文件中命中，请先 read 取得逐字原文再 replace"}, false, touched
		}
		return reactObs{Obs: r.Stdout, Err: gateErr(r.OK, r.Err)}, r.OK, touched
	default:
		return reactObs{Err: "未知 tool: " + a.Tool + "（允许 plan/run/read/replace/write/done）"}, false, nil
	}
}

func gateErr(ok bool, errStr string) string {
	if !ok && errStr != "" {
		return errStr
	}
	return ""
}

func relPath(repo, abs string) string {
	if rel, err := filepath.Rel(repo, abs); err == nil {
		return rel
	}
	return abs
}

type gateResult struct {
	passed bool
	detail string
}

// selfImproveBuildTestGate verifies edits in three stages — anti-cosmetic (real Go
// logic changed), go build, go test (CI shape) — feeding failures back for repair.
func (o *Options) selfImproveBuildTestGate(
	ctx context.Context, repo, logDir string, changed []string, requiresEdit bool,
	snap map[string]string, thread *reactThread, recv *[]contract.Receipt, seq *int,
) gateResult {
	add := func(tool string, ok bool, out, errStr string) {
		*seq++
		r := contract.Receipt{Seq: *seq, Tool: tool, OK: ok, Stdout: truncateStr(out, selfImproveObsCap)}
		if errStr != "" {
			r.Err = truncateStr(errStr, selfImproveObsCap)
		}
		*recv = append(*recv, r)
	}

	finish := func(verified bool, detail string) gateResult {
		if err := thread.planCompletionError(); err != nil {
			add("selfimprove.plan_gate", false, "", err.Error())
			return gateResult{detail: detail + "; plan: " + err.Error()}
		}
		return gateResult{passed: verified, detail: detail}
	}

	if len(changed) == 0 {
		// A research-only task legitimately finishes without code. But a code-change
		// objective ending with zero Go edits is an honest failure (premature give-up).
		if requiresEdit {
			return gateResult{passed: false, detail: "模型结束但未改动任何 Go 后端代码，自改目标未达成（过早放弃/研究未落地）"}
		}
		return finish(true, "无 Go 改动；未执行 build/test")
	}

	evaluate := func() (bool, string) {
		cosmeticDetail := ""
		if cosmeticOnly(repo, changed, snap) {
			cosmeticDetail = "表面改动：你修改的 .go 文件在剥离注释、空白、文档字符串后与原文完全相同，没有改变任何 Go 逻辑。" +
				"请落地一个真正改变运行时行为的后端改进，例如：在 pipeline/selfimprove.go 的 reactAction 新增一个工具字段、在 executeReactAction 接上对应处理分支并实现其逻辑，或新增一条校验/处理规则；严禁只改注释或文案。"
		}
		br := o.run("run", map[string]any{
			"command":   []string{"go", "build", "./..."},
			"cwd":       repo,
			"timeout_s": selfImproveBuildSecs,
			"env":       cleanGateEnv(),
		})
		add("selfimprove.build", br.OK, tail(br.Stdout, 800), gateErr(br.OK, br.Stderr+"\n"+br.Err))

		// Match CI: exclude the asr/doccontract packages (environment-coupled tests).
		tr := o.run("run", map[string]any{
			"command":   []string{"sh", "-c", "go test $(go list ./... 2>/dev/null | grep -vE '/(asr|doccontract)$')"},
			"cwd":       repo,
			"timeout_s": selfImproveBuildSecs,
			"env":       cleanGateEnv(),
		})
		if cosmeticDetail != "" || !br.OK || !tr.OK {
			failures := []string{}
			if cosmeticDetail != "" {
				failures = append(failures, cosmeticDetail)
			}
			if !br.OK {
				failures = append(failures, "go build 失败:\n"+tail(br.Stdout+"\n"+br.Stderr+"\n"+br.Err, 2000))
			}
			if !tr.OK {
				failures = append(failures, "go test 失败:\n"+tail(tr.Stdout+"\n"+tr.Stderr+"\n"+tr.Err, 2000))
			}
			add("selfimprove.test", tr.OK, tail(tr.Stdout, 800), gateErr(tr.OK, tr.Stderr+"\n"+tr.Err))
			return false, strings.Join(failures, "\n")
		}

		add("selfimprove.test", true, "anti-cosmetic 通过（确有 Go 逻辑改动）；go build ./... 通过；go test（CI 口径）通过\n"+tail(tr.Stdout, 800), "")
		return true, ""
	}

	passed, detail := evaluate()
	if passed {
		return finish(true, "anti-cosmetic、go build、go test 通过")
	}
	if strings.HasPrefix(detail, "表面改动") {
		add("selfimprove.cosmetic", false, "", detail)
	} else {
		add("selfimprove.gate", false, "", "首轮验证未过："+detail)
	}

	// Repair turns build on the (windowed) main transcript plus a short local tail.
	var gateLocal []contract.Message
	planTurns := 0
	for fix := 1; fix <= selfImproveMaxFix; fix++ {
		gateLocal = append(gateLocal, contract.Message{Role: "user", Content: "OBSERVATION: 你的改动未通过自动验证：\n" + detail +
			"\n请先 read 相关文件，再用 replace 做小步修复（只输出一个 JSON 动作；不要输出整个文件、不要只改注释）。"})
		resp, err := o.chatWithFallback(ctx, []string{"fast"}, provider.ChatRequest{
			Messages:       mergeReactMessages(append(thread.messages(), gateLocal...)),
			MaxTokens:      8000,
			ResponseFormat: noJSON(),
		})
		if err != nil {
			return finish(false, detail+"; 修复轮模型调用失败: "+err.Error())
		}
		o.turnTokens += int64(resp.Usage.TotalTokens)
		gateLocal = append(gateLocal, contract.Message{Role: "assistant", Content: resp.Content})
		act, perr := parseReactAction(resp.Content)
		if perr != nil {
			detail = "修复轮动作无法解析: " + perr.Error()
			continue
		}
		if act.Done {
			// model gave up — honest result
			return finish(false, detail+"; 模型放弃修复: "+act.Summary)
		}
		obs, ok, touched := o.executeThreadAction(repo, logDir, act, snap, thread)
		for _, f := range touched {
			changed = appendIfMissing(changed, f)
		}
		add("selfimprove.fix"+fmt.Sprint(fix), ok, obs.Obs, obs.Err)
		gateLocal = append(gateLocal, contract.Message{Role: "user", Content: "OBSERVATION:\n" + obs.Obs})
		if strings.TrimSpace(act.Tool) == "plan" {
			planTurns++
			if planTurns >= 8 {
				return finish(false, "修复计划动作超过上限；"+detail)
			}
			fix--
			continue
		}
		passed, detail = evaluate()
		if passed {
			return finish(true, "anti-cosmetic、go build、go test 通过")
		}
	}
	return finish(false, detail)
}

func appendIfMissing(xs []string, s string) []string {
	for _, x := range xs {
		if x == s {
			return xs
		}
	}
	return append(xs, s)
}

// --- anti-cosmetic: compare Go logic after stripping comments/whitespace ---

// stripBlockComments removes /* ... */ comments (may span lines), replacing each
// with a space so adjacent tokens do not fuse.
func stripBlockComments(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if i+1 < len(s) && s[i] == '/' && s[i+1] == '*' {
			j := strings.Index(s[i+2:], "*/")
			if j < 0 {
				break
			}
			i = i + 2 + j + 2
			b.WriteByte(' ')
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// indexLineComment returns the byte index of a `//` line comment that is not inside a
// string/rune literal, or -1.
func indexLineComment(line string) int {
	var inD, inB, inS bool
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch c {
		case '"':
			if !inB && !inS && (i == 0 || line[i-1] != '\\') {
				inD = !inD
			}
		case '`':
			if !inD && !inS {
				inB = !inB
			}
		case '\'':
			if !inD && !inB && (i == 0 || line[i-1] != '\\') {
				inS = !inS
			}
		case '/':
			if i+1 < len(line) && line[i+1] == '/' && !inD && !inB && !inS {
				return i
			}
		}
	}
	return -1
}

// goCodeSignature reduces Go source to its logical skeleton: comments and blank
// lines removed, each surviving line trimmed. Two versions with identical signatures
// differ only cosmetically.
func goCodeSignature(src string) string {
	s := stripBlockComments(src)
	var keep []string
	for _, ln := range strings.Split(s, "\n") {
		if i := indexLineComment(ln); i >= 0 {
			ln = ln[:i]
		}
		ln = strings.TrimSpace(ln)
		if ln != "" {
			keep = append(keep, ln)
		}
	}
	return strings.Join(keep, "\n")
}

// cosmeticOnly reports whether every touched Go file is logically unchanged versus
// its pre-edit snapshot (comments/whitespace/doc text only).
func cosmeticOnly(repo string, changed []string, snap map[string]string) bool {
	for _, rel := range changed {
		abs := filepath.Join(repo, rel)
		before := snap[abs]
		afterBytes, err := os.ReadFile(abs)
		if err != nil {
			continue
		}
		if goCodeSignature(before) != goCodeSignature(string(afterBytes)) {
			return false
		}
	}
	return true
}

// tail keeps the last n chars (compiler/test diagnostics are at the tail; see R14).
func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-n:]
}

// singleLinePreview flattens a raw model reply for diagnostic logging in receipts.
func singleLinePreview(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return truncateStr(strings.TrimSpace(s), n)
}

// Merge adjacent observations for chat templates requiring alternating roles.
func mergeReactMessages(messages []contract.Message) []contract.Message {
	var out []contract.Message
	for _, m := range messages {
		if len(out) > 0 && out[len(out)-1].Role == m.Role {
			out[len(out)-1].Content += "\n\n" + m.Content
		} else {
			out = append(out, m)
		}
	}
	return out
}
