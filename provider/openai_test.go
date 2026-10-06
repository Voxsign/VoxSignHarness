package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"voicesign-harness/contract"
)

// newTestClient baseat httptest serveservice      openaiClient(endpoint rule izeafterreferto ts). 
func newTestClient(ts *httptest.Server, apiKey string, responseFormat bool, params map[string]any) *openaiClient {
	return &openaiClient{
		name:           "test",
		url:            normalizeEndpoint(ts.URL),
		model:          "gpt-test",
		apiKey:         apiKey,
		responseFormat: responseFormat,
		timeoutMs:      5000,
		params:         params,
		httpClient:     ts.Client(),
	}
}

// cannedBody is      OpenAI become   . 
const cannedBody = `{
  "choices": [
    {
      "index": 0,
      "message": {"role": "assistant", "content": "{\"actions\":[],\"final\":\"ok\"}"},
      "finish_reason": "stop"
    }
  ],
  "usage": {"prompt_tokens": 11, "completion_tokens": 7, "total_tokens": 18}
}`

func TestNormalizeEndpoint(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://model.peterzou.com", "https://model.peterzou.com/chat/completions"}, // base  state
		{"https://api.openai.com/v1", "https://api.openai.com/v1/chat/completions"},   //   /v1
		{"https://host/v1/chat/completions", "https://host/v1/chat/completions"},      // finish path
		{"https://host/", "https://host/chat/completions"},                            // tail   base
		{"https://host/v1/", "https://host/v1/chat/completions"},                      // tail   /v1
	}
	for _, c := range cases {
		if got := normalizeEndpoint(c.in); got != c.want {
			t.Errorf("normalizeEndpoint(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestOpenAIHappyPath(t *testing.T) {
	var gotAuth, gotCT string
	var bodyMap map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &bodyMap)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(cannedBody))
	}))
	defer ts.Close()

	c := newTestClient(ts, "secret-key-123", true, nil)
	resp, err := c.Chat(context.Background(), ChatRequest{
		Messages:  []contract.Message{{Role: contract.RoleUser, Content: "hi"}},
		MaxTokens: 64,
	})
	if err != nil {
		t.Fatalf("Chat 报错: %v", err)
	}
	// resolve : content / finish_reason / usage
	if resp.Content != `{"actions":[],"final":"ok"}` {
		t.Errorf("Content = %q", resp.Content)
	}
	if resp.FinishReason != "stop" {
		t.Errorf("FinishReason = %q", resp.FinishReason)
	}
	if resp.Usage.PromptTokens != 11 || resp.Usage.CompletionTokens != 7 || resp.Usage.TotalTokens != 18 {
		t.Errorf("Usage = %+v", resp.Usage)
	}
	//   headstore ity
	if gotAuth != "Bearer secret-key-123" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q", gotCT)
	}
	//  requirebodyclose charseg
	if bodyMap["model"] != "gpt-test" {
		t.Errorf("body.model = %v", bodyMap["model"])
	}
	if bodyMap["temperature"].(float64) != 0 {
		t.Errorf("body.temperature = %v", bodyMap["temperature"])
	}
	if bodyMap["max_tokens"].(float64) != 64 {
		t.Errorf("body.max_tokens = %v", bodyMap["max_tokens"])
	}
}

func TestOpenAINoAuthHeaderWhenKeyEmpty(t *testing.T) {
	var sawAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(cannedBody))
	}))
	defer ts.Close()

	c := newTestClient(ts, "", true, nil)
	if _, err := c.Chat(context.Background(), ChatRequest{}); err != nil {
		t.Fatalf("Chat 报错: %v", err)
	}
	if sawAuth != "" {
		t.Errorf("apiKey 为空时不应发送 Authorization, got %q", sawAuth)
	}
}

func TestOpenAIResponseFormatToggle(t *testing.T) {
	// openstart:  requirebody   response_format={"type":"json_object"}; close then outnow. 
	t.Run("on", func(t *testing.T) {
		var m map[string]any
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &m)
			_, _ = w.Write([]byte(cannedBody))
		}))
		defer ts.Close()
		c := newTestClient(ts, "", true, nil)
		if _, err := c.Chat(context.Background(), ChatRequest{}); err != nil {
			t.Fatal(err)
		}
		rf, ok := m["response_format"].(map[string]any)
		if !ok {
			t.Fatal("response_format 开启时请求体应含 response_format")
		}
		if rf["type"] != "json_object" {
			t.Errorf("response_format.type = %v", rf["type"])
		}
	})

	t.Run("off", func(t *testing.T) {
		var m map[string]any
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &m)
			_, _ = w.Write([]byte(cannedBody))
		}))
		defer ts.Close()
		c := newTestClient(ts, "", false, nil)
		if _, err := c.Chat(context.Background(), ChatRequest{}); err != nil {
			t.Fatal(err)
		}
		if _, ok := m["response_format"]; ok {
			t.Error("response_format 关闭时请求体不应含 response_format")
		}
	})
}

func TestOpenAIParamsMerged(t *testing.T) {
	var m map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &m)
		_, _ = w.Write([]byte(cannedBody))
	}))
	defer ts.Close()

	c := newTestClient(ts, "", true, map[string]any{"reasoning_effort": "none"})
	if _, err := c.Chat(context.Background(), ChatRequest{}); err != nil {
		t.Fatal(err)
	}
	if m["reasoning_effort"] != "none" {
		t.Errorf("透传 Params 未合并: reasoning_effort = %v", m["reasoning_effort"])
	}
}

func TestOpenAIRetryOn429ThenSucceed(t *testing.T) {
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate limited"}`))
			return
		}
		_, _ = w.Write([]byte(cannedBody))
	}))
	defer ts.Close()

	c := newTestClient(ts, "", true, nil)
	if _, err := c.Chat(context.Background(), ChatRequest{}); err != nil {
		t.Fatalf("429 重试后应成功: %v", err)
	}
	if calls < 2 {
		t.Errorf("429 应触发至少 2 次请求, got %d", calls)
	}
}

func TestOpenAINoRetryOn401(t *testing.T) {
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad key"}`))
	}))
	defer ts.Close()

	c := newTestClient(ts, "wrong", true, nil)
	_, err := c.Chat(context.Background(), ChatRequest{})
	if err == nil {
		t.Fatal("401 应报错")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("错误应含状态码 401, got %q", err.Error())
	}
	if calls != 1 {
		t.Errorf("401 不应重试, 请求次数 = %d, want 1", calls)
	}
}

func TestOpenAIRetryExhaustedOn500(t *testing.T) {
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	}))
	defer ts.Close()

	c := newTestClient(ts, "", true, nil)
	_, err := c.Chat(context.Background(), ChatRequest{})
	if err == nil {
		t.Fatal("持续 500 应最终报错")
	}
	if calls != 3 {
		t.Errorf("500 应重试至 3 次（初始+2），got %d", calls)
	}
}

func TestOpenAIMalformedJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`this is not json {{{`))
	}))
	defer ts.Close()

	c := newTestClient(ts, "", true, nil)
	if _, err := c.Chat(context.Background(), ChatRequest{}); err == nil {
		t.Fatal("畸形 JSON 应报错")
	}
}

func TestOpenAIToolCallsForcedError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// content asemptybutoutnow tool_calls: endpoint restrict function-calling  form
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"x"}]}}],"usage":{}}`))
	}))
	defer ts.Close()

	c := newTestClient(ts, "", true, nil)
	_, err := c.Chat(context.Background(), ChatRequest{})
	if err == nil {
		t.Fatal("content 空 + tool_calls 应明确报错")
	}
	if !strings.Contains(err.Error(), "tool_calls") {
		t.Errorf("错误应提示 tool_calls/function-calling, got %q", err.Error())
	}
}

// TestIntegrationRealEndpoint is  integrate  : has VHS_API_KEY timeto typein  ping    max_tokens=5, 
//   endpointlink ; no key    ed.         disconnectlangseeonface  httptest useexample. 
func TestIntegrationRealEndpoint(t *testing.T) {
	key := os.Getenv("VHS_API_KEY")
	if key == "" {
		t.Skip("未设置 VHS_API_KEY，跳过真实端点集成测试")
	}
	c := &openaiClient{
		name:           "center",
		url:            "https://model.peterzou.com/v1/chat/completions",
		model:          "gpt-4o-mini",
		apiKey:         key,
		responseFormat: false, // ping  needrequire json_object,   on  showword end
		timeoutMs:      30000,
		httpClient:     &http.Client{},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30000)
	defer cancel()
	resp, err := c.Chat(ctx, ChatRequest{
		Messages:  []contract.Message{{Role: contract.RoleUser, Content: "ping"}},
		MaxTokens: 5,
	})
	if err != nil {
		t.Fatalf("真实端点 ping 失败: %v", err)
	}
	t.Logf("ping 成功: content=%q usage=%+v", resp.Content, resp.Usage)
}
