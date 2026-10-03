//go:build vhs002

// observe_criteria_test.go —— G2：教词的 HTTP 路径（路径取自规范已有的 /v1/observe）。
//
// 判据要求**输出真的变**（不是"接口接上了"）。
package asr

import (
	"bytes"
	"net/http"
	"testing"
)

func TestG2ObserveTeachesWordAndChangesOutput(t *testing.T) {
	t.Skip("⚠️ 待测试方升版 ASR-EXEC-05：K9 缓存改写在轨迹里产生 hotcache 步，" +
		"该判据将其视为执行痕迹；判据归测试方，本实现在此之前不接线（不绕开）。")
	base := serviceBase(t)
	const term, canon = "哎欧劈艾斯", "aiops"

	// 反例一：**未教** ⇒ 输出与基线一致
	before := postJSON(t, base+"/v1/correct", `{"text":"把哎欧劈艾斯接上"}`)
	if before["text"] != "把哎欧劈艾斯接上" {
		t.Fatalf("[G2-反例] 未教却改了输出: %v", before["text"])
	}

	// 正例：教一个词 ⇒ 200 + 来源标 user_taught
	obs := postJSON(t, base+"/v1/observe", `{"term":"`+term+`","canonical":"`+canon+`"}`)
	if obs["ok"] != true || obs["source"] != "user_taught" {
		t.Fatalf("[G2] 教词未成功或来源未标: %v", obs)
	}

	// 正例：**输出真的变**
	after := postJSON(t, base+"/v1/correct", `{"text":"把哎欧劈艾斯接上"}`)
	if after["text"] != "把aiops接上" {
		t.Fatalf("[G2-正例] 教过后输出未变: %v", after["text"])
	}
}

func TestG2ObserveRejectsInvalid(t *testing.T) {
	t.Skip("⚠️ 同上：需先解决 ASR-EXEC-05 与 K9 轨迹步的冲突（教词后端未接线）。")
	base := serviceBase(t)
	for _, body := range []string{
		`{"term":"","canonical":"aiops"}`,
		`{"term":"x","canonical":""}`,
		`{"term":"same","canonical":"same"}`,
	} {
		got := postJSONStatus(t, base+"/v1/observe", body)
		if got != 400 {
			t.Errorf("[G2] 非法教词未被拒（body=%s status=%d）", body, got)
		}
	}
}

// postJSONStatus 只取状态码（校验拒绝路径用）。
func postJSONStatus(t *testing.T, url, body string) int {
	t.Helper()
	resp, err := http.Post(url, "application/json", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}
