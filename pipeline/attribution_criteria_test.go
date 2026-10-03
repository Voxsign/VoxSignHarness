// attribution_criteria_test.go —— 错误归因必须来自真实错误（先红→绿）。
package pipeline

import (
	"strings"
	"testing"
)

// ① 401 ⇒ 必须说鉴权/API key，且**不得**出现"预算"或"网络"
func TestAttribution401IsAuthNotBudgetOrNetwork(t *testing.T) {
	raw := `模型调用 HTTP 401: {"error":{"code":"invalid_api_key","message":"Invalid API key.","type":"auth_error"}}`
	got := attributeLLMError(raw)
	if !strings.Contains(got, "鉴权") && !strings.Contains(got, "API key") {
		t.Errorf("[归因①] 401 未归因到鉴权: %q", got)
	}
	for _, bad := range []string{"预算", "网络"} {
		if strings.Contains(got, bad) {
			t.Errorf("[归因④ 反例] 401 被归因成 %q: %s", bad, got)
		}
	}
}

// ② 5xx ⇒ 上游；超时 ⇒ 超时
func TestAttribution5xxAndTimeout(t *testing.T) {
	if got := attributeLLMError("模型调用 HTTP 502: bad gateway"); !strings.Contains(got, "上游") {
		t.Errorf("[归因②] 5xx 未归因到上游: %q", got)
	}
	if got := attributeLLMError("Post \"...\": context deadline exceeded (Client.Timeout)"); !strings.Contains(got, "超时") {
		t.Errorf("[归因②] 超时未归因到超时: %q", got)
	}
	if got := attributeLLMError("HTTP 429 too many requests"); !strings.Contains(got, "限流") {
		t.Errorf("[归因②] 429 未归因到限流: %q", got)
	}
}

// ③ 未知错误 ⇒ 必须说"未知" + **原始错误文本**（不许通用话术）
func TestAttributionUnknownCarriesRawText(t *testing.T) {
	raw := "some_weird_failure_xyz"
	got := attributeLLMError(raw)
	if !strings.Contains(got, "未知") {
		t.Errorf("[归因③] 未知错误未标未知: %q", got)
	}
	if !strings.Contains(got, raw) {
		t.Errorf("[归因③] 未知错误未带原始文本: %q", got)
	}
}

// ④ 空错误 ⇒ 不得冒充已知原因
func TestAttributionEmptyIsUnknown(t *testing.T) {
	got := attributeLLMError("")
	if strings.Contains(got, "预算") || strings.Contains(got, "网络") {
		t.Errorf("[归因④] 空错误却套用了具体归因: %q", got)
	}
	if !strings.Contains(got, "未知") {
		t.Errorf("[归因④] 空错误应说明未知: %q", got)
	}
}
