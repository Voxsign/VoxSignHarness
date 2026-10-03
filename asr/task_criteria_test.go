//go:build vhsui

// task_criteria_test.go —— /v1/task：只规划、不执行；域门禁在规划期生效；document 不落盘。
package asr

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func taskCall(t *testing.T, base, body string) map[string]any {
	t.Helper()
	resp, err := http.Post(base+"/v1/task", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

// ① 只规划不执行：可规划的任务返回步骤，且 execute=false。
func TestTaskPlansWithoutExecuting(t *testing.T) {
	base, _ := newUIServer(t)
	got := taskCall(t, base, `{"task":"把这个项目里所有 TODO 整理成一份文档","document":"TODO: x"}`)
	if got["execute"] != false {
		t.Fatalf("[task] execute 必须为 false（只规划）: %v", got["execute"])
	}
	p, _ := got["plan"].(map[string]any)
	if p == nil {
		t.Fatal("[task] 缺 plan")
	}
	steps, _ := p["steps"].([]any)
	if len(steps) == 0 {
		t.Errorf("[task] 可规划任务应给出步骤: %+v", p)
	}
	if p["source"] == nil || p["source"] == "" {
		t.Errorf("[task] 必须回传 Source（可解释）: %+v", p)
	}
}

// ② 域门禁在**规划期**生效：能力缺口 ⇒ 拒绝 + 该找谁（owner 前缀）。
func TestTaskRefusesOutOfScopeAtPlanningTime(t *testing.T) {
	base, _ := newUIServer(t)
	got := taskCall(t, base, `{"task":"帮我部署到生产服务器"}`)
	p, _ := got["plan"].(map[string]any)
	if p == nil {
		t.Fatal("[task] 缺 plan")
	}
	if p["refused"] != true {
		t.Errorf("[task] 越域/缺口计划应在规划期被拒: %+v", p)
	}
	missing, _ := p["missing"].([]any)
	if len(missing) == 0 {
		t.Fatal("[task] 拒绝必须给出原因与 owner")
	}
	joined := ""
	for _, m := range missing {
		joined += m.(string) + "|"
	}
	if !strings.Contains(joined, "人：") && !strings.Contains(joined, "网关：") {
		t.Errorf("[task] 卡点必须带 owner 前缀（找谁）: %v", missing)
	}
}

// ③ document **不落盘**（除 reallog 外，dataDir 不得多文件）。
func TestTaskDocumentNotPersisted(t *testing.T) {
	base, logPath := newUIServer(t)
	dataDir := filepath.Dir(logPath)
	before := map[string]bool{}
	_ = filepath.Walk(dataDir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			before[p] = true
		}
		return nil
	})
	big := strings.Repeat("机密文档内容。", 200)
	taskCall(t, base, `{"task":"总结这个文档","document":"`+big+`"}`)
	err := filepath.Walk(dataDir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !before[p] {
			t.Errorf("[task] document 被落盘: %s", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// ④ 页面：有任务入口 + **明确标注只规划不执行**。
func TestTaskPageHasUploadAndPlanOnlyLabel(t *testing.T) {
	page := fetchPage(t)
	for _, want := range []string{`id="doc"`, `id="task"`, "planTask()", "只规划，不执行", `id="file"`} {
		if !strings.Contains(page, want) {
			t.Errorf("[task] 页面缺 %q", want)
		}
	}
}
