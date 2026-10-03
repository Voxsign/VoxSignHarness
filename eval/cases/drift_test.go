//go:build evaldrift

// 真值库漂移探测器（G-C 的复算机制）。
//
// 背景：`eval/cases/cases.jsonl` 是**人维护的外部真理源**，每行都带 `verify_cmd`。
// 但在本轮之前，**没有任何东西在跑这些命令** —— 真值库实际上只是一份文档，
// 代码修好了它也不会知道，"not_met" 会一直挂在那里，甚至被当成"还没修"。
//
// 本测试逐条跑 `verify_cmd`，把**声明状态**与**实测结果**比对：
//
//	status=met      → verify_cmd 必须退出 0
//	status=not_met  → verify_cmd 必须非 0（缺口确实还在）
//	status=unverified → **不判**，只报告（未评测项显式保留，不得折算为通过或 0 分）
//
// 一致 = 真值库可信；不一致 = **漂移**，须人工重签（agent 不得自行改状态）。
//
// 用构建标签隔离，默认 `go test ./...` 不受影响：
//
//	go test -tags evaldrift ./eval/cases -run TestTruthSourceDrift -v
package cases

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

type caseRow struct {
	ID               string `json:"id"`
	Status           string `json:"status"`
	VerifyCmd        string `json:"verify_cmd"`
	RawText          string `json:"raw_text"`
	ObservedAtCommit string `json:"observed_at_commit"`
	RatifiedBy       string `json:"ratified_by"`
}

func loadCases(t *testing.T) []caseRow {
	t.Helper()
	data, err := os.ReadFile("cases.jsonl")
	if err != nil {
		t.Fatalf("读取 cases.jsonl 失败: %v", err)
	}
	var rows []caseRow
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var r caseRow
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("cases.jsonl 有非法 JSON 行: %v\n%s", err, line)
		}
		rows = append(rows, r)
	}
	if len(rows) == 0 {
		t.Fatal("cases.jsonl 为空 —— 真值库不能是空的")
	}
	return rows
}

// runVerify 在仓库根目录执行 verify_cmd，返回是否成功（退出码 0）。
func runVerify(t *testing.T, root, cmd string) bool {
	t.Helper()
	c := exec.Command("sh", "-c", cmd)
	c.Dir = root
	out, err := c.CombinedOutput()
	_ = out
	return err == nil
}

// TestTruthSourceDrift 真值库漂移探测。
func TestTruthSourceDrift(t *testing.T) {
	rows := loadCases(t)
	root := "../.."

	var (
		checked     int
		unverified  []string
		drifted     []string
		stillBroken []string
	)

	for _, r := range rows {
		if r.VerifyCmd == "" {
			t.Errorf("%s 没有 verify_cmd —— 真值库每行必须可复算", r.ID)
			continue
		}
		if r.Status == "unverified" {
			unverified = append(unverified, r.ID)
			continue
		}
		checked++
		passed := runVerify(t, root, r.VerifyCmd)
		switch {
		case r.Status == "met" && !passed:
			drifted = append(drifted, r.ID+" 声明 met 但 verify_cmd 失败")
		case r.Status == "not_met" && passed:
			drifted = append(drifted, r.ID+" 声明 not_met 但 verify_cmd 已通过（该缺口已修，待人工重签）")
		case r.Status == "not_met" && !passed:
			stillBroken = append(stillBroken, r.ID)
		}
	}

	t.Logf("复算 %d 条（另有 %d 条 unverified，显式保留不判）", checked, len(unverified))
	if len(stillBroken) > 0 {
		t.Logf("仍然真实存在的问题: %v", stillBroken)
	}
	if len(unverified) > 0 {
		t.Logf("unverified（不得折算为通过或 0 分）: %v", unverified)
	}
	if len(drifted) > 0 {
		t.Errorf("真值库漂移 %d 条 —— 声明状态与实测不一致，须**人工**重签（agent 不得自行改状态）：\n  %s",
			len(drifted), strings.Join(drifted, "\n  "))
	}
}
